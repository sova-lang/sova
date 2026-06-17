package passes

import "sova/internal/ir"

// clearUnused marks a symbol as used in whichever package owns it. Cross-package references show up only after import resolution, so we search all packages.
func clearUnused(pc *PassContext, sym ir.SymID) {
	if sym == 0 {
		return
	}
	for _, pkg := range pc.Pkgs {
		if s, ok := pkg.Syms.GetByID(sym); ok {
			s.Flags &^= ir.SF_Unused
			return
		}
	}
}

type PassDetectUnused struct{}

func (p *PassDetectUnused) Name() string       { return "detect_unused" }
func (p *PassDetectUnused) Scope() PassScope   { return PerPackage }
func (p *PassDetectUnused) Requires() []string { return []string{"mangle"} }
func (p *PassDetectUnused) NoErrors() bool     { return true }

func (p *PassDetectUnused) Run(pc *PassContext) error {
	for _, f := range pc.Pkg.Files {
		for _, st := range f.Hir.Statements {
			p.markUnused(pc, st)
		}
	}

	for _, f := range pc.Pkg.Files {
		for _, st := range f.Hir.Statements {
			p.trackUsage(pc, st)
		}
	}

	return nil
}

func (p *PassDetectUnused) markUnused(pc *PassContext, st ir.Stmt) {
	switch s := st.(type) {
	case *ir.BlockStmt:
		for _, stmt := range s.Stmts {
			p.markUnused(pc, stmt)
		}
	case *ir.VarDeclStmt:
		for _, target := range s.Targets {
			if target.Name != nil {
				if sym, ok := pc.Pkg.Syms.GetByID(target.Name.Sym); ok {
					sym.Flags |= ir.SF_Unused
				}
			}
		}
	case *ir.FuncDeclStmt:
		if !s.IsWired {
			for _, param := range s.Params {
				if sym, ok := pc.Pkg.Syms.GetByID(param.Name.Sym); ok {
					sym.Flags |= ir.SF_Unused
				}
			}
		}
		p.markUnused(pc, s.Body)
	case *ir.IfStmt:
		p.markUnused(pc, s.Then)
		for _, elif := range s.ElseIfs {
			p.markUnused(pc, elif.Then)
		}
		if s.Else != nil {
			p.markUnused(pc, s.Else)
		}
	case *ir.SwitchStmt:
		for _, c := range s.Cases {
			for _, stmt := range c.Stmts {
				p.markUnused(pc, stmt)
			}
		}
		for _, stmt := range s.Default {
			p.markUnused(pc, stmt)
		}
	case *ir.ForStmt:
		if s.CondInt != nil && s.CondInt.Init != nil && len(s.CondInt.Init.Targets) > 0 {
			target := &s.CondInt.Init.Targets[0]
			if target.Name != nil {
				if sym, ok := pc.Pkg.Syms.GetByID(target.Name.Sym); ok {
					sym.Flags |= ir.SF_Unused
				}
			}
		} else if s.CondIn != nil {
			if sym, ok := pc.Pkg.Syms.GetByID(s.CondIn.InFirstVar.Sym); ok {
				sym.Flags |= ir.SF_Unused
			}
			if s.CondIn.InSecondVar != nil {
				if sym, ok := pc.Pkg.Syms.GetByID(s.CondIn.InSecondVar.Sym); ok {
					sym.Flags |= ir.SF_Unused
				}
			}
			if s.CondIn.InThirdVar != nil {
				if sym, ok := pc.Pkg.Syms.GetByID(s.CondIn.InThirdVar.Sym); ok {
					sym.Flags |= ir.SF_Unused
				}
			}
		} else if s.CondRange != nil {
			if sym, ok := pc.Pkg.Syms.GetByID(s.CondRange.RangeVar.Sym); ok {
				sym.Flags |= ir.SF_Unused
			}
		}
		p.markUnused(pc, s.Body)
	case *ir.WhileStmt:
		p.markUnused(pc, s.Body)
	}
}

