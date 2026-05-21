package passes

import (
	"sova/internal/diag"
	"sova/internal/ir"
)

// PassBindDeclare is a pass that declares symbols and binds them with their nodes to their respective scopes.
type PassBindDeclare struct{}

func (p *PassBindDeclare) Name() string       { return "bind_declare" }
func (p *PassBindDeclare) Scope() PassScope   { return PerPackage }
func (p *PassBindDeclare) Requires() []string { return []string{"resolve_libs", "inline_mixins"} }
func (p *PassBindDeclare) NoErrors() bool     { return false }

func (p *PassBindDeclare) Run(pc *PassContext) error {
	pkg := pc.Pkg

	for _, f := range pc.Pkg.Files {
		p.preRegisterTypes(pc, f)
	}

	for _, f := range pc.Pkg.Files {
		p.validateExterns(pc, f)
		for _, st := range f.Hir.Statements {
			if err := p.bindStmtScopes(pkg, st, pkg.Root); err != nil {
				return err
			}
		}
	}

	return nil
}

func (p *PassBindDeclare) preRegisterTypes(pc *PassContext, f *ir.PreparsedFile) {
	pkgPath := pc.Pkg.Path.String()
	for _, st := range f.Hir.Statements {
		switch td := st.(type) {
		case *ir.TypeDeclStmt:
			pc.Types.StructOf(pkgPath, td.Name.Name, nil)
		case *ir.InterfaceDeclStmt:
			pc.Types.InterfaceOf(pkgPath, td.Name.Name)
		case *ir.EnumDeclStmt:
			pc.Types.EnumOf(pkgPath, td.Name.Name, nil, nil, true)
		case *ir.ExternDeclStmt:
			for _, t := range td.Types {
				pc.Types.StructOf(pkgPath, t.Name.Name, nil)
			}
			for _, iface := range td.Interfaces {
				pc.Types.InterfaceOf(pkgPath, iface.Name.Name)
			}
		}
	}
}

func (p *PassBindDeclare) validateExterns(pc *PassContext, f *ir.PreparsedFile) {
	if f.Hir.Side.Kind != ir.SideShared {
		return
	}
	for _, st := range f.Hir.Statements {
		ext, ok := st.(*ir.ExternDeclStmt)
		if !ok {
			continue
		}
		for _, fn := range ext.Funcs {
			if fn.Mapping != nil && fn.Mapping.Simple != nil {
				pc.Diag.Report(diag.ErrSharedExternRequiresBothSides, fn.Name.Span, fn.Name.Name)
			}
		}
		for _, v := range ext.Vars {
			if v.Mapping != nil && v.Mapping.Simple != nil {
				pc.Diag.Report(diag.ErrSharedExternRequiresBothSides, v.Name.Span, v.Name.Name)
			}
		}
	}
}

