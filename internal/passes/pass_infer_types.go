package passes

import (
	"fmt"
	"strings"

	"sova/internal/diag"
	"sova/internal/ir"
)

type PassInferTypes struct {
	currentReturnTyp ir.TypID
}

func (p *PassInferTypes) Name() string     { return "infer_types" }

func (p *PassInferTypes) Scope() PassScope { return PerPackage }

func (p *PassInferTypes) Requires() []string {
	return []string{"resolve_names", "resolve_typerefs", "precompute_signatures"}
}

func (p *PassInferTypes) NoErrors() bool { return false }

func (p *PassInferTypes) Run(pc *PassContext) error {
	for _, f := range pc.Pkg.Files {
		p.resolveStmts(pc, f.Hir.Statements)
	}

	return nil
}

func (p *PassInferTypes) preComputeStructCtors(pc *PassContext, stmts []ir.Stmt) {
	for _, st := range stmts {
		if ir.IsNilStmt(st) {
			continue
		}

		td, ok := st.(*ir.TypeDeclStmt)
		if !ok || td.Name.Sym == 0 {
			continue
		}

		sym, ok := pc.Pkg.Syms.GetByID(td.Name.Sym)
		if !ok || sym.Typ == 0 {
			continue
		}

		structTy, ok := pc.Types.GetByID(sym.Typ)
		if !ok || structTy.Kind != ir.TK_Struct {
			continue
		}

		ctorInfos := make([]ir.StructCtorInfo, 0, len(td.Ctors))
		for _, ctor := range td.Ctors {
			funcTyp := pc.Types.FuncOf(ctor.Params, sym.Typ)
			ctorInfos = append(ctorInfos, ir.StructCtorInfo{Sym: ctor.Sym, FuncTyp: funcTyp})
		}

		structTy.StructCtors = ctorInfos
	}
}

func (p *PassInferTypes) preComputeEnumCases(pc *PassContext, stmts []ir.Stmt) {
	for _, st := range stmts {
		if ir.IsNilStmt(st) {
			continue
		}

		ed, ok := st.(*ir.EnumDeclStmt)
		if !ok || ed.Name.Sym == 0 {
			continue
		}

		isNumeric := len(ed.Fields) == 0
		nextValue := int64(0)
		var caseInfos []ir.EnumCaseInfo
		for _, c := range ed.Cases {
			if c.Value != nil {
				nextValue = *c.Value
			}

			caseInfos = append(caseInfos, ir.EnumCaseInfo{
				Name:    c.Name.Name,
				Ordinal: c.Ordinal,
				Value:   nextValue,
			})
			nextValue++
		}

		var enumFields []ir.EnumFieldInfo
		for _, field := range ed.Fields {
			fieldType := ir.TypID(0)
			if field.Type != nil {
				fieldType = field.Type.Typ
			}

			enumFields = append(enumFields, ir.EnumFieldInfo{
				Name: field.Name.Name,
				Type: fieldType,
			})
		}

		enumTyp := pc.Types.EnumOf(pc.Pkg.Path.String(), ed.Name.Name, caseInfos, enumFields, isNumeric)
		if enumTy, ok := pc.Types.GetByID(enumTyp); ok {
			enumTy.EnumCases = caseInfos
			enumTy.EnumFields = enumFields
			enumTy.IsNumeric = isNumeric
		}

		pc.Pkg.Syms.SetType(ed.Name.Sym, enumTyp)
		for _, c := range ed.Cases {
			if c.Name.Sym != 0 {
				pc.Pkg.Syms.SetType(c.Name.Sym, enumTyp)
			}
		}
	}
}

func (p *PassInferTypes) preComputeStructMethods(pc *PassContext, stmts []ir.Stmt) {
	for _, st := range stmts {
		if ir.IsNilStmt(st) {
			continue
		}

		td, ok := st.(*ir.TypeDeclStmt)
		if !ok || td.Name.Sym == 0 {
			continue
		}

		sym, ok := pc.Pkg.Syms.GetByID(td.Name.Sym)
		if !ok || sym.Typ == 0 {
			continue
		}

		structTy, ok := pc.Types.GetByID(sym.Typ)
		if !ok || structTy.Kind != ir.TK_Struct {
			continue
		}

		methodInfos := make([]ir.StructMethodInfo, 0, len(td.Methods))
		for _, method := range td.Methods {
			if method == nil || method.Func == nil {
				continue
			}

			fn := method.Func
			if method.ThisSym != 0 {
				pc.Pkg.Syms.SetType(method.ThisSym, sym.Typ)
			}

			for _, param := range fn.Params {
				if param.Type != nil && param.Type.Typ != 0 && param.Name.Sym != 0 {
					pc.Pkg.Syms.SetType(param.Name.Sym, param.Type.Typ)
				}
			}

			returnType := ir.TypID(0)
			if fn.ReturnType != nil {
				returnType = fn.ReturnType.Typ
			}

			if returnType == 0 {
				returnType = pc.Types.TypNone()
			}

			funcTyp := pc.Types.FuncOf(fn.Params, returnType)
			pc.Pkg.Syms.SetType(fn.Name.Sym, funcTyp)
			methodInfos = append(methodInfos, ir.StructMethodInfo{Name: fn.Name.Name, Sym: fn.Name.Sym, FuncTyp: funcTyp, IsShared: method.IsShared})
		}

		structTy.StructMethods = methodInfos
	}
}

func (p *PassInferTypes) preComputeStructFields(pc *PassContext, stmts []ir.Stmt) {
	for _, st := range stmts {
		if ir.IsNilStmt(st) {
			continue
		}

		td, ok := st.(*ir.TypeDeclStmt)
		if !ok {
			continue
		}

		fields := make([]ir.StructFieldInfo, 0, len(td.Fields))
		for _, field := range td.Fields {
			fieldType := ir.TypID(0)
			if field.Type != nil {
				fieldType = field.Type.Typ
			}

			fields = append(fields, ir.StructFieldInfo{
				Name:       field.Name.Name,
				Type:       fieldType,
				Private:    field.Private,
				Sym:        field.Name.Sym,
				IsReactive: hasAnnotation(field.Annotations, "reactive"),
				IsShared:   field.IsShared,
			})
			if field.Name.Sym != 0 && fieldType != 0 {
				pc.Pkg.Syms.SetType(field.Name.Sym, fieldType)
			}
		}

		structTyp := pc.Types.StructOf(pc.Pkg.Path.String(), td.Name.Name, fields)
		if structTy, ok := pc.Types.GetByID(structTyp); ok {
			structTy.StructFields = fields
		}

		if td.Name.Sym != 0 {
			pc.Pkg.Syms.SetType(td.Name.Sym, structTyp)
		}
	}
}

func (p *PassInferTypes) preComputeTopLevelVarSignatures(pc *PassContext, stmts []ir.Stmt) {
	for _, st := range stmts {
		if ir.IsNilStmt(st) {
			continue
		}

		vd, ok := st.(*ir.VarDeclStmt)
		if !ok {
			continue
		}

		for i := range vd.Targets {
			target := &vd.Targets[i]
			if target.Name == nil || target.Name.Sym == 0 {
				continue
			}

			if target.TypeAnn == nil || target.TypeAnn.Typ == 0 {
				continue
			}

			pc.Pkg.Syms.SetType(target.Name.Sym, target.TypeAnn.Typ)
		}
	}
}

func (p *PassInferTypes) preComputeExternSignatures(pc *PassContext, stmts []ir.Stmt) {
	for _, st := range stmts {
		if ir.IsNilStmt(st) {
			continue
		}

		ext, ok := st.(*ir.ExternDeclStmt)
		if !ok {
			continue
		}

		for _, fn := range ext.Funcs {
			if fn.Name.Sym == 0 {
				continue
			}

			for _, param := range fn.Params {
				if param.Name.Sym != 0 && param.Type != nil && param.Type.Typ != 0 {
					pc.Pkg.Syms.SetType(param.Name.Sym, param.Type.Typ)
				}
			}

			var returnType ir.TypID
			if fn.ReturnType != nil && fn.ReturnType.Typ != 0 {
				returnType = fn.ReturnType.Typ
			} else {
				returnType = pc.Types.TypNone()
			}

			var funcTyp ir.TypID
			if fn.IsAsync {
				funcTyp = pc.Types.AsyncFuncOf(fn.Params, returnType)
			} else {
				funcTyp = pc.Types.FuncOf(fn.Params, returnType)
			}

			pc.Pkg.Syms.SetType(fn.Name.Sym, funcTyp)
		}

		for _, v := range ext.Vars {
			if v.Name.Sym == 0 {
				continue
			}

			if v.Type != nil && v.Type.Typ != 0 {
				pc.Pkg.Syms.SetType(v.Name.Sym, v.Type.Typ)
			}
		}
	}
}

func (p *PassInferTypes) preComputeFuncSignatures(pc *PassContext, stmts []ir.Stmt) {
	for _, st := range stmts {
		if ir.IsNilStmt(st) {
			continue
		}

		fn, ok := st.(*ir.FuncDeclStmt)
		if !ok {
			continue
		}

		if fn.Name.Sym == 0 {
			continue
		}

		for _, param := range fn.Params {
			if param.Name.Sym == 0 {
				continue
			}

			if param.Type != nil && param.Type.Typ != 0 {
				pc.Pkg.Syms.SetType(param.Name.Sym, param.Type.Typ)
			}
		}

		var returnType ir.TypID
		if fn.ReturnType != nil {
			returnType = fn.ReturnType.Typ
		}

		if returnType == 0 {
			returnType = pc.Types.TypNone()
		}

		var funcTyp ir.TypID
		if fn.IsWired {
			wireStateTyp := wireStateTypeFromCache(pc)
			inner := returnType
			if inner == 0 {
				inner = pc.Types.TypNone()
			}

			tupleTyp := pc.Types.TupleOf(
				ir.TupleField{Name: "value", Type: inner},
				ir.TupleField{Name: "state", Type: wireStateTyp},
			)
			funcTyp = pc.Types.AsyncFuncOf(fn.Params, tupleTyp)
		} else {
			funcTyp = pc.Types.FuncOf(fn.Params, returnType)
		}

		pc.Pkg.Syms.SetType(fn.Name.Sym, funcTyp)
	}
}

func interfaceFileSide(pc *PassContext, ifaceSym ir.SymID) ir.SideKind {
	if pc == nil || pc.Pkg == nil || ifaceSym == 0 {
		return ir.SideShared
	}

	for _, f := range pc.Pkg.Files {
		if f.Hir == nil {
			continue
		}

		for _, st := range f.Hir.Statements {
			if iface, ok := st.(*ir.InterfaceDeclStmt); ok && iface.Name.Sym == ifaceSym {
				return f.Hir.Side.Kind
			}
		}
	}

	return ir.SideShared
}

