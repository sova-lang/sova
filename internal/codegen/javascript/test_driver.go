package javascript

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"

	"sova/internal/codegen"
	"sova/internal/codegen/javascript/jsgen"
	"sova/internal/ir"
)

func jsTestModeFromCache(ctx *codegen.EmitContext) bool {
	if ctx == nil || ctx.Cache == nil {
		return false
	}

	raw, ok := ctx.Cache["build_config"]
	if !ok {
		return false
	}

	cfg, ok := raw.(interface{ TestModeValue() bool })
	return ok && cfg.TestModeValue()
}

func jsHashTestName(pkgPath string, groupPath []string, name string) string {
	h := sha1.New()
	h.Write([]byte(pkgPath))
	for _, g := range groupPath {
		h.Write([]byte("/"))
		h.Write([]byte(g))
	}

	h.Write([]byte("/"))
	h.Write([]byte(name))
	return hex.EncodeToString(h.Sum(nil))[:12]
}

func jsFullTestName(groupPath []string, name string) string {
	if len(groupPath) == 0 {
		return name
	}

	return strings.Join(groupPath, " > ") + " > " + name
}

func emitJSTestRuntime(file *jsgen.File) {
	file.Add(jsgen.Raw(`
if (typeof console === 'undefined') {
  var __sovaJSTestOut = [];
  var console = {
    log: function() { __sovaJSTestOut.push(Array.prototype.slice.call(arguments).join(' ')); },
    error: function() { __sovaJSTestOut.push(Array.prototype.slice.call(arguments).join(' ')); },
    warn: function() { __sovaJSTestOut.push(Array.prototype.slice.call(arguments).join(' ')); },
    info: function() { __sovaJSTestOut.push(Array.prototype.slice.call(arguments).join(' ')); },
    debug: function() { __sovaJSTestOut.push(Array.prototype.slice.call(arguments).join(' ')); }
  };
}
var __sovaJSTestRegistry = {};
var __sovaJSOnceFired = {};
function __sovaJSFireOnce(key, fn) { if (__sovaJSOnceFired[key]) return; __sovaJSOnceFired[key] = true; fn(); }
function __sovaJSTestRegister(name, body) { __sovaJSTestRegistry[name] = body; }
var __sovaJSMockRegistry = {};
function __sovaJSMockRegister(name, value, hasReturn) { __sovaJSMockRegistry[name] = { value: value, hasReturn: hasReturn, calls: [] }; }
function __sovaJSMockHook(name, args) {
  var entry = __sovaJSMockRegistry[name];
  if (!entry) return null;
  entry.calls.push(args);
  return { value: entry.hasReturn ? entry.value : undefined };
}
function __sovaJSMockReset() { __sovaJSMockRegistry = {}; }
function __sovaJSMockCallCount(name) { var e = __sovaJSMockRegistry[name]; return e ? e.calls.length : 0; }
function __sovaJSMockLastArgs(name) { var e = __sovaJSMockRegistry[name]; if (!e || e.calls.length === 0) return []; return e.calls[e.calls.length - 1]; }
var __sovaJSRandSeed = 0;
var __sovaJSRandReal = false;
function __sovaJSRandom() {
  if (__sovaJSRandReal) return Math.floor(Math.random() * 0x7fffffff);
  var s = (__sovaJSRandSeed + 0x9E3779B9) >>> 0;
  __sovaJSRandSeed = s;
  s ^= s >>> 16; s = Math.imul(s, 0x85EBCA6B) >>> 0;
  s ^= s >>> 13; s = Math.imul(s, 0xC2B2AE35) >>> 0;
  s ^= s >>> 16;
  return s & 0x7fffffff;
}
var __sovaJSClockOffsetMs = 0;
var __sovaJSClockReal = false;
function __sovaJSAdvanceTime(secs) { __sovaJSClockOffsetMs += Math.floor(secs * 1000); }
function __sovaJSUseRealTime() { __sovaJSClockReal = true; }
function __sovaJSMockTime(unixSecs) { __sovaJSClockOffsetMs = (unixSecs * 1000) - Date.now(); __sovaJSClockReal = false; }
function __sovaJSNow() {
  if (__sovaJSClockReal) return Math.floor(Date.now() / 1000);
  return Math.floor((Date.now() + __sovaJSClockOffsetMs) / 1000);
}
function __sovaJSUnixMillis() {
  if (__sovaJSClockReal) return Date.now();
  return Date.now() + __sovaJSClockOffsetMs;
}
function __sovaJSUnixNano() {
  return __sovaJSUnixMillis() * 1000000;
}
async function __sovaJSTestRun(name) {
  var fn = __sovaJSTestRegistry[name];
  if (!fn) return { failures: [], panic: "no such test: " + name };
  var ctx = { failures: [] };
  try { await fn(ctx); }
  catch (e) { return { failures: ctx.failures, panic: String(e && e.stack || e) }; }
  return { failures: ctx.failures, panic: "" };
}
function __sovaJSTestRecord(ctx, source, location, vars) {
  ctx.failures.push({ source: source, location: location, hasOperands: false, vars: vars || {} });
}
function __sovaJSTestRecordCompare(ctx, source, lhs, rhs, location, vars) {
  ctx.failures.push({ source: source, lhs: lhs, rhs: rhs, hasOperands: true, location: location, vars: vars || {} });
}
`))
}

