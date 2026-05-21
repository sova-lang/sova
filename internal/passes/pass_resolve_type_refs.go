package passes

import "sova/internal/ir"

// PassResolveTypeRefs is a pass that resolves type references in the code to their respective type IDs.
type PassResolveTypeRefs struct {
	imports         map[string]*ir.PackageContext
	allPkgs         []*ir.PackageContext
	currentPkgPath  string
	genericParamMap map[string]ir.TypID                  // generic param name -> TypID, populated per generic decl
	aliases         map[string]map[string]*ir.TypeAliasStmt // pkgPath -> name -> alias decl, populated once per build
	aliasResolving  map[string]bool                      // cycle guard: "pkgPath:name" currently being resolved
}

func (p *PassResolveTypeRefs) Name() string       { return "resolve_typerefs" }
func (p *PassResolveTypeRefs) Scope() PassScope   { return PerPackage }
func (p *PassResolveTypeRefs) Requires() []string { return []string{"bind_declare"} }
func (p *PassResolveTypeRefs) NoErrors() bool     { return false }

func (p *PassResolveTypeRefs) Run(pc *PassContext) error {
	tt := pc.Types
	p.allPkgs = pc.Pkgs
	p.currentPkgPath = pc.Pkg.Path.String()
	if p.aliases == nil {
		p.aliases = map[string]map[string]*ir.TypeAliasStmt{}
		for _, pkg := range pc.Pkgs {
			pkgPath := pkg.Path.String()
			for _, f := range pkg.Files {
				for _, st := range f.Hir.Statements {
					if ta, ok := st.(*ir.TypeAliasStmt); ok && ta.Name.Name != "" {
						if p.aliases[pkgPath] == nil {
							p.aliases[pkgPath] = map[string]*ir.TypeAliasStmt{}
						}
						p.aliases[pkgPath][ta.Name.Name] = ta
					}
				}
			}
		}
		p.aliasResolving = map[string]bool{}
	}
	for _, f := range pc.Pkg.Files {
		p.imports = buildImportAliasMap(f.Hir, pc.Pkgs)
		p.resolveStmts(tt, f.Hir.Statements)
	}
	p.imports = nil
	p.currentPkgPath = ""
	return nil
}

// mergeGenericParams produces a copy of prev (or a fresh map) with each of `params` mapped to a fresh `TypeParamOf(ownerKey, name)` TypID. Used at the entry of a generic decl so unknown type names inside the decl body resolve to the right parameter symbol. Owner-key disambiguates `T` declared in two different generic decls. Constraints attached to each param are not enforced here; they live on the TypeParamDecl for later cross-checking when type arguments get supplied at instantiation sites.
func mergeGenericParams(prev map[string]ir.TypID, ownerKey string, params []ir.TypeParamDecl, tt *ir.TypeTable) map[string]ir.TypID {
	out := map[string]ir.TypID{}
	for k, v := range prev {
		out[k] = v
	}
	for _, p := range params {
		out[p.Name] = tt.TypeParamOf(ownerKey, p.Name)
	}
	return out
}

func buildImportAliasMap(f *ir.File, all []*ir.PackageContext) map[string]*ir.PackageContext {
	out := map[string]*ir.PackageContext{}
	if f == nil {
		return out
	}
	for _, st := range f.Statements {
		imp, ok := st.(*ir.ImportStmt)
		if !ok {
			continue
		}
		alias := imp.Alias
		if alias == "" && len(imp.Path) > 0 {
			alias = imp.Path[len(imp.Path)-1]
		}
		key := imp.Path.String()
		for _, pkg := range all {
			if pkg.Path.String() == key {
				out[alias] = pkg
				break
			}
		}
	}
	return out
}