// bindAnnotations walks each annotation arg expression and registers its scope chain. Annotations may sit on top-level declarations or on type members; in both cases the args resolve against the declaration's enclosing scope.
func (p *PassBindDeclare) bindAnnotations(pkg *ir.PackageContext, annos []ir.Annotation, scope ir.ScopeID) error {
	for i := range annos {
		for _, arg := range annos[i].Args {
			if err := p.bindExprScopes(pkg, arg, scope); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *PassBindDeclare) bindStmtScopes(pkg *ir.PackageContext, st ir.Stmt, scope ir.ScopeID) error {
	if ir.IsNilStmt(st) {
		return nil
	}
	pkg.Scopes.BindNode(st.ID(), scope)

	switch st := st.(type) {
	case *ir.BlockStmt:
		newScope := pkg.Scopes.NewScope(scope)
		for _, s := range st.Stmts {
			if err := p.bindStmtScopes(pkg, s, newScope); err != nil {
				return err
			}
		}
	case *ir.VarDeclStmt:
		var flags ir.SymbolFlags
		if st.IsConst {
			flags = ir.SF_Const
		}
		for i := range st.Targets {
			target := &st.Targets[i]
			if target.Name != nil {
				sym := pkg.Syms.NewSymbol(ir.SK_Variable, target.Name.Name, scope, 0, st.ID(), flags)
				target.Name.Sym = sym
				pkg.Syms.SetDoc(sym, st.GetDoc())
				pkg.Scopes.DeclareSymbol(scope, target.Name.Name, sym, pkg.Syms)
			}
		}

		if st.Init != nil {
			return p.bindExprScopes(pkg, st.Init, scope)
		}
	case *ir.FuncDeclStmt:
		sym := pkg.Syms.NewSymbol(ir.SK_Function, st.Name.Name, scope, 0, st.ID()) // 0 = type, will be set later
		st.Name.Sym = sym
		pkg.Syms.SetDoc(sym, st.GetDoc())

		if err := p.bindAnnotations(pkg, st.Annotations, scope); err != nil {
			return err
		}

		if pkg.Scopes.DeclareSymbol(scope, st.Name.Name, sym, pkg.Syms) {
			funcScope := pkg.Scopes.NewScope(scope)

			for _, param := range st.Params {
				paramSym := pkg.Syms.NewSymbol(ir.SK_Variable, param.Name.Name, funcScope, 0, param.ID())
				param.Name.Sym = paramSym
				pkg.Scopes.DeclareSymbol(funcScope, param.Name.Name, paramSym, pkg.Syms)
			}

			return p.bindStmtScopes(pkg, st.Body, funcScope)
		}
	case *ir.ExternDeclStmt:
		for _, fn := range st.Funcs {
			sym := pkg.Syms.NewSymbol(ir.SK_Function, fn.Name.Name, scope, 0, fn.ID())
			fn.Name.Sym = sym
			pkg.Syms.SetDoc(sym, fn.GetDoc())
			pkg.Scopes.DeclareSymbol(scope, fn.Name.Name, sym, pkg.Syms)
		}
		for _, v := range st.Vars {
			flags := ir.SF_None
			if v.IsConst {
				flags = ir.SF_Const
			}
			sym := pkg.Syms.NewSymbol(ir.SK_Variable, v.Name.Name, scope, 0, v.ID(), flags)
			v.Name.Sym = sym
			pkg.Syms.SetDoc(sym, v.GetDoc())
			pkg.Scopes.DeclareSymbol(scope, v.Name.Name, sym, pkg.Syms)
		}
		for _, t := range st.Types {
			if err := p.bindStmtScopes(pkg, t, scope); err != nil {
				return err
			}
		}
		for _, iface := range st.Interfaces {
			if err := p.bindStmtScopes(pkg, iface, scope); err != nil {
				return err
			}
		}
	case *ir.TypeDeclStmt:
		typeSym := pkg.Syms.NewSymbol(ir.SK_Function, st.Name.Name, scope, 0, st.ID())
		st.Name.Sym = typeSym
		pkg.Syms.SetDoc(typeSym, st.GetDoc())
		pkg.Scopes.DeclareSymbol(scope, st.Name.Name, typeSym, pkg.Syms)

		typeScope := pkg.Scopes.NewScope(scope)
		if err := p.bindAnnotations(pkg, st.Annotations, scope); err != nil {
			return err
		}
		for _, field := range st.Fields {
			fieldSym := pkg.Syms.NewSymbol(ir.SK_Variable, field.Name.Name, typeScope, 0, field.ID())
			field.Name.Sym = fieldSym
			pkg.Scopes.DeclareSymbol(typeScope, field.Name.Name, fieldSym, pkg.Syms)
			if err := p.bindAnnotations(pkg, field.Annotations, scope); err != nil {
				return err
			}
			if field.Default != nil {
				if err := p.bindExprScopes(pkg, field.Default, scope); err != nil {
					return err
				}
			}
		}
		for _, method := range st.Methods {
			fn := method.Func
			methodSym := pkg.Syms.NewSymbol(ir.SK_Function, fn.Name.Name, typeScope, 0, fn.ID(), ir.SF_TypeMethod)
			fn.Name.Sym = methodSym
			pkg.Scopes.DeclareSymbol(typeScope, fn.Name.Name, methodSym, pkg.Syms)

			if err := p.bindAnnotations(pkg, method.Annotations, scope); err != nil {
				return err
			}

			methodScope := pkg.Scopes.NewScope(typeScope)
			thisSym := pkg.Syms.NewSymbol(ir.SK_Variable, "this", methodScope, 0, fn.ID())
			pkg.Scopes.DeclareSymbol(methodScope, "this", thisSym, pkg.Syms)
			method.ThisSym = thisSym

			for _, param := range fn.Params {
				paramSym := pkg.Syms.NewSymbol(ir.SK_Variable, param.Name.Name, methodScope, 0, param.ID())
				param.Name.Sym = paramSym
				pkg.Scopes.DeclareSymbol(methodScope, param.Name.Name, paramSym, pkg.Syms)
				if param.Default != nil {
					if err := p.bindExprScopes(pkg, param.Default, methodScope); err != nil {
						return err
					}
				}
			}
			if fn.Body != nil {
				if err := p.bindStmtScopes(pkg, fn.Body, methodScope); err != nil {
					return err
				}
			}
		}
		for _, ctor := range st.Ctors {
			ctorSym := pkg.Syms.NewSymbol(ir.SK_Function, "new", typeScope, 0, ctor.ID())
			ctor.Sym = ctorSym
			pkg.Scopes.DeclareSymbol(typeScope, "new", ctorSym, pkg.Syms)

			if err := p.bindAnnotations(pkg, ctor.Annotations, scope); err != nil {
				return err
			}

			ctorScope := pkg.Scopes.NewScope(typeScope)
			thisSym := pkg.Syms.NewSymbol(ir.SK_Variable, "this", ctorScope, 0, ctor.ID())
			pkg.Scopes.DeclareSymbol(ctorScope, "this", thisSym, pkg.Syms)
			ctor.ThisSym = thisSym

			for _, param := range ctor.Params {
				paramSym := pkg.Syms.NewSymbol(ir.SK_Variable, param.Name.Name, ctorScope, 0, param.ID())
				param.Name.Sym = paramSym
				pkg.Scopes.DeclareSymbol(ctorScope, param.Name.Name, paramSym, pkg.Syms)
				if param.Default != nil {
					if err := p.bindExprScopes(pkg, param.Default, ctorScope); err != nil {
						return err
					}
				}
			}
			if ctor.Body != nil {
				if err := p.bindStmtScopes(pkg, ctor.Body, ctorScope); err != nil {
					return err
				}
			}
		}
		for _, cast := range st.Casts {
			castName := "cast_" + cast.Param.Type.CustomName
			if cast.Param.Type.Kind == ir.TK_PrimitiveString {
				castName = "cast_string"
			} else if cast.Param.Type.Kind == ir.TK_PrimitiveInt {
				castName = "cast_int"
			} else if cast.Param.Type.Kind == ir.TK_PrimitiveFloat {
				castName = "cast_float"
			} else if cast.Param.Type.Kind == ir.TK_PrimitiveBool {
				castName = "cast_bool"
			} else if cast.Param.Type.Kind == ir.TK_PrimitiveChar {
				castName = "cast_char"
			}
			castSym := pkg.Syms.NewSymbol(ir.SK_Function, castName, typeScope, 0, cast.ID())
			cast.Sym = castSym
			pkg.Scopes.DeclareSymbol(typeScope, castName, castSym, pkg.Syms)

			if err := p.bindAnnotations(pkg, cast.Annotations, scope); err != nil {
				return err
			}

			castScope := pkg.Scopes.NewScope(typeScope)
			if cast.Param != nil {
				paramSym := pkg.Syms.NewSymbol(ir.SK_Variable, cast.Param.Name.Name, castScope, 0, cast.Param.ID())
				cast.Param.Name.Sym = paramSym
				pkg.Scopes.DeclareSymbol(castScope, cast.Param.Name.Name, paramSym, pkg.Syms)
			}
			if cast.Body != nil {
				if err := p.bindStmtScopes(pkg, cast.Body, castScope); err != nil {
					return err
				}
			}
		}
	case *ir.ImportStmt:
		alias := st.Alias
		if alias == "" {
			return nil
		}
		if existing, ok := pkg.Scopes.LookupOnlyCurrent(scope, alias); ok {
			if existingSym, found := pkg.Syms.GetByID(existing); found && existingSym.Kind == ir.SK_Package && existingSym.PackagePath == st.Path.String() {
				return nil
			}
		}
		sym := pkg.Syms.NewSymbol(ir.SK_Package, alias, scope, 0, st.ID())
		if pkgSym, ok := pkg.Syms.GetByID(sym); ok {
			pkgSym.PackagePath = st.Path.String()
		}
		pkg.Scopes.DeclareSymbol(scope, alias, sym, pkg.Syms)
	case *ir.TypeAliasStmt:
		aliasSym := pkg.Syms.NewSymbol(ir.SK_Function, st.Name.Name, scope, 0, st.ID())
		st.Name.Sym = aliasSym
		pkg.Syms.SetDoc(aliasSym, st.GetDoc())
		pkg.Scopes.DeclareSymbol(scope, st.Name.Name, aliasSym, pkg.Syms)
	case *ir.InterfaceDeclStmt:
		ifaceSym := pkg.Syms.NewSymbol(ir.SK_Function, st.Name.Name, scope, 0, st.ID())
		st.Name.Sym = ifaceSym
		pkg.Syms.SetDoc(ifaceSym, st.GetDoc())
		pkg.Scopes.DeclareSymbol(scope, st.Name.Name, ifaceSym, pkg.Syms)

		ifaceScope := pkg.Scopes.NewScope(scope)
		for _, sig := range st.Methods {
			sigSym := pkg.Syms.NewSymbol(ir.SK_Function, sig.Name.Name, ifaceScope, 0, sig.ID(), ir.SF_TypeMethod)
			sig.Name.Sym = sigSym
			pkg.Scopes.DeclareSymbol(ifaceScope, sig.Name.Name, sigSym, pkg.Syms)

			sigParamScope := pkg.Scopes.NewScope(ifaceScope)
			for _, param := range sig.Params {
				paramSym := pkg.Syms.NewSymbol(ir.SK_Variable, param.Name.Name, sigParamScope, 0, param.ID())
				param.Name.Sym = paramSym
				pkg.Scopes.DeclareSymbol(sigParamScope, param.Name.Name, paramSym, pkg.Syms)
			}
		}
	case *ir.EnumDeclStmt:
		// Create enum symbol
		enumSym := pkg.Syms.NewSymbol(ir.SK_Function, st.Name.Name, scope, 0, st.ID())
		st.Name.Sym = enumSym
		pkg.Syms.SetDoc(enumSym, st.GetDoc())
		pkg.Scopes.DeclareSymbol(scope, st.Name.Name, enumSym, pkg.Syms)

		// Create scope for enum cases and methods
		enumScope := pkg.Scopes.NewScope(scope)

		// Declare each case as a symbol in the enum scope
		for i, c := range st.Cases {
			c.Ordinal = i
			caseSym := pkg.Syms.NewSymbol(ir.SK_Variable, c.Name.Name, enumScope, 0, c.ID())
			c.Name.Sym = caseSym
			pkg.Scopes.DeclareSymbol(enumScope, c.Name.Name, caseSym, pkg.Syms)

			// Bind case argument expressions
			for _, arg := range c.Args {
				if err := p.bindExprScopes(pkg, arg, scope); err != nil {
					return err
				}
			}
		}

		// Bind field default expressions
		for _, field := range st.Fields {
			if field.Default != nil {
				if err := p.bindExprScopes(pkg, field.Default, scope); err != nil {
					return err
				}
			}
		}

		// Handle methods - create a scope with "this" binding
		for _, method := range st.Methods {
			methodSym := pkg.Syms.NewSymbol(ir.SK_Function, method.Name.Name, enumScope, 0, method.ID())
			method.Name.Sym = methodSym
			pkg.Scopes.DeclareSymbol(enumScope, method.Name.Name, methodSym, pkg.Syms)

			methodScope := pkg.Scopes.NewScope(enumScope)

			// Add implicit "this" parameter with the enum type
			thisSym := pkg.Syms.NewSymbol(ir.SK_Variable, "this", methodScope, 0, method.ID())
			pkg.Scopes.DeclareSymbol(methodScope, "this", thisSym, pkg.Syms)

			// Bind method parameters
			for _, param := range method.Params {
				paramSym := pkg.Syms.NewSymbol(ir.SK_Variable, param.Name.Name, methodScope, 0, param.ID())
				param.Name.Sym = paramSym
				pkg.Scopes.DeclareSymbol(methodScope, param.Name.Name, paramSym, pkg.Syms)
			}

			// Bind method body
			if err := p.bindStmtScopes(pkg, method.Body, methodScope); err != nil {
				return err
			}
		}
	case *ir.ExprStmt:
		return p.bindExprScopes(pkg, st.Expr, scope)
	case *ir.FieldAssignmentStmt:
		return p.bindExprScopes(pkg, st.Value, scope)
	case *ir.MultiAssignmentStmt:
		return p.bindExprScopes(pkg, st.Value, scope)
	case *ir.IfStmt:
		if err := p.bindExprScopes(pkg, st.Cond, scope); err != nil {
			return err
		}
		if err := p.bindStmtScopes(pkg, st.Then, scope); err != nil {
			return err
		}
		for _, eb := range st.ElseIfs {
			if err := p.bindExprScopes(pkg, eb.Cond, scope); err != nil {
				return err
			}
			if err := p.bindStmtScopes(pkg, eb.Then, scope); err != nil {
				return err
			}
		}
		if st.Else != nil {
			if err := p.bindStmtScopes(pkg, st.Else, scope); err != nil {
				return err
			}
		}
	case *ir.SwitchStmt:
		if err := p.bindExprScopes(pkg, st.Expr, scope); err != nil {
			return err
		}

		for _, c := range st.Cases {
			newScope := pkg.Scopes.NewScope(scope)
			for _, v := range c.Values {
				if err := p.bindExprScopes(pkg, v, newScope); err != nil {
					return err
				}
			}
			for _, s := range c.Stmts {
				if err := p.bindStmtScopes(pkg, s, newScope); err != nil {
					return err
				}
			}
		}

		if st.Default != nil {
			newScope := pkg.Scopes.NewScope(scope)
			for _, s := range st.Default {
				if err := p.bindStmtScopes(pkg, s, newScope); err != nil {
					return err
				}
			}
		}
	case *ir.ReturnStmt:
		for _, result := range st.Results {
			if err := p.bindExprScopes(pkg, result, scope); err != nil {
				return err
			}
		}
	case *ir.GuardStmt:
		if err := p.bindExprScopes(pkg, st.Cond, scope); err != nil {
			return err
		}

		for _, ret := range st.Returns {
			if err := p.bindExprScopes(pkg, ret, scope); err != nil {
				return err
			}
		}
	case *ir.ForStmt:
		forScope := pkg.Scopes.NewScope(scope)

		if st.CondType == ir.ForCondInt {
			if st.CondInt.Init != nil && len(st.CondInt.Init.Targets) > 0 {
				target := &st.CondInt.Init.Targets[0]
				if target.Name != nil {
					sym := pkg.Syms.NewSymbol(ir.SK_Variable, target.Name.Name, forScope, 0, st.ID())
					target.Name.Sym = sym

					if pkg.Scopes.DeclareSymbol(forScope, target.Name.Name, sym, pkg.Syms) && st.CondInt.Init.Init != nil {
						if err := p.bindExprScopes(pkg, st.CondInt.Init.Init, forScope); err != nil {
							return err
						}
					}
				}
			}

			if st.CondInt.Cond != nil {
				if err := p.bindExprScopes(pkg, st.CondInt.Cond, forScope); err != nil {
					return err
				}
			}

			if st.CondInt.Post != nil {
				if err := p.bindExprScopes(pkg, st.CondInt.Post, forScope); err != nil {
					return err
				}
			}
		} else if st.CondType == ir.ForCondIn {
			{
				sym := pkg.Syms.NewSymbol(ir.SK_Variable, st.CondIn.InFirstVar.Name, forScope, 0, st.ID()) // 0 = type, will be set later
				st.CondIn.InFirstVar.Sym = sym

				pkg.Scopes.DeclareSymbol(forScope, st.CondIn.InFirstVar.Name, sym, pkg.Syms)
			}

			if st.CondIn.InSecondVar != nil {
				sym := pkg.Syms.NewSymbol(ir.SK_Variable, st.CondIn.InSecondVar.Name, forScope, 0, st.ID()) // 0 = type, will be set later
				st.CondIn.InSecondVar.Sym = sym

				pkg.Scopes.DeclareSymbol(forScope, st.CondIn.InSecondVar.Name, sym, pkg.Syms)
			}

			if st.CondIn.InThirdVar != nil {
				sym := pkg.Syms.NewSymbol(ir.SK_Variable, st.CondIn.InThirdVar.Name, forScope, 0, st.ID()) // 0 = type, will be set later
				st.CondIn.InThirdVar.Sym = sym

				pkg.Scopes.DeclareSymbol(forScope, st.CondIn.InThirdVar.Name, sym, pkg.Syms)
			}

			if err := p.bindExprScopes(pkg, st.CondIn.IterExpr, scope); err != nil {
				return err
			}
		} else if st.CondType == ir.ForCondRange {
			sym := pkg.Syms.NewSymbol(ir.SK_Variable, st.CondRange.RangeVar.Name, forScope, 0, st.ID()) // 0 = type, will be set later
			st.CondRange.RangeVar.Sym = sym

			pkg.Scopes.DeclareSymbol(forScope, st.CondRange.RangeVar.Name, sym, pkg.Syms)

			if err := p.bindExprScopes(pkg, st.CondRange.RangeStart, scope); err != nil {
				return err
			}

			if err := p.bindExprScopes(pkg, st.CondRange.RangeEnd, scope); err != nil {
				return err
			}
		}

		if st.Body != nil {
			pkg.Scopes.BindNode(ir.BlockID(st.Body), forScope)
			for _, s := range ir.BlockStmts(st.Body) {
				if err := p.bindStmtScopes(pkg, s, forScope); err != nil {
					return err
				}
			}
		}
		return nil
	case *ir.WhileStmt:
		if err := p.bindExprScopes(pkg, st.Cond, scope); err != nil {
			return err
		}

		return p.bindStmtScopes(pkg, st.Body, scope)
	case *ir.TestDeclStmt:
		pkg.Scopes.BindNode(st.ID(), scope)
		if st.Body != nil {
			return p.bindStmtScopes(pkg, st.Body, scope)
		}
	case *ir.GroupDeclStmt:
		pkg.Scopes.BindNode(st.ID(), scope)
		for _, s := range st.Body {
			if err := p.bindStmtScopes(pkg, s, scope); err != nil {
				return err
			}
		}
	case *ir.SetupStmt:
		pkg.Scopes.BindNode(st.ID(), scope)
		if st.Body != nil {
			return p.bindStmtScopes(pkg, st.Body, scope)
		}
	case *ir.TeardownStmt:
		pkg.Scopes.BindNode(st.ID(), scope)
		if st.Body != nil {
			return p.bindStmtScopes(pkg, st.Body, scope)
		}
	case *ir.AssertStmt:
		pkg.Scopes.BindNode(st.ID(), scope)
		return p.bindExprScopes(pkg, st.Expr, scope)
	case *ir.AsSessionStmt:
		pkg.Scopes.BindNode(st.ID(), scope)
		if st.Body != nil {
			return p.bindStmtScopes(pkg, st.Body, scope)
		}
	case *ir.GoStmt:
		pkg.Scopes.BindNode(st.ID(), scope)
		if st.Body != nil {
			return p.bindStmtScopes(pkg, st.Body, scope)
		}
		if st.Call != nil {
			return p.bindExprScopes(pkg, st.Call, scope)
		}
	case *ir.DeferStmt:
		pkg.Scopes.BindNode(st.ID(), scope)
		if st.Body != nil {
			return p.bindStmtScopes(pkg, st.Body, scope)
		}
		if st.Call != nil {
			return p.bindExprScopes(pkg, st.Call, scope)
		}
	case *ir.SelectStmt:
		pkg.Scopes.BindNode(st.ID(), scope)
		for _, cc := range st.Cases {
			caseScope := pkg.Scopes.NewScope(scope)
			if cc.Body != nil {
				pkg.Scopes.BindNode(ir.BlockID(cc.Body), caseScope)
			}
			if cc.ChanExpr != nil {
				if err := p.bindExprScopes(pkg, cc.ChanExpr, caseScope); err != nil {
					return err
				}
			}
			if cc.SendValue != nil {
				if err := p.bindExprScopes(pkg, cc.SendValue, caseScope); err != nil {
					return err
				}
			}
			for i := range cc.Targets {
				tgt := &cc.Targets[i]
				if tgt.Name == nil {
					continue
				}
				var ownerID ir.NodeID
				if cc.Body != nil {
					ownerID = ir.BlockID(cc.Body)
				}
				sym := pkg.Syms.NewSymbol(ir.SK_Variable, tgt.Name.Name, caseScope, 0, ownerID)
				tgt.Name.Sym = sym
				pkg.Scopes.DeclareSymbol(caseScope, tgt.Name.Name, sym, pkg.Syms)
			}
			if cc.Body != nil {
				for _, s := range ir.BlockStmts(cc.Body) {
					if err := p.bindStmtScopes(pkg, s, caseScope); err != nil {
						return err
					}
				}
			}
		}
		if st.Default != nil {
			defaultScope := pkg.Scopes.NewScope(scope)
			pkg.Scopes.BindNode(st.Default.ID(), defaultScope)
			for _, s := range st.Default.Stmts {
				if err := p.bindStmtScopes(pkg, s, defaultScope); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (p *PassBindDeclare) bindExprScopes(pkg *ir.PackageContext, expr ir.Expr, scope ir.ScopeID) error {
	if ir.IsNilExpr(expr) {
		return nil
	}

	pkg.Scopes.BindNode(expr.ID(), scope)

	switch x := expr.(type) {
	case *ir.WhenExpr:
		if err := p.bindExprScopes(pkg, x.Expr, scope); err != nil {
			return err
		}
		for _, c := range x.Cases {
			for _, v := range c.Values {
				if err := p.bindExprScopes(pkg, v, scope); err != nil {
					return err
				}
			}

			if err := p.bindExprScopes(pkg, c.Then, scope); err != nil {
				return err
			}
		}
	case *ir.UnaryExpr:
		return p.bindExprScopes(pkg, x.Expr, scope)
	case *ir.PrefixUnaryExpr:
		return p.bindExprScopes(pkg, x.Expr, scope)
	case *ir.PostfixUnaryExpr:
		return p.bindExprScopes(pkg, x.Expr, scope)
	case *ir.BinaryExpr:
		if err := p.bindExprScopes(pkg, x.Left, scope); err != nil {
			return err
		}
		return p.bindExprScopes(pkg, x.Right, scope)
	case *ir.TenaryExpr:
		if err := p.bindExprScopes(pkg, x.Cond, scope); err != nil {
			return err
		}
		if err := p.bindExprScopes(pkg, x.Then, scope); err != nil {
			return err
		}
		return p.bindExprScopes(pkg, x.Else, scope)
	case *ir.GroupedExpr:
		return p.bindExprScopes(pkg, x.Expr, scope)
	case *ir.AsExpr:
		return p.bindExprScopes(pkg, x.Expr, scope)
	case *ir.OptionUnwrapExpr:
		return p.bindExprScopes(pkg, x.Expr, scope)
	case *ir.AssignmentExpr:
		return p.bindExprScopes(pkg, x.Right, scope)
	case *ir.IndexExpr:
		if err := p.bindExprScopes(pkg, x.Expr, scope); err != nil {
			return err
		}
		return p.bindExprScopes(pkg, x.Index, scope)
	case *ir.FieldAccessExpr:
		return p.bindExprScopes(pkg, x.Expr, scope)
	case *ir.RangeExpr:
		if err := p.bindExprScopes(pkg, x.Start, scope); err != nil {
			return err
		}

		if err := p.bindExprScopes(pkg, x.End, scope); err != nil {
			return err
		}

		if x.Inc != nil {
			return p.bindExprScopes(pkg, x.Inc, scope)
		}
	case *ir.FuncCallExpr:
		if err := p.bindExprScopes(pkg, x.Callee, scope); err != nil {
			return err
		}

		for _, arg := range x.Args {
			if err := p.bindExprScopes(pkg, arg.Expr, scope); err != nil {
				return err
			}
		}
	case *ir.ComposableCallExpr:
		if err := p.bindExprScopes(pkg, x.Callee, scope); err != nil {
			return err
		}
		for _, arg := range x.Args {
			if err := p.bindExprScopes(pkg, arg.Expr, scope); err != nil {
				return err
			}
		}
		for _, child := range x.Children {
			if child.Expr != nil {
				if err := p.bindExprScopes(pkg, child.Expr, scope); err != nil {
					return err
				}
			}
			if child.Stmt != nil {
				if err := p.bindStmtScopes(pkg, child.Stmt, scope); err != nil {
					return err
				}
			}
		}
	case *ir.FuncLitExpr:
		newScope := pkg.Scopes.NewScope(scope)
		for _, param := range x.Params {
			pkg.Scopes.BindNode(param.ID(), newScope)

			sym := pkg.Syms.NewSymbol(ir.SK_Variable, param.Name.Name, newScope, 0, param.ID()) // 0 = type, will be set later
			param.Name.Sym = sym

			pkg.Scopes.DeclareSymbol(newScope, param.Name.Name, sym, pkg.Syms)
		}

		return p.bindStmtScopes(pkg, x.Body, newScope)
	case *ir.CoalesceExpr:
		if x.Left != nil {
			if err := p.bindExprScopes(pkg, x.Left, scope); err != nil {
				return err
			}
		}
		if x.Default != nil {
			if err := p.bindExprScopes(pkg, x.Default, scope); err != nil {
				return err
			}
		}
	case *ir.ArrayLiteral:
		for _, el := range x.Elems {
			if err := p.bindExprScopes(pkg, el, scope); err != nil {
				return err
			}
		}
	case *ir.MapLiteral:
		for _, en := range x.Entries {
			if err := p.bindExprScopes(pkg, en.Key, scope); err != nil {
				return err
			}
			if err := p.bindExprScopes(pkg, en.Value, scope); err != nil {
				return err
			}
		}
	case *ir.TupleLiteral:
		for _, el := range x.Elems {
			if err := p.bindExprScopes(pkg, el, scope); err != nil {
				return err
			}
		}
	case *ir.StringTemplateExpr:
		for _, part := range x.Parts {
			if part.Expr != nil {
				if err := p.bindExprScopes(pkg, part.Expr, scope); err != nil {
					return err
				}
			}
		}
	case *ir.NewExpr:
		for i := range x.Args {
			if err := p.bindExprScopes(pkg, x.Args[i].Expr, scope); err != nil {
				return err
			}
		}
	case *ir.SessionExpr:
		_ = x
	}
	return nil
}