func (e *CodeEmitter) emitJSTestImplFuncs(ctx *codegen.EmitContext) {
	entries := readJSTestRegistryView(ctx)

	setupAllSeen := map[ir.NodeID]bool{}

	teardownAllSeen := map[ir.NodeID]bool{}

	for _, entry := range entries {
		for _, s := range entry.SetupAlls {
			if setupAllSeen[s.ID()] {
				continue
			}

			setupAllSeen[s.ID()] = true
			s := s
			ent := entry
			fnName := jsSetupAllBodyName(s)
			var body []jsgen.Code
			if s.Body != nil {
				for _, st := range s.Body.Stmts {
					body = append(body, e.buildStmtAsCode(ctx, ent.Pkg, ent.File.Hir, st))
				}
			}

			e.jf.Add(jsgen.Func(fnName).Params().Block(body...))
		}

		for _, t := range entry.TeardownAlls {
			if teardownAllSeen[t.ID()] {
				continue
			}

			teardownAllSeen[t.ID()] = true
			t := t
			ent := entry
			fnName := jsTeardownAllBodyName(t)
			var body []jsgen.Code
			if t.Body != nil {
				for _, st := range t.Body.Stmts {
					body = append(body, e.buildStmtAsCode(ctx, ent.Pkg, ent.File.Hir, st))
				}
			}

			e.jf.Add(jsgen.Func(fnName).Params().Block(body...))
		}
	}

	for _, entry := range entries {
		entry := entry
		funcName := "__sovaJSTestImpl_" + jsHashTestName(entry.Pkg.Path.String(), entry.GroupPath, entry.Decl.Name)
		fullName := jsFullTestName(entry.GroupPath, entry.Decl.Name)
		var body []jsgen.Code
		for _, s := range entry.SetupAlls {
			body = append(body, jsgen.Raw(fmt.Sprintf("__sovaJSFireOnce(%q, %s)", jsSetupAllOnceKey(s), jsSetupAllBodyName(s))))
		}

		for _, setBody := range entry.SetupBodies {
			for _, st := range setBody {
				body = append(body, e.buildStmtAsCode(ctx, entry.Pkg, entry.File.Hir, st))
			}
		}

		if entry.Decl.Body != nil {
			for _, st := range entry.Decl.Body.Stmts {
				body = append(body, e.buildStmtAsCode(ctx, entry.Pkg, entry.File.Hir, st))
			}
		}

		for _, tdBody := range entry.TeardownBodies {
			for _, st := range tdBody {
				body = append(body, e.buildStmtAsCode(ctx, entry.Pkg, entry.File.Hir, st))
			}
		}

		e.jf.Add(jsgen.Func(funcName).Async().Params("__t").Block(body...))
		e.jf.Add(jsgen.Raw(fmt.Sprintf("__sovaJSTestRegister(%q, %s);", fullName, funcName)))
	}
}

func jsSetupAllBodyName(s *ir.SetupStmt) string {
	return fmt.Sprintf("__sovaJSSetupAllBody_%d", uint64(s.ID()))
}

func jsSetupAllOnceKey(s *ir.SetupStmt) string {
	return fmt.Sprintf("setupall:%d", uint64(s.ID()))
}

func jsTeardownAllBodyName(t *ir.TeardownStmt) string {
	return fmt.Sprintf("__sovaJSTeardownAllBody_%d", uint64(t.ID()))
}

func readJSTestRegistryView(ctx *codegen.EmitContext) []codegen.TestRegistryEntryView {
	if ctx == nil || ctx.Cache == nil {
		return nil
	}

	raw, ok := ctx.Cache[codegen.TestRegistryViewCacheKey]
	if !ok {
		return nil
	}

	if entries, ok := raw.([]codegen.TestRegistryEntryView); ok {
		return entries
	}

	return nil
}