func (p *PassDetectUnused) trackUsage(pc *PassContext, st ir.Stmt) {
	switch s := st.(type) {
	case *ir.BlockStmt:
		for _, stmt := range s.Stmts {
			p.trackUsage(pc, stmt)
		}
	case *ir.VarDeclStmt:
		if s.Init != nil {
			p.trackUsageExpr(pc, s.Init)
		}
	case *ir.FuncDeclStmt:
		for _, param := range s.Params {
			if param.Default != nil {
				p.trackUsageExpr(pc, param.Default)
			}
		}
		p.trackUsage(pc, s.Body)
	case *ir.ExprStmt:
		p.trackUsageExpr(pc, s.Expr)
	case *ir.FieldAssignmentStmt:
		if sym, ok := pc.Pkg.Syms.GetByID(s.Receiver.Sym); ok {
			sym.Flags &^= ir.SF_Unused
		}
		p.trackUsageExpr(pc, s.Value)
	case *ir.MultiAssignmentStmt:
		p.trackUsageExpr(pc, s.Value)
		for _, target := range s.Targets {
			if target.Name != nil {
				if sym, ok := pc.Pkg.Syms.GetByID(target.Name.Sym); ok {
					sym.Flags &^= ir.SF_Unused
				}
			}
		}
	case *ir.IfStmt:
		p.trackUsageExpr(pc, s.Cond)
		p.trackUsage(pc, s.Then)
		for _, elif := range s.ElseIfs {
			p.trackUsageExpr(pc, elif.Cond)
			p.trackUsage(pc, elif.Then)
		}
		if s.Else != nil {
			p.trackUsage(pc, s.Else)
		}
	case *ir.SwitchStmt:
		p.trackUsageExpr(pc, s.Expr)
		for _, c := range s.Cases {
			for _, v := range c.Values {
				p.trackUsageExpr(pc, v)
			}
			for _, stmt := range c.Stmts {
				p.trackUsage(pc, stmt)
			}
		}
		for _, stmt := range s.Default {
			p.trackUsage(pc, stmt)
		}
	case *ir.ReturnStmt:
		for _, result := range s.Results {
			p.trackUsageExpr(pc, result)
		}
	case *ir.GuardStmt:
		p.trackUsageExpr(pc, s.Cond)
		for _, ret := range s.Returns {
			p.trackUsageExpr(pc, ret)
		}
	case *ir.ForStmt:
		if s.CondInt != nil {
			if s.CondInt.Init != nil && s.CondInt.Init.Init != nil {
				p.trackUsageExpr(pc, s.CondInt.Init.Init)
			}
			if s.CondInt.Cond != nil {
				p.trackUsageExpr(pc, s.CondInt.Cond)
			}
			if s.CondInt.Post != nil {
				p.trackUsageExpr(pc, s.CondInt.Post)
			}
		} else if s.CondIn != nil {
			p.trackUsageExpr(pc, s.CondIn.IterExpr)
		} else if s.CondRange != nil {
			p.trackUsageExpr(pc, s.CondRange.RangeStart)
			p.trackUsageExpr(pc, s.CondRange.RangeEnd)
		}
		p.trackUsage(pc, s.Body)
	case *ir.WhileStmt:
		p.trackUsageExpr(pc, s.Cond)
		p.trackUsage(pc, s.Body)
	case *ir.GoStmt:
		if s.Call != nil {
			p.trackUsageExpr(pc, s.Call)
		}
		if s.Body != nil {
			p.trackUsage(pc, s.Body)
		}
	case *ir.DeferStmt:
		if s.Call != nil {
			p.trackUsageExpr(pc, s.Call)
		}
		if s.Body != nil {
			p.trackUsage(pc, s.Body)
		}
	}
}

