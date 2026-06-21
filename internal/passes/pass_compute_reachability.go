package passes

import "sova/internal/ir"

type PassComputeReachability struct{}

func (p *PassComputeReachability) Name() string       { return "compute_reachability" }

func (p *PassComputeReachability) Scope() PassScope   { return PerBuild }

func (p *PassComputeReachability) Requires() []string { return []string{"mangle"} }

func (p *PassComputeReachability) NoErrors() bool     { return true }

func (p *PassComputeReachability) Run(pc *PassContext) error {
	for _, pkg := range pc.Pkgs {
		for _, f := range pkg.Files {
			for _, st := range f.Hir.Statements {
				p.seedRoots(pc, st)
			}
		}
	}

	for {
		changed := false
		for _, pkg := range pc.Pkgs {
			for _, f := range pkg.Files {
				for _, st := range f.Hir.Statements {
					if p.traceIfReachable(pc, st) {
						changed = true
					}
				}
			}
		}

		if !changed {
			break
		}
	}

	return nil
}

func (p *PassComputeReachability) seedRoots(pc *PassContext, st ir.Stmt) {
	switch x := st.(type) {
	case *ir.FuncDeclStmt:
		if x.Name.Sym == 0 {
			return
		}

		if x.IsWired || x.Name.Name == "main" || x.Name.Name == "boot" {
			p.markSym(pc, x.Name.Sym)
		}

	case *ir.VarDeclStmt:
		for _, t := range x.Targets {
			if t.Name != nil {
				p.markSym(pc, t.Name.Sym)
			}
		}

	case *ir.TypeDeclStmt:
		if x.IsExtern && x.Name.Sym != 0 {
			p.markSym(pc, x.Name.Sym)
		}

	case *ir.TestDeclStmt:
		p.markBlock(pc, x.Body)
	}
}

func (p *PassComputeReachability) traceIfReachable(pc *PassContext, st ir.Stmt) bool {
	switch x := st.(type) {
	case *ir.FuncDeclStmt:
		if !p.symReachable(pc, x.Name.Sym) {
			return false
		}

		return p.markBlock(pc, x.Body)
	case *ir.TypeDeclStmt:
		if !p.symReachable(pc, x.Name.Sym) {
			return false
		}

		changed := false
		for _, fld := range x.Fields {
			if fld.Type != nil {
				if p.markTypeID(pc, fld.Type.Typ) {
					changed = true
				}
			}
		}

		for _, m := range x.Methods {
			if m.Func == nil {
				continue
			}

			if m.Func.Name.Sym != 0 {
				if p.markSym(pc, m.Func.Name.Sym) {
					changed = true
				}
			}

			if p.markBlock(pc, m.Func.Body) {
				changed = true
			}
		}

		for _, c := range x.Ctors {
			if c.Sym != 0 {
				if p.markSym(pc, c.Sym) {
					changed = true
				}
			}

			if p.markBlock(pc, c.Body) {
				changed = true
			}
		}

		for _, c := range x.Casts {
			if c.Sym != 0 {
				if p.markSym(pc, c.Sym) {
					changed = true
				}
			}

			if p.markBlock(pc, c.Body) {
				changed = true
			}
		}

		return changed
	case *ir.VarDeclStmt:
		anyTargetReachable := false
		for _, t := range x.Targets {
			if t.Name != nil && p.symReachable(pc, t.Name.Sym) {
				anyTargetReachable = true
				break
			}
		}

		if !anyTargetReachable {
			return false
		}

		return p.markExpr(pc, x.Init)
	}

	return false
}

func (p *PassComputeReachability) symReachable(pc *PassContext, sym ir.SymID) bool {
	if sym == 0 {
		return false
	}

	for _, pkg := range pc.Pkgs {
		if s, ok := pkg.Syms.GetByID(sym); ok {
			return s.Flags&ir.SF_Reachable != 0
		}
	}

	return false
}