func (p *PassResolveTypeRefs) resolveStmts(tt *ir.TypeTable, stmts []ir.Stmt) {
	for _, st := range stmts {
		if ir.IsNilStmt(st) {
			continue
		}
		switch s := st.(type) {
		case *ir.BlockStmt:
			p.resolveStmts(tt, s.Stmts)
		case *ir.VarDeclStmt:
			for i, target := range s.Targets {
				if target.TypeAnn != nil {
					s.Targets[i].TypeAnn.Typ = p.resolveTypeRef(tt, target.TypeAnn)
				}
			}

			if s.Init != nil {
				p.resolveExpr(tt, s.Init)
			}
		case *ir.FuncDeclStmt:
			prevMap := p.genericParamMap
			if len(s.TypeParams) > 0 {
				p.genericParamMap = mergeGenericParams(prevMap, p.currentPkgPath+":"+s.Name.Name, s.TypeParams, tt)
			}
			if s.ReturnType != nil {
				s.ReturnType.Typ = p.resolveTypeRef(tt, s.ReturnType)
			}

			for _, param := range s.Params {
				if param.Type != nil {
					param.Type.Typ = p.resolveTypeRef(tt, param.Type)
				}

				if param.Default != nil {
					p.resolveExpr(tt, param.Default)
				}
			}

			p.resolveStmts(tt, ir.BlockStmts(s.Body))
			p.genericParamMap = prevMap
		case *ir.ExternDeclStmt:
			for _, fn := range s.Funcs {
				prevMap := p.genericParamMap
				if len(fn.TypeParams) > 0 {
					p.genericParamMap = mergeGenericParams(prevMap, p.currentPkgPath+":"+fn.Name.Name, fn.TypeParams, tt)
				}
				if fn.ReturnType != nil {
					fn.ReturnType.Typ = p.resolveTypeRef(tt, fn.ReturnType)
				}

				for _, param := range fn.Params {
					if param.Type != nil {
						param.Type.Typ = p.resolveTypeRef(tt, param.Type)
					}
				}
				p.genericParamMap = prevMap
			}
			for _, v := range s.Vars {
				if v.Type != nil {
					v.Type.Typ = p.resolveTypeRef(tt, v.Type)
				}
			}
			for _, t := range s.Types {
				p.resolveStmts(tt, []ir.Stmt{t})
			}
			for _, iface := range s.Interfaces {
				p.resolveStmts(tt, []ir.Stmt{iface})
			}
		case *ir.TypeDeclStmt:
			prevMap := p.genericParamMap
			if len(s.TypeParams) > 0 {
				p.genericParamMap = mergeGenericParams(prevMap, p.currentPkgPath+":"+s.Name.Name, s.TypeParams, tt)
			}
			for _, field := range s.Fields {
				if field.Type != nil {
					field.Type.Typ = p.resolveTypeRef(tt, field.Type)
				}
				if field.Default != nil {
					p.resolveExpr(tt, field.Default)
				}
			}
			for _, ctor := range s.Ctors {
				for _, param := range ctor.Params {
					if param.Type != nil {
						param.Type.Typ = p.resolveTypeRef(tt, param.Type)
					}
					if param.Default != nil {
						p.resolveExpr(tt, param.Default)
					}
				}
				if ctor.Body != nil {
					p.resolveStmts(tt, ir.BlockStmts(ctor.Body))
				}
			}
			for _, method := range s.Methods {
				fn := method.Func
				if fn.ReturnType != nil {
					fn.ReturnType.Typ = p.resolveTypeRef(tt, fn.ReturnType)
				}
				for _, param := range fn.Params {
					if param.Type != nil {
						param.Type.Typ = p.resolveTypeRef(tt, param.Type)
					}
					if param.Default != nil {
						p.resolveExpr(tt, param.Default)
					}
				}
				if fn.Body != nil {
					p.resolveStmts(tt, ir.BlockStmts(fn.Body))
				}
			}
			for _, cast := range s.Casts {
				if cast.Param != nil && cast.Param.Type != nil {
					cast.Param.Type.Typ = p.resolveTypeRef(tt, cast.Param.Type)
				}
				if cast.ReturnType != nil {
					cast.ReturnType.Typ = p.resolveTypeRef(tt, cast.ReturnType)
				}
				if cast.Body != nil {
					p.resolveStmts(tt, ir.BlockStmts(cast.Body))
				}
			}
			p.genericParamMap = prevMap
		case *ir.TypeAliasStmt:
			_ = p.resolveAlias(tt, p.currentPkgPath, s.Name.Name)
		case *ir.InterfaceDeclStmt:
			for _, sig := range s.Methods {
				if sig.ReturnType != nil {
					sig.ReturnType.Typ = p.resolveTypeRef(tt, sig.ReturnType)
				}
				for _, param := range sig.Params {
					if param.Type != nil {
						param.Type.Typ = p.resolveTypeRef(tt, param.Type)
					}
				}
			}
		case *ir.EnumDeclStmt:
			// Resolve field types
			for _, field := range s.Fields {
				if field.Type != nil {
					field.Type.Typ = p.resolveTypeRef(tt, field.Type)
				}
				if field.Default != nil {
					p.resolveExpr(tt, field.Default)
				}
			}

			// Resolve case argument expressions
			for _, c := range s.Cases {
				for _, arg := range c.Args {
					p.resolveExpr(tt, arg)
				}
			}

			// Resolve method types
			for _, method := range s.Methods {
				if method.ReturnType != nil {
					method.ReturnType.Typ = p.resolveTypeRef(tt, method.ReturnType)
				}

				for _, param := range method.Params {
					if param.Type != nil {
						param.Type.Typ = p.resolveTypeRef(tt, param.Type)
					}

					if param.Default != nil {
						p.resolveExpr(tt, param.Default)
					}
				}

				p.resolveStmts(tt, ir.BlockStmts(method.Body))
			}
		case *ir.ExprStmt:
			p.resolveExpr(tt, s.Expr)
		case *ir.FieldAssignmentStmt:
			p.resolveExpr(tt, s.Value)
		case *ir.IfStmt:
			p.resolveExpr(tt, s.Cond)
			p.resolveStmts(tt, ir.BlockStmts(s.Then))
			for _, eb := range s.ElseIfs {
				p.resolveExpr(tt, eb.Cond)
				p.resolveStmts(tt, ir.BlockStmts(eb.Then))
			}
			if s.Else != nil {
				p.resolveStmts(tt, ir.BlockStmts(s.Else))
			}
		case *ir.SwitchStmt:
			p.resolveExpr(tt, s.Expr)
			for _, cs := range s.Cases {
				for _, ce := range cs.Values {
					p.resolveExpr(tt, ce)
				}
				p.resolveStmts(tt, cs.Stmts)
			}
			if s.Default != nil {
				p.resolveStmts(tt, s.Default)
			}
		case *ir.ReturnStmt:
			for _, result := range s.Results {
				p.resolveExpr(tt, result)
			}
		case *ir.GuardStmt:
			p.resolveExpr(tt, s.Cond)
			for _, ret := range s.Returns {
				p.resolveExpr(tt, ret)
			}
		case *ir.ForStmt:
			if s.CondInt != nil && s.CondInt.Init != nil {
				for i, target := range s.CondInt.Init.Targets {
					if target.TypeAnn != nil {
						s.CondInt.Init.Targets[i].TypeAnn.Typ = p.resolveTypeRef(tt, target.TypeAnn)
					}
				}

				if s.CondInt.Init.Init != nil {
					p.resolveExpr(tt, s.CondInt.Init.Init)
				}
			} else if s.CondIn != nil {
				p.resolveExpr(tt, s.CondIn.IterExpr)
			} else if s.CondRange != nil {
				p.resolveExpr(tt, s.CondRange.RangeStart)
				p.resolveExpr(tt, s.CondRange.RangeEnd)
			}

			p.resolveStmts(tt, ir.BlockStmts(s.Body))
		case *ir.WhileStmt:
			p.resolveExpr(tt, s.Cond)
			p.resolveStmts(tt, ir.BlockStmts(s.Body))
		case *ir.TestDeclStmt:
			p.resolveStmts(tt, ir.BlockStmts(s.Body))
		case *ir.GoStmt:
			if s.Call != nil {
				p.resolveExpr(tt, s.Call)
			}
			p.resolveStmts(tt, ir.BlockStmts(s.Body))
		case *ir.DeferStmt:
			if s.Call != nil {
				p.resolveExpr(tt, s.Call)
			}
			p.resolveStmts(tt, ir.BlockStmts(s.Body))
		case *ir.SelectStmt:
			for _, cc := range s.Cases {
				if cc.ChanExpr != nil {
					p.resolveExpr(tt, cc.ChanExpr)
				}
				if cc.SendValue != nil {
					p.resolveExpr(tt, cc.SendValue)
				}
				for i := range cc.Targets {
					if cc.Targets[i].TypeAnn != nil {
						cc.Targets[i].TypeAnn.Typ = p.resolveTypeRef(tt, cc.Targets[i].TypeAnn)
					}
				}
				p.resolveStmts(tt, ir.BlockStmts(cc.Body))
			}
			p.resolveStmts(tt, ir.BlockStmts(s.Default))
		}
	}
}

