package javascript

import (
	"sova/internal/codegen"
	"sova/internal/codegen/javascript/jsgen"
	"sova/internal/ir"
)

func lookupBuiltinIntrinsicJS(ctx *codegen.EmitContext, callee ir.Expr) string {
	if ctx == nil || ctx.Cache == nil {
		return ""
	}

	raw, ok := ctx.Cache["builtin_intrinsics"]
	if !ok {
		return ""
	}

	intrinsics, ok := raw.(map[ir.SymID]string)
	if !ok {
		return ""
	}

	vr, ok := callee.(*ir.VarRef)
	if !ok || vr.Ref.Sym == 0 {
		return ""
	}

	return intrinsics[vr.Ref.Sym]
}

func emitBuiltinIntrinsicCallJS(intrinsic string, args []*jsgen.Statement) *jsgen.Statement {
	switch intrinsic {
	case "print", "println":
		return jsgen.Id("console").Dot("log").Call(args...)
	case "len":
		if len(args) != 1 {
			return nil
		}

		return jsgen.Raw("((__v) => (__v == null ? 0 : (__v.length !== undefined ? __v.length : (__v.size !== undefined ? __v.size : Object.keys(__v).length))))").Call(args[0])
	case "error":
		if len(args) != 1 {
			return nil
		}

		return jsgen.Raw("new Error").Call(args[0])
	case "after":
		if len(args) != 1 {
			return nil
		}

		return jsgen.Raw("(() => { const __ch = new __SovaChan(1); setTimeout(() => { __ch.send(null); }, ").Add(args[0]).Add(jsgen.Raw("); return __ch; })()"))
	case "every":
		if len(args) != 1 {
			return nil
		}

		return jsgen.Raw("(() => { const __ch = new __SovaChan(1); setInterval(() => { if (__ch.buf.length < __ch.cap) { __ch.send(null); } }, ").Add(args[0]).Add(jsgen.Raw("); return __ch; })()"))
	}

	return nil
}