func (p *PassDetectUnused) trackUsageExpr(pc *PassContext, expr ir.Expr) {
	switch e := expr.(type) {
	case *ir.VarRef:
		if sym, ok := pc.Pkg.Syms.GetByID(e.Ref.Sym); ok {
			sym.Flags &^= ir.SF_Unused
		}
	case *ir.WhenExpr:
		p.trackUsageExpr(pc, e.Expr)
		for _, c := range e.Cases {
			for _, v := range c.Values {
				p.trackUsageExpr(pc, v)
			}
			p.trackUsageExpr(pc, c.Then)
		}
		if e.Default != nil {
			p.trackUsageExpr(pc, e.Default)
		}
	case *ir.UnaryExpr:
		p.trackUsageExpr(pc, e.Expr)
	case *ir.PrefixUnaryExpr:
		p.trackUsageExpr(pc, e.Expr)
	case *ir.PostfixUnaryExpr:
		p.trackUsageExpr(pc, e.Expr)
	case *ir.BinaryExpr:
		p.trackUsageExpr(pc, e.Left)
		p.trackUsageExpr(pc, e.Right)
	case *ir.CoalesceExpr:
		p.trackUsageExpr(pc, e.Left)
		p.trackUsageExpr(pc, e.Default)
	case *ir.TenaryExpr:
		p.trackUsageExpr(pc, e.Cond)
		p.trackUsageExpr(pc, e.Then)
		p.trackUsageExpr(pc, e.Else)
	case *ir.GroupedExpr:
		p.trackUsageExpr(pc, e.Expr)
	case *ir.AssignmentExpr:
		if sym, ok := pc.Pkg.Syms.GetByID(e.Left.Sym); ok {
			sym.Flags &^= ir.SF_Unused
		}
		p.trackUsageExpr(pc, e.Right)
	case *ir.AsExpr:
		p.trackUsageExpr(pc, e.Expr)
	case *ir.InstanceofExpr:
		p.trackUsageExpr(pc, e.Expr)
	case *ir.OptionUnwrapExpr:
		p.trackUsageExpr(pc, e.Expr)
	case *ir.IndexExpr:
		p.trackUsageExpr(pc, e.Expr)
		p.trackUsageExpr(pc, e.Index)
	case *ir.SliceRangeExpr:
		p.trackUsageExpr(pc, e.Expr)
		if e.Low != nil {
			p.trackUsageExpr(pc, e.Low)
		}
		if e.High != nil {
			p.trackUsageExpr(pc, e.High)
		}
	case *ir.FieldAccessExpr:
		if e.ResolvedSym != 0 {
			clearUnused(pc, e.ResolvedSym)
		}
		p.trackUsageExpr(pc, e.Expr)
	case *ir.RangeExpr:
		p.trackUsageExpr(pc, e.Start)
		p.trackUsageExpr(pc, e.End)
		if e.Inc != nil {
			p.trackUsageExpr(pc, e.Inc)
		}
	case *ir.FuncCallExpr:
		p.trackUsageExpr(pc, e.Callee)
		for _, arg := range e.Args {
			p.trackUsageExpr(pc, arg.Expr)
		}
	case *ir.FuncLitExpr:
		for _, param := range e.Params {
			if param.Default != nil {
				p.trackUsageExpr(pc, param.Default)
			}
		}
		for _, stmt := range ir.BlockStmts(e.Body) {
			p.trackUsage(pc, stmt)
		}
	case *ir.ArrayLiteral:
		for _, elem := range e.Elems {
			p.trackUsageExpr(pc, elem)
		}
	case *ir.MapLiteral:
		for _, kv := range e.Entries {
			p.trackUsageExpr(pc, kv.Key)
			p.trackUsageExpr(pc, kv.Value)
		}
	case *ir.TupleLiteral:
		for _, elem := range e.Elems {
			p.trackUsageExpr(pc, elem)
		}
	case *ir.StringTemplateExpr:
		for _, part := range e.Parts {
			if part.Expr != nil {
				p.trackUsageExpr(pc, part.Expr)
			}
		}
	case *ir.NewExpr:
		if sym, ok := pc.Pkg.Syms.GetByID(e.TypeName.Sym); ok {
			sym.Flags &^= ir.SF_Unused
		}
		for _, arg := range e.Args {
			p.trackUsageExpr(pc, arg.Expr)
		}
	}
}
