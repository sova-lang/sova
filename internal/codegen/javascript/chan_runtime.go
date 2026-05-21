package javascript

import (
	"fmt"
	"strings"

	"sova/internal/codegen"
	"sova/internal/codegen/javascript/jsgen"
	"sova/internal/ir"
)

// buildSelectStmt lowers a Sova `select { ... }` to an `await __sovaSelect([...], defaultCb)` call. Each case becomes a `{op, chan, value?, cb}` object literal; recv-bind cases destructure the `[value, ok]` tuple in the cb's parameter list so the bindings are visible to the body. The whole expression must be awaited; the surrounding function gets marked async by `propagate_async` (which treats SelectStmt as inherently async on the JS target).
func (e *CodeEmitter) buildSelectStmt(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, s *ir.SelectStmt) *jsgen.Statement {
	e.usesChanRuntime = true
	var sb strings.Builder
	sb.WriteString("await __sovaSelect([")
	for i, cc := range s.Cases {
		if i > 0 {
			sb.WriteString(",")
		}
		switch cc.Kind {
		case ir.SelectCaseSend:
			sb.WriteString("{op:'send',chan:")
			sb.WriteString(stmtString(e.buildExpr(ctx, pkg, f, cc.ChanExpr)))
			sb.WriteString(",value:")
			sb.WriteString(stmtString(e.buildExpr(ctx, pkg, f, cc.SendValue)))
			sb.WriteString(",cb:async()=>")
			sb.WriteString(renderBlock(e, ctx, pkg, f, cc.Body))
			sb.WriteString("}")
		case ir.SelectCaseRecvBind:
			sb.WriteString("{op:'recv',chan:")
			sb.WriteString(stmtString(e.buildExpr(ctx, pkg, f, cc.ChanExpr)))
			sb.WriteString(",cb:async(__pair)=>")
			var prelude strings.Builder
			for i, tgt := range cc.Targets {
				if tgt.Name == nil {
					continue
				}
				name := symNameWithUnused(ctx, pkg, tgt.Name.Sym)
				if name == "_" {
					continue
				}
				prelude.WriteString(fmt.Sprintf("const %s=__pair[%d];", name, i))
			}
			sb.WriteString("{")
			sb.WriteString(prelude.String())
			sb.WriteString(renderBlockInner(e, ctx, pkg, f, cc.Body))
			sb.WriteString("}")
			sb.WriteString("}")
		case ir.SelectCaseRecvDiscard:
			sb.WriteString("{op:'recv',chan:")
			sb.WriteString(stmtString(e.buildExpr(ctx, pkg, f, cc.ChanExpr)))
			sb.WriteString(",cb:async()=>")
			sb.WriteString(renderBlock(e, ctx, pkg, f, cc.Body))
			sb.WriteString("}")
		}
	}
	sb.WriteString("]")
	if s.Default != nil {
		sb.WriteString(",async()=>")
		sb.WriteString(renderBlock(e, ctx, pkg, f, s.Default))
	}
	sb.WriteString(")")
	return jsgen.Raw(sb.String())
}

func stmtString(s *jsgen.Statement) string {
	if s == nil {
		return ""
	}
	return s.String()
}

func renderBlock(e *CodeEmitter, ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, b *ir.BlockStmt) string {
	if b == nil {
		return "{}"
	}
	return "{" + renderBlockInner(e, ctx, pkg, f, b) + "}"
}

func renderBlockInner(e *CodeEmitter, ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, b *ir.BlockStmt) string {
	if b == nil {
		return ""
	}
	var sb strings.Builder
	for _, st := range b.Stmts {
		c := e.buildStmtAsCode(ctx, pkg, f, st)
		if stmtVal, ok := c.(*jsgen.Statement); ok {
			sb.WriteString(stmtVal.String())
			sb.WriteString(";")
		}
	}
	return sb.String()
}

