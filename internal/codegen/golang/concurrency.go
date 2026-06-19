package golang

import (
	"sova/internal/codegen"
	"sova/internal/ir"

	"github.com/dave/jennifer/jen"
)

func (e *CodeEmitter) emitGoStmt(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, block *jen.Group, s *ir.GoStmt) {
	block.Add(jen.Go().Func().Params().BlockFunc(func(g *jen.Group) {
		if s.Call != nil {
			g.Add(e.buildExpr(ctx, pkg, f, s.Call))
			return
		}

		if s.Body != nil {
			e.emitBlock(ctx, pkg, f, g, s.Body.Stmts)
		}
	}).Call())
}

func (e *CodeEmitter) emitDeferStmt(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, block *jen.Group, s *ir.DeferStmt) {
	if s.Call != nil {
		block.Add(jen.Defer().Add(e.buildExpr(ctx, pkg, f, s.Call)))
		return
	}

	if s.Body != nil {
		block.Add(jen.Defer().Func().Params().BlockFunc(func(g *jen.Group) {
			e.emitBlock(ctx, pkg, f, g, s.Body.Stmts)
		}).Call())
	}
}

func (e *CodeEmitter) emitSelectStmt(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, block *jen.Group, s *ir.SelectStmt) {
	block.Add(jen.Select().BlockFunc(func(g *jen.Group) {
		for _, cc := range s.Cases {
			switch cc.Kind {
			case ir.SelectCaseSend:
				chCode := e.buildExpr(ctx, pkg, f, cc.ChanExpr)
				valCode := e.buildExpr(ctx, pkg, f, cc.SendValue)
				g.Case(chCode.Op("<-").Add(valCode)).BlockFunc(func(cg *jen.Group) {
					if cc.Body != nil {
						e.emitBlock(ctx, pkg, f, cg, cc.Body.Stmts)
					}
				})
			case ir.SelectCaseRecvBind:
				chCode := e.buildExpr(ctx, pkg, f, cc.ChanExpr)
				var lhs []jen.Code
				hasReal := false
				for i := range cc.Targets {
					if cc.Targets[i].Name == nil {
						lhs = append(lhs, jen.Id("_"))
						continue
					}

					name := symNameWithUnused(ctx, pkg, cc.Targets[i].Name.Sym)
					if name != "_" {
						hasReal = true
					}

					lhs = append(lhs, jen.Id(name))
				}

				if !hasReal {
					g.Case(jen.Op("<-").Add(chCode)).BlockFunc(func(cg *jen.Group) {
						if cc.Body != nil {
							e.emitBlock(ctx, pkg, f, cg, cc.Body.Stmts)
						}
					})
					continue
				}

				if len(lhs) == 1 {
					lhs = append(lhs, jen.Id("_"))
				}

				g.Case(jen.List(lhs...).Op(":=").Op("<-").Add(chCode)).BlockFunc(func(cg *jen.Group) {
					if cc.Body != nil {
						e.emitBlock(ctx, pkg, f, cg, cc.Body.Stmts)
					}
				})
			case ir.SelectCaseRecvDiscard:
				chCode := e.buildExpr(ctx, pkg, f, cc.ChanExpr)
				g.Case(jen.Op("<-").Add(chCode)).BlockFunc(func(cg *jen.Group) {
					if cc.Body != nil {
						e.emitBlock(ctx, pkg, f, cg, cc.Body.Stmts)
					}
				})
			}
		}

		if s.Default != nil {
			g.Default().BlockFunc(func(cg *jen.Group) {
				e.emitBlock(ctx, pkg, f, cg, s.Default.Stmts)
			})
		}
	}))
}

func matchChanMethod(ctx *codegen.EmitContext, call *ir.FuncCallExpr) (string, ir.Expr, bool) {
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