func (p *PassInferTypes) resolveStmts(pc *PassContext, stmts []ir.Stmt) {
	for i, st := range stmts {
		if ir.IsNilStmt(st) {
			continue
		}

		if p.isTerminator(st) && i < len(stmts)-1 {
			pc.Diag.Report(diag.ErrUnreachableCode, st.Span())
			break
		}
	}

	for _, st := range stmts {
		if ir.IsNilStmt(st) {
			continue
		}

		switch st := st.(type) {
		case *ir.BlockStmt:
			p.resolveStmts(pc, st.Stmts)
		case *ir.VarDeclStmt:
			if funcLit, ok := st.Init.(*ir.FuncLitExpr); ok && len(st.Targets) == 1 {
				target := &st.Targets[0]
				for _, param := range funcLit.Params {
					if param.Type != nil && param.Type.Typ != 0 {
						pc.Pkg.Syms.SetType(param.Name.Sym, param.Type.Typ)
					} else if param.Default != nil {
						ti := p.synthesizeTypeFromExpr(pc, param.Default)
						pc.Pkg.Syms.SetType(param.Name.Sym, ti)
					} else {
						pc.Pkg.Syms.SetType(param.Name.Sym, pc.Types.TypError())
						pc.Diag.Report(diag.ErrTypeInferenceFailed, param.Name.Span, fmt.Sprintf("parameter '%s'", param.Name.Name))
					}
				}

				funcTyp := pc.Types.FuncOf(funcLit.Params, funcLit.ReturnType.Typ)
				if target.Name != nil {
					pc.Pkg.Syms.SetType(target.Name.Sym, funcTyp)
				}

				funcLit.SetType(funcTyp)

				p.resolveStmts(pc, ir.BlockStmts(funcLit.Body))

				if target.TypeAnn != nil && target.TypeAnn.Typ != 0 {
					expected := target.TypeAnn.Typ
					if ok, _ := isTypeAssignable(pc.Types, expected, funcTyp); !ok {
						exTy, _ := pc.Types.GetByID(expected)
						funcTy, _ := pc.Types.GetByID(funcTyp)
						pc.Diag.Report(diag.ErrTypeMismatch, st.Span(), typeKeyDisplay(exTy), typeKeyDisplay(funcTy))
					}
				} else {
					target.TypeAnn = &ir.TypeRef{Typ: funcTyp}
				}
			} else {
				if len(st.Targets) == 1 && st.Targets[0].TypeAnn != nil && st.Targets[0].TypeAnn.Typ != 0 {
					p.applyLiteralTypeHint(pc, st.Init, st.Targets[0].TypeAnn.Typ)
				}

				tInit := p.synthesizeTypeFromExpr(pc, st.Init)

				if len(st.Targets) == 1 {
					target := &st.Targets[0]
					if target.TypeAnn != nil && target.TypeAnn.Typ != 0 {

						expected := target.TypeAnn.Typ
						if ok, _ := isTypeAssignable(pc.Types, expected, tInit); !ok {
							if wrapped, castOK := tryInsertCast(pc.Types, expected, tInit, st.Init); castOK {
								st.Init = wrapped
							} else {
								pc.Diag.Report(diag.ErrTypeMismatch, st.Span(), typeName(pc, expected), typeName(pc, tInit))
							}
						}

						if target.Name != nil {
							pc.Pkg.Syms.SetType(target.Name.Sym, expected)
						}
					} else {
						if target.Name != nil {
							pc.Pkg.Syms.SetType(target.Name.Sym, tInit)
						}

						target.TypeAnn = &ir.TypeRef{Typ: tInit}
					}
				} else {
					tupleTyp, ok := pc.Types.GetByID(tInit)
					if !ok || tupleTyp.Kind != ir.TK_Tuple {
						pc.Diag.Report(diag.ErrTypeMismatch, st.Span(), "tuple", "non-tuple")
					} else {
						if len(st.Targets) != len(tupleTyp.Fields) {
							pc.Diag.Report(diag.ErrTypeMismatch, st.Span(),
								fmt.Sprintf("expected %d values", len(st.Targets)),
								fmt.Sprintf("got %d values", len(tupleTyp.Fields)))
						} else {
							for i, target := range st.Targets {
								fieldTyp := tupleTyp.Fields[i].Type
								if target.Name != nil {
									pc.Pkg.Syms.SetType(target.Name.Sym, fieldTyp)
								}

								if target.TypeAnn == nil {
									st.Targets[i].TypeAnn = &ir.TypeRef{Typ: fieldTyp}
								}
							}
						}
					}
				}
			}

		case *ir.FuncDeclStmt:
			for _, param := range st.Params {
				if param.Type != nil && param.Type.Typ != 0 {
					pc.Pkg.Syms.SetType(param.Name.Sym, param.Type.Typ)
				} else if param.Default != nil {
					ti := p.synthesizeTypeFromExpr(pc, param.Default)
					pc.Pkg.Syms.SetType(param.Name.Sym, ti)
				} else {
					pc.Pkg.Syms.SetType(param.Name.Sym, pc.Types.TypError())
					pc.Diag.Report(diag.ErrTypeInferenceFailed, param.Name.Span, fmt.Sprintf("parameter '%s'", param.Name.Name))
				}
			}

			prevReturn := p.currentReturnTyp
			if st.ReturnType != nil {
				p.currentReturnTyp = st.ReturnType.Typ
			} else {
				p.currentReturnTyp = 0
			}

			p.resolveStmts(pc, ir.BlockStmts(st.Body))
			p.currentReturnTyp = prevReturn

			var returnType ir.TypID
			if st.ReturnType == nil || st.ReturnType.Typ == 0 {
				returnTypes := p.collectReturnTypes(pc, ir.BlockStmts(st.Body))

				if len(returnTypes) == 0 {
					returnType = pc.Types.TypNone()
				} else {
					var noneReturns []struct {
						Typ  ir.TypID
						Span diag.TextSpan
					}

					var nonNoneReturns []struct {
						Typ  ir.TypID
						Span diag.TextSpan
					}

					for _, rt := range returnTypes {
						if rt.Typ == pc.Types.TypNone() {
							noneReturns = append(noneReturns, rt)
						} else {
							nonNoneReturns = append(nonNoneReturns, rt)
						}
					}

					if len(noneReturns) > 0 && len(nonNoneReturns) > 0 {
						baseType := nonNoneReturns[0].Typ
						for i := 1; i < len(nonNoneReturns); i++ {
							rt := nonNoneReturns[i]
							if ok, _ := isTypeAssignable(pc.Types, baseType, rt.Typ); !ok {
								if ok2, _ := isTypeAssignable(pc.Types, rt.Typ, baseType); ok2 {
									baseType = rt.Typ
								} else {
									pc.Diag.Report(diag.ErrOptionReturnMismatch, rt.Span)
									baseType = pc.Types.TypError()
									break
								}
							}
						}

						if baseType != pc.Types.TypError() {
							returnType = pc.Types.OptionOf(baseType)
						} else {
							returnType = pc.Types.TypError()
						}
					} else if len(noneReturns) > 0 {
						returnType = pc.Types.TypNone()
					} else {
						returnType = nonNoneReturns[0].Typ
						for i := 1; i < len(nonNoneReturns); i++ {
							rt := nonNoneReturns[i]
							if rt.Typ != returnType {
								pc.Diag.Report(diag.ErrTypeMismatch, rt.Span,
									fmt.Sprintf("return type at statement %d", i+1),
									fmt.Sprintf("%s vs %s", typeName(pc, returnType), typeName(pc, rt.Typ)))
							}
						}
					}
				}

				if st.ReturnType == nil {
					st.ReturnType = &ir.TypeRef{Typ: returnType}
				} else {
					st.ReturnType.Typ = returnType
				}
			} else {
				returnType = st.ReturnType.Typ

				returnTypes := p.collectReturnTypes(pc, ir.BlockStmts(st.Body))
				for _, rt := range returnTypes {
					if ok, _ := isTypeAssignable(pc.Types, returnType, rt.Typ); !ok {
						pc.Diag.Report(diag.ErrTypeMismatch, rt.Span,
							typeName(pc, returnType), typeName(pc, rt.Typ))
					}
				}
			}

			var funcTyp ir.TypID
			if st.IsWired {
				wireStateTyp := wireStateTypeFromCache(pc)
				inner := returnType
				if inner == 0 {
					inner = pc.Types.TypNone()
				}

				tupleTyp := pc.Types.TupleOf(
					ir.TupleField{Name: "value", Type: inner},
					ir.TupleField{Name: "state", Type: wireStateTyp},
				)
				funcTyp = pc.Types.AsyncFuncOf(st.Params, tupleTyp)
			} else {
				funcTyp = pc.Types.FuncOf(st.Params, returnType)
			}

			pc.Pkg.Syms.SetType(st.Name.Sym, funcTyp)

		case *ir.ExternDeclStmt:
			for _, fn := range st.Funcs {
				var returnType ir.TypID
				if fn.ReturnType != nil && fn.ReturnType.Typ != 0 {
					returnType = fn.ReturnType.Typ
				} else {
					returnType = pc.Types.TypNone()
				}

				var funcTyp ir.TypID
				if fn.IsAsync {
					funcTyp = pc.Types.AsyncFuncOf(fn.Params, returnType)
				} else {
					funcTyp = pc.Types.FuncOf(fn.Params, returnType)
				}

				pc.Pkg.Syms.SetType(fn.Name.Sym, funcTyp)
			}

			for _, v := range st.Vars {
				if v.Type != nil && v.Type.Typ != 0 {
					pc.Pkg.Syms.SetType(v.Name.Sym, v.Type.Typ)
				} else {
					pc.Pkg.Syms.SetType(v.Name.Sym, pc.Types.TypError())
					pc.Diag.Report(diag.ErrTypeInferenceFailed, v.Name.Span, fmt.Sprintf("extern variable '%s'", v.Name.Name))
				}
			}

			for _, t := range st.Types {
				p.resolveStmts(pc, []ir.Stmt{t})
			}

			for _, iface := range st.Interfaces {
				p.resolveStmts(pc, []ir.Stmt{iface})
			}

		case *ir.TypeAliasStmt:
			if st.Target != nil && st.Target.Typ != 0 && st.Name.Sym != 0 {
				pc.Pkg.Syms.SetType(st.Name.Sym, st.Target.Typ)
			}

		case *ir.InterfaceDeclStmt:
			ifaceTyp := pc.Types.InterfaceOf(pc.Pkg.Path.String(), st.Name.Name)
			pc.Pkg.Syms.SetType(st.Name.Sym, ifaceTyp)

			var sigs []ir.InterfaceSigInfo
			for _, sig := range st.Methods {
				for _, param := range sig.Params {
					if param.Type != nil && param.Type.Typ != 0 {
						pc.Pkg.Syms.SetType(param.Name.Sym, param.Type.Typ)
					}
				}

				retType := ir.TypID(0)
				if sig.ReturnType != nil {
					retType = sig.ReturnType.Typ
				}

				if retType == 0 {
					retType = pc.Types.TypNone()
				}

				funcTyp := pc.Types.FuncOf(sig.Params, retType)
				pc.Pkg.Syms.SetType(sig.Name.Sym, funcTyp)
				isShared := sig.IsShared || interfaceFileSide(pc, st.Name.Sym) == ir.SideShared
				sigs = append(sigs, ir.InterfaceSigInfo{Name: sig.Name.Name, FuncTyp: funcTyp, IsShared: isShared})
			}

			if ifaceTy, ok := pc.Types.GetByID(ifaceTyp); ok {
				ifaceTy.InterfaceMethods = sigs
				if st.IsExtern {
					ifaceTy.IsExtern = true
					ifaceTy.ExternModule = st.ExternModule
				}
			}

		case *ir.TypeDeclStmt:
			var fields []ir.StructFieldInfo
			for _, field := range st.Fields {
				fieldType := ir.TypID(0)
				if field.Type != nil {
					fieldType = field.Type.Typ
				}

				if field.Default != nil {
					if fieldType != 0 {
						p.applyLiteralTypeHint(pc, field.Default, fieldType)
					}

					dt := p.synthesizeTypeFromExpr(pc, field.Default)
					if fieldType == 0 {
						fieldType = dt
					} else if dt != 0 {
						if ok, _ := isTypeAssignable(pc.Types, fieldType, dt); !ok {
							pc.Diag.Report(diag.ErrTypeMismatch, field.Default.Span(), typeName(pc, fieldType), typeName(pc, dt))
						}
					}
				}

				fields = append(fields, ir.StructFieldInfo{
					Name:       field.Name.Name,
					Type:       fieldType,
					Private:    field.Private,
					Sym:        field.Name.Sym,
					IsReactive: hasAnnotation(field.Annotations, "reactive"),
					IsShared:   field.IsShared,
				})
				if field.Name.Sym != 0 {
					pc.Pkg.Syms.SetType(field.Name.Sym, fieldType)
				}
			}

			structTyp := pc.Types.StructOf(pc.Pkg.Path.String(), st.Name.Name, fields)
			if structTy, ok := pc.Types.GetByID(structTyp); ok {
				structTy.StructFields = fields
				if st.IsExtern {
					structTy.IsExtern = true
					structTy.ExternModule = st.ExternModule
				}

				if mentionsComposable(st.MixedIn) {
					structTy.IsComposable = true
				}

				if len(st.TypeParams) > 0 {
					names := make([]string, 0, len(st.TypeParams))
					for _, p := range st.TypeParams {
						names = append(names, p.Name)
					}

					structTy.TypeParams = names
				}
			}

			pc.Pkg.Syms.SetType(st.Name.Sym, structTyp)

			var methodInfos []ir.StructMethodInfo
			for _, method := range st.Methods {
				fn := method.Func
				if method.ThisSym != 0 {
					pc.Pkg.Syms.SetType(method.ThisSym, structTyp)
				}

				for _, param := range fn.Params {
					if param.Type != nil && param.Type.Typ != 0 {
						pc.Pkg.Syms.SetType(param.Name.Sym, param.Type.Typ)
					} else if param.Default != nil {
						ti := p.synthesizeTypeFromExpr(pc, param.Default)
						pc.Pkg.Syms.SetType(param.Name.Sym, ti)
					} else {
						pc.Pkg.Syms.SetType(param.Name.Sym, pc.Types.TypError())
						pc.Diag.Report(diag.ErrTypeInferenceFailed, param.Name.Span, fmt.Sprintf("method parameter '%s'", param.Name.Name))
					}
				}

				returnType := ir.TypID(0)
				if fn.ReturnType != nil {
					returnType = fn.ReturnType.Typ
				}

				if returnType == 0 {
					returnType = pc.Types.TypNone()
				}

				funcTyp := pc.Types.FuncOf(fn.Params, returnType)
				pc.Pkg.Syms.SetType(fn.Name.Sym, funcTyp)
				methodInfos = append(methodInfos, ir.StructMethodInfo{Name: fn.Name.Name, Sym: fn.Name.Sym, FuncTyp: funcTyp, IsShared: method.IsShared})
			}

			var ctorInfos []ir.StructCtorInfo
			for _, ctor := range st.Ctors {
				ctorScope, _ := pc.Pkg.Scopes.EnclosingScope(ir.BlockID(ctor.Body))
				if thisSym, found := pc.Pkg.Scopes.Lookup(ctorScope, "this"); found {
					pc.Pkg.Syms.SetType(thisSym, structTyp)
				}

				for _, param := range ctor.Params {
					if param.Type != nil && param.Type.Typ != 0 {
						pc.Pkg.Syms.SetType(param.Name.Sym, param.Type.Typ)
					} else if param.Default != nil {
						ti := p.synthesizeTypeFromExpr(pc, param.Default)
						pc.Pkg.Syms.SetType(param.Name.Sym, ti)
					} else {
						pc.Pkg.Syms.SetType(param.Name.Sym, pc.Types.TypError())
						pc.Diag.Report(diag.ErrTypeInferenceFailed, param.Name.Span, fmt.Sprintf("ctor parameter '%s'", param.Name.Name))
					}

					if param.Default != nil {
						_ = p.synthesizeTypeFromExpr(pc, param.Default)
					}
				}

				funcTyp := pc.Types.FuncOf(ctor.Params, structTyp)
				pc.Pkg.Syms.SetType(ctor.Sym, funcTyp)
				ctorInfos = append(ctorInfos, ir.StructCtorInfo{Sym: ctor.Sym, FuncTyp: funcTyp})
			}

			for _, ref := range st.MixedIn {
				if ref.Sym == 0 {
					continue
				}

				symPkg := pc.Pkg
				if ref.Qualifier != "" {
					stScope, _ := pc.Pkg.Scopes.EnclosingScope(st.ID())
					if qSym, ok := pc.Pkg.Scopes.Lookup(stScope, ref.Qualifier); ok {
						if pkgSym, found := pc.Pkg.Syms.GetByID(qSym); found && pkgSym.Kind == ir.SK_Package {
							if target := findPackageByPath(pc, pkgSym.PackagePath); target != nil {
								symPkg = target
							}
						}
					}
				}

				embedSym, ok := symPkg.Syms.GetByID(ref.Sym)
				if !ok || embedSym.Typ == 0 {
					continue
				}

				embedTy, ok := pc.Types.GetByID(embedSym.Typ)
				if !ok || embedTy.Kind != ir.TK_Struct {
					continue
				}

				for _, fld := range embedTy.StructFields {
					promoted := fld
					promoted.IsPromoted = true
					promoted.PromotedFromExtern = embedTy.IsExtern
					if structTy, ok := pc.Types.GetByID(structTyp); ok {
						structTy.StructFields = append(structTy.StructFields, promoted)
					}
				}

				for _, m := range embedTy.StructMethods {
					promoted := m
					promoted.IsPromoted = true
					promoted.PromotedFromExtern = embedTy.IsExtern
					methodInfos = append(methodInfos, promoted)
				}
			}

			if structTy, ok := pc.Types.GetByID(structTyp); ok {
				structTy.StructCtors = ctorInfos
				structTy.StructMethods = methodInfos
			}

			for _, method := range st.Methods {
				fn := method.Func
				if fn.Body != nil {
					p.resolveStmts(pc, ir.BlockStmts(fn.Body))
				}
			}

			for _, ctor := range st.Ctors {
				if ctor.Body != nil {
					p.resolveStmts(pc, ir.BlockStmts(ctor.Body))
				}
			}

			var castInfos []ir.StructCastInfo
			for _, cast := range st.Casts {
				if cast.Param == nil {
					continue
				}

				sourceTyp := ir.TypID(0)
				if cast.Param.Type != nil && cast.Param.Type.Typ != 0 {
					sourceTyp = cast.Param.Type.Typ
				}

				if cast.Param.Name.Sym != 0 && sourceTyp != 0 {
					pc.Pkg.Syms.SetType(cast.Param.Name.Sym, sourceTyp)
				}

				returnTyp := structTyp
				if cast.ReturnType != nil && cast.ReturnType.Typ != 0 {
					returnTyp = cast.ReturnType.Typ
				}

				funcTyp := pc.Types.FuncOf([]*ir.FuncParam{cast.Param}, returnTyp)
				pc.Pkg.Syms.SetType(cast.Sym, funcTyp)
				if cast.Body != nil {
					p.resolveStmts(pc, ir.BlockStmts(cast.Body))
				}

				if sourceTyp != 0 {
					castInfos = append(castInfos, ir.StructCastInfo{
						Sym:       cast.Sym,
						SourceTyp: sourceTyp,
						FuncTyp:   funcTyp,
					})
				}
			}

			if len(castInfos) > 0 {
				if structTy, ok := pc.Types.GetByID(structTyp); ok {
					structTy.StructCasts = castInfos
				}
			}

			hasToString := false
			hasHashCode := false
			for _, m := range methodInfos {
				if m.Name == "toString" {
					hasToString = true
				}

				if m.Name == "hashCode" {
					hasHashCode = true
				}
			}

			if !hasToString {
				ft := pc.Types.FuncOf(nil, pc.Types.PrimString())
				methodInfos = append(methodInfos, ir.StructMethodInfo{Name: "toString", FuncTyp: ft})
			}

			if !hasHashCode {
				ft := pc.Types.FuncOf(nil, pc.Types.PrimInt())
				methodInfos = append(methodInfos, ir.StructMethodInfo{Name: "hashCode", FuncTyp: ft})
			}

			for _, field := range st.Fields {
				if !hasAnnotation(field.Annotations, "reactive") {
					continue
				}

				fieldName := field.Name.Name
				fieldTyp := ir.TypID(0)
				if field.Type != nil {
					fieldTyp = field.Type.Typ
				}

				exported := upperFirst(fieldName)
				setParam := &ir.FuncParam{
					Name: ir.NameRef{Name: "v"},
					Type: &ir.TypeRef{Typ: fieldTyp},
				}

				setFt := pc.Types.FuncOf([]*ir.FuncParam{setParam}, pc.Types.TypNone())
				methodInfos = append(methodInfos, ir.StructMethodInfo{
					Name:    "set" + exported,
					FuncTyp: setFt,
				})
				obsFnReturnTyp := pc.Types.FuncOf(nil, pc.Types.TypNone())
				obsParam := &ir.FuncParam{
					Name: ir.NameRef{Name: "fn"},
					Type: &ir.TypeRef{Typ: pc.Types.FuncOf([]*ir.FuncParam{
						{Name: ir.NameRef{Name: "old"}, Type: &ir.TypeRef{Typ: fieldTyp}},
						{Name: ir.NameRef{Name: "new"}, Type: &ir.TypeRef{Typ: fieldTyp}},
					}, pc.Types.TypNone())},
				}

				obsFt := pc.Types.FuncOf([]*ir.FuncParam{obsParam}, obsFnReturnTyp)
				methodInfos = append(methodInfos, ir.StructMethodInfo{
					Name:    "observe" + exported,
					FuncTyp: obsFt,
				})
			}

			var implementsList []ir.TypID
			for _, impl := range st.Implements {
				if impl.Sym == 0 {
					continue
				}

				ifaceSym, ok := pc.Pkg.Syms.GetByID(impl.Sym)
				if !ok {
					continue
				}

				ifaceTy, ok := pc.Types.GetByID(ifaceSym.Typ)
				if !ok || ifaceTy.Kind != ir.TK_Interface {
					pc.Diag.Report(diag.ErrTypeMismatch, impl.Span, "interface", impl.Name)
					continue
				}

				implementsList = append(implementsList, ifaceSym.Typ)
				for _, want := range ifaceTy.InterfaceMethods {
					match := ir.StructMethodInfo{}

					hit := false
					for _, m := range methodInfos {
						if m.Name == want.Name {
							match = m
							hit = true
							break
						}
					}

					if !hit {
						pc.Diag.Report(diag.ErrInterfaceNotImplemented, st.Name.Span, st.Name.Name, impl.Name, want.Name)
						continue
					}

					if pc.Types.GetFunctionSignatureKey(match.FuncTyp) != pc.Types.GetFunctionSignatureKey(want.FuncTyp) {
						pc.Diag.Report(diag.ErrInterfaceMethodSigMismatch, st.Name.Span, st.Name.Name, want.Name, impl.Name)
					}

					if want.IsShared && !match.IsShared {
						pc.Diag.Report(diag.ErrInterfaceSharedMethodNotShared, st.Name.Span, st.Name.Name, impl.Name, want.Name)
					}
				}
			}

			if structTy, ok := pc.Types.GetByID(structTyp); ok {
				structTy.StructCtors = ctorInfos
				structTy.StructMethods = methodInfos
				structTy.StructImplements = implementsList
			}

		case *ir.EnumDeclStmt:

			isNumeric := len(st.Fields) == 0

			nextValue := int64(0)
			var caseInfos []ir.EnumCaseInfo

			for _, c := range st.Cases {
				if c.Value != nil {
					nextValue = *c.Value
				}

				caseInfos = append(caseInfos, ir.EnumCaseInfo{
					Name:    c.Name.Name,
					Ordinal: c.Ordinal,
					Value:   nextValue,
				})
				nextValue++

				if !isNumeric {
					for i, arg := range c.Args {
						argType := p.synthesizeTypeFromExpr(pc, arg)
						if i < len(st.Fields) && st.Fields[i].Type != nil {
							fieldType := st.Fields[i].Type.Typ
							if fieldType != 0 {
								if ok, _ := isTypeAssignable(pc.Types, fieldType, argType); !ok {
									fieldTy, _ := pc.Types.GetByID(fieldType)
									argTy, _ := pc.Types.GetByID(argType)
									fieldKey := "unknown"
									argKey := "unknown"
									if fieldTy != nil {
										fieldKey = string(typeKeyDisplay(fieldTy))
									}

									if argTy != nil {
										argKey = string(typeKeyDisplay(argTy))
									}

									pc.Diag.Report(diag.ErrTypeMismatch, arg.Span(), fieldKey, argKey)
								}
							}
						}
					}
				}
			}

			var enumFields []ir.EnumFieldInfo
			for _, field := range st.Fields {
				fieldType := ir.TypID(0)
				if field.Type != nil {
					fieldType = field.Type.Typ
				}

				enumFields = append(enumFields, ir.EnumFieldInfo{
					Name: field.Name.Name,
					Type: fieldType,
				})
			}

			enumTyp := pc.Types.EnumOf(pc.Pkg.Path.String(), st.Name.Name, caseInfos, enumFields, isNumeric)
			if enumTy, ok := pc.Types.GetByID(enumTyp); ok {
				enumTy.EnumCases = caseInfos
				enumTy.EnumFields = enumFields
				enumTy.IsNumeric = isNumeric
			}

			pc.Pkg.Syms.SetType(st.Name.Sym, enumTyp)

			for _, c := range st.Cases {
				pc.Pkg.Syms.SetType(c.Name.Sym, enumTyp)
			}

			var enumMethods []ir.EnumMethodInfo
			for _, method := range st.Methods {

				methodScope, _ := pc.Pkg.Scopes.EnclosingScope(ir.BlockID(method.Body))
				if thisSym, found := pc.Pkg.Scopes.Lookup(methodScope, "this"); found {
					pc.Pkg.Syms.SetType(thisSym, enumTyp)
				}

				for _, param := range method.Params {
					if param.Type != nil && param.Type.Typ != 0 {
						pc.Pkg.Syms.SetType(param.Name.Sym, param.Type.Typ)
					} else if param.Default != nil {
						ti := p.synthesizeTypeFromExpr(pc, param.Default)
						pc.Pkg.Syms.SetType(param.Name.Sym, ti)
					} else {
						pc.Pkg.Syms.SetType(param.Name.Sym, pc.Types.TypError())
						pc.Diag.Report(diag.ErrTypeInferenceFailed, param.Name.Span, fmt.Sprintf("parameter '%s'", param.Name.Name))
					}
				}

				p.resolveStmts(pc, ir.BlockStmts(method.Body))

				var returnType ir.TypID
				if method.ReturnType == nil || method.ReturnType.Typ == 0 {
					returnTypes := p.collectReturnTypes(pc, ir.BlockStmts(method.Body))
					if len(returnTypes) == 0 {
						returnType = pc.Types.TypNone()
					} else {
						returnType = returnTypes[0].Typ
					}

					if method.ReturnType == nil {
						method.ReturnType = &ir.TypeRef{Typ: returnType}
					} else {
						method.ReturnType.Typ = returnType
					}
				} else {
					returnType = method.ReturnType.Typ
				}

				funcTyp := pc.Types.FuncOf(method.Params, returnType)
				pc.Pkg.Syms.SetType(method.Name.Sym, funcTyp)

				methodOrigName := method.Name.Name
				enumMethods = append(enumMethods, ir.EnumMethodInfo{
					Name: methodOrigName,
					Type: funcTyp,
				})
			}

			if enumTy, ok := pc.Types.GetByID(enumTyp); ok {
				enumTy.EnumMethods = enumMethods
			}

		case *ir.ExprStmt:
			p.synthesizeTypeFromExpr(pc, st.Expr)
		case *ir.FieldAssignmentStmt:
			if st.Receiver.Sym == 0 {
				p.synthesizeTypeFromExpr(pc, st.Value)
				continue
			}

			recvSym, ok := pc.Pkg.Syms.GetByID(st.Receiver.Sym)
			if !ok {
				p.synthesizeTypeFromExpr(pc, st.Value)
				continue
			}

			cur := recvSym.Typ
			for _, fld := range st.Fields {
				ty, ok := pc.Types.GetByID(cur)
				if !ok || ty.Kind != ir.TK_Struct {
					pc.Diag.Report(diag.ErrTypeNotIndexable, fld.Span, typeName(pc, cur))
					cur = pc.Types.TypError()
					break
				}

				found := false
				for _, sf := range ty.StructFields {
					if sf.Name == fld.Name {
						cur = sf.Type
						found = true
						break
					}
				}

				if !found {
					pc.Diag.Report(diag.ErrTypeNotIndexable, fld.Span, fmt.Sprintf("type %s has no field '%s'", ty.StructName, fld.Name))
					cur = pc.Types.TypError()
					break
				}
			}

			if cur != 0 && cur != pc.Types.TypError() {
				preTypeEmptyLiteralFromContext(pc.Types, st.Value, cur)
			}

			valTyp := p.synthesizeTypeFromExpr(pc, st.Value)
			if cur != 0 && cur != pc.Types.TypError() {
				if assignable, _ := isTypeAssignable(pc.Types, cur, valTyp); !assignable {
					pc.Diag.Report(diag.ErrTypeMismatch, st.Span(), typeName(pc, cur), typeName(pc, valTyp))
				}
			}

		case *ir.IndexAssignmentStmt:
			tRecv := p.synthesizeTypeFromExpr(pc, st.Receiver)
			tIdx := p.synthesizeTypeFromExpr(pc, st.Index)
			if recvTy, ok := pc.Types.GetByID(tRecv); ok {
				switch recvTy.Kind {
				case ir.TK_Map:
					preTypeEmptyLiteralFromContext(pc.Types, st.Value, recvTy.ValueType)
				case ir.TK_Slice, ir.TK_Array:
					preTypeEmptyLiteralFromContext(pc.Types, st.Value, recvTy.ElemType)
				}
			}

			tVal := p.synthesizeTypeFromExpr(pc, st.Value)
			recvTy, ok := pc.Types.GetByID(tRecv)
			if !ok {
				continue
			}

			switch recvTy.Kind {
			case ir.TK_Map:
				if assignable, _ := isTypeAssignable(pc.Types, recvTy.KeyType, tIdx); !assignable {
					pc.Diag.Report(diag.ErrTypeMismatch, st.Index.Span(), typeName(pc, recvTy.KeyType), typeName(pc, tIdx))
				}

				if assignable, _ := isTypeAssignable(pc.Types, recvTy.ValueType, tVal); !assignable {
					pc.Diag.Report(diag.ErrTypeMismatch, st.Value.Span(), typeName(pc, recvTy.ValueType), typeName(pc, tVal))
				}

			case ir.TK_Slice, ir.TK_Array:
				if tIdx != pc.Types.PrimInt() {
					pc.Diag.Report(diag.ErrTypeMismatch, st.Index.Span(), "int", typeName(pc, tIdx))
				}

				if assignable, _ := isTypeAssignable(pc.Types, recvTy.ElemType, tVal); !assignable {
					pc.Diag.Report(diag.ErrTypeMismatch, st.Value.Span(), typeName(pc, recvTy.ElemType), typeName(pc, tVal))
				}

			default:
				pc.Diag.Report(diag.ErrTypeNotIndexable, st.Receiver.Span(), typeKeyDisplay(recvTy))
			}

		case *ir.MultiAssignmentStmt:
			if len(st.Targets) == 1 && st.Targets[0].Name != nil && st.Targets[0].Name.Sym != 0 {
				if tgtSym, ok := pc.Pkg.Syms.GetByID(st.Targets[0].Name.Sym); ok {
					preTypeEmptyLiteralFromContext(pc.Types, st.Value, tgtSym.Typ)
				}
			}

			tValue := p.synthesizeTypeFromExpr(pc, st.Value)

			if len(st.Targets) == 1 {

				break
			}

			tupleTyp, ok := pc.Types.GetByID(tValue)
			if !ok || tupleTyp.Kind != ir.TK_Tuple {
				pc.Diag.Report(diag.ErrTypeMismatch, st.Span(), "tuple", "non-tuple")
			} else {
				if len(st.Targets) != len(tupleTyp.Fields) {
					pc.Diag.Report(diag.ErrTypeMismatch, st.Span(),
						fmt.Sprintf("expected %d values", len(st.Targets)),
						fmt.Sprintf("got %d values", len(tupleTyp.Fields)))
				}
			}

		case *ir.IfStmt:
			tCond := p.synthesizeTypeFromExpr(pc, st.Cond)
			if tCond != pc.Types.PrimBool() {
				pc.Diag.Report(diag.ErrTypeMismatch, st.Cond.Span(), "bool", typeName(pc, tCond))
			}

			thenNarrow, elseNarrow := p.detectNoneNarrowing(pc, st.Cond)
			p.withNarrowedTypes(pc, thenNarrow, func() {
				p.resolveStmts(pc, ir.BlockStmts(st.Then))
			})
			for _, eb := range st.ElseIfs {
				tCond := p.synthesizeTypeFromExpr(pc, eb.Cond)
				if tCond != pc.Types.PrimBool() {
					pc.Diag.Report(diag.ErrTypeMismatch, eb.Cond.Span(), "bool", typeName(pc, tCond))
				}

				ebThen, _ := p.detectNoneNarrowing(pc, eb.Cond)
				p.withNarrowedTypes(pc, ebThen, func() {
					p.resolveStmts(pc, ir.BlockStmts(eb.Then))
				})
			}

			if st.Else != nil {
				p.withNarrowedTypes(pc, elseNarrow, func() {
					p.resolveStmts(pc, ir.BlockStmts(st.Else))
				})
			}

		case *ir.SwitchStmt:
			if st.Expr != nil {
				p.synthesizeTypeFromExpr(pc, st.Expr)
			}

			for _, cs := range st.Cases {
				for _, ce := range cs.Values {
					p.synthesizeTypeFromExpr(pc, ce)
				}

				p.resolveStmts(pc, cs.Stmts)
			}

			if st.Default != nil {
				p.resolveStmts(pc, st.Default)
			}

		case *ir.ReturnStmt:
			for i, result := range st.Results {
				if p.currentReturnTyp != 0 {
					preTypeEmptyLiteralFromContext(pc.Types, result, p.currentReturnTyp)
				}

				rt := p.synthesizeTypeFromExpr(pc, result)
				if p.currentReturnTyp != 0 && rt != 0 && rt != p.currentReturnTyp {
					if assignable, _ := isTypeAssignable(pc.Types, p.currentReturnTyp, rt); !assignable {
						if wrapped, castOK := tryInsertCast(pc.Types, p.currentReturnTyp, rt, result); castOK {
							st.Results[i] = wrapped
						}
					}
				}
			}

		case *ir.GuardStmt:
			tCond := p.synthesizeTypeFromExpr(pc, st.Cond)
			if tCond != pc.Types.PrimBool() && !pc.Types.IsTypeOfKind(tCond, ir.TK_Option) {
				pc.Diag.Report(diag.ErrTypeMismatch, st.Cond.Span(), "bool or option<T>", typeName(pc, tCond))
			}

			if pc.Types.IsTypeOfKind(tCond, ir.TK_Option) {
				if vr, ok := st.Cond.(*ir.VarRef); !ok {
					pc.Diag.Report(diag.ErrInvalidGuardOptionUnwrap, st.Cond.Span())
				} else {
					tCondType, _ := pc.Types.GetByID(tCond)
					s, _ := pc.Pkg.Syms.GetByID(vr.Ref.Sym)
					oldTyp := s.Typ
					pc.Pkg.Syms.SetType(vr.Ref.Sym, tCondType.ElemType)
					defer pc.Pkg.Syms.SetType(vr.Ref.Sym, oldTyp)

				}
			}

			for _, ret := range st.Returns {
				p.synthesizeTypeFromExpr(pc, ret)
			}

		case *ir.ForStmt:
			if st.CondInt != nil {
				if st.CondInt.Init != nil {
					ti := p.synthesizeTypeFromExpr(pc, st.CondInt.Init.Init)
					expected := pc.Types.PrimInt()
					if ok, _ := isTypeAssignable(pc.Types, expected, ti); !ok {
						exTy, _ := pc.Types.GetByID(expected)
						tiTy, _ := pc.Types.GetByID(ti)
						pc.Diag.Report(diag.ErrTypeMismatch, st.CondInt.Init.Span(), typeKeyDisplay(exTy), typeKeyDisplay(tiTy))
					}
				}

				if st.CondInt.Cond != nil {
					tCond := p.synthesizeTypeFromExpr(pc, st.CondInt.Cond)
					if tCond != pc.Types.PrimBool() {
						pc.Diag.Report(diag.ErrTypeMismatch, st.CondInt.Cond.Span(), "bool", typeName(pc, tCond))
					}
				}

				if st.CondInt.Post != nil {
					p.synthesizeTypeFromExpr(pc, st.CondInt.Post)
				}
			} else if st.CondIn != nil {
				ti := p.synthesizeTypeFromExpr(pc, st.CondIn.IterExpr)
				iterTy, _ := pc.Types.GetByID(ti)

				var firstExpected, secondExpected, thirdExpected ir.TypID
				switch iterTy.Kind {
				case ir.TK_Array, ir.TK_Slice:
					firstExpected = iterTy.ElemType
					secondExpected = pc.Types.PrimInt()
				case ir.TK_Map:
					firstExpected = iterTy.KeyType
					secondExpected = iterTy.ValueType
					thirdExpected = pc.Types.PrimInt()
				case ir.TK_Struct:
					if nextSym, elem := findIterableNext(pc, iterTy); nextSym != 0 {
						firstExpected = elem
						secondExpected = pc.Types.PrimInt()
						st.CondIn.IterNextSym = nextSym
					} else {
						pc.Diag.Report(diag.ErrTypeMismatch, st.CondIn.IterExpr.Span(), "iterable type", typeKeyDisplay(iterTy))
					}

				default:
					pc.Diag.Report(diag.ErrTypeMismatch, st.CondIn.IterExpr.Span(), "iterable type", typeKeyDisplay(iterTy))
				}

				if st.CondIn.InFirstVar.Sym != 0 && firstExpected != 0 {
					pc.Pkg.Syms.SetType(st.CondIn.InFirstVar.Sym, firstExpected)
				}

				if st.CondIn.InSecondVar != nil {
					if secondExpected == 0 {
						pc.Diag.Report(diag.ErrTypeMismatch, st.CondIn.InSecondVar.Span, "iterable yielding a second value", typeKeyDisplay(iterTy))
					} else if st.CondIn.InSecondVar.Sym != 0 {
						pc.Pkg.Syms.SetType(st.CondIn.InSecondVar.Sym, secondExpected)
					}
				}

				if st.CondIn.InThirdVar != nil {
					if thirdExpected == 0 {
						pc.Diag.Report(diag.ErrTypeMismatch, st.CondIn.InThirdVar.Span, "map type for index variable", typeKeyDisplay(iterTy))
					} else if st.CondIn.InThirdVar.Sym != 0 {
						pc.Pkg.Syms.SetType(st.CondIn.InThirdVar.Sym, thirdExpected)
					}
				}
			} else if st.CondRange != nil {
				tStart := p.synthesizeTypeFromExpr(pc, st.CondRange.RangeStart)
				tEnd := p.synthesizeTypeFromExpr(pc, st.CondRange.RangeEnd)
				expected := pc.Types.PrimInt()

				if ok, _ := isTypeAssignable(pc.Types, expected, tStart); !ok {
					exTy, _ := pc.Types.GetByID(expected)
					tStartTy, _ := pc.Types.GetByID(tStart)
					pc.Diag.Report(diag.ErrTypeMismatch, st.CondRange.RangeStart.Span(), typeKeyDisplay(exTy), typeKeyDisplay(tStartTy))
				}

				if ok, _ := isTypeAssignable(pc.Types, expected, tEnd); !ok {
					exTy, _ := pc.Types.GetByID(expected)
					tEndTy, _ := pc.Types.GetByID(tEnd)
					pc.Diag.Report(diag.ErrTypeMismatch, st.CondRange.RangeEnd.Span(), typeKeyDisplay(exTy), typeKeyDisplay(tEndTy))
				}
			}

			p.resolveStmts(pc, ir.BlockStmts(st.Body))
		case *ir.WhileStmt:
			tCond := p.synthesizeTypeFromExpr(pc, st.Cond)
			if tCond != pc.Types.PrimBool() {
				pc.Diag.Report(diag.ErrTypeMismatch, st.Cond.Span(), "bool", typeName(pc, tCond))
			}

			p.resolveStmts(pc, ir.BlockStmts(st.Body))
		case *ir.TestDeclStmt:
			if st.Body != nil {
				p.resolveStmts(pc, ir.BlockStmts(st.Body))
			}

		case *ir.GroupDeclStmt:
			p.resolveStmts(pc, st.Body)
		case *ir.SetupStmt:
			if st.Body != nil {
				p.resolveStmts(pc, ir.BlockStmts(st.Body))
			}

		case *ir.TeardownStmt:
			if st.Body != nil {
				p.resolveStmts(pc, ir.BlockStmts(st.Body))
			}

		case *ir.AssertStmt:
			_ = p.synthesizeTypeFromExpr(pc, st.Expr)
		case *ir.AsSessionStmt:
			if st.Body != nil {
				p.resolveStmts(pc, ir.BlockStmts(st.Body))
			}

		case *ir.GoStmt:
			if st.Body != nil {
				p.resolveStmts(pc, ir.BlockStmts(st.Body))
			}

			if st.Call != nil {
				_ = p.synthesizeTypeFromExpr(pc, st.Call)
			}

		case *ir.DeferStmt:
			if st.Body != nil {
				p.resolveStmts(pc, ir.BlockStmts(st.Body))
			}

			if st.Call != nil {
				_ = p.synthesizeTypeFromExpr(pc, st.Call)
			}

		case *ir.SelectStmt:
			for _, cc := range st.Cases {
				if cc.ChanExpr != nil {
					_ = p.synthesizeTypeFromExpr(pc, cc.ChanExpr)
				}

				if cc.SendValue != nil {
					_ = p.synthesizeTypeFromExpr(pc, cc.SendValue)
				}

				if cc.Kind == ir.SelectCaseRecvBind {
					chTy := ir.TypID(0)
					if cc.ChanExpr != nil {
						chTy = cc.ChanExpr.GetType()
					}

					var elemTyp ir.TypID
					if ty, ok := pc.Types.GetByID(chTy); ok && ty.Kind == ir.TK_Chan {
						elemTyp = ty.ElemType
					}

					for i := range cc.Targets {
						tgt := &cc.Targets[i]
						if tgt.Name == nil {
							continue
						}

						var bindTyp ir.TypID
						if i == 0 {
							bindTyp = elemTyp
						} else {
							bindTyp = pc.Types.PrimBool()
						}

						pc.Pkg.Syms.SetType(tgt.Name.Sym, bindTyp)
						if tgt.TypeAnn == nil {
							tgt.TypeAnn = &ir.TypeRef{Typ: bindTyp}
						}
					}
				}

				if cc.Body != nil {
					p.resolveStmts(pc, ir.BlockStmts(cc.Body))
				}
			}

			if st.Default != nil {
				p.resolveStmts(pc, st.Default.Stmts)
			}
		}
	}
}

