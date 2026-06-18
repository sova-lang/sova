package passes

import (
	"sova/internal/diag"
	"sova/internal/ir"
)

// PassResolveNames is a pass that resolves names in the code. All usages of a name get transformed and resolved through symbol IDs for later simple renaming/mangling.
// It also handles scope resolution for symbols and checks if the usages are valid.
type PassResolveNames struct {
	currentFile *ir.PreparsedFile
}

func (p *PassResolveNames) Name() string       { return "resolve_names" }
func (p *PassResolveNames) Scope() PassScope   { return PerPackage }
func (p *PassResolveNames) Requires() []string { return []string{"bind_declare"} }
func (p *PassResolveNames) NoErrors() bool     { return false }

func (p *PassResolveNames) Run(pc *PassContext) error {
	pkg := pc.Pkg
	for _, f := range pkg.Files {
		if f.Hir.Side.Kind == ir.SideSynth {
			continue
		}
		p.currentFile = f
		for _, st := range f.Hir.Statements {
			p.resolveStmtNames(pc, st, pkg)
		}
	}
	p.currentFile = nil
	return nil
}

// resolveViaUsing looks up an unqualified name across the current file's `using *` and `using { ... }` imports. When the name matches exactly one exposed export from the imports, the function returns the target package and the resolved sym in that package's arena. When no match or multiple matches exist, ok is false. Package-private names (`_foo`) are skipped because they aren't visible outside their declaring package.
func (p *PassResolveNames) resolveViaUsing(pc *PassContext, name string) (*ir.PackageContext, ir.SymID, string, bool) {
	if p.currentFile == nil || p.currentFile.Hir == nil {
		return nil, 0, "", false
	}
	if isPackagePrivateName(name) {
		return nil, 0, "", false
	}
	var matches []struct {
		Pkg   *ir.PackageContext
		Sym   ir.SymID
		Alias string
	}
	for _, st := range p.currentFile.Hir.Statements {
		imp, ok := st.(*ir.ImportStmt)
		if !ok {
			continue
		}
		if !imp.UsingAll && !containsString(imp.UsingList, name) {
			continue
		}
		target := findPackageByPath(pc, imp.Path.String())
		if target == nil {
			continue
		}
		if sym, ok := target.Scopes.LookupOnlyCurrent(target.Root, name); ok {
			matches = append(matches, struct {
				Pkg   *ir.PackageContext
				Sym   ir.SymID
				Alias string
			}{Pkg: target, Sym: sym, Alias: imp.Alias})
		}
	}
	if len(matches) == 1 {
		return matches[0].Pkg, matches[0].Sym, matches[0].Alias, true
	}
	return nil, 0, "", false
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// isPackagePrivateName reports whether `name` follows Sova's package-private convention: a single leading underscore (`_foo`, `_bar`). Names with a double-underscore prefix (`__mountInto`, `__strixOwnsChildren`) are framework-internal helpers that intentionally cross package boundaries and are exempt. Plain identifiers without a leading underscore are public and equally exempt.
func isPackagePrivateName(name string) bool {
	if len(name) == 0 || name[0] != '_' {
		return false
	}
	if len(name) >= 2 && name[1] == '_' {
		return false
	}
	return true
}

// resolveAnnotations walks each annotation's arg expressions through the normal name-resolution path so that const references inside annotations get their Sym set before the folding pass.
func (p *PassResolveNames) resolveAnnotations(pc *PassContext, annos []ir.Annotation, pkg *ir.PackageContext) {
	for i := range annos {
		for _, arg := range annos[i].Args {
			p.resolveExprNames(pc, arg, pkg)
		}
	}
}

func (p *PassResolveNames) resolveStmtNames(pc *PassContext, s ir.Stmt, pkg *ir.PackageContext) {
	if ir.IsNilStmt(s) {
		return
	}
	switch v := s.(type) {
	case *ir.BlockStmt:
		for _, st := range v.Stmts {
			p.resolveStmtNames(pc, st, pkg)
		}
	case *ir.VarDeclStmt:
		if v.Init != nil {
			p.resolveExprNames(pc, v.Init, pkg)
		}
	case *ir.FuncDeclStmt:
		p.resolveAnnotations(pc, v.Annotations, pkg)
		for _, param := range v.Params {
			if param.Default != nil {
				p.resolveExprNames(pc, param.Default, pkg)
			}
		}

		p.resolveStmtNames(pc, v.Body, pkg)
	case *ir.ExprStmt:
		p.resolveExprNames(pc, v.Expr, pkg)
	case *ir.FieldAssignmentStmt:
		scope, _ := pkg.Scopes.EnclosingScope(v.ID())
		if id, ok := pkg.Scopes.Lookup(scope, v.Receiver.Name); ok {
			v.Receiver.Sym = id
		} else {
			pc.Diag.Report(diag.ErrUndeclaredSymbol, v.Receiver.Span, v.Receiver.Name)
		}
		p.resolveExprNames(pc, v.Value, pkg)
	case *ir.IndexAssignmentStmt:
		p.resolveExprNames(pc, v.Receiver, pkg)
		p.resolveExprNames(pc, v.Index, pkg)
		p.resolveExprNames(pc, v.Value, pkg)
	case *ir.MultiAssignmentStmt:
		scope, _ := pkg.Scopes.EnclosingScope(v.ID())
		for i := range v.Targets {
			target := &v.Targets[i]
			if target.Name == nil {
				continue
			}
			p.resolveNameRef(pc, target.Name, scope, pkg)
		}
		p.resolveExprNames(pc, v.Value, pkg)
	case *ir.IfStmt:
		p.resolveExprNames(pc, v.Cond, pkg)
		p.resolveStmtNames(pc, v.Then, pkg)
		for _, ei := range v.ElseIfs {
			p.resolveExprNames(pc, ei.Cond, pkg)
			p.resolveStmtNames(pc, ei.Then, pkg)
		}
		if v.Else != nil {
			p.resolveStmtNames(pc, v.Else, pkg)
		}
	case *ir.SwitchStmt:
		p.resolveExprNames(pc, v.Expr, pkg)
		for _, cs := range v.Cases {
			for _, ce := range cs.Values {
				p.resolveExprNames(pc, ce, pkg)
			}
			for _, st := range cs.Stmts {
				p.resolveStmtNames(pc, st, pkg)
			}
		}
		if v.Default != nil {
			for _, st := range v.Default {
				p.resolveStmtNames(pc, st, pkg)
			}
		}
	case *ir.ReturnStmt:
		for _, result := range v.Results {
			p.resolveExprNames(pc, result, pkg)
		}
	case *ir.GuardStmt:
		p.resolveExprNames(pc, v.Cond, pkg)
		for _, ret := range v.Returns {
			p.resolveExprNames(pc, ret, pkg)
		}
	case *ir.ForStmt:
		if v.CondInt != nil && v.CondInt.Init != nil && v.CondInt.Init.Init != nil {
			p.resolveExprNames(pc, v.CondInt.Init.Init, pkg)
		} else if v.CondIn != nil {
			p.resolveExprNames(pc, v.CondIn.IterExpr, pkg)
		} else if v.CondRange != nil {
			p.resolveExprNames(pc, v.CondRange.RangeStart, pkg)
			p.resolveExprNames(pc, v.CondRange.RangeEnd, pkg)
		}

		p.resolveStmtNames(pc, v.Body, pkg)
	case *ir.WhileStmt:
		p.resolveExprNames(pc, v.Cond, pkg)
		p.resolveStmtNames(pc, v.Body, pkg)
	case *ir.TestDeclStmt:
		if v.Body != nil {
			p.resolveStmtNames(pc, v.Body, pkg)
		}
	case *ir.GroupDeclStmt:
		for _, st := range v.Body {
			p.resolveStmtNames(pc, st, pkg)
		}
	case *ir.SetupStmt:
		if v.Body != nil {
			p.resolveStmtNames(pc, v.Body, pkg)
		}
	case *ir.TeardownStmt:
		if v.Body != nil {
			p.resolveStmtNames(pc, v.Body, pkg)
		}
	case *ir.AssertStmt:
		p.resolveExprNames(pc, v.Expr, pkg)
	case *ir.AsSessionStmt:
		if v.Body != nil {
			p.resolveStmtNames(pc, v.Body, pkg)
		}
	case *ir.GoStmt:
		if v.Body != nil {
			p.resolveStmtNames(pc, v.Body, pkg)
		}
		if v.Call != nil {
			p.resolveExprNames(pc, v.Call, pkg)
		}
	case *ir.DeferStmt:
		if v.Body != nil {
			p.resolveStmtNames(pc, v.Body, pkg)
		}
		if v.Call != nil {
			p.resolveExprNames(pc, v.Call, pkg)
		}
	case *ir.SelectStmt:
		for _, cc := range v.Cases {
			if cc.ChanExpr != nil {
				p.resolveExprNames(pc, cc.ChanExpr, pkg)
			}
			if cc.SendValue != nil {
				p.resolveExprNames(pc, cc.SendValue, pkg)
			}
			if cc.Body != nil {
				p.resolveStmtNames(pc, cc.Body, pkg)
			}
		}
		if v.Default != nil {
			p.resolveStmtNames(pc, v.Default, pkg)
		}
	case *ir.TypeDeclStmt:
		scope, _ := pkg.Scopes.EnclosingScope(v.ID())
		p.resolveAnnotations(pc, v.Annotations, pkg)
		for i := range v.Implements {
			if id, ok := pkg.Scopes.Lookup(scope, v.Implements[i].Name); ok {
				v.Implements[i].Sym = id
			} else {
				pc.Diag.Report(diag.ErrUndeclaredSymbol, v.Implements[i].Span, v.Implements[i].Name)
			}
		}
		for i := range v.MixedIn {
			ref := &v.MixedIn[i]
			if ref.Qualifier == "" {
				if id, ok := pkg.Scopes.Lookup(scope, ref.Name); ok {
					ref.Sym = id
					continue
				}
				if _, sym, alias, ok := p.resolveViaUsing(pc, ref.Name); ok {
					ref.Sym = sym
					ref.Qualifier = alias
				}
				continue
			}
			qSym, ok := pkg.Scopes.Lookup(scope, ref.Qualifier)
			if !ok {
				continue
			}
			pkgSym, ok := pkg.Syms.GetByID(qSym)
			if !ok || pkgSym.Kind != ir.SK_Package {
				continue
			}
			target := findPackageByPath(pc, pkgSym.PackagePath)
			if target == nil {
				continue
			}
			if memberSym, found := target.Scopes.LookupOnlyCurrent(target.Root, ref.Name); found {
				ref.Sym = memberSym
			}
		}
		for _, field := range v.Fields {
			p.resolveAnnotations(pc, field.Annotations, pkg)
			if field.Default != nil {
				p.resolveExprNames(pc, field.Default, pkg)
			}
		}
		for _, method := range v.Methods {
			p.resolveAnnotations(pc, method.Annotations, pkg)
		}
		for _, ctor := range v.Ctors {
			p.resolveAnnotations(pc, ctor.Annotations, pkg)
		}
		for _, ctor := range v.Ctors {
			for _, param := range ctor.Params {
				if param.Default != nil {
					p.resolveExprNames(pc, param.Default, pkg)
				}
			}
			if ctor.Body != nil {
				p.resolveStmtNames(pc, ctor.Body, pkg)
			}
		}
		for _, cast := range v.Casts {
			p.resolveAnnotations(pc, cast.Annotations, pkg)
			if cast.Body != nil {
				p.resolveStmtNames(pc, cast.Body, pkg)
			}
		}
		for _, method := range v.Methods {
			fn := method.Func
			for _, param := range fn.Params {
				if param.Default != nil {
					p.resolveExprNames(pc, param.Default, pkg)
				}
			}
			if fn.Body != nil {
				p.resolveStmtNames(pc, fn.Body, pkg)
			}
		}
	case *ir.ExternDeclStmt:
		for _, t := range v.Types {
			p.resolveStmtNames(pc, t, pkg)
		}
		for _, iface := range v.Interfaces {
			p.resolveStmtNames(pc, iface, pkg)
		}
	case *ir.EnumDeclStmt:
		// Resolve field default expressions
		for _, field := range v.Fields {
			if field.Default != nil {
				p.resolveExprNames(pc, field.Default, pkg)
			}
		}

		// Resolve case argument expressions
		for _, c := range v.Cases {
			for _, arg := range c.Args {
				p.resolveExprNames(pc, arg, pkg)
			}
		}

		// Resolve method bodies
		for _, method := range v.Methods {
			for _, param := range method.Params {
				if param.Default != nil {
					p.resolveExprNames(pc, param.Default, pkg)
				}
			}
			p.resolveStmtNames(pc, method.Body, pkg)
		}
	}
}

func (p *PassResolveNames) resolveExprNames(pc *PassContext, expr ir.Expr, pkg *ir.PackageContext) {
	if ir.IsNilExpr(expr) {
		return
	}

	start, _ := pkg.Scopes.EnclosingScope(expr.ID())
	switch x := expr.(type) {
	case *ir.WhenExpr:
		p.resolveExprNames(pc, x.Expr, pkg)
		for _, c := range x.Cases {
			for _, v := range c.Values {
				p.resolveExprNames(pc, v, pkg)
			}
			p.resolveExprNames(pc, c.Then, pkg)
		}
		if x.Default != nil {
			p.resolveExprNames(pc, x.Default, pkg)
		}
	case *ir.UnaryExpr:
		p.resolveExprNames(pc, x.Expr, pkg)
	case *ir.PrefixUnaryExpr:
		p.resolveExprNames(pc, x.Expr, pkg)
	case *ir.PostfixUnaryExpr:
		p.resolveExprNames(pc, x.Expr, pkg)
	case *ir.BinaryExpr:
		p.resolveExprNames(pc, x.Left, pkg)
		p.resolveExprNames(pc, x.Right, pkg)
	case *ir.CoalesceExpr:
		if x.Left != nil {
			p.resolveExprNames(pc, x.Left, pkg)
		}
		if x.Default != nil {
			p.resolveExprNames(pc, x.Default, pkg)
		}
	case *ir.TenaryExpr:
		p.resolveExprNames(pc, x.Cond, pkg)
		p.resolveExprNames(pc, x.Then, pkg)
		p.resolveExprNames(pc, x.Else, pkg)
	case *ir.GroupedExpr:
		p.resolveExprNames(pc, x.Expr, pkg)
	case *ir.AsExpr:
		p.resolveExprNames(pc, x.Expr, pkg)
	case *ir.InstanceofExpr:
		p.resolveExprNames(pc, x.Expr, pkg)
	case *ir.OptionUnwrapExpr:
		p.resolveExprNames(pc, x.Expr, pkg)
	case *ir.AssignmentExpr:
		if id, ok := pkg.Scopes.Lookup(start, x.Left.Name); ok {
			x.Left.Sym = id
		} else {
			pc.Diag.Report(diag.ErrUndeclaredSymbol, x.Left.Span, x.Left.Name)
		}
		p.resolveExprNames(pc, x.Right, pkg)
	case *ir.IndexExpr:
		p.resolveExprNames(pc, x.Expr, pkg)
		p.resolveExprNames(pc, x.Index, pkg)
	case *ir.SliceRangeExpr:
		p.resolveExprNames(pc, x.Expr, pkg)
		if x.Low != nil {
			p.resolveExprNames(pc, x.Low, pkg)
		}
		if x.High != nil {
			p.resolveExprNames(pc, x.High, pkg)
		}
	case *ir.FieldAccessExpr:
		p.resolveExprNames(pc, x.Expr, pkg)
	case *ir.VarRef:
		if id, ok := pkg.Scopes.Lookup(start, x.Ref.Name); ok {
			x.Ref.Sym = id
		} else if _, sym, alias, ok := p.resolveViaUsing(pc, x.Ref.Name); ok {
			x.Ref.Sym = sym
			x.Ref.Qualifier = alias
		} else {
			pc.Diag.Report(diag.ErrUndeclaredSymbol, x.Ref.Span, x.Ref.Name)
		}
	case *ir.RangeExpr:
		p.resolveExprNames(pc, x.Start, pkg)
		p.resolveExprNames(pc, x.End, pkg)

		if x.Inc != nil {
			p.resolveExprNames(pc, x.Inc, pkg)
		}
	case *ir.FuncCallExpr:
		p.resolveExprNames(pc, x.Callee, pkg)
		for _, arg := range x.Args {
			p.resolveExprNames(pc, arg.Expr, pkg)
		}
	case *ir.ComposableCallExpr:
		p.resolveExprNames(pc, x.Callee, pkg)
		for _, arg := range x.Args {
			p.resolveExprNames(pc, arg.Expr, pkg)
		}
		for _, child := range x.Children {
			if child.Expr != nil {
				p.resolveExprNames(pc, child.Expr, pkg)
			}
			if child.Stmt != nil {
				p.resolveStmtNames(pc, child.Stmt, pkg)
			}
		}
	case *ir.FuncLitExpr:
		for _, param := range x.Params {
			scope, _ := pkg.Scopes.EnclosingScope(param.ID())
			p.resolveNameRef(pc, &param.Name, scope, pkg)

			if param.Default != nil {
				p.resolveExprNames(pc, param.Default, pkg)
			}
		}

		p.resolveStmtNames(pc, x.Body, pkg)
	case *ir.ArrayLiteral:
		for _, el := range x.Elems {
			p.resolveExprNames(pc, el, pkg)
		}
	case *ir.MapLiteral:
		for _, en := range x.Entries {
			p.resolveExprNames(pc, en.Key, pkg)
			p.resolveExprNames(pc, en.Value, pkg)
		}
	case *ir.TupleLiteral:
		for _, el := range x.Elems {
			p.resolveExprNames(pc, el, pkg)
		}
	case *ir.StringTemplateExpr:
		for _, part := range x.Parts {
			if part.Expr != nil {
				p.resolveExprNames(pc, part.Expr, pkg)
			}
		}
	case *ir.NewExpr:
		scope, _ := pkg.Scopes.EnclosingScope(x.ID())
		if x.Qualifier != "" {
			qSym, ok := pkg.Scopes.Lookup(scope, x.Qualifier)
			if !ok {
				pc.Diag.Report(diag.ErrUndeclaredSymbol, x.TypeName.Span, x.Qualifier)
			} else if pkgSym, found := pkg.Syms.GetByID(qSym); !found || pkgSym.Kind != ir.SK_Package {
				pc.Diag.Report(diag.ErrUndeclaredSymbol, x.TypeName.Span, x.Qualifier+"."+x.TypeName.Name)
			} else if targetPkg := findPackageByPath(pc, pkgSym.PackagePath); targetPkg == nil {
				pc.Diag.Report(diag.ErrUndeclaredSymbol, x.TypeName.Span, pkgSym.PackagePath)
			} else if memberSym, found := targetPkg.Scopes.LookupOnlyCurrent(targetPkg.Root, x.TypeName.Name); !found {
				pc.Diag.Report(diag.ErrUndeclaredSymbol, x.TypeName.Span, x.Qualifier+"."+x.TypeName.Name)
			} else if isPackagePrivateName(x.TypeName.Name) && targetPkg != pkg {
				pc.Diag.Report(diag.ErrPrivateSymbolAccess, x.TypeName.Span, x.TypeName.Name, pkgSym.PackagePath)
			} else {
				x.TypeName.Sym = memberSym
			}
		} else if id, ok := pkg.Scopes.Lookup(scope, x.TypeName.Name); ok {
			x.TypeName.Sym = id
		} else if _, sym, alias, ok := p.resolveViaUsing(pc, x.TypeName.Name); ok {
			x.TypeName.Sym = sym
			x.Qualifier = alias
		} else {
			pc.Diag.Report(diag.ErrUndeclaredSymbol, x.TypeName.Span, x.TypeName.Name)
		}
		for i := range x.Args {
			p.resolveExprNames(pc, x.Args[i].Expr, pkg)
		}
	default:
		// Unhandled expression type - this shouldn't happen
		// If we see this, it means we forgot to add a case for a new expression type
	}
}

func (p *PassResolveNames) resolveNameRef(pc *PassContext, nameRef *ir.NameRef, start ir.ScopeID, pkg *ir.PackageContext) {
	if id, ok := pkg.Scopes.Lookup(start, nameRef.Name); ok {
		nameRef.Sym = id
	} else {
		pc.Diag.Report(diag.ErrUndeclaredSymbol, nameRef.Span, nameRef.Name)
	}
}