func (p *PassComputeReachability) markSym(pc *PassContext, sym ir.SymID) bool {
	if sym == 0 {
		return false
	}

	for _, pkg := range pc.Pkgs {
		if s, ok := pkg.Syms.GetByID(sym); ok {
			if s.Flags&ir.SF_Reachable != 0 {
				return false
			}

			s.Flags |= ir.SF_Reachable
			p.markOwningType(pc, sym)
			return true
		}
	}

	return false
}

func (p *PassComputeReachability) markOwningType(pc *PassContext, sym ir.SymID) {
	for _, ty := range pc.Types.All() {
		if ty == nil {
			continue
		}

		if ty.Kind == ir.TK_Struct {
			for _, c := range ty.Struct.Ctors {
				if c.Sym == sym {
					p.markTypeID(pc, ty.ID)
					return
				}
			}

			for _, m := range ty.Struct.Methods {
				if m.Sym == sym {
					p.markTypeID(pc, ty.ID)
					return
				}
			}
		}
	}
}

func (p *PassComputeReachability) markTypeID(pc *PassContext, typID ir.TypID) bool {
	if typID == 0 {
		return false
	}

	ty, ok := pc.Types.GetByID(typID)
	if !ok {
		return false
	}

	changed := false
	switch ty.Kind {
	case ir.TK_Struct, ir.TK_Enum:
		for _, pkg := range pc.Pkgs {
			for sym, s := range pkg.Syms.ByID() {
				if s != nil && s.Typ == typID && s.Name == ty.Struct.Name {
					if p.markSym(pc, sym) {
						changed = true
					}
				}
			}
		}

	case ir.TK_Slice, ir.TK_Array, ir.TK_Option, ir.TK_Chan:
		if p.markTypeID(pc, ty.ElemType) {
			changed = true
		}

	case ir.TK_Map:
		if p.markTypeID(pc, ty.KeyType) {
			changed = true
		}

		if p.markTypeID(pc, ty.ValueType) {
			changed = true
		}

	case ir.TK_Function:
		for _, p2 := range ty.ParamTypes {
			if p2 != nil && p2.Type != nil {
				if p.markTypeID(pc, p2.Type.Typ) {
					changed = true
				}
			}
		}

		if p.markTypeID(pc, ty.ReturnType) {
			changed = true
		}

	case ir.TK_Tuple:
		for _, t := range ty.Fields {
			if p.markTypeID(pc, t.Type) {
				changed = true
			}
		}
	}

	return changed
}

func (p *PassComputeReachability) markBlock(pc *PassContext, b *ir.BlockStmt) bool {
	if b == nil {
		return false
	}

	changed := false
	for _, s := range b.Stmts {
		if p.markStmt(pc, s) {
			changed = true
		}
	}

	return changed
}