func (e *CodeEmitter) emitJSAssertStmt(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, s *ir.AssertStmt) *jsgen.Statement {
	span := s.Span()
	location := fmt.Sprintf("%s:%d:%d", span.File, span.StartLn, span.StartCol)
	source := jsAssertSourceText(ctx, pkg, s.Expr)
	vars := jsCollectAssertVars(pkg, s.Expr)
	varsLit := jsBuildVarsLiteral(e, ctx, pkg, f, vars)
	if bin, ok := s.Expr.(*ir.BinaryExpr); ok && jsIsComparisonOp(bin.Op) {
		lhs := e.buildExpr(ctx, pkg, f, bin.Left)
		rhs := e.buildExpr(ctx, pkg, f, bin.Right)
		cond := e.buildExpr(ctx, pkg, f, s.Expr)
		return jsgen.Raw("{ var __lhs = ").Add(lhs).
			Add(jsgen.Raw("; var __rhs = ")).Add(rhs).
			Add(jsgen.Raw("; if (!(")).Add(cond).Add(jsgen.Raw(fmt.Sprintf(")) __sovaJSTestRecordCompare(__t, %q, __lhs, __rhs, %q, ", source, location))).
			Add(varsLit).Add(jsgen.Raw("); }"))
	}

	cond := e.buildExpr(ctx, pkg, f, s.Expr)
	return jsgen.Raw("if (!(").Add(cond).Add(jsgen.Raw(fmt.Sprintf(")) __sovaJSTestRecord(__t, %q, %q, ", source, location))).
		Add(varsLit).Add(jsgen.Raw(");"))
}

type jsCapturedVar struct {
	OrigName string
	Expr     ir.Expr
}

func jsCollectAssertVars(pkg *ir.PackageContext, expr ir.Expr) []jsCapturedVar {
	seen := map[string]bool{}

	var out []jsCapturedVar
	var walk func(ir.Expr)
	walk = func(e ir.Expr) {
		if e == nil {
			return
		}

		switch x := e.(type) {
		case *ir.VarRef:
			name := x.Ref.Name
			if name == "" || seen[name] {
				return
			}

			if pkg != nil && x.Ref.Sym != 0 {
				if sym, ok := pkg.Syms.GetByID(x.Ref.Sym); ok {
					if sym.Kind != ir.SK_Variable {
						return
					}
				}
			}

			seen[name] = true
			out = append(out, jsCapturedVar{OrigName: name, Expr: x})
		case *ir.BinaryExpr:
			walk(x.Left)
			walk(x.Right)
		case *ir.UnaryExpr:
			walk(x.Expr)
		case *ir.PrefixUnaryExpr:
			walk(x.Expr)
		case *ir.PostfixUnaryExpr:
			walk(x.Expr)
		case *ir.GroupedExpr:
			walk(x.Expr)
		case *ir.CoalesceExpr:
			walk(x.Left)
			walk(x.Default)
		case *ir.TenaryExpr:
			walk(x.Cond)
			walk(x.Then)
			walk(x.Else)
		case *ir.IndexExpr:
			walk(x.Expr)
			walk(x.Index)
		case *ir.FieldAccessExpr:
			walk(x.Expr)
		case *ir.FuncCallExpr:
			walk(x.Callee)
			for _, a := range x.Args {
				walk(a.Expr)
			}
		}
	}

	walk(expr)
	return out
}

func jsBuildVarsLiteral(e *CodeEmitter, ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, vars []jsCapturedVar) *jsgen.Statement {
	if len(vars) == 0 {
		return jsgen.Raw("{}")
	}

	out := jsgen.Raw("{")
	for i, v := range vars {
		if i > 0 {
			out = out.Add(jsgen.Raw(", "))
		}

		out = out.Add(jsgen.Raw(fmt.Sprintf("%q: ", v.OrigName))).Add(e.buildExpr(ctx, pkg, f, v.Expr))
	}

	return out.Add(jsgen.Raw("}"))
}

func jsIsComparisonOp(op ir.Op) bool {
	switch op {
	case ir.OpEq, ir.OpNeq, ir.OpLt, ir.OpLte, ir.OpGt, ir.OpGte:
		return true
	}

	return false
}

func jsAssertSourceText(ctx *codegen.EmitContext, pkg *ir.PackageContext, expr ir.Expr) string {
	if expr == nil {
		return ""
	}

	span := expr.Span()
	if span.File == "" {
		return ""
	}

	for _, file := range pkg.Files {
		if file.Filename != span.File {
			continue
		}

		lines := strings.Split(file.Content, "\n")
		if span.StartLn-1 < 0 || span.StartLn-1 >= len(lines) {
			return ""
		}

		line := lines[span.StartLn-1]
		startCol := span.StartCol - 1
		endCol := span.EndCol - 1
		if startCol < 0 {
			startCol = 0
		}

		if endCol > len(line) {
			endCol = len(line)
		}

		if endCol > startCol {
			return line[startCol:endCol]
		}

		return strings.TrimSpace(line)
	}

	return ""
}