func (p *PassInferTypes) synthesizeTypeFromExpr(pc *PassContext, expr ir.Expr) ir.TypID {
	if ir.IsNilExpr(expr) {
		return 0
	}

	tt := pc.Types
	sa := pc.Pkg.Syms
	switch x := expr.(type) {
	case *ir.WhenExpr:
		valueType := p.synthesizeTypeFromExpr(pc, x.Expr)

		var returnType ir.TypID
		for _, c := range x.Cases {
			for _, v := range c.Values {
				tVal := p.synthesizeTypeFromExpr(pc, v)
				if valueType == 0 {
					valueType = tVal
				} else if ok, _ := isTypeAssignable(tt, valueType, tVal); !ok {
					pc.Diag.Report(diag.ErrTypeMismatch, v.Span(), typeName(pc, valueType), typeName(pc, tVal))
				}
			}

			tRet := p.synthesizeTypeFromExpr(pc, c.Then)
			if returnType == 0 {
				returnType = tRet
			} else if ok, _ := isTypeAssignable(tt, returnType, tRet); !ok {
				returnType = tt.PrimAny()
			}
		}

		tDef := p.synthesizeTypeFromExpr(pc, x.Default)
		if returnType == 0 {
			returnType = tDef
		} else if ok, _ := isTypeAssignable(tt, returnType, tDef); !ok {
			returnType = tt.PrimAny()
		}

		x.SetType(returnType)

		return returnType
	case *ir.UnaryExpr:
		t := p.synthesizeTypeFromExpr(pc, x.Expr)
		switch x.Op {
		case ir.OpLNot:
			if t != tt.PrimBool() {
				pc.Diag.Report(diag.ErrTypeMismatch, x.Span(), "bool", typeName(pc, t))
				t = tt.TypError()
			}

			x.SetType(tt.PrimBool())
			return tt.PrimBool()
		case ir.OpAdd, ir.OpSub:
			if t != tt.PrimInt() && t != tt.PrimFloat() {
				pc.Diag.Report(diag.ErrTypeMismatch, x.Span(), "int or float", typeName(pc, t))
				t = tt.TypError()
			}

			x.SetType(t)
			return t
		case ir.OpNot:
			if t != tt.PrimInt() {
				pc.Diag.Report(diag.ErrTypeMismatch, x.Span(), "int", typeName(pc, t))
				t = tt.TypError()
			}

			x.SetType(t)
			return t
		default:
			x.SetType(tt.TypError())
			return tt.TypError()
		}

	case *ir.PrefixUnaryExpr:
		t := p.synthesizeTypeFromExpr(pc, x.Expr)
		if t != tt.PrimInt() && t != tt.PrimFloat() {
			pc.Diag.Report(diag.ErrTypeMismatch, x.Span(), "int or float", typeName(pc, t))
			t = tt.TypError()
		}

		x.SetType(t)
		return t

	case *ir.PostfixUnaryExpr:
		t := p.synthesizeTypeFromExpr(pc, x.Expr)
		if t != tt.PrimInt() && t != tt.PrimFloat() {
			pc.Diag.Report(diag.ErrTypeMismatch, x.Span(), "int or float", typeName(pc, t))
			t = tt.TypError()
		}

		x.SetType(t)
		return t
	case *ir.BinaryExpr:
		return p.synthesizeBinaryExprType(pc, x)
	case *ir.CoalesceExpr:
		tLeft := p.synthesizeTypeFromExpr(pc, x.Left)
		tDefault := p.synthesizeTypeFromExpr(pc, x.Default)

		leftTy, ok := tt.GetByID(tLeft)
		if !ok || (leftTy.Kind != ir.TK_Option && leftTy.Kind != ir.TK_PrimitiveNone) {
			pc.Diag.Report(diag.ErrTypeMismatch, x.Left.Span(), "option<T>", typeName(pc, tLeft))
			x.SetType(tt.TypError())
			return tt.TypError()
		}

		if ok, _ := isTypeAssignable(tt, leftTy.ElemType, tDefault); !ok {
			pc.Diag.Report(diag.ErrTypeMismatch, x.Default.Span(), typeName(pc, leftTy.ElemType), typeName(pc, tDefault))
			x.SetType(tt.TypError())
			return tt.TypError()
		}

		x.SetType(leftTy.ElemType)
		return leftTy.ElemType
	case *ir.TenaryExpr:
		tc := p.synthesizeTypeFromExpr(pc, x.Cond)
		if tc != tt.PrimBool() {
			pc.Diag.Report(diag.ErrTypeMismatch, x.Cond.Span(), "bool", typeName(pc, tc))
		}

		tThen := p.synthesizeTypeFromExpr(pc, x.Then)
		tElse := p.synthesizeTypeFromExpr(pc, x.Else)

		if tThen == tElse && tThen != 0 {
			x.SetType(tThen)
			return tThen
		}

		if t, ok := func() (ir.TypID, bool) {
			isNum := func(t ir.TypID) bool { return t == tt.PrimInt() || t == tt.PrimFloat() }

			if isNum(tThen) && isNum(tElse) {
				if tThen == tElse {
					return tThen, true
				}

				return tt.PrimFloat(), true
			}

			return 0, false
		}(); ok {
			x.SetType(t)
			return t
		}

		if ok, _ := isTypeAssignable(tt, tThen, tElse); ok {
			x.SetType(tElse)
			return tElse
		}

		if ok, _ := isTypeAssignable(tt, tElse, tThen); ok {
			x.SetType(tThen)
			return tThen
		}

		pc.Diag.Report(diag.ErrTypeMismatch, x.Span(), typeName(pc, tThen), typeName(pc, tElse))
		x.SetType(tt.TypError())
		return tt.TypError()
	case *ir.GroupedExpr:
		t := p.synthesizeTypeFromExpr(pc, x.Expr)
		x.SetType(t)
		return t
	case *ir.OptionUnwrapExpr:
		srcTy := p.synthesizeTypeFromExpr(pc, x.Expr)
		srcInfo, ok := tt.GetByID(srcTy)
		if ok && srcInfo.Kind == ir.TK_Option {
			x.SetType(srcInfo.ElemType)
			return srcInfo.ElemType
		}

		x.IsNoOp = true
		x.SetType(srcTy)
		return srcTy
	case *ir.AsExpr:
		srcTy := p.synthesizeTypeFromExpr(pc, x.Expr)
		if x.Target == nil || x.Target.Typ == 0 {
			x.SetType(tt.TypError())
			return tt.TypError()
		}

		dstTy := x.Target.Typ
		if srcTy != 0 {
			if srcInfo, ok := tt.GetByID(srcTy); ok && srcInfo.Kind == ir.TK_Option {
				if dstInfo, ok2 := tt.GetByID(dstTy); !ok2 || dstInfo.Kind != ir.TK_Option {
					pc.Diag.Report(diag.ErrCastFromOption, x.Span(), typeName(pc, srcInfo.ElemType), typeName(pc, dstTy))
					x.SetType(tt.TypError())
					return tt.TypError()
				}
			}
		}

		if srcTy != 0 && srcTy != tt.PrimAny() && dstTy != tt.PrimAny() && srcTy != dstTy {
			if !isPrimitiveConversionAllowed(tt, srcTy, dstTy) && !isHandleWrapperCast(tt, srcTy, dstTy) {
				if ok, _ := isTypeAssignable(tt, dstTy, srcTy); !ok {
					if ok2, _ := isTypeAssignable(tt, srcTy, dstTy); !ok2 {
						pc.Diag.Report(diag.ErrCastNotAllowed, x.Span(), typeName(pc, srcTy), typeName(pc, dstTy))
						x.SetType(tt.TypError())
						return tt.TypError()
					}
				}
			}
		}

		if x.Safe {
			t := tt.OptionOf(dstTy)
			x.SetType(t)
			return t
		}

		x.SetType(dstTy)
		return dstTy
	case *ir.InstanceofExpr:
		p.synthesizeTypeFromExpr(pc, x.Expr)
		x.SetType(tt.PrimBool())
		return tt.PrimBool()
	case *ir.AssignmentExpr:
		leftSym, ok := sa.GetByID(x.Left.Sym)
		if !ok {
			return tt.TypError()
		}

		preTypeEmptyLiteralFromContext(tt, x.Right, leftSym.Typ)
		tRight := p.synthesizeTypeFromExpr(pc, x.Right)
		if ok, _ := isTypeAssignable(tt, leftSym.Typ, tRight); ok {
			x.SetType(leftSym.Typ)
			return leftSym.Typ
		}

		leftTy, _ := tt.GetByID(leftSym.Typ)
		tRightTy, _ := tt.GetByID(tRight)
		pc.Diag.Report(diag.ErrTypeMismatch, x.Span(), typeKeyDisplay(leftTy), typeKeyDisplay(tRightTy))

		x.SetType(tt.TypError())
		return tt.TypError()
	case *ir.IndexExpr:
		tBase := p.synthesizeTypeFromExpr(pc, x.Expr)
		baseTy, ok := tt.GetByID(tBase)
		tIndex := p.synthesizeTypeFromExpr(pc, x.Index)
		tIndexTy, _ := tt.GetByID(tIndex)
		if !ok {
			return tt.TypError()
		}

		switch baseTy.Kind {
		case ir.TK_Array, ir.TK_Slice:
			if tIndex != tt.PrimInt() {
				pc.Diag.Report(diag.ErrTypeMismatch, x.Index.Span(), "int", tIndexTy)
				return tt.TypError()
			}

			x.SetType(baseTy.ElemType)
			return baseTy.ElemType
		case ir.TK_Map:
			if ok, _ := isTypeAssignable(tt, baseTy.KeyType, tIndex); !ok {
				pc.Diag.Report(diag.ErrTypeMismatch, x.Index.Span(), baseTy.KeyType, tIndexTy)
				return tt.TypError()
			}

			x.SetType(baseTy.ValueType)
			return baseTy.ValueType
		case ir.TK_PrimitiveString:
			if tIndex != tt.PrimInt() {
				pc.Diag.Report(diag.ErrTypeMismatch, x.Index.Span(), "int", tIndexTy)
				return tt.TypError()
			}

			x.SetType(tt.PrimByte())
			return tt.PrimByte()
		case ir.TK_PrimitiveAny:

			x.SetType(tt.PrimAny())
			return tt.PrimAny()
		default:
			pc.Diag.Report(diag.ErrTypeNotIndexable, x.Expr.Span(), typeKeyDisplay(baseTy))
			return tt.TypError()
		}

	case *ir.SliceRangeExpr:
		tBase := p.synthesizeTypeFromExpr(pc, x.Expr)
		baseTy, ok := tt.GetByID(tBase)
		if !ok {
			return tt.TypError()
		}

		if x.Low != nil {
			tLow := p.synthesizeTypeFromExpr(pc, x.Low)
			if tLow != tt.PrimInt() {
				pc.Diag.Report(diag.ErrTypeMismatch, x.Low.Span(), "int", typeName(pc, tLow))
				return tt.TypError()
			}
		}

		if x.High != nil {
			tHigh := p.synthesizeTypeFromExpr(pc, x.High)
			if tHigh != tt.PrimInt() {
				pc.Diag.Report(diag.ErrTypeMismatch, x.High.Span(), "int", typeName(pc, tHigh))
				return tt.TypError()
			}
		}

		switch baseTy.Kind {
		case ir.TK_Slice:
			x.SetType(tBase)
			return tBase
		case ir.TK_Array:
			sliceTyp := tt.SliceOf(baseTy.ElemType)
			x.SetType(sliceTyp)
			return sliceTyp
		case ir.TK_PrimitiveString:
			x.SetType(tt.PrimString())
			return tt.PrimString()
		default:
			pc.Diag.Report(diag.ErrTypeNotIndexable, x.Expr.Span(), typeKeyDisplay(baseTy))
			return tt.TypError()
		}

	case *ir.FieldAccessExpr:
		return p.synthesizeFieldAccessExprType(pc, x)
	case *ir.VarRef:
		symPkg := pc.Pkg
		if x.Ref.Qualifier != "" {
			scope, _ := pc.Pkg.Scopes.EnclosingScope(x.ID())
			if qSym, ok := pc.Pkg.Scopes.Lookup(scope, x.Ref.Qualifier); ok {
				if pkgSym, found := pc.Pkg.Syms.GetByID(qSym); found && pkgSym.Kind == ir.SK_Package {
					if target := findPackageByPath(pc, pkgSym.PackagePath); target != nil {
						symPkg = target
					}
				}
			}
		}

		s, ok := symPkg.Syms.GetByID(x.Ref.Sym)
		if !ok {
			pc.Diag.Report(diag.ErrUndeclaredSymbol, x.Span(), x.Ref.Name)
			return tt.TypError()
		}

		x.SetType(s.Typ)
		return s.Typ
	case *ir.RangeExpr:
		tStart := p.synthesizeTypeFromExpr(pc, x.Start)
		tEnd := p.synthesizeTypeFromExpr(pc, x.End)
		if tStart != tt.PrimInt() && tStart != tt.PrimFloat() {
			pc.Diag.Report(diag.ErrTypeMismatch, x.Start.Span(), "int or float", typeName(pc, tStart))
			x.SetType(tt.TypError())
			return tt.TypError()
		}

		if tEnd != tt.PrimInt() && tEnd != tt.PrimFloat() {
			pc.Diag.Report(diag.ErrTypeMismatch, x.End.Span(), "int or float", typeName(pc, tEnd))
			x.SetType(tt.TypError())
			return tt.TypError()
		}

		if ok, _ := isTypeAssignable(tt, tEnd, tStart); !ok {
			pc.Diag.Report(diag.ErrTypeMismatch, x.End.Span(), typeName(pc, tStart), typeName(pc, tEnd))
			x.SetType(tt.TypError())
			return tt.TypError()
		}

		if x.Inc != nil {
			tInc := p.synthesizeTypeFromExpr(pc, x.Inc)
			if ok, _ := isTypeAssignable(tt, tStart, tInc); !ok {
				pc.Diag.Report(diag.ErrTypeMismatch, x.Inc.Span(), typeName(pc, tStart), typeName(pc, tInc))
				x.SetType(tt.TypError())
				return tt.TypError()
			}
		}

		retType := tt.SliceOf(tStart)
		x.SetType(retType)
		return retType
	case *ir.FuncCallExpr:
		return p.synthesizeFuncCallExprType(pc, x)
	case *ir.FuncLitExpr:
		if x.GetType() != 0 {
			return x.GetType()
		}

		for _, param := range x.Params {
			if param.Type != nil && param.Type.Typ != 0 {
				pc.Pkg.Syms.SetType(param.Name.Sym, param.Type.Typ)
			} else if param.Default != nil {
				ti := p.synthesizeTypeFromExpr(pc, param.Default)
				pc.Pkg.Syms.SetType(param.Name.Sym, ti)
			} else {
				pc.Pkg.Syms.SetType(param.Name.Sym, pc.Types.TypError())
				pc.Diag.Report(diag.ErrTypeInferenceFailed, param.Name.Span, fmt.Sprintf("parameter '%s'", param.Name.Name))
			}
		}

		retTyp := pc.Types.TypNone()
		if x.ReturnType != nil {
			retTyp = x.ReturnType.Typ
		}

		funcTyp := pc.Types.FuncOf(x.Params, retTyp)
		x.SetType(funcTyp)

		p.resolveStmts(pc, ir.BlockStmts(x.Body))

		return funcTyp
	case *ir.LitInt:
		t := tt.PrimInt()
		x.SetType(t)
		return t
	case *ir.LitFloat:
		t := tt.PrimFloat()
		x.SetType(t)
		return t
	case *ir.LitBool:
		t := tt.PrimBool()
		x.SetType(t)
		return t
	case *ir.LitString:
		t := tt.PrimString()
		x.SetType(t)
		return t
	case *ir.LitChar:
		t := tt.PrimChar()
		x.SetType(t)
		return t
	case *ir.LitNone:
		t := tt.TypNone()
		x.SetType(t)
		return t
	case *ir.ArrayLiteral:
		if existing := x.GetType(); existing != 0 {
			for _, el := range x.Elems {
				_ = p.synthesizeTypeFromExpr(pc, el)
			}

			return existing
		}

		if len(x.Elems) == 0 {
			t := tt.SliceOf(tt.PrimAny())
			x.SetType(t)
			return t
		}

		et := p.synthesizeTypeFromExpr(pc, x.Elems[0])
		for i := 1; i < len(x.Elems); i++ {
			et2 := p.synthesizeTypeFromExpr(pc, x.Elems[i])
			if ok, _ := isTypeAssignable(tt, et, et2); !ok {
				et = tt.PrimAny()
				break
			} else {
				et = et2
			}
		}

		t := tt.SliceOf(et)
		x.SetType(t)
		return t
	case *ir.MapLiteral:
		if existing := x.GetType(); existing != 0 {
			for _, entry := range x.Entries {
				_ = p.synthesizeTypeFromExpr(pc, entry.Key)
				_ = p.synthesizeTypeFromExpr(pc, entry.Value)
			}

			return existing
		}

		if len(x.Entries) == 0 {

			t := tt.MapOf(tt.PrimAny(), tt.PrimAny())
			x.SetType(t)
			return t
		}

		kt := p.synthesizeTypeFromExpr(pc, x.Entries[0].Key)
		vt := p.synthesizeTypeFromExpr(pc, x.Entries[0].Value)
		for _, entry := range x.Entries[1:] {
			kt2 := p.synthesizeTypeFromExpr(pc, entry.Key)
			vt2 := p.synthesizeTypeFromExpr(pc, entry.Value)
			if ok, _ := isTypeAssignable(tt, kt, kt2); !ok {
				kt = tt.PrimAny()
			}

			if ok, _ := isTypeAssignable(tt, vt, vt2); !ok {
				vt = tt.PrimAny()
			}
		}

		t := tt.MapOf(kt, vt)
		x.SetType(t)
		return t
	case *ir.TupleLiteral:
		if len(x.Elems) == 0 {
			return tt.TypError()
		}

		fields := make([]ir.TupleField, len(x.Elems))
		for i, el := range x.Elems {
			et := p.synthesizeTypeFromExpr(pc, el)
			fields[i] = ir.TupleField{Type: et}
		}

		t := tt.TupleOf(fields...)
		x.SetType(t)
		return t
	case *ir.StringTemplateExpr:
		for _, part := range x.Parts {
			if part.Expr != nil {
				p.synthesizeTypeFromExpr(pc, part.Expr)
			}
		}

		t := tt.PrimString()
		x.SetType(t)
		return t
	case *ir.SessionExpr:
		if cached, ok := pc.Cache[WireStateCacheKey]; ok && cached != nil {
			if sessionTyp, ok2 := pc.Cache[SessionTypeCacheKey].(ir.TypID); ok2 {
				x.SetType(sessionTyp)
				return sessionTyp
			}
		}

		pc.Diag.Report(diag.ErrSessionOutsideWired, x.Span())
		x.SetType(tt.TypError())
		return tt.TypError()
	case *ir.ChanInitExpr:
		if x.ElemType == nil || x.ElemType.Typ == 0 {
			x.SetType(tt.TypError())
			return tt.TypError()
		}

		if x.Capacity != nil {
			capTy := p.synthesizeTypeFromExpr(pc, x.Capacity)
			if capTy != tt.PrimInt() && capTy != tt.PrimAny() {
				pc.Diag.Report(diag.ErrTypeMismatch, x.Capacity.Span(), "int", typeName(pc, capTy))
			}
		}

		chTyp := tt.ChanOf(x.ElemType.Typ)
		x.SetType(chTyp)
		return chTyp
	case *ir.NewExpr:
		return p.synthesizeNewExprType(pc, x)
	case *ir.ComposableCallExpr:
		return p.synthesizeComposableCallType(pc, x)
	default:
		return tt.TypError()
	}
}