func (p *PassComputeReachability) markStmt(pc *PassContext, s ir.Stmt) bool {
	if ir.IsNilStmt(s) {
		return false
	}

	changed := false
	switch x := s.(type) {
	case *ir.BlockStmt:
		for _, ss := range x.Stmts {
			if p.markStmt(pc, ss) {
				changed = true
			}
		}

	case *ir.VarDeclStmt:
		if p.markExpr(pc, x.Init) {
			changed = true
		}

		for _, t := range x.Targets {
			if t.TypeAnn != nil {
				if p.markTypeID(pc, t.TypeAnn.Typ) {
					changed = true
				}
			}
		}

	case *ir.ExprStmt:
		if p.markExpr(pc, x.Expr) {
			changed = true
		}

	case *ir.FieldAssignmentStmt:
		if p.markExpr(pc, x.Value) {
			changed = true
		}

	case *ir.MultiAssignmentStmt:
		if p.markExpr(pc, x.Value) {
			changed = true
		}

	case *ir.IndexAssignmentStmt:
		if p.markExpr(pc, x.Value) {
			changed = true
		}

		if p.markExpr(pc, x.Index) {
			changed = true
		}

	case *ir.ReturnStmt:
		for _, r := range x.Results {
			if p.markExpr(pc, r) {
				changed = true
			}
		}

	case *ir.IfStmt:
		if p.markExpr(pc, x.Cond) {
			changed = true
		}

		if p.markBlock(pc, x.Then) {
			changed = true
		}

		for _, eb := range x.ElseIfs {
			if p.markExpr(pc, eb.Cond) {
				changed = true
			}

			if p.markBlock(pc, eb.Then) {
				changed = true
			}
		}

		if p.markBlock(pc, x.Else) {
			changed = true
		}

	case *ir.SwitchStmt:
		if p.markExpr(pc, x.Expr) {
			changed = true
		}

		for _, c := range x.Cases {
			for _, v := range c.Values {
				if p.markExpr(pc, v) {
					changed = true
				}
			}

			for _, ss := range c.Stmts {
				if p.markStmt(pc, ss) {
					changed = true
				}
			}
		}

		for _, ss := range x.Default {
			if p.markStmt(pc, ss) {
				changed = true
			}
		}

	case *ir.GuardStmt:
		if p.markExpr(pc, x.Cond) {
			changed = true
		}

		for _, r := range x.Returns {
			if p.markExpr(pc, r) {
				changed = true
			}
		}

	case *ir.ForStmt:
		if x.CondInt != nil {
			if x.CondInt.Init != nil {
				if p.markExpr(pc, x.CondInt.Init.Init) {
					changed = true
				}
			}

			if p.markExpr(pc, x.CondInt.Cond) {
				changed = true
			}

			if p.markExpr(pc, x.CondInt.Post) {
				changed = true
			}
		}

		if x.CondIn != nil {
			if p.markExpr(pc, x.CondIn.IterExpr) {
				changed = true
			}
		}

		if x.CondRange != nil {
			if p.markExpr(pc, x.CondRange.RangeStart) {
				changed = true
			}

			if p.markExpr(pc, x.CondRange.RangeEnd) {
				changed = true
			}
		}

		if p.markBlock(pc, x.Body) {
			changed = true
		}

	case *ir.WhileStmt:
		if p.markExpr(pc, x.Cond) {
			changed = true
		}

		if p.markBlock(pc, x.Body) {
			changed = true
		}

	case *ir.GoStmt:
		if p.markExpr(pc, x.Call) {
			changed = true
		}

		if p.markBlock(pc, x.Body) {
			changed = true
		}

	case *ir.DeferStmt:
		if p.markExpr(pc, x.Call) {
			changed = true
		}

		if p.markBlock(pc, x.Body) {
			changed = true
		}

	case *ir.AssertStmt:
		if p.markExpr(pc, x.Expr) {
			changed = true
		}

	case *ir.AsSessionStmt:
		if p.markBlock(pc, x.Body) {
			changed = true
		}
	}

	return changed
}

