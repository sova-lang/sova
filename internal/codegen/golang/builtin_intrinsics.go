package golang

import (
	"sova/internal/codegen"
	"sova/internal/ir"

	"github.com/dave/jennifer/jen"
)

// emitSovaErrorType writes the Go-side runtime representation of Sova's built-in `error` struct: a tiny `sovaError` value type with a `Message` field, a `message()` accessor matching the Sova-side method, and an `Error()` shim so it satisfies Go's `error` interface for free interop with stdlib bindings.
func emitSovaErrorType(block *jen.Group) {
	block.Add(jen.Type().Id("sovaError").Struct(
		jen.Id("Message").String().Tag(map[string]string{"json": "message"}),
	))
	block.Add(jen.Func().Params(jen.Id("e").Op("*").Id("sovaError")).Id("message").Params().String().Block(
		jen.Return(jen.Id("e").Dot("Message")),
	))
	block.Add(jen.Func().Params(jen.Id("e").Op("*").Id("sovaError")).Id("Error").Params().String().Block(
		jen.Return(jen.Id("e").Dot("Message")),
	))
}

// lookupBuiltinIntrinsic returns the canonical name ("print", "len", "error") if the call's callee resolves to a compiler-registered built-in symbol, or "" when the call is a regular user-defined function. Resolution goes through the `builtin_intrinsics` cache key seeded by `compiler.injectBuiltinsIntoPackage`.
func lookupBuiltinIntrinsic(ctx *codegen.EmitContext, callee ir.Expr) string {
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

// emitBuiltinIntrinsicCall translates a known built-in call into native Go. `print` lowers to `fmt.Println(args...)`, `len` to Go's builtin `len(x)` (polymorphic via the host language), and `error` to a `*sovaError` allocation. Returns nil when the intrinsic has no Go-side specialisation today - caller falls back to normal dispatch.
func emitBuiltinIntrinsicCall(ctx *codegen.EmitContext, intrinsic string, args []jen.Code, argTypes []ir.TypID) *jen.Statement {
	switch intrinsic {
	case "print":
		return jen.Qual("fmt", "Print").Call(args...)
	case "println":
		return jen.Qual("fmt", "Println").Call(args...)
	case "len":
		if len(args) != 1 {
			return nil
		}
		return jen.Id("int64").Call(jen.Qual("", "len").Call(args[0]))
	case "error":
		if len(args) != 1 {
			return nil
		}
		return jen.Op("&").Id("sovaError").Values(jen.Dict{
			jen.Id("Message"): args[0],
		})
	case "after":
		if len(args) != 1 {
			return nil
		}
		return jen.Func().Params().Chan().Any().Block(
			jen.Id("ch").Op(":=").Make(jen.Chan().Any(), jen.Lit(1)),
			jen.Qual("time", "AfterFunc").Call(
				jen.Qual("time", "Duration").Call(args[0]).Op("*").Qual("time", "Millisecond"),
				jen.Func().Params().Block(jen.Id("ch").Op("<-").Nil()),
			),
			jen.Return(jen.Id("ch")),
		).Call()
	case "every":
		if len(args) != 1 {
			return nil
		}
		return jen.Func().Params().Chan().Any().Block(
			jen.Id("ch").Op(":=").Make(jen.Chan().Any(), jen.Lit(1)),
			jen.Id("ticker").Op(":=").Qual("time", "NewTicker").Call(
				jen.Qual("time", "Duration").Call(args[0]).Op("*").Qual("time", "Millisecond"),
			),
			jen.Go().Func().Params().Block(
				jen.For().Range().Id("ticker").Dot("C").Block(
					jen.Select().Block(
						jen.Case(jen.Id("ch").Op("<-").Nil()).Empty(),
						jen.Default().Empty(),
					),
				),
			).Call(),
			jen.Return(jen.Id("ch")),
		).Call()
	}
	return nil
}