func (p *PassResolveTypeRefs) resolveExpr(tt *ir.TypeTable, expr ir.Expr) {
	if ir.IsNilExpr(expr) {
		return
	}
	switch x := expr.(type) {
	case *ir.WhenExpr:
		p.resolveExpr(tt, x.Expr)
		for _, c := range x.Cases {
			for _, v := range c.Values {
				p.resolveExpr(tt, v)
			}
			p.resolveExpr(tt, c.Then)
		}
		if x.Default != nil {
			p.resolveExpr(tt, x.Default)
		}
	case *ir.UnaryExpr:
		p.resolveExpr(tt, x.Expr)
	case *ir.PrefixUnaryExpr:
		p.resolveExpr(tt, x.Expr)
	case *ir.PostfixUnaryExpr:
		p.resolveExpr(tt, x.Expr)
	case *ir.BinaryExpr:
		p.resolveExpr(tt, x.Left)
		p.resolveExpr(tt, x.Right)
	case *ir.TenaryExpr:
		p.resolveExpr(tt, x.Cond)
		p.resolveExpr(tt, x.Then)
		p.resolveExpr(tt, x.Else)
	case *ir.GroupedExpr:
		p.resolveExpr(tt, x.Expr)
	case *ir.AsExpr:
		if x.Target != nil {
			x.Target.Typ = p.resolveTypeRef(tt, x.Target)
		}
		p.resolveExpr(tt, x.Expr)
	case *ir.AssignmentExpr:
		p.resolveExpr(tt, x.Right)
	case *ir.IndexExpr:
		p.resolveExpr(tt, x.Expr)
		p.resolveExpr(tt, x.Index)
	case *ir.FieldAccessExpr:
		p.resolveExpr(tt, x.Expr)
	case *ir.RangeExpr:
		p.resolveExpr(tt, x.Start)
		p.resolveExpr(tt, x.End)

		if x.Inc != nil {
			p.resolveExpr(tt, x.Inc)
		}
	case *ir.FuncCallExpr:
		p.resolveExpr(tt, x.Callee)
		for _, arg := range x.Args {
			p.resolveExpr(tt, arg.Expr)
		}
	case *ir.ComposableCallExpr:
		p.resolveExpr(tt, x.Callee)
		for _, arg := range x.Args {
			p.resolveExpr(tt, arg.Expr)
		}
		for _, child := range x.Children {
			if child.Expr != nil {
				p.resolveExpr(tt, child.Expr)
			}
			if child.Stmt != nil {
				p.resolveStmts(tt, []ir.Stmt{child.Stmt})
			}
		}
	case *ir.FuncLitExpr:
		if x.ReturnType != nil {
			x.ReturnType.Typ = p.resolveTypeRef(tt, x.ReturnType)
		}

		for _, param := range x.Params {
			if param.Type != nil {
				param.Type.Typ = p.resolveTypeRef(tt, param.Type)
			}

			if param.Default != nil {
				p.resolveExpr(tt, param.Default)
			}
		}

		p.resolveStmts(tt, ir.BlockStmts(x.Body))
	case *ir.CoalesceExpr:
		if x.Left != nil {
			p.resolveExpr(tt, x.Left)
		}
		if x.Default != nil {
			p.resolveExpr(tt, x.Default)
		}
	case *ir.ArrayLiteral:
		for _, elem := range x.Elems {
			p.resolveExpr(tt, elem)
		}
	case *ir.MapLiteral:
		for _, kv := range x.Entries {
			p.resolveExpr(tt, kv.Key)
			p.resolveExpr(tt, kv.Value)
		}
	case *ir.TupleLiteral:
		for _, elem := range x.Elems {
			p.resolveExpr(tt, elem)
		}
	case *ir.StringTemplateExpr:
		for _, part := range x.Parts {
			if part.Expr != nil {
				p.resolveExpr(tt, part.Expr)
			}
		}
	case *ir.NewExpr:
		for _, ta := range x.TypeArgs {
			ta.Typ = p.resolveTypeRef(tt, ta)
		}
		for _, arg := range x.Args {
			p.resolveExpr(tt, arg.Expr)
		}
	case *ir.ChanInitExpr:
		if x.ElemType != nil {
			x.ElemType.Typ = p.resolveTypeRef(tt, x.ElemType)
		}
		if x.Capacity != nil {
			p.resolveExpr(tt, x.Capacity)
		}
	}
}