func (p *PassComputeReachability) markExpr(pc *PassContext, e ir.Expr) bool {
	if ir.IsNilExpr(e) {
		return false
	}

	changed := false
	if t := e.GetType(); t != 0 {
		if p.markTypeID(pc, t) {
			changed = true
		}
	}

	switch x := e.(type) {
	case *ir.VarRef:
		if p.markSym(pc, x.Ref.Sym) {
			changed = true
		}

	case *ir.FieldAccessExpr:
		if p.markExpr(pc, x.Expr) {
			changed = true
		}

		if p.markSym(pc, x.ResolvedSym) {
			changed = true
		}

	case *ir.FuncCallExpr:
		if p.markExpr(pc, x.Callee) {
			changed = true
		}

		for _, ta := range x.TypeArgs {
			if ta != nil {
				if p.markTypeID(pc, ta.Typ) {
					changed = true
				}
			}
		}

		for _, a := range x.Args {
			if p.markExpr(pc, a.Expr) {
				changed = true
			}
		}

	case *ir.ComposableCallExpr:
		if p.markExpr(pc, x.Callee) {
			changed = true
		}

		if p.markSym(pc, x.CtorSym) {
			changed = true
		}

		for _, a := range x.Args {
			if p.markExpr(pc, a.Expr) {
				changed = true
			}
		}

		for _, c := range x.Children {
			if c.Expr != nil {
				if p.markExpr(pc, c.Expr) {
					changed = true
				}
			}

			if c.Stmt != nil {
				if p.markStmt(pc, c.Stmt) {
					changed = true
				}
			}
		}

	case *ir.NewExpr:
		if x.TypeName.Sym != 0 {
			if p.markSym(pc, x.TypeName.Sym) {
				changed = true
			}
		}

		if p.markSym(pc, x.CtorSym) {
			changed = true
		}

		for _, ta := range x.TypeArgs {
			if ta != nil {
				if p.markTypeID(pc, ta.Typ) {
					changed = true
				}
			}
		}

		for _, a := range x.Args {
			if p.markExpr(pc, a.Expr) {
				changed = true
			}
		}

	case *ir.BinaryExpr:
		if p.markExpr(pc, x.Left) {
			changed = true
		}

		if p.markExpr(pc, x.Right) {
			changed = true
		}

	case *ir.UnaryExpr:
		if p.markExpr(pc, x.Expr) {
			changed = true
		}

	case *ir.GroupedExpr:
		if p.markExpr(pc, x.Expr) {
			changed = true
		}

	case *ir.AssignmentExpr:
		if p.markExpr(pc, x.Right) {
			changed = true
		}

	case *ir.OptionUnwrapExpr:
		if p.markExpr(pc, x.Expr) {
			changed = true
		}

	case *ir.CoalesceExpr:
		if p.markExpr(pc, x.Left) {
			changed = true
		}

		if p.markExpr(pc, x.Default) {
			changed = true
		}

	case *ir.AsExpr:
		if p.markExpr(pc, x.Expr) {
			changed = true
		}

		if x.Target != nil {
			if p.markTypeID(pc, x.Target.Typ) {
				changed = true
			}
		}

	case *ir.InstanceofExpr:
		if p.markExpr(pc, x.Expr) {
			changed = true
		}

		if x.Target != nil {
			if p.markTypeID(pc, x.Target.Typ) {
				changed = true
			}
		}

	case *ir.IndexExpr:
		if p.markExpr(pc, x.Expr) {
			changed = true
		}

		if p.markExpr(pc, x.Index) {
			changed = true
		}

	case *ir.SliceRangeExpr:
		if p.markExpr(pc, x.Expr) {
			changed = true
		}

		if p.markExpr(pc, x.Low) {
			changed = true
		}

		if p.markExpr(pc, x.High) {
			changed = true
		}

	case *ir.ArrayLiteral:
		for _, el := range x.Elems {
			if p.markExpr(pc, el) {
				changed = true
			}
		}

	case *ir.MapLiteral:
		for _, entry := range x.Entries {
			if p.markExpr(pc, entry.Key) {
				changed = true
			}

			if p.markExpr(pc, entry.Value) {
				changed = true
			}
		}

	case *ir.TupleLiteral:
		for _, el := range x.Elems {
			if p.markExpr(pc, el) {
				changed = true
			}
		}

	case *ir.FuncLitExpr:
		if p.markBlock(pc, x.Body) {
			changed = true
		}

	case *ir.TenaryExpr:
		if p.markExpr(pc, x.Cond) {
			changed = true
		}

		if p.markExpr(pc, x.Then) {
			changed = true
		}

		if p.markExpr(pc, x.Else) {
			changed = true
		}

	case *ir.RangeExpr:
		if p.markExpr(pc, x.Start) {
			changed = true
		}

		if p.markExpr(pc, x.End) {
			changed = true
		}

	case *ir.WhenExpr:
		if p.markExpr(pc, x.Expr) {
			changed = true
		}

		for _, c := range x.Cases {
			for _, v := range c.Values {
				if p.markExpr(pc, v) {
					changed = true
				}
			}

			if p.markExpr(pc, c.Then) {
				changed = true
			}
		}

		if p.markExpr(pc, x.Default) {
			changed = true
		}
	}

	return changed
}