func (p *PassInferTypes) synthesizeComposableCallType(pc *PassContext, x *ir.ComposableCallExpr) ir.TypID {
	tt := pc.Types
	calleeSym := composableCalleeSym(x.Callee)
	if calleeSym == 0 {
		pc.Diag.Report(diag.ErrTypeMismatch, x.Span(), "composable type", "expression")
		x.SetType(tt.TypError())
		return tt.TypError()
	}

	symPkg := composableCalleePkg(pc, x.Callee)
	sym, ok := symPkg.Syms.GetByID(calleeSym)
	if !ok || sym.Typ == 0 {
		pc.Diag.Report(diag.ErrTypeMismatch, x.Span(), "composable type", "unresolved")
		x.SetType(tt.TypError())
		return tt.TypError()
	}

	ty, ok := tt.GetByID(sym.Typ)
	if !ok || ty.Kind != ir.TK_Struct || !ty.IsComposable {
		pc.Diag.Report(diag.ErrTypeMismatch, x.Span(), "composable type", sym.Name)
		x.SetType(tt.TypError())
		return tt.TypError()
	}

	if x.CtorSym != 0 {
		x.SetType(sym.Typ)
		return sym.Typ
	}

	argTypes := make([]ir.TypID, len(x.Args))
	for i, arg := range x.Args {
		argTypes[i] = p.synthesizeTypeFromExpr(pc, arg.Expr)
	}

	if len(x.Args) > 0 || len(ty.StructCtors) > 0 {
		matchedSym := ir.SymID(0)
		var matchedArgs []ir.FuncCallArg
		for _, ci := range ty.StructCtors {
			ftDef, ok := tt.GetByID(ci.FuncTyp)
			if !ok {
				continue
			}

			resolved, ok := resolveCallArgs(tt, ftDef.ParamTypes, x.Args, argTypes)
			if !ok {
				continue
			}

			matchedSym = ci.Sym
			matchedArgs = resolved
			break
		}

		if matchedSym == 0 && len(x.Args) > 0 {
			pc.Diag.Report(diag.ErrFuncParamMismatch, x.Span(), "constructor", sym.Name)
		}

		if matchedArgs != nil {
			x.Args = matchedArgs
		}

		x.CtorSym = matchedSym
	}

	for i := range x.Children {
		child := &x.Children[i]
		if child.Expr != nil {
			childTyp := p.synthesizeTypeFromExpr(pc, child.Expr)
			if wrapped := p.maybeInsertComposableCast(pc, child.Expr, childTyp); wrapped != nil {
				child.Expr = wrapped
			}
		}

		if child.Stmt != nil {
			p.resolveStmts(pc, []ir.Stmt{child.Stmt})
		}
	}

	x.TargetTyp = sym.Typ
	x.SetType(sym.Typ)
	return sym.Typ
}