func matchChanMethodJS(ctx *codegen.EmitContext, call *ir.FuncCallExpr) (string, ir.Expr, bool) {
	fa, ok := call.Callee.(*ir.FieldAccessExpr)
	if !ok || len(fa.Fields) != 1 {
		return "", nil, false
	}
	method := fa.Fields[0].Name
	if method != "send" && method != "recv" && method != "close" {
		return "", nil, false
	}
	ty, found := ctx.Types.GetByID(fa.Expr.GetType())
	if !found || ty.Kind != ir.TK_Chan {
		return "", nil, false
	}
	return method, fa.Expr, true
}

// SovaChanRuntime is the JavaScript runtime that backs Sova's `chan<T>` on the frontend. It provides a buffered/unbuffered FIFO with promise-based send/recv, supporting `ch.send(v)`, `ch.recv()` (returns `[value, ok]`), and `ch.close()` (subsequent recv returns the zero-value tuple with ok=false). Single-threaded scheduling: send/recv resolve on the microtask queue.
const SovaChanRuntime = `
class __SovaChan {
  constructor(capacity) {
    this.cap = (capacity | 0) > 0 ? (capacity | 0) : 0;
    this.buf = [];
    this.sendWaiters = [];
    this.recvWaiters = [];
    this.closed = false;
  }
  send(v) {
    if (this.closed) { throw new Error("send on closed channel"); }
    if (this.recvWaiters.length > 0) {
      const w = this.recvWaiters.shift();
      w([v, true]);
      return Promise.resolve();
    }
    if (this.buf.length < this.cap) {
      this.buf.push(v);
      return Promise.resolve();
    }
    return new Promise((resolve) => { this.sendWaiters.push({ v, resolve }); });
  }
  recv() {
    if (this.buf.length > 0) {
      const v = this.buf.shift();
      if (this.sendWaiters.length > 0) {
        const w = this.sendWaiters.shift();
        this.buf.push(w.v);
        w.resolve();
      }
      return Promise.resolve([v, true]);
    }
    if (this.sendWaiters.length > 0) {
      const w = this.sendWaiters.shift();
      w.resolve();
      return Promise.resolve([w.v, true]);
    }
    if (this.closed) {
      return Promise.resolve([undefined, false]);
    }
    return new Promise((resolve) => { this.recvWaiters.push(resolve); });
  }
  close() {
    if (this.closed) { throw new Error("close of closed channel"); }
    this.closed = true;
    while (this.recvWaiters.length > 0) {
      const w = this.recvWaiters.shift();
      w([undefined, false]);
    }
  }
}
async function __sovaSelect(cases, defaultCb) {
  for (let i = 0; i < cases.length; i++) {
    const c = cases[i];
    if (c.op === 'recv') {
      if (c.chan.buf.length > 0 || c.chan.sendWaiters.length > 0 || c.chan.closed) {
        const v = await c.chan.recv();
        return c.cb(v);
      }
    } else if (c.op === 'send') {
      if (!c.chan.closed && (c.chan.recvWaiters.length > 0 || c.chan.buf.length < c.chan.cap)) {
        await c.chan.send(c.value);
        return c.cb();
      }
    }
  }
  if (defaultCb) { return defaultCb(); }
  return new Promise((resolve, reject) => {
    let fired = false;
    const registrations = [];
    const fire = (cb, arg) => {
      if (fired) { return; }
      fired = true;
      for (const r of registrations) { r.cancel(); }
      Promise.resolve(cb(arg)).then(resolve, reject);
    };
    for (const c of cases) {
      if (c.op === 'recv') {
        const w = (v) => fire(c.cb, v);
        c.chan.recvWaiters.push(w);
        registrations.push({ cancel: () => {
          const i = c.chan.recvWaiters.indexOf(w);
          if (i >= 0) { c.chan.recvWaiters.splice(i, 1); }
        }});
      } else if (c.op === 'send') {
        const w = { v: c.value, resolve: () => fire(c.cb) };
        c.chan.sendWaiters.push(w);
        registrations.push({ cancel: () => {
          const i = c.chan.sendWaiters.indexOf(w);
          if (i >= 0) { c.chan.sendWaiters.splice(i, 1); }
        }});
      }
    }
  });
}
`
