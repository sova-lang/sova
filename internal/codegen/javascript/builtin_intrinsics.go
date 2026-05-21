package javascript

import (
	"sova/internal/codegen"
	"sova/internal/codegen/javascript/jsgen"
	"sova/internal/ir"
)

// lookupBuiltinIntrinsicJS mirrors the Go-side helper: returns the canonical built-in name when the call resolves to a compiler-injected symbol, or "" otherwise. Reads from the `builtin_intrinsics` cache key seeded by `compiler.injectBuiltinsIntoPackage` for every package the compiler sees.
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

// emitBuiltinIntrinsicCallJS lowers a known built-in to native JS. `print` / `println` both map to `console.log` (JS conflates the two); `len` uses `.length` when available and falls back to `Object.keys(...).length` for plain objects; `error("msg")` maps to JS's native `Error` constructor which already exposes `.message` on instances, matching the Sova surface.
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