func composableCalleeSym(callee ir.Expr) ir.SymID {
	switch c := callee.(type) {
	case *ir.VarRef:
		return c.Ref.Sym
	case *ir.FieldAccessExpr:
		if c.ResolvedSym != 0 {
			return c.ResolvedSym
		}
	}

	return 0
}

func composableCalleePkg(pc *PassContext, callee ir.Expr) *ir.PackageContext {
	switch c := callee.(type) {
	case *ir.VarRef:
		if c.Ref.Qualifier == "" {
			return pc.Pkg
		}

		scope, _ := pc.Pkg.Scopes.EnclosingScope(c.ID())
		if qSym, ok := pc.Pkg.Scopes.Lookup(scope, c.Ref.Qualifier); ok {
			if pkgSym, found := pc.Pkg.Syms.GetByID(qSym); found && pkgSym.Kind == ir.SK_Package {
				if target := findPackageByPath(pc, pkgSym.PackagePath); target != nil {
					return target
				}
			}
		}

	case *ir.FieldAccessExpr:

	}

	return pc.Pkg
}

func (p *PassInferTypes) maybeInsertComposableCast(pc *PassContext, child ir.Expr, childTyp ir.TypID) ir.Expr {
	if childTyp == 0 {
		return nil
	}

	if ty, ok := pc.Types.GetByID(childTyp); ok && ty.Kind == ir.TK_Struct && ty.IsComposable {
		return nil
	}

	type match struct {
		Sym     ir.SymID
		FuncTyp ir.TypID
		Target  ir.TypID
	}

	var matches []match
	for _, pkg := range pc.Pkgs {
		for _, f := range pkg.Files {
			for _, st := range f.Hir.Statements {
				td, ok := st.(*ir.TypeDeclStmt)
				if !ok {
					continue
				}

				if td.Name.Sym == 0 {
					continue
				}

				sym, ok := pkg.Syms.GetByID(td.Name.Sym)
				if !ok || sym.Typ == 0 {
					continue
				}

				ty, ok := pc.Types.GetByID(sym.Typ)
				if !ok || ty.Kind != ir.TK_Struct || !ty.IsComposable {
					continue
				}

				for _, ci := range ty.StructCasts {
					if ci.SourceTyp == childTyp {
						matches = append(matches, match{Sym: ci.Sym, FuncTyp: ci.FuncTyp, Target: sym.Typ})
					}
				}
			}
		}
	}

	if len(matches) != 1 {
		return nil
	}

	m := matches[0]
	if name, ok := pc.Names.GetOriginalName(m.Sym); ok {
		_ = name
	}

	callee := &ir.VarRef{
		Ref: ir.NameRef{Sym: m.Sym},
	}

	callee.SetType(m.FuncTyp)
	call := &ir.FuncCallExpr{
		Callee: callee,
		Args:   []ir.FuncCallArg{{Expr: child}},
	}

	call.SetType(m.Target)
	return call
}

func hasAnnotation(annos []ir.Annotation, name string) bool {
	for _, a := range annos {
		if a.Name.Name == name {
			return true
		}
	}

	return false
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}

	r := []rune(s)
	if r[0] >= 'a' && r[0] <= 'z' {
		r[0] = r[0] - 'a' + 'A'
	}

	return string(r)
}

func (p *PassInferTypes) rewriteComposableCalleeToCtor(pc *PassContext, x *ir.FuncCallExpr, argTypes []ir.TypID) {
	varRef, ok := x.Callee.(*ir.VarRef)
	if !ok || varRef.Ref.Sym == 0 {
		return
	}

	symPkg := composableCalleePkg(pc, varRef)
	sym, ok := symPkg.Syms.GetByID(varRef.Ref.Sym)
	if !ok || sym.Typ == 0 {
		return
	}

	ty, ok := pc.Types.GetByID(sym.Typ)
	if !ok || ty.Kind != ir.TK_Struct || !ty.IsComposable {
		return
	}

	for _, ci := range ty.StructCtors {
		ftDef, ok := pc.Types.GetByID(ci.FuncTyp)
		if !ok {
			continue
		}

		resolved, ok := resolveCallArgs(pc.Types, ftDef.ParamTypes, x.Args, argTypes)
		if !ok {
			continue
		}

		x.Args = resolved
		varRef.Ref.Sym = ci.Sym
		return
	}
}

func isTypeAssignable(tt *ir.TypeTable, dst, src ir.TypID) (bool, string) {
	if dst == src {
		return true, "same type"
	}

	if dst == 0 || src == 0 {
		return false, "unknown type"
	}

	if dst == tt.PrimAny() {
		return true, "T is assignable to any (widening)"
	}

	if dstTy, _ := tt.GetByID(dst); dstTy != nil && dstTy.Kind == ir.TK_TypeParam {
		return true, "generic type parameter is assignable to/from any type"
	}

	if srcTy, _ := tt.GetByID(src); srcTy != nil && srcTy.Kind == ir.TK_TypeParam {
		return true, "generic type parameter is assignable to/from any type"
	}

	if (dst == tt.PrimFloat() && src == tt.PrimInt()) || (dst == tt.PrimInt() && src == tt.PrimFloat()) {
		return true, "implicit conversion between int and float"
	}

	if (dst == tt.PrimByte() && src == tt.PrimInt()) || (dst == tt.PrimInt() && src == tt.PrimByte()) {
		return true, "implicit conversion between byte and int"
	}

	if dstTy, _ := tt.GetByID(dst); dstTy != nil && dstTy.Kind == ir.TK_Option {
		if src == tt.TypNone() {
			return true, "none to option"
		}

		if srcTy, _ := tt.GetByID(src); srcTy != nil && srcTy.Kind == ir.TK_Option {
			if dstTy.ElemType == srcTy.ElemType {
				return true, "option to option with same element type"
			}

			return false, "option element type mismatch"
		}

		if src == dstTy.ElemType {
			return true, "implicit lifting T to option<T>"
		}

		if ok, _ := isTypeAssignable(tt, dstTy.ElemType, src); ok {
			return true, "implicit lifting with conversion"
		}

		return false, "incompatible type for option"
	}

	if tsStructEqual(tt, dst, src) {
		return true, ""
	}

	if dstTy, _ := tt.GetByID(dst); dstTy != nil && dstTy.Kind == ir.TK_Map {
		if srcTy, _ := tt.GetByID(src); srcTy != nil && srcTy.Kind == ir.TK_Map {
			keyOK, _ := isTypeAssignable(tt, dstTy.KeyType, srcTy.KeyType)
			valOK, _ := isTypeAssignable(tt, dstTy.ValueType, srcTy.ValueType)
			if keyOK && valOK {
				return true, "map element-wise widen"
			}
		}
	}

	if dstTy, _ := tt.GetByID(dst); dstTy != nil && dstTy.Kind == ir.TK_Slice {
		if srcTy, _ := tt.GetByID(src); srcTy != nil && srcTy.Kind == ir.TK_Slice {
			if ok, _ := isTypeAssignable(tt, dstTy.ElemType, srcTy.ElemType); ok {
				return true, "slice element widen"
			}
		}
	}

	if dstTy, _ := tt.GetByID(dst); dstTy != nil && dstTy.Kind == ir.TK_Interface {
		if srcTy, _ := tt.GetByID(src); srcTy != nil && srcTy.Kind == ir.TK_Struct {
			for _, impl := range srcTy.StructImplements {
				if impl == dst {
					return true, "struct implements interface"
				}
			}
		}
	}

	if dstTy, _ := tt.GetByID(dst); dstTy != nil && dstTy.Kind == ir.TK_Function {
		if srcTy, _ := tt.GetByID(src); srcTy != nil && srcTy.Kind == ir.TK_Function {
			if len(dstTy.ParamTypes) == len(srcTy.ParamTypes) {
				allOK := true
				for i := range dstTy.ParamTypes {
					dp := dstTy.ParamTypes[i]
					sp := srcTy.ParamTypes[i]
					if dp == nil || sp == nil || dp.Type == nil || sp.Type == nil {
						allOK = false
						break
					}

					if ok, _ := isTypeAssignable(tt, sp.Type.Typ, dp.Type.Typ); !ok {
						allOK = false
						break
					}
				}

				if allOK {
					if ok, _ := isTypeAssignable(tt, dstTy.ReturnType, srcTy.ReturnType); ok {
						return true, "function type compatible (param contravariant, return covariant under type-param erasure)"
					}
				}
			}
		}
	}

	if dstTy, _ := tt.GetByID(dst); dstTy != nil && dstTy.Kind == ir.TK_Tuple {
		if srcTy, _ := tt.GetByID(src); srcTy != nil && srcTy.Kind == ir.TK_Tuple && len(dstTy.Fields) == len(srcTy.Fields) {
			allOK := true
			for i := range dstTy.Fields {
				if ok, _ := isTypeAssignable(tt, dstTy.Fields[i].Type, srcTy.Fields[i].Type); !ok {
					allOK = false
					break
				}
			}

			if allOK {
				return true, "tuple element-wise widen"
			}
		}
	}

	return false, "types are not assignable"
}