func (p *PassResolveTypeRefs) resolveTypeRef(tt *ir.TypeTable, tr *ir.TypeRef) ir.TypID {
	if tr.Typ != 0 {
		return tr.Typ
	}
	switch tr.Kind {
	case ir.TK_PrimitiveAny:
		return tt.PrimAny()
	case ir.TK_PrimitiveInt:
		return tt.PrimInt()
	case ir.TK_PrimitiveFloat:
		return tt.PrimFloat()
	case ir.TK_PrimitiveBool:
		return tt.PrimBool()
	case ir.TK_PrimitiveString:
		return tt.PrimString()
	case ir.TK_PrimitiveChar:
		return tt.PrimChar()
	case ir.TK_PrimitiveByte:
		return tt.PrimByte()
	case ir.TK_Option:
		et := p.resolveTypeRef(tt, tr.Elem)
		return tt.DeclareType(ir.OptionType(et))
	case ir.TK_Slice:
		et := p.resolveTypeRef(tt, tr.Elem)
		return tt.DeclareType(ir.SliceType(et))
	case ir.TK_Chan:
		et := p.resolveTypeRef(tt, tr.Elem)
		return tt.ChanOf(et)
	case ir.TK_Array:
		et := p.resolveTypeRef(tt, tr.Elem)
		return tt.DeclareType(ir.ArrayType(et, tr.Dim))
	case ir.TK_Map:
		kt := p.resolveTypeRef(tt, tr.Key)
		vt := p.resolveTypeRef(tt, tr.Value)
		return tt.DeclareType(ir.MapType(kt, vt))
	case ir.TK_Tuple:
		fields := make([]ir.TupleField, len(tr.Tuple))
		for i, f := range tr.Tuple {
			fields[i] = ir.TupleField{Name: f.Name, Type: p.resolveTypeRef(tt, f.Type)}
		}
		return tt.DeclareType(ir.TupleType(fields...))
	case ir.TK_Function:
		params := make([]*ir.FuncParam, 0, len(tr.FuncParams))
		for _, p2 := range tr.FuncParams {
			if p2.Type == nil {
				continue
			}
			paramTyp := p.resolveTypeRef(tt, p2.Type)
			params = append(params, &ir.FuncParam{
				Name: ir.NameRef{Name: p2.Name},
				Type: &ir.TypeRef{Typ: paramTyp},
			})
		}
		retTyp := tt.TypNone()
		if tr.FuncReturn != nil {
			retTyp = p.resolveTypeRef(tt, tr.FuncReturn)
		}
		return tt.FuncOf(params, retTyp)
	case ir.TK_Enum:
		if tr.CustomName == "" {
			return 0
		}
		for _, argRef := range tr.TypeArgs {
			argRef.Typ = p.resolveTypeRef(tt, argRef)
		}
		if tr.CustomQualifier == "" {
			if id, ok := p.genericParamMap[tr.CustomName]; ok {
				tr.Kind = ir.TK_TypeParam
				return id
			}
		}
		lookup := tt
		lookupPkg := p.currentPkgPath
		if tr.CustomQualifier != "" {
			pkg, ok := p.imports[tr.CustomQualifier]
			if !ok {
				return 0
			}
			lookup = pkg.Types
			lookupPkg = pkg.Path.String()
		}
		if id := p.resolveAlias(tt, lookupPkg, tr.CustomName); id != 0 {
			return id
		}
		for _, kind := range []string{"struct", "interface", "enum"} {
			if id, ok := lookup.GetByType(ir.TypeKey(kind + ":" + lookupPkg + ":" + tr.CustomName)); ok {
				return id
			}
			if id, ok := lookup.GetByType(ir.TypeKey(kind + "::" + tr.CustomName)); ok {
				return id
			}
		}
		return 0
	case ir.TK_TypeParam:
		if tr.CustomName == "?" {
			return tt.PrimAny()
		}
		if id, ok := p.genericParamMap[tr.CustomName]; ok {
			return id
		}
		return 0
	default:
		return 0
	}
}

// resolveAlias looks up `name` in the alias table for `pkgPath` and recursively resolves the alias's target type. The result is cached on the alias's Target TypeRef so re-resolution returns the same TypID. Returns 0 when no alias exists or the cycle guard fires.
func (p *PassResolveTypeRefs) resolveAlias(tt *ir.TypeTable, pkgPath, name string) ir.TypID {
	pkgAliases := p.aliases[pkgPath]
	if pkgAliases == nil {
		return 0
	}
	alias := pkgAliases[name]
	if alias == nil || alias.Target == nil {
		return 0
	}
	if alias.Target.Typ != 0 {
		tt.RegisterAlias(pkgPath, name, alias.Target.Typ)
		return alias.Target.Typ
	}
	guard := pkgPath + ":" + name
	if p.aliasResolving[guard] {
		return 0
	}
	p.aliasResolving[guard] = true
	defer delete(p.aliasResolving, guard)
	prevPkg := p.currentPkgPath
	p.currentPkgPath = pkgPath
	resolved := p.resolveTypeRef(tt, alias.Target)
	p.currentPkgPath = prevPkg
	if resolved != 0 {
		alias.Target.Typ = resolved
		tt.RegisterAlias(pkgPath, name, resolved)
	}
	return resolved
}
