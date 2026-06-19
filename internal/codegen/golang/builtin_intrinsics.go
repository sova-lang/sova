package golang

import (
	"sova/internal/codegen"
	"sova/internal/ir"

	"github.com/dave/jennifer/jen"
)

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

func emitSovaAnyIndex(block *jen.Group) {
	block.Add(jen.Func().Id("__sovaAnyIndex").Params(
		jen.Id("value").Any(),
		jen.Id("key").Any(),
	).Any().Block(
		jen.If(jen.Id("value").Op("==").Nil()).Block(jen.Return(jen.Nil())),
		jen.Switch(jen.Id("v").Op(":=").Id("value").Assert(jen.Id("type"))).Block(
			jen.Case(jen.Map(jen.String()).Any()).Block(
				jen.If(jen.List(jen.Id("k"), jen.Id("ok")).Op(":=").Id("key").Assert(jen.String()), jen.Id("ok")).Block(
					jen.Return(jen.Id("v").Index(jen.Id("k"))),
				),
				jen.Return(jen.Nil()),
			),
			jen.Case(jen.Map(jen.Any()).Any()).Block(
				jen.Return(jen.Id("v").Index(jen.Id("key"))),
			),
			jen.Case(jen.Index().Any()).Block(
				jen.Var().Id("idx").Int(),
				jen.Switch(jen.Id("k").Op(":=").Id("key").Assert(jen.Id("type"))).Block(
					jen.Case(jen.Int()).Block(jen.Id("idx").Op("=").Id("k")),
					jen.Case(jen.Int64()).Block(jen.Id("idx").Op("=").Int().Parens(jen.Id("k"))),
					jen.Default().Block(jen.Return(jen.Nil())),
				),
				jen.If(jen.Id("idx").Op("<").Lit(0).Op("||").Id("idx").Op(">=").Qual("", "len").Call(jen.Id("v"))).Block(
					jen.Return(jen.Nil()),
				),
				jen.Return(jen.Id("v").Index(jen.Id("idx"))),
			),
		),
		jen.Return(jen.Nil()),
	))
}

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