func assertFunctionParameterCompatibility(pc *PassContext, tt *ir.TypeTable, funcType *ir.Type, argTypes []ir.TypID, args []ir.FuncCallArg) (bool, string) {
	paramCount := len(funcType.ParamTypes)
	argCount := len(argTypes)

	if paramCount == 0 && argCount == 0 {
		return true, "no parameters and no arguments"
	}

	requiredParamCount := 0
	for _, param := range funcType.ParamTypes {
		if param.Default == nil && !param.IsVariadic {
			requiredParamCount++
		}
	}

	isVariadic := false
	if paramCount > 0 {
		lastParam := funcType.ParamTypes[paramCount-1]
		isVariadic = lastParam.IsVariadic
	}

	if !isVariadic {
		if argCount < requiredParamCount {
			return false, fmt.Sprintf("not enough arguments: expected at least %d, got %d", requiredParamCount, argCount)
		}

		if argCount > paramCount {
			return false, fmt.Sprintf("too many arguments: expected at most %d, got %d", paramCount, argCount)
		}
	} else {
		nonVariadicRequired := requiredParamCount
		if funcType.ParamTypes[paramCount-1].IsVariadic {
			nonVariadicRequired = paramCount - 1
			for i := 0; i < paramCount-1; i++ {
				if funcType.ParamTypes[i].Default != nil {
					nonVariadicRequired--
				}
			}
		}

		if argCount < nonVariadicRequired {
			return false, fmt.Sprintf("not enough arguments for variadic function: expected at least %d, got %d", nonVariadicRequired, argCount)
		}
	}

	for i, param := range funcType.ParamTypes {
		if isVariadic && i == paramCount-1 {
			variadicType := param.Type
			for j := i; j < argCount; j++ {
				argType := argTypes[j]
				if ok, _ := isTypeAssignable(tt, variadicType.Typ, argType); !ok {
					return false, "variadic argument type mismatch"
				}
			}

			break
		} else {
			if i >= argCount {
				if param.Default == nil {
					return false, fmt.Sprintf("missing required argument at position %d", i)
				}

				continue
			}

			argType := argTypes[i]
			if argType != 0 {
				if ok, _ := isTypeAssignable(tt, param.Type.Typ, argType); !ok {
					if i < len(args) && args[i].Expr != nil {
						if wrapped, castOK := tryInsertCast(tt, param.Type.Typ, argType, args[i].Expr); castOK {
							args[i].Expr = wrapped
							argTypes[i] = param.Type.Typ
							continue
						}
					}

					return false, fmt.Sprintf("argument type mismatch wanted %s, got %s", typeName(pc, param.Type.Typ), typeName(pc, argType))
				}
			} else {
				if param.Default == nil {
					return false, fmt.Sprintf("missing required argument at position %d", i)
				}
			}
		}
	}

	return true, "all parameters and arguments are compatible"
}

func tsStructEqual(tt *ir.TypeTable, a, b ir.TypID) bool {
	if a == b {
		return true
	}

	ta, okA := tt.GetByID(a)
	tb, okB := tt.GetByID(b)
	if !okA || !okB {
		return false
	}

	if ta.Kind != tb.Kind {
		return false
	}

	switch ta.Kind {
	case ir.TK_Array:
		return ta.ElemType == tb.ElemType && ta.Dim == tb.Dim && ta.Key == "" && tb.Key == ""
	case ir.TK_Slice:
		return ta.ElemType == tb.ElemType && ta.Key == "" && tb.Key == ""
	case ir.TK_Map:
		return ta.KeyType == tb.KeyType && ta.ValueType == tb.ValueType
	case ir.TK_Tuple:
		if len(ta.Fields) != len(tb.Fields) {
			return false
		}

		for i := range ta.Fields {
			if ta.Fields[i].Type != tb.Fields[i].Type {
				return false
			}
		}

		return true
	case ir.TK_Function:
		if len(ta.ParamTypes) != len(tb.ParamTypes) {
			return false
		}

		for i := range ta.ParamTypes {
			pa := ta.ParamTypes[i]
			pb := tb.ParamTypes[i]
			if pa.Type != pb.Type || pa.IsVariadic != pb.IsVariadic {
				return false
			}
		}

		if ta.ReturnType != tb.ReturnType {
			return false
		}

		return true
	default:
		return false
	}
}

func typeName(pc *PassContext, t ir.TypID) string {
	return renderTypeForDiag(pc.Types, t, map[ir.TypID]bool{})
}

func renderTypeForDiag(tt *ir.TypeTable, id ir.TypID, seen map[ir.TypID]bool) string {
	if id == 0 || id == tt.TypError() {
		return "<unresolved>"
	}

	if seen[id] {
		return "<recursive>"
	}

	seen[id] = true
	defer delete(seen, id)
	ty, ok := tt.GetByID(id)
	if !ok {
		return "<unknown>"
	}

	switch ty.Kind {
	case ir.TK_PrimitiveInt:
		return "int"
	case ir.TK_PrimitiveFloat:
		return "float"
	case ir.TK_PrimitiveString:
		return "string"
	case ir.TK_PrimitiveBool:
		return "bool"
	case ir.TK_PrimitiveChar:
		return "char"
	case ir.TK_PrimitiveByte:
		return "byte"
	case ir.TK_PrimitiveAny:
		return "any"
	case ir.TK_PrimitiveNone:
		return "none"
	case ir.TK_Option:
		return "option<" + renderTypeForDiag(tt, ty.ElemType, seen) + ">"
	case ir.TK_Slice:
		return "[]" + renderTypeForDiag(tt, ty.ElemType, seen)
	case ir.TK_Array:
		return fmt.Sprintf("[%d]%s", ty.Dim, renderTypeForDiag(tt, ty.ElemType, seen))
	case ir.TK_Map:
		return "map<" + renderTypeForDiag(tt, ty.KeyType, seen) + ", " + renderTypeForDiag(tt, ty.ValueType, seen) + ">"
	case ir.TK_Tuple:
		parts := make([]string, 0, len(ty.Fields))
		for _, f := range ty.Fields {
			if f.Name != "" {
				parts = append(parts, f.Name+": "+renderTypeForDiag(tt, f.Type, seen))
			} else {
				parts = append(parts, renderTypeForDiag(tt, f.Type, seen))
			}
		}

		return "(" + strings.Join(parts, ", ") + ")"
	case ir.TK_Chan:
		return "chan<" + renderTypeForDiag(tt, ty.ElemType, seen) + ">"
	case ir.TK_Function:
		parts := make([]string, 0, len(ty.ParamTypes))
		for _, p := range ty.ParamTypes {
			label := ""
			if p.Name.Name != "" {
				label = p.Name.Name + ": "
			}

			if p.Type != nil {
				parts = append(parts, label+renderTypeForDiag(tt, p.Type.Typ, seen))
			} else {
				parts = append(parts, label+"<unresolved>")
			}
		}

		prefix := "func"
		if ty.IsAsync {
			prefix = "async func"
		}

		head := prefix + "(" + strings.Join(parts, ", ") + ")"
		if ty.ReturnType == 0 || ty.ReturnType == tt.TypNone() {
			return head
		}

		return head + ": " + renderTypeForDiag(tt, ty.ReturnType, seen)
	case ir.TK_Struct:
		return qualifyTypeName(ty.PackagePath, ty.StructName)
	case ir.TK_Enum:
		return qualifyTypeName(ty.PackagePath, ty.EnumName)
	case ir.TK_Interface:
		return qualifyTypeName(ty.PackagePath, ty.InterfaceName)
	case ir.TK_TypeParam:
		if ty.ParamName != "" {
			return ty.ParamName
		}
	}

	if string(ty.Key) != "" && !strings.HasPrefix(string(ty.Key), "!") {
		return string(ty.Key)
	}

	return "<unresolved>"
}

func typeKeyDisplay(ty *ir.Type) string {
	if ty == nil {
		return "<unresolved>"
	}

	switch ty.Kind {
	case ir.TK_PrimitiveInt:
		return "int"
	case ir.TK_PrimitiveFloat:
		return "float"
	case ir.TK_PrimitiveString:
		return "string"
	case ir.TK_PrimitiveBool:
		return "bool"
	case ir.TK_PrimitiveChar:
		return "char"
	case ir.TK_PrimitiveByte:
		return "byte"
	case ir.TK_PrimitiveAny:
		return "any"
	case ir.TK_PrimitiveNone:
		return "none"
	case ir.TK_Function:
		if ty.IsAsync {
			return "async func(...)"
		}

		return "func(...)"
	case ir.TK_Struct:
		return qualifyTypeName(ty.PackagePath, ty.StructName)
	case ir.TK_Enum:
		return qualifyTypeName(ty.PackagePath, ty.EnumName)
	case ir.TK_Interface:
		return qualifyTypeName(ty.PackagePath, ty.InterfaceName)
	case ir.TK_Tuple:
		return "tuple"
	case ir.TK_Slice, ir.TK_Array:
		return "array"
	case ir.TK_Map:
		return "map"
	case ir.TK_Chan:
		return "chan"
	case ir.TK_Option:
		return "option"
	case ir.TK_TypeParam:
		if ty.ParamName != "" {
			return ty.ParamName
		}
	}

	if key := string(ty.Key); key != "" && !strings.HasPrefix(key, "!") && !strings.HasPrefix(key, "func:") {
		return key
	}

	return "<unresolved>"
}

func qualifyTypeName(pkgPath, name string) string {
	if name == "" {
		return "<unknown>"
	}

	if pkgPath != "" {
		if idx := strings.LastIndex(pkgPath, "/"); idx >= 0 {
			pkgPath = pkgPath[idx+1:]
		}
	}

	if pkgPath == "" {
		return name
	}

	return pkgPath + "." + name
}

func (p *PassInferTypes) resolveOverload(pc *PassContext, candidates []ir.SymID, argTypes []ir.TypID) ir.SymID {
	tt := pc.Types
	sa := pc.Pkg.Syms

	var bestMatch ir.SymID
	var bestScore = -1

	for _, candSym := range candidates {
		symbol, ok := sa.GetByID(candSym)
		if !ok || symbol.Kind != ir.SK_Function {
			continue
		}

		funcType, ok := tt.GetByID(symbol.Typ)
		if !ok || funcType.Kind != ir.TK_Function {
			continue
		}

		paramCount := len(funcType.ParamTypes)
		argCount := len(argTypes)

		requiredParams := 0
		for _, param := range funcType.ParamTypes {
			if param.Default == nil && !param.IsVariadic {
				requiredParams++
			}
		}

		if argCount < requiredParams || argCount > paramCount {
			continue
		}

		score := 0
		compatible := true

		for i := 0; i < argCount && i < paramCount; i++ {
			paramType := funcType.ParamTypes[i].Type.Typ
			argType := argTypes[i]

			if paramType == argType {
				score += 10
			} else if ok, _ := isTypeAssignable(tt, paramType, argType); ok {
				score += 5
			} else {
				compatible = false
				break
			}
		}

		if compatible && score > bestScore {
			bestScore = score
			bestMatch = candSym
		}
	}

	return bestMatch
}

func (p *PassInferTypes) applyLiteralTypeHint(pc *PassContext, e ir.Expr, hint ir.TypID) {
	if e == nil || hint == 0 {
		return
	}

	ty, ok := pc.Types.GetByID(hint)
	if !ok {
		return
	}

	switch lit := e.(type) {
	case *ir.MapLiteral:
		if ty.Kind != ir.TK_Map {
			return
		}

		lit.SetType(hint)
		for i := range lit.Entries {
			p.applyLiteralTypeHint(pc, lit.Entries[i].Key, ty.KeyType)
			p.applyLiteralTypeHint(pc, lit.Entries[i].Value, ty.ValueType)
		}

	case *ir.ArrayLiteral:
		switch ty.Kind {
		case ir.TK_Slice, ir.TK_Array:
			lit.SetType(hint)
			for i := range lit.Elems {
				p.applyLiteralTypeHint(pc, lit.Elems[i], ty.ElemType)
			}

		case ir.TK_Option:
			p.applyLiteralTypeHint(pc, e, ty.ElemType)
		}
	}
}

type noneNarrowing struct {
	pkg     *ir.PackageContext
	sym     ir.SymID
	newTyp  ir.TypID
	prevTyp ir.TypID
}

func (p *PassInferTypes) detectNoneNarrowing(pc *PassContext, cond ir.Expr) (thenNarrow, elseNarrow []noneNarrowing) {
	bin, ok := cond.(*ir.BinaryExpr)
	if !ok {
		return nil, nil
	}

	if bin.Op != ir.OpNeq && bin.Op != ir.OpEq {
		return nil, nil
	}

	var varRef *ir.VarRef
	if _, isNone := bin.Right.(*ir.LitNone); isNone {
		if vr, ok := bin.Left.(*ir.VarRef); ok {
			varRef = vr
		}
	} else if _, isNone := bin.Left.(*ir.LitNone); isNone {
		if vr, ok := bin.Right.(*ir.VarRef); ok {
			varRef = vr
		}
	}

	if varRef == nil || varRef.Ref.Sym == 0 {
		return nil, nil
	}

	sym, ok := pc.Pkg.Syms.GetByID(varRef.Ref.Sym)
	if !ok {
		return nil, nil
	}

	ty, ok := pc.Types.GetByID(sym.Typ)
	if !ok || ty.Kind != ir.TK_Option {
		return nil, nil
	}

	narrow := noneNarrowing{pkg: pc.Pkg, sym: varRef.Ref.Sym, newTyp: ty.ElemType, prevTyp: sym.Typ}

	if bin.Op == ir.OpNeq {
		return []noneNarrowing{narrow}, nil
	}

	return nil, []noneNarrowing{narrow}
}

func (p *PassInferTypes) withNarrowedTypes(pc *PassContext, narrowings []noneNarrowing, fn func()) {
	if len(narrowings) == 0 {
		fn()
		return
	}

	for _, n := range narrowings {
		n.pkg.Syms.SetType(n.sym, n.newTyp)
	}

	defer func() {
		for _, n := range narrowings {
			n.pkg.Syms.SetType(n.sym, n.prevTyp)
		}
	}()
	fn()
}

func (p *PassInferTypes) isTerminator(st ir.Stmt) bool {
	switch st.(type) {
	case *ir.ReturnStmt, *ir.BreakStmt, *ir.ContinueStmt:
		return true
	default:
		return false
	}
}

func (p *PassInferTypes) collectReturnTypes(pc *PassContext, stmts []ir.Stmt) []struct {
	Typ  ir.TypID
	Span diag.TextSpan
} {
	var types []struct {
		Typ  ir.TypID
		Span diag.TextSpan
	}

	for _, st := range stmts {
		switch s := st.(type) {
		case *ir.ReturnStmt:
			if len(s.Results) == 0 {
				types = append(types, struct {
					Typ  ir.TypID
					Span diag.TextSpan
				}{Typ: pc.Types.TypNone(), Span: s.Span()})
			} else if len(s.Results) == 1 {
				typ := p.synthesizeTypeFromExpr(pc, s.Results[0])
				types = append(types, struct {
					Typ  ir.TypID
					Span diag.TextSpan
				}{Typ: typ, Span: s.Span()})
			} else {
				var fields []ir.TupleField
				for _, result := range s.Results {
					typ := p.synthesizeTypeFromExpr(pc, result)
					fields = append(fields, ir.TupleField{Name: "", Type: typ})
				}

				tupleTyp := pc.Types.TupleOf(fields...)
				types = append(types, struct {
					Typ  ir.TypID
					Span diag.TextSpan
				}{Typ: tupleTyp, Span: s.Span()})
			}

		case *ir.BlockStmt:
			types = append(types, p.collectReturnTypes(pc, s.Stmts)...)
		case *ir.IfStmt:
			types = append(types, p.collectReturnTypes(pc, ir.BlockStmts(s.Then))...)
			for _, elif := range s.ElseIfs {
				types = append(types, p.collectReturnTypes(pc, ir.BlockStmts(elif.Then))...)
			}

			if s.Else != nil {
				types = append(types, p.collectReturnTypes(pc, ir.BlockStmts(s.Else))...)
			}

		case *ir.SwitchStmt:
			for _, c := range s.Cases {
				types = append(types, p.collectReturnTypes(pc, c.Stmts)...)
			}

			if s.Default != nil {
				types = append(types, p.collectReturnTypes(pc, s.Default)...)
			}

		case *ir.ForStmt:
			types = append(types, p.collectReturnTypes(pc, ir.BlockStmts(s.Body))...)
		case *ir.WhileStmt:
			types = append(types, p.collectReturnTypes(pc, ir.BlockStmts(s.Body))...)
		}
	}

	return types
}

func wireStateTypeFromCache(pc *PassContext) ir.TypID {
	if cached, ok := pc.Cache[WireStateCacheKey]; ok {
		if id, ok := cached.(ir.TypID); ok {
			return id
		}
	}

	return 0
}

func isHandleWrapperCast(tt *ir.TypeTable, srcTy, dstTy ir.TypID) bool {
	hasHandle := func(t ir.TypID) bool {
		info, ok := tt.GetByID(t)
		if !ok || info.Kind != ir.TK_Struct {
			return false
		}

		for _, sf := range info.StructFields {
			if sf.Name == "handle" && sf.Type == tt.PrimAny() {
				return true
			}
		}

		return false
	}

	return hasHandle(srcTy) && hasHandle(dstTy)
}

func isPrimitiveConversionAllowed(tt *ir.TypeTable, srcTy, dstTy ir.TypID) bool {
	str := tt.PrimString()
	in := tt.PrimInt()
	fl := tt.PrimFloat()
	bl := tt.PrimBool()
	ch := tt.PrimChar()
	bt := tt.PrimByte()
	isPrim := func(t ir.TypID) bool { return t == str || t == in || t == fl || t == bl || t == ch || t == bt }

	if !isPrim(srcTy) || !isPrim(dstTy) {
		return false
	}

	if dstTy == str {
		return true
	}

	if srcTy == str {
		return dstTy == in || dstTy == fl || dstTy == bl
	}

	if (srcTy == in && dstTy == fl) || (srcTy == fl && dstTy == in) {
		return true
	}

	if (srcTy == in && dstTy == ch) || (srcTy == ch && dstTy == in) {
		return true
	}

	if (srcTy == in && dstTy == bt) || (srcTy == bt && dstTy == in) {
		return true
	}

	return false
}

func findPackageByPath(pc *PassContext, path string) *ir.PackageContext {
	for _, pkg := range pc.Pkgs {
		if pkg.Path.String() == path {
			return pkg
		}
	}

	return nil
}

func operatorMethodName(op ir.Op) (string, bool) {
	switch op {
	case ir.OpAdd, ir.OpSub, ir.OpMul, ir.OpDiv, ir.OpMod, ir.OpEq:
		return "op" + string(op), true
	}

	return "", false
}

func fieldAccessIsThroughThis(pc *PassContext, recv ir.Expr) bool {
	for {
		switch n := recv.(type) {
		case *ir.GroupedExpr:
			recv = n.Expr
		case *ir.FieldAccessExpr:
			recv = n.Expr
		case *ir.VarRef:
			if n.Ref.Sym == 0 {
				return n.Ref.Name == "this"
			}

			sym, ok := pc.Pkg.Syms.GetByID(n.Ref.Sym)
			if !ok {
				return false
			}

			return sym.Name == "this"
		default:
			return false
		}
	}
}

func preTypeEmptyLiteralFromContext(tt *ir.TypeTable, expr ir.Expr, expectedTyp ir.TypID) {
	if expectedTyp == 0 || ir.IsNilExpr(expr) {
		return
	}

	expectedTy, ok := tt.GetByID(expectedTyp)
	if !ok {
		return
	}

	switch x := expr.(type) {
	case *ir.ArrayLiteral:
		if len(x.Elems) != 0 || x.GetType() != 0 {
			return
		}

		if expectedTy.Kind == ir.TK_Slice || expectedTy.Kind == ir.TK_Array {
			x.SetType(expectedTyp)
		}

	case *ir.MapLiteral:
		if len(x.Entries) != 0 || x.GetType() != 0 {
			return
		}

		if expectedTy.Kind == ir.TK_Map {
			x.SetType(expectedTyp)
		}
	}
}

func tryInsertCast(tt *ir.TypeTable, targetTyp, sourceTyp ir.TypID, arg ir.Expr) (ir.Expr, bool) {
	if targetTyp == 0 || sourceTyp == 0 || targetTyp == sourceTyp {
		return arg, false
	}

	ty, ok := tt.GetByID(targetTyp)
	if !ok {
		return arg, false
	}

	for _, ci := range ty.StructCasts {
		if ci.SourceTyp == sourceTyp {
			return wrapCastCall(tt, ci, arg, targetTyp), true
		}

		if len(ty.TypeParams) > 0 {
			if typePatternMatches(tt, ci.SourceTyp, sourceTyp, map[string]ir.TypID{}) {
				return wrapCastCall(tt, ci, arg, targetTyp), true
			}
		}
	}

	return arg, false
}

func wrapCastCall(tt *ir.TypeTable, ci ir.StructCastInfo, arg ir.Expr, targetTyp ir.TypID) ir.Expr {
	if erased := eraseTypeParams(tt, ci.SourceTyp); erased != 0 {
		applyCodegenLiteralHint(tt, arg, erased)
	}

	callee := &ir.VarRef{Ref: ir.NameRef{Sym: ci.Sym}}

	callee.SetType(ci.FuncTyp)
	call := &ir.FuncCallExpr{
		Callee: callee,
		Args:   []ir.FuncCallArg{{Expr: arg}},
	}

	call.SetType(targetTyp)
	return call
}

func eraseTypeParams(tt *ir.TypeTable, typID ir.TypID) ir.TypID {
	if typID == 0 {
		return 0
	}

	ty, ok := tt.GetByID(typID)
	if !ok {
		return typID
	}

	switch ty.Kind {
	case ir.TK_TypeParam:
		return tt.PrimAny()
	case ir.TK_Slice:
		return tt.SliceOf(eraseTypeParams(tt, ty.ElemType))
	case ir.TK_Array:
		return tt.DeclareType(ir.ArrayType(eraseTypeParams(tt, ty.ElemType), ty.Dim))
	case ir.TK_Option:
		return tt.OptionOf(eraseTypeParams(tt, ty.ElemType))
	case ir.TK_Map:
		return tt.MapOf(eraseTypeParams(tt, ty.KeyType), eraseTypeParams(tt, ty.ValueType))
	case ir.TK_Chan:
		return tt.ChanOf(eraseTypeParams(tt, ty.ElemType))
	case ir.TK_Tuple:
		erased := make([]ir.TupleField, len(ty.Fields))
		for i, f := range ty.Fields {
			erased[i] = ir.TupleField{Name: f.Name, Type: eraseTypeParams(tt, f.Type)}
		}

		return tt.TupleOf(erased...)
	}

	return typID
}

func buildTypeArgSubstitution(paramNames []string, typeArgs []*ir.TypeRef) map[string]ir.TypID {
	if len(paramNames) == 0 || len(typeArgs) == 0 || len(paramNames) != len(typeArgs) {
		return nil
	}

	sub := make(map[string]ir.TypID, len(paramNames))
	for i, name := range paramNames {
		if typeArgs[i] != nil && typeArgs[i].Typ != 0 {
			sub[name] = typeArgs[i].Typ
		}
	}

	if len(sub) == 0 {
		return nil
	}

	return sub
}

func substituteType(tt *ir.TypeTable, typID ir.TypID, sub map[string]ir.TypID) ir.TypID {
	if typID == 0 || len(sub) == 0 {
		return typID
	}

	ty, ok := tt.GetByID(typID)
	if !ok {
		return typID
	}

	switch ty.Kind {
	case ir.TK_TypeParam:
		if mapped, hit := sub[ty.ParamName]; hit {
			return mapped
		}

		return typID
	case ir.TK_Slice:
		return tt.SliceOf(substituteType(tt, ty.ElemType, sub))
	case ir.TK_Array:
		return tt.DeclareType(ir.ArrayType(substituteType(tt, ty.ElemType, sub), ty.Dim))
	case ir.TK_Option:
		return tt.OptionOf(substituteType(tt, ty.ElemType, sub))
	case ir.TK_Map:
		return tt.MapOf(substituteType(tt, ty.KeyType, sub), substituteType(tt, ty.ValueType, sub))
	case ir.TK_Chan:
		return tt.ChanOf(substituteType(tt, ty.ElemType, sub))
	case ir.TK_Tuple:
		fields := make([]ir.TupleField, len(ty.Fields))
		for i, f := range ty.Fields {
			fields[i] = ir.TupleField{Name: f.Name, Type: substituteType(tt, f.Type, sub)}
		}

		return tt.TupleOf(fields...)
	}

	return typID
}

func substituteFuncParams(tt *ir.TypeTable, params []*ir.FuncParam, sub map[string]ir.TypID) []*ir.FuncParam {
	if len(sub) == 0 {
		return params
	}

	out := make([]*ir.FuncParam, len(params))
	for i, p := range params {
		if p == nil {
			continue
		}

		clone := *p
		if p.Type != nil {
			clone.Type = &ir.TypeRef{Typ: substituteType(tt, p.Type.Typ, sub)}
		}

		out[i] = &clone
	}

	return out
}

func applyCodegenLiteralHint(tt *ir.TypeTable, e ir.Expr, hint ir.TypID) {
	if e == nil || hint == 0 {
		return
	}

	ty, ok := tt.GetByID(hint)
	if !ok {
		return
	}

	switch lit := e.(type) {
	case *ir.ArrayLiteral:
		if ty.Kind != ir.TK_Slice && ty.Kind != ir.TK_Array {
			return
		}

		lit.SetType(hint)
		for i := range lit.Elems {
			applyCodegenLiteralHint(tt, lit.Elems[i], ty.ElemType)
		}

	case *ir.MapLiteral:
		if ty.Kind != ir.TK_Map {
			return
		}

		lit.SetType(hint)
		for i := range lit.Entries {
			applyCodegenLiteralHint(tt, lit.Entries[i].Key, ty.KeyType)
			applyCodegenLiteralHint(tt, lit.Entries[i].Value, ty.ValueType)
		}
	}
}

func typePatternMatches(tt *ir.TypeTable, pattern, concrete ir.TypID, sub map[string]ir.TypID) bool {
	if pattern == 0 || concrete == 0 {
		return false
	}

	if pattern == concrete {
		return true
	}

	pTy, pok := tt.GetByID(pattern)
	cTy, cok := tt.GetByID(concrete)
	if !pok || !cok {
		return false
	}

	if pTy.Kind == ir.TK_TypeParam {
		if existing, bound := sub[pTy.ParamName]; bound {
			return existing == concrete
		}

		sub[pTy.ParamName] = concrete
		return true
	}

	if pTy.Kind != cTy.Kind {
		return false
	}

	switch pTy.Kind {
	case ir.TK_Slice, ir.TK_Array, ir.TK_Option, ir.TK_Chan:
		return typePatternMatches(tt, pTy.ElemType, cTy.ElemType, sub)
	case ir.TK_Map:
		return typePatternMatches(tt, pTy.KeyType, cTy.KeyType, sub) &&
			typePatternMatches(tt, pTy.ValueType, cTy.ValueType, sub)
	case ir.TK_Tuple:
		if len(pTy.Fields) != len(cTy.Fields) {
			return false
		}

		for i := range pTy.Fields {
			if !typePatternMatches(tt, pTy.Fields[i].Type, cTy.Fields[i].Type, sub) {
				return false
			}
		}

		return true
	case ir.TK_Function:
		if len(pTy.ParamTypes) != len(cTy.ParamTypes) {
			return false
		}

		for i := range pTy.ParamTypes {
			if pTy.ParamTypes[i].Type == nil || cTy.ParamTypes[i].Type == nil {
				continue
			}

			if !typePatternMatches(tt, pTy.ParamTypes[i].Type.Typ, cTy.ParamTypes[i].Type.Typ, sub) {
				return false
			}
		}

		return typePatternMatches(tt, pTy.ReturnType, cTy.ReturnType, sub)
	}

	return pattern == concrete
}

func resolveCallArgs(tt *ir.TypeTable, params []*ir.FuncParam, providedArgs []ir.FuncCallArg, argTypes []ir.TypID) ([]ir.FuncCallArg, bool) {
	result := make([]ir.FuncCallArg, len(params))
	filled := make([]bool, len(params))
	positional := 0
	sawNamed := false
	for ai, arg := range providedArgs {
		if arg.Name != "" {
			sawNamed = true
			idx := -1
			for i, p := range params {
				if p.Name.Name == arg.Name {
					idx = i
					break
				}
			}

			if idx < 0 || filled[idx] {
				return nil, false
			}

			if params[idx].Type != nil && params[idx].Type.Typ != 0 {
				if assignable, _ := isTypeAssignable(tt, params[idx].Type.Typ, argTypes[ai]); !assignable {
					if wrapped, ok := tryInsertCast(tt, params[idx].Type.Typ, argTypes[ai], arg.Expr); ok {
						arg.Expr = wrapped
					} else {
						return nil, false
					}
				}
			}

			result[idx] = arg
			filled[idx] = true
			continue
		}

		if sawNamed {
			return nil, false
		}

		if positional >= len(params) || filled[positional] {
			return nil, false
		}

		if params[positional].Type != nil && params[positional].Type.Typ != 0 {
			if assignable, _ := isTypeAssignable(tt, params[positional].Type.Typ, argTypes[ai]); !assignable {
				if wrapped, ok := tryInsertCast(tt, params[positional].Type.Typ, argTypes[ai], arg.Expr); ok {
					arg.Expr = wrapped
				} else {
					return nil, false
				}
			}
		}

		result[positional] = arg
		filled[positional] = true
		positional++
	}

	for i, ok := range filled {
		if ok {
			continue
		}

		if params[i].Default == nil {
			return nil, false
		}

		result[i] = ir.FuncCallArg{Name: params[i].Name.Name, Expr: params[i].Default}
	}

	return result, true
}

func findIterableNext(pc *PassContext, ty *ir.Type) (ir.SymID, ir.TypID) {
	if ty == nil || ty.Kind != ir.TK_Struct {
		return 0, 0
	}

	for _, m := range ty.StructMethods {
		if m.Name != "next" || m.Sym == 0 {
			continue
		}

		fnTy, ok := pc.Types.GetByID(m.FuncTyp)
		if !ok || fnTy.Kind != ir.TK_Function {
			continue
		}

		if len(fnTy.ParamTypes) != 0 {
			continue
		}

		retTy, ok := pc.Types.GetByID(fnTy.ReturnType)
		if !ok || retTy.Kind != ir.TK_Option {
			continue
		}

		return m.Sym, retTy.ElemType
	}

	return 0, 0
}

func (p *PassInferTypes) synthesizeBinaryExprType(pc *PassContext, x *ir.BinaryExpr) ir.TypID {
	tt := pc.Types
	l := p.synthesizeTypeFromExpr(pc, x.Left)
	r := p.synthesizeTypeFromExpr(pc, x.Right)
	_ = r

	if leftTy, ok := tt.GetByID(l); ok && leftTy.Kind == ir.TK_Struct {
		if opName, isOp := operatorMethodName(x.Op); isOp {
			for _, m := range leftTy.StructMethods {
				if m.Name == opName {
					if fnTy, ok := tt.GetByID(m.FuncTyp); ok && fnTy.Kind == ir.TK_Function {
						x.SetType(fnTy.ReturnType)
						return fnTy.ReturnType
					}
				}
			}
		}
	}

	isNum := func(t ir.TypID) bool { return t == tt.PrimInt() || t == tt.PrimFloat() || t == tt.PrimByte() }

	commonNum := func(a, b ir.TypID) (ir.TypID, bool) {
		if a == b && isNum(a) {
			return a, true
		}

		if isNum(a) && isNum(b) {
			if a == tt.PrimFloat() || b == tt.PrimFloat() {
				return tt.PrimFloat(), true
			}

			return tt.PrimInt(), true
		}

		return 0, false
	}

	switch x.Op {
	case ir.OpAdd, ir.OpSub, ir.OpMul, ir.OpDiv:
		if x.Op == ir.OpAdd && l == tt.PrimString() || r == tt.PrimString() {
			x.SetType(tt.PrimString())
			return tt.PrimString()
		}

		if x.Op == ir.OpAdd {
			if lTy, lok := tt.GetByID(l); lok && lTy.Kind == ir.TK_Slice {
				if rTy, rok := tt.GetByID(r); rok && rTy.Kind == ir.TK_Slice {
					if ok, _ := isTypeAssignable(tt, lTy.ElemType, rTy.ElemType); ok {
						x.SetType(l)
						return l
					}

					if ok, _ := isTypeAssignable(tt, rTy.ElemType, lTy.ElemType); ok {
						x.SetType(r)
						return r
					}
				}
			}
		}

		if t, ok := commonNum(l, r); ok {
			x.SetType(t)
			return t
		}

		pc.Diag.Report(diag.ErrTypeMismatch, x.Span(), "numeric (int or float)", typeName(pc, l)+", "+typeName(pc, r))
		x.SetType(tt.TypError())
		return tt.TypError()

	case ir.OpMod:
		if l == tt.PrimInt() && r == tt.PrimInt() {
			x.SetType(tt.PrimInt())
			return tt.PrimInt()
		}

		pc.Diag.Report(diag.ErrTypeMismatch, x.Span(), "int % int", typeName(pc, l)+", "+typeName(pc, r))
		x.SetType(tt.TypError())
		return tt.TypError()

	case ir.OpAnd, ir.OpOr, ir.OpXor:
		if l == tt.PrimInt() && r == tt.PrimInt() {
			x.SetType(tt.PrimInt())
			return tt.PrimInt()
		}

		pc.Diag.Report(diag.ErrTypeMismatch, x.Span(), "int (bitwise)", typeName(pc, l)+", "+typeName(pc, r))
		x.SetType(tt.TypError())
		return tt.TypError()

	case ir.OpShl, ir.OpShr:
		if l == tt.PrimInt() && r == tt.PrimInt() {
			x.SetType(tt.PrimInt())
			return tt.PrimInt()
		}

		pc.Diag.Report(diag.ErrTypeMismatch, x.Span(), "int << int / int >> int", typeName(pc, l)+", "+typeName(pc, r))
		x.SetType(tt.TypError())
		return tt.TypError()

	case ir.OpLAnd, ir.OpLOr:
		if l == tt.PrimBool() && r == tt.PrimBool() {
			x.SetType(tt.PrimBool())
			return tt.PrimBool()
		}

		pc.Diag.Report(diag.ErrTypeMismatch, x.Span(), "bool && bool / bool || bool", typeName(pc, l)+", "+typeName(pc, r))
		x.SetType(tt.TypError())
		return tt.TypError()

	case ir.OpEq, ir.OpNeq:
		if _, ok := commonNum(l, r); ok {
			x.SetType(tt.PrimBool())
			return tt.PrimBool()
		}

		if okAB, _ := isTypeAssignable(tt, l, r); okAB {
			x.SetType(tt.PrimBool())
			return tt.PrimBool()
		}

		if okBA, _ := isTypeAssignable(tt, r, l); okBA {
			x.SetType(tt.PrimBool())
			return tt.PrimBool()
		}

		pc.Diag.Report(diag.ErrTypeMismatch, x.Span(), "comparable types for ==", typeName(pc, l)+", "+typeName(pc, r))
		x.SetType(tt.TypError())
		return tt.TypError()

	case ir.OpLt, ir.OpLte, ir.OpGt, ir.OpGte:
		if _, ok := commonNum(l, r); ok {
			x.SetType(tt.PrimBool())
			return tt.PrimBool()
		}

		pc.Diag.Report(diag.ErrTypeMismatch, x.Span(), "numeric comparison", typeName(pc, l)+", "+typeName(pc, r))
		x.SetType(tt.TypError())
		return tt.TypError()
	}

	pc.Diag.Report(diag.ErrInvalidOperator, x.Span(), "unknown binary op: "+string(x.Op))
	x.SetType(tt.TypError())
	return tt.TypError()
}

func (p *PassInferTypes) synthesizeFieldAccessExprType(pc *PassContext, x *ir.FieldAccessExpr) ir.TypID {
	tt := pc.Types

	var pkgQualifiedStartField int
	var pkgQualifiedCur ir.TypID
	if vr, ok := x.Expr.(*ir.VarRef); ok && vr.Ref.Sym != 0 {
		if recvSym, found := pc.Pkg.Syms.GetByID(vr.Ref.Sym); found && recvSym.Kind == ir.SK_Package && len(x.Fields) >= 1 {
			targetPkg := findPackageByPath(pc, recvSym.PackagePath)
			if targetPkg == nil {
				pc.Diag.Report(diag.ErrUndeclaredSymbol, x.Fields[0].Span, recvSym.PackagePath)
				x.SetType(tt.TypError())
				return tt.TypError()
			}

			memberSym, found := targetPkg.Scopes.LookupOnlyCurrent(targetPkg.Root, x.Fields[0].Name)
			if !found {
				pc.Diag.Report(diag.ErrUndeclaredSymbol, x.Fields[0].Span, recvSym.PackagePath+"."+x.Fields[0].Name)
				x.SetType(tt.TypError())
				return tt.TypError()
			}

			if isPackagePrivateName(x.Fields[0].Name) && targetPkg != pc.Pkg {
				pc.Diag.Report(diag.ErrPrivateSymbolAccess, x.Fields[0].Span, x.Fields[0].Name, recvSym.PackagePath)
				x.SetType(tt.TypError())
				return tt.TypError()
			}

			memberInfo, _ := targetPkg.Syms.GetByID(memberSym)
			if len(x.Fields) == 1 {
				x.ResolvedSym = memberSym
				x.SetType(memberInfo.Typ)
				return memberInfo.Typ
			}

			x.ResolvedSym = memberSym
			pkgQualifiedStartField = 1
			pkgQualifiedCur = memberInfo.Typ
		}
	}

	var cur ir.TypID
	if pkgQualifiedStartField > 0 {
		cur = pkgQualifiedCur
	} else {
		cur = p.synthesizeTypeFromExpr(pc, x.Expr)
	}

	fieldsToWalk := x.Fields[pkgQualifiedStartField:]
	for _, fld := range fieldsToWalk {
		ty, ok := tt.GetByID(cur)
		if !ok {
			pc.Diag.Report(diag.ErrUnknownType, fld.Span, "base")
			x.SetType(tt.TypError())
			return tt.TypError()
		}

		switch ty.Kind {
		case ir.TK_Chan:
			switch fld.Name {
			case "send":
				sendParam := &ir.FuncParam{Name: ir.NameRef{Name: "v"}, Type: &ir.TypeRef{Typ: ty.ElemType}}

				cur = tt.AsyncFuncOf([]*ir.FuncParam{sendParam}, tt.TypNone())
			case "recv":
				tupleTyp := tt.TupleOf(
					ir.TupleField{Name: "value", Type: ty.ElemType},
					ir.TupleField{Name: "ok", Type: tt.PrimBool()},
				)
				cur = tt.AsyncFuncOf(nil, tupleTyp)
			case "close":
				cur = tt.FuncOf(nil, tt.TypNone())
			default:
				pc.Diag.Report(diag.ErrUnknownType, fld.Span, "chan method '"+fld.Name+"' (expected send/recv/close)")
				x.SetType(tt.TypError())
				return tt.TypError()
			}

		case ir.TK_Map:
			if ty.KeyType != tt.PrimString() && ty.KeyType != tt.PrimAny() {
				ktName := typeName(pc, ty.KeyType)
				pc.Diag.Report(diag.ErrTypeMismatch, fld.Span, "map<string, _>", "map<"+ktName+", _>")
				x.SetType(tt.TypError())
				return tt.TypError()
			}

			cur = ty.ValueType

		case ir.TK_Struct:
			found := false
			for _, sf := range ty.StructFields {
				if sf.Name == fld.Name {
					found = true
					if sf.Private && !fieldAccessIsThroughThis(pc, x.Expr) {
						pc.Diag.Report(diag.ErrPrivateFieldAccess, fld.Span, fld.Name, ty.StructName, fld.Name)
					}

					cur = sf.Type
					break
				}
			}

			if !found {
				if x.MethodSym != 0 {
					for _, m := range ty.StructMethods {
						if m.Sym == x.MethodSym {
							found = true
							cur = m.FuncTyp
							break
						}
					}
				}

				if !found {
					for _, m := range ty.StructMethods {
						if m.Name == fld.Name {
							found = true
							cur = m.FuncTyp
							break
						}
					}
				}
			}

			if !found {
				pc.Diag.Report(diag.ErrTypeNotIndexable, fld.Span, fmt.Sprintf("type %s has no field or method '%s'", ty.StructName, fld.Name))
				x.SetType(tt.TypError())
				return tt.TypError()
			}

		case ir.TK_Interface:
			found := false
			for _, m := range ty.InterfaceMethods {
				if m.Name == fld.Name {
					found = true
					cur = m.FuncTyp
					break
				}
			}

			if !found {
				pc.Diag.Report(diag.ErrTypeNotIndexable, fld.Span, fmt.Sprintf("interface %s has no method '%s'", ty.InterfaceName, fld.Name))
				x.SetType(tt.TypError())
				return tt.TypError()
			}

		case ir.TK_Enum:
			foundCase := false
			for _, c := range ty.EnumCases {
				if c.Name == fld.Name {
					foundCase = true
					break
				}
			}

			if !foundCase {
				foundField := false
				for _, f := range ty.EnumFields {
					if f.Name == fld.Name {
						foundField = true
						cur = f.Type
						break
					}
				}

				if !foundField {
					foundMethod := false
					for _, m := range ty.EnumMethods {
						if m.Name == fld.Name {
							foundMethod = true
							cur = m.Type
							break
						}
					}

					if !foundMethod {
						pc.Diag.Report(diag.ErrTypeNotIndexable, fld.Span, fmt.Sprintf("enum %s has no case, field, or method named '%s'", ty.EnumName, fld.Name))
						x.SetType(tt.TypError())
						return tt.TypError()
					}
				}
			}

		default:
			baseName := typeName(pc, cur)
			pc.Diag.Report(diag.ErrTypeNotIndexable, fld.Span, baseName)
			x.SetType(tt.TypError())
			return tt.TypError()
		}
	}

	x.SetType(cur)
	return cur
}

func (p *PassInferTypes) synthesizeFuncCallExprType(pc *PassContext, x *ir.FuncCallExpr) ir.TypID {
	tt := pc.Types

	prelimArgTypes := make([]ir.TypID, len(x.Args))
	for i, arg := range x.Args {
		prelimArgTypes[i] = p.synthesizeTypeFromExpr(pc, arg.Expr)
	}

	if varRef, ok := x.Callee.(*ir.VarRef); ok {
		scope, _ := pc.Pkg.Scopes.EnclosingScope(x.ID())
		candidates := pc.Pkg.Scopes.LookupAll(scope, varRef.Ref.Name)

		if len(candidates) > 1 {
			bestMatch := p.resolveOverload(pc, candidates, prelimArgTypes)
			if bestMatch != 0 {
				varRef.Ref.Sym = bestMatch
			}
		}
	}

	if fa, ok := x.Callee.(*ir.FieldAccessExpr); ok && len(fa.Fields) > 0 {
		recvTy := p.synthesizeTypeFromExpr(pc, fa.Expr)
		methodName := fa.Fields[len(fa.Fields)-1].Name
		if ty, ok := tt.GetByID(recvTy); ok && ty.Kind == ir.TK_Struct {
			var candidates []ir.SymID
			for _, m := range ty.StructMethods {
				if m.Name == methodName && m.Sym != 0 {
					candidates = append(candidates, m.Sym)
				}
			}

			if len(candidates) > 1 {
				best := p.resolveOverload(pc, candidates, prelimArgTypes)
				if best != 0 {
					fa.MethodSym = best
					for _, pkg := range pc.Pkgs {
						if bestSym, ok := pkg.Syms.GetByID(best); ok {
							fa.SetType(bestSym.Typ)
							break
						}
					}
				}
			}
		}
	}

	p.rewriteComposableCalleeToCtor(pc, x, prelimArgTypes)

	funcTy := p.synthesizeTypeFromExpr(pc, x.Callee)
	funcTyDef, ok := tt.GetByID(funcTy)
	if !ok || funcTyDef.Kind != ir.TK_Function {
		pc.Diag.Report(diag.ErrTypeMismatch, x.Callee.Span(), "function", typeName(pc, funcTy))
		x.SetType(tt.TypError())
		return tt.TypError()
	}

	hasNamedArgs := false
	for _, arg := range x.Args {
		if arg.Name != "" {
			hasNamedArgs = true
			break
		}
	}

	var argTypes []ir.TypID
	if hasNamedArgs {
		argTypes = make([]ir.TypID, len(funcTyDef.ParamTypes))
		reorderedArgs := make([]ir.FuncCallArg, len(funcTyDef.ParamTypes))
		positionalIndex := 0
		used := make([]bool, len(x.Args))

		for i, arg := range x.Args {
			if arg.Name == "" {
				if positionalIndex >= len(funcTyDef.ParamTypes) {
					pc.Diag.Report(diag.ErrFuncParamMismatch, x.Span(), typeKeyDisplay(funcTyDef), "too many positional arguments")
					x.SetType(tt.TypError())
					return tt.TypError()
				}

				reorderedArgs[positionalIndex] = arg
				used[i] = true
				positionalIndex++
			}
		}

		for i, arg := range x.Args {
			if used[i] {
				continue
			}

			paramIndex := -1
			for pi, param := range funcTyDef.ParamTypes {
				if param.Name.Name == arg.Name {
					paramIndex = pi
					break
				}
			}

			if paramIndex == -1 {
				pc.Diag.Report(diag.ErrFuncParamMismatch, x.Span(), typeKeyDisplay(funcTyDef), fmt.Sprintf("unknown parameter name '%s'", arg.Name))
				x.SetType(tt.TypError())
				return tt.TypError()
			}

			if reorderedArgs[paramIndex].Expr != nil {
				pc.Diag.Report(diag.ErrFuncParamMismatch, x.Span(), typeKeyDisplay(funcTyDef), fmt.Sprintf("parameter '%s' specified multiple times", arg.Name))
				x.SetType(tt.TypError())
				return tt.TypError()
			}

			reorderedArgs[paramIndex] = arg
		}

		x.Args = reorderedArgs

		for i, arg := range reorderedArgs {
			if arg.Expr != nil {
				argTypes[i] = p.synthesizeTypeFromExpr(pc, arg.Expr)
			} else {
				argTypes[i] = 0
			}
		}
	} else {
		argTypes = make([]ir.TypID, len(x.Args))
		for i, arg := range x.Args {
			argTypes[i] = p.synthesizeTypeFromExpr(pc, arg.Expr)
			if i < len(funcTyDef.ParamTypes) && funcTyDef.ParamTypes[i] != nil && funcTyDef.ParamTypes[i].Type != nil {
				p.applyLiteralTypeHint(pc, arg.Expr, funcTyDef.ParamTypes[i].Type.Typ)
				argTypes[i] = arg.Expr.GetType()
			}
		}
	}

	if ok, reason := assertFunctionParameterCompatibility(pc, tt, funcTyDef, argTypes, x.Args); !ok {
		pc.Diag.Report(diag.ErrFuncParamMismatch, x.Span(), typeKeyDisplay(funcTyDef), reason)
		x.SetType(tt.TypError())
		return tt.TypError()
	}

	x.SetType(funcTyDef.ReturnType)
	return funcTyDef.ReturnType
}

func (p *PassInferTypes) synthesizeNewExprType(pc *PassContext, x *ir.NewExpr) ir.TypID {
	tt := pc.Types

	argTypes := make([]ir.TypID, len(x.Args))
	for i, arg := range x.Args {
		argTypes[i] = p.synthesizeTypeFromExpr(pc, arg.Expr)
	}

	if x.TypeName.Sym == 0 {
		x.SetType(tt.TypError())
		return tt.TypError()
	}

	symPkg := pc.Pkg
	if x.Qualifier != "" {
		scope, _ := pc.Pkg.Scopes.EnclosingScope(x.ID())
		if qSym, ok := pc.Pkg.Scopes.Lookup(scope, x.Qualifier); ok {
			if pkgSym, found := pc.Pkg.Syms.GetByID(qSym); found && pkgSym.Kind == ir.SK_Package {
				if target := findPackageByPath(pc, pkgSym.PackagePath); target != nil {
					symPkg = target
				}
			}
		}
	}

	sym, ok := symPkg.Syms.GetByID(x.TypeName.Sym)
	if !ok || sym.Typ == 0 {
		x.SetType(tt.TypError())
		return tt.TypError()
	}

	ty, ok := tt.GetByID(sym.Typ)
	if !ok || ty.Kind != ir.TK_Struct {
		pc.Diag.Report(diag.ErrTypeMismatch, x.TypeName.Span, "user-defined type", x.TypeName.Name)
		x.SetType(tt.TypError())
		return tt.TypError()
	}

	if x.CtorSym != 0 {
		x.SetType(sym.Typ)
		return sym.Typ
	}

	typeArgSub := buildTypeArgSubstitution(ty.TypeParams, x.TypeArgs)
	if len(x.Args) > 0 {
		matchedSym := ir.SymID(0)
		var matchedArgs []ir.FuncCallArg
		bestScore := -1
		for _, ci := range ty.StructCtors {
			ftDef, ok := tt.GetByID(ci.FuncTyp)
			if !ok {
				continue
			}

			params := substituteFuncParams(tt, ftDef.ParamTypes, typeArgSub)
			resolved, ok := resolveCallArgs(tt, params, x.Args, argTypes)
			if !ok {
				continue
			}

			score := 0
			for _, p := range params {
				if p == nil || p.Type == nil {
					continue
				}

				if p.Type.Typ != tt.PrimAny() {
					score++
				}
			}

			if score > bestScore {
				bestScore = score
				matchedSym = ci.Sym
				matchedArgs = resolved
			}
		}

		if matchedSym == 0 {
			pc.Diag.Report(diag.ErrFuncParamMismatch, x.Span(), "constructor", x.TypeName.Name)
		} else {
			x.Args = matchedArgs
			if len(typeArgSub) > 0 {
				for i, ci := range ty.StructCtors {
					if ci.Sym != matchedSym {
						continue
					}

					if ftDef, ok := tt.GetByID(ci.FuncTyp); ok {
						for j := range x.Args {
							if x.Args[j].Expr == nil || j >= len(ftDef.ParamTypes) || ftDef.ParamTypes[j] == nil || ftDef.ParamTypes[j].Type == nil {
								continue
							}

							erased := eraseTypeParams(tt, ftDef.ParamTypes[j].Type.Typ)
							applyCodegenLiteralHint(tt, x.Args[j].Expr, erased)
						}
					}

					_ = i
					break
				}
			}
		}

		x.CtorSym = matchedSym
	}

	x.SetType(sym.Typ)
	return sym.Typ
}
