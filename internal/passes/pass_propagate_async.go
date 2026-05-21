package passes

import (
	"sova/internal/diag"
	"sova/internal/ir"
)

// PassPropagateAsync auto-lifts the async flag: any function that transitively calls an async function becomes async itself. The pass also emits diagnostics for async calls reached from contexts where lifting is impossible (top-level statements and constant initializers).
type PassPropagateAsync struct{}

func (p *PassPropagateAsync) Name() string       { return "propagate_async" }
func (p *PassPropagateAsync) Scope() PassScope   { return PerBuild }
func (p *PassPropagateAsync) Requires() []string { return []string{"infer_types"} }
func (p *PassPropagateAsync) NoErrors() bool     { return false }

func (p *PassPropagateAsync) Run(pc *PassContext) error {
	for {
		changed := false
		for _, pkg := range pc.Pkgs {
			for _, f := range pkg.Files {
				for _, st := range f.Hir.Statements {
					switch x := st.(type) {
					case *ir.FuncDeclStmt:
						if x.IsAsync {
							continue
						}
						if p.bodyHasAsyncCall(pc, pkg, x.Body) {
							x.IsAsync = true
							p.upgradeSymTypeToAsync(pc, pkg, x.Name.Sym)
							changed = true
						}
					case *ir.TypeDeclStmt:
						for _, m := range x.Methods {
							if m.Func == nil || m.Func.IsAsync {
								continue
							}
							if p.bodyHasAsyncCall(pc, pkg, m.Func.Body) {
								m.Func.IsAsync = true
								p.upgradeSymTypeToAsync(pc, pkg, m.Func.Name.Sym)
								changed = true
							}
						}
					}
				}
			}
		}
		// Also mark any FuncLitExpr (closure) whose body transitively calls
		// an async function. A closure is its own async boundary: we don't
		// promote the surrounding function to async (declaring a closure is
		// synchronous), but the closure itself must be `async` so that
		// awaited calls inside it are legal at the JS layer.
		for _, pkg := range pc.Pkgs {
			for _, f := range pkg.Files {
				for _, st := range f.Hir.Statements {
					if p.markFuncLitsInStmt(pc, pkg, st) {
						changed = true
					}
				}
			}
		}
		if !changed {
			break
		}
	}

	for _, pkg := range pc.Pkgs {
		for _, f := range pkg.Files {
			for _, st := range f.Hir.Statements {
				switch s := st.(type) {
				case *ir.FuncDeclStmt:
					p.markCallsInStmt(pc, pkg, s.Body)
				case *ir.VarDeclStmt:
					if s.Init != nil {
						if p.exprHasAsyncCall(pc, pkg, s.Init) {
							pc.Diag.Report(diag.ErrAsyncInSyncContext, s.Init.Span(), "top-level initializer")
						}
					}
				case *ir.ExprStmt:
					if p.exprHasAsyncCall(pc, pkg, s.Expr) {
						pc.Diag.Report(diag.ErrAsyncInSyncContext, s.Expr.Span(), "top-level statement")
					}
				case *ir.TypeDeclStmt:
					for _, m := range s.Methods {
						p.markCallsInStmt(pc, pkg, m.Func.Body)
					}
					for _, ctor := range s.Ctors {
						p.markCallsInStmt(pc, pkg, ctor.Body)
					}
				case *ir.TestDeclStmt:
					if s.Body != nil {
						p.markCallsInStmt(pc, pkg, s.Body)
					}
				case *ir.GroupDeclStmt:
					for _, gst := range s.Body {
						switch gs := gst.(type) {
						case *ir.TestDeclStmt:
							if gs.Body != nil {
								p.markCallsInStmt(pc, pkg, gs.Body)
							}
						case *ir.SetupStmt:
							if gs.Body != nil {
								p.markCallsInStmt(pc, pkg, gs.Body)
							}
						case *ir.TeardownStmt:
							if gs.Body != nil {
								p.markCallsInStmt(pc, pkg, gs.Body)
							}
						}
					}
				case *ir.SetupStmt:
					if s.Body != nil {
						p.markCallsInStmt(pc, pkg, s.Body)
					}
				case *ir.TeardownStmt:
					if s.Body != nil {
						p.markCallsInStmt(pc, pkg, s.Body)
					}
				}
			}
		}
	}
	return nil
}

// markFuncLitsInStmt walks `st` for any FuncLitExpr nodes whose body transitively calls an async function and flips their IsAsync flag. Returns true on any new marking so the outer fixed-point loop knows to iterate again (a closure may become async only after the function it calls is itself promoted to async on a later pass).
func (p *PassPropagateAsync) markFuncLitsInStmt(pc *PassContext, pkg *ir.PackageContext, st ir.Stmt) bool {
	if ir.IsNilStmt(st) {
		return false
	}
	changed := false
	switch s := st.(type) {
	case *ir.BlockStmt:
		for _, ss := range s.Stmts {
			if p.markFuncLitsInStmt(pc, pkg, ss) {
				changed = true
			}
		}
	case *ir.VarDeclStmt:
		if p.markFuncLitsInExpr(pc, pkg, s.Init) {
			changed = true
		}
	case *ir.ExprStmt:
		if p.markFuncLitsInExpr(pc, pkg, s.Expr) {
			changed = true
		}
	case *ir.FieldAssignmentStmt:
		if p.markFuncLitsInExpr(pc, pkg, s.Value) {
			changed = true
		}
	case *ir.MultiAssignmentStmt:
		if p.markFuncLitsInExpr(pc, pkg, s.Value) {
			changed = true
		}
	case *ir.IfStmt:
		if p.markFuncLitsInExpr(pc, pkg, s.Cond) {
			changed = true
		}
		for _, ss := range ir.BlockStmts(s.Then) {
			if p.markFuncLitsInStmt(pc, pkg, ss) {
				changed = true
			}
		}
		for _, eb := range s.ElseIfs {
			if p.markFuncLitsInExpr(pc, pkg, eb.Cond) {
				changed = true
			}
			for _, ss := range ir.BlockStmts(eb.Then) {
				if p.markFuncLitsInStmt(pc, pkg, ss) {
					changed = true
				}
			}
		}
		for _, ss := range ir.BlockStmts(s.Else) {
			if p.markFuncLitsInStmt(pc, pkg, ss) {
				changed = true
			}
		}
	case *ir.SwitchStmt:
		if p.markFuncLitsInExpr(pc, pkg, s.Expr) {
			changed = true
		}
		for _, c := range s.Cases {
			for _, v := range c.Values {
				if p.markFuncLitsInExpr(pc, pkg, v) {
					changed = true
				}
			}
			for _, ss := range c.Stmts {
				if p.markFuncLitsInStmt(pc, pkg, ss) {
					changed = true
				}
			}
		}
		for _, ss := range s.Default {
			if p.markFuncLitsInStmt(pc, pkg, ss) {
				changed = true
			}
		}
	case *ir.ReturnStmt:
		for _, r := range s.Results {
			if p.markFuncLitsInExpr(pc, pkg, r) {
				changed = true
			}
		}
	case *ir.GuardStmt:
		if p.markFuncLitsInExpr(pc, pkg, s.Cond) {
			changed = true
		}
		for _, r := range s.Returns {
			if p.markFuncLitsInExpr(pc, pkg, r) {
				changed = true
			}
		}
	case *ir.ForStmt:
		for _, ss := range ir.BlockStmts(s.Body) {
			if p.markFuncLitsInStmt(pc, pkg, ss) {
				changed = true
			}
		}
	case *ir.WhileStmt:
		if p.markFuncLitsInExpr(pc, pkg, s.Cond) {
			changed = true
		}
		for _, ss := range ir.BlockStmts(s.Body) {
			if p.markFuncLitsInStmt(pc, pkg, ss) {
				changed = true
			}
		}
	case *ir.FuncDeclStmt:
		for _, ss := range ir.BlockStmts(s.Body) {
			if p.markFuncLitsInStmt(pc, pkg, ss) {
				changed = true
			}
		}
	case *ir.TypeDeclStmt:
		for _, m := range s.Methods {
			if p.markFuncLitsInStmt(pc, pkg, m.Func) {
				changed = true
			}
		}
		for _, ctor := range s.Ctors {
			for _, ss := range ir.BlockStmts(ctor.Body) {
				if p.markFuncLitsInStmt(pc, pkg, ss) {
					changed = true
				}
			}
		}
	case *ir.TestDeclStmt:
		for _, ss := range ir.BlockStmts(s.Body) {
			if p.markFuncLitsInStmt(pc, pkg, ss) {
				changed = true
			}
		}
	case *ir.GoStmt:
		if p.markFuncLitsInExpr(pc, pkg, s.Call) {
			changed = true
		}
		for _, ss := range ir.BlockStmts(s.Body) {
			if p.markFuncLitsInStmt(pc, pkg, ss) {
				changed = true
			}
		}
	case *ir.DeferStmt:
		if p.markFuncLitsInExpr(pc, pkg, s.Call) {
			changed = true
		}
		for _, ss := range ir.BlockStmts(s.Body) {
			if p.markFuncLitsInStmt(pc, pkg, ss) {
				changed = true
			}
		}
	}
	return changed
}

// markFuncLitsInExpr is the expression-walking counterpart of markFuncLitsInStmt. On every FuncLitExpr it finds, the closure's body is checked against `bodyHasAsyncCall`; if any async call is reachable, IsAsync is set and the walk continues into the body so nested closures are also resolved.
func (p *PassPropagateAsync) markFuncLitsInExpr(pc *PassContext, pkg *ir.PackageContext, e ir.Expr) bool {
	if ir.IsNilExpr(e) {
		return false
	}
	changed := false
	switch x := e.(type) {
	case *ir.FuncLitExpr:
		if !x.IsAsync && p.bodyHasAsyncCall(pc, pkg, x.Body) {
			x.IsAsync = true
			changed = true
		}
		for _, ss := range ir.BlockStmts(x.Body) {
			if p.markFuncLitsInStmt(pc, pkg, ss) {
				changed = true
			}
		}
	case *ir.FuncCallExpr:
		if p.markFuncLitsInExpr(pc, pkg, x.Callee) {
			changed = true
		}
		for _, a := range x.Args {
			if p.markFuncLitsInExpr(pc, pkg, a.Expr) {
				changed = true
			}
		}
	case *ir.NewExpr:
		for _, a := range x.Args {
			if p.markFuncLitsInExpr(pc, pkg, a.Expr) {
				changed = true
			}
		}
	case *ir.BinaryExpr:
		if p.markFuncLitsInExpr(pc, pkg, x.Left) {
			changed = true
		}
		if p.markFuncLitsInExpr(pc, pkg, x.Right) {
			changed = true
		}
	case *ir.UnaryExpr:
		if p.markFuncLitsInExpr(pc, pkg, x.Expr) {
			changed = true
		}
	case *ir.PrefixUnaryExpr:
		if p.markFuncLitsInExpr(pc, pkg, x.Expr) {
			changed = true
		}
	case *ir.PostfixUnaryExpr:
		if p.markFuncLitsInExpr(pc, pkg, x.Expr) {
			changed = true
		}
	case *ir.AssignmentExpr:
		if p.markFuncLitsInExpr(pc, pkg, x.Right) {
			changed = true
		}
	case *ir.IndexExpr:
		if p.markFuncLitsInExpr(pc, pkg, x.Expr) {
			changed = true
		}
		if p.markFuncLitsInExpr(pc, pkg, x.Index) {
			changed = true
		}
	case *ir.FieldAccessExpr:
		if p.markFuncLitsInExpr(pc, pkg, x.Expr) {
			changed = true
		}
	case *ir.GroupedExpr:
		if p.markFuncLitsInExpr(pc, pkg, x.Expr) {
			changed = true
		}
	case *ir.TenaryExpr:
		if p.markFuncLitsInExpr(pc, pkg, x.Cond) {
			changed = true
		}
		if p.markFuncLitsInExpr(pc, pkg, x.Then) {
			changed = true
		}
		if p.markFuncLitsInExpr(pc, pkg, x.Else) {
			changed = true
		}
	case *ir.CoalesceExpr:
		if p.markFuncLitsInExpr(pc, pkg, x.Left) {
			changed = true
		}
		if p.markFuncLitsInExpr(pc, pkg, x.Default) {
			changed = true
		}
	case *ir.ArrayLiteral:
		for _, el := range x.Elems {
			if p.markFuncLitsInExpr(pc, pkg, el) {
				changed = true
			}
		}
	case *ir.MapLiteral:
		for _, en := range x.Entries {
			if p.markFuncLitsInExpr(pc, pkg, en.Key) {
				changed = true
			}
			if p.markFuncLitsInExpr(pc, pkg, en.Value) {
				changed = true
			}
		}
	case *ir.TupleLiteral:
		for _, el := range x.Elems {
			if p.markFuncLitsInExpr(pc, pkg, el) {
				changed = true
			}
		}
	case *ir.AsExpr:
		if p.markFuncLitsInExpr(pc, pkg, x.Expr) {
			changed = true
		}
	}
	return changed
}

func (p *PassPropagateAsync) upgradeSymTypeToAsync(pc *PassContext, pkg *ir.PackageContext, sym ir.SymID) {
	s, ok := pkg.Syms.GetByID(sym)
	if !ok {
		return
	}
	ft, ok := pc.Types.GetByID(s.Typ)
	if !ok || ft.Kind != ir.TK_Function {
		return
	}
	newTyp := pc.Types.AsyncFuncOf(ft.ParamTypes, ft.ReturnType)
	pkg.Syms.SetType(sym, newTyp)
}

func (p *PassPropagateAsync) bodyHasAsyncCall(pc *PassContext, pkg *ir.PackageContext, b *ir.BlockStmt) bool {
	return p.bodyCallsAsync(pc, pkg, b)
}

func (p *PassPropagateAsync) exprHasAsyncCall(pc *PassContext, pkg *ir.PackageContext, e ir.Expr) bool {
	return p.exprCallsAsync(pc, pkg, e)
}

func (p *PassPropagateAsync) markCallsInStmt(pc *PassContext, pkg *ir.PackageContext, s ir.Stmt) {
	if ir.IsNilStmt(s) {
		return
	}
	switch v := s.(type) {
	case *ir.BlockStmt:
		for _, ss := range v.Stmts {
			p.markCallsInStmt(pc, pkg, ss)
		}
	case *ir.VarDeclStmt:
		p.markCallsInExpr(pc, pkg, v.Init)
	case *ir.ExprStmt:
		p.markCallsInExpr(pc, pkg, v.Expr)
	case *ir.FieldAssignmentStmt:
		p.markCallsInExpr(pc, pkg, v.Value)
	case *ir.MultiAssignmentStmt:
		p.markCallsInExpr(pc, pkg, v.Value)
	case *ir.IfStmt:
		p.markCallsInExpr(pc, pkg, v.Cond)
		p.markCallsInStmt(pc, pkg, v.Then)
		for _, eb := range v.ElseIfs {
			p.markCallsInExpr(pc, pkg, eb.Cond)
			p.markCallsInStmt(pc, pkg, eb.Then)
		}
		if v.Else != nil {
			p.markCallsInStmt(pc, pkg, v.Else)
		}
	case *ir.SwitchStmt:
		p.markCallsInExpr(pc, pkg, v.Expr)
		for _, c := range v.Cases {
			for _, val := range c.Values {
				p.markCallsInExpr(pc, pkg, val)
			}
			for _, ss := range c.Stmts {
				p.markCallsInStmt(pc, pkg, ss)
			}
		}
		for _, ss := range v.Default {
			p.markCallsInStmt(pc, pkg, ss)
		}
	case *ir.ReturnStmt:
		for _, r := range v.Results {
			p.markCallsInExpr(pc, pkg, r)
		}
	case *ir.GuardStmt:
		p.markCallsInExpr(pc, pkg, v.Cond)
		for _, r := range v.Returns {
			p.markCallsInExpr(pc, pkg, r)
		}
	case *ir.ForStmt:
		if v.CondInt != nil {
			if v.CondInt.Init != nil {
				p.markCallsInExpr(pc, pkg, v.CondInt.Init.Init)
			}
			p.markCallsInExpr(pc, pkg, v.CondInt.Cond)
			p.markCallsInExpr(pc, pkg, v.CondInt.Post)
		}
		if v.CondRange != nil {
			p.markCallsInExpr(pc, pkg, v.CondRange.RangeStart)
			p.markCallsInExpr(pc, pkg, v.CondRange.RangeEnd)
		}
		if v.CondIn != nil {
			p.markCallsInExpr(pc, pkg, v.CondIn.IterExpr)
		}
		p.markCallsInStmt(pc, pkg, v.Body)
	case *ir.WhileStmt:
		p.markCallsInExpr(pc, pkg, v.Cond)
		p.markCallsInStmt(pc, pkg, v.Body)
	case *ir.AssertStmt:
		p.markCallsInExpr(pc, pkg, v.Expr)
	case *ir.AsSessionStmt:
		if v.Body != nil {
			p.markCallsInStmt(pc, pkg, v.Body)
		}
	case *ir.GoStmt:
		if v.Call != nil {
			p.markCallsInExpr(pc, pkg, v.Call)
		}
		if v.Body != nil {
			p.markCallsInStmt(pc, pkg, v.Body)
		}
	case *ir.DeferStmt:
		if v.Call != nil {
			p.markCallsInExpr(pc, pkg, v.Call)
		}
		if v.Body != nil {
			p.markCallsInStmt(pc, pkg, v.Body)
		}
	case *ir.SelectStmt:
		for _, cc := range v.Cases {
			if cc.ChanExpr != nil {
				p.markCallsInExpr(pc, pkg, cc.ChanExpr)
			}
			if cc.SendValue != nil {
				p.markCallsInExpr(pc, pkg, cc.SendValue)
			}
			if cc.Body != nil {
				p.markCallsInStmt(pc, pkg, cc.Body)
			}
		}
		if v.Default != nil {
			p.markCallsInStmt(pc, pkg, v.Default)
		}
	}
}

func (p *PassPropagateAsync) markCallsInExpr(pc *PassContext, pkg *ir.PackageContext, e ir.Expr) {
	if ir.IsNilExpr(e) {
		return
	}
	switch x := e.(type) {
	case *ir.FuncCallExpr:
		p.calleeIsAsync(pc, pkg, x)
		p.markCallsInExpr(pc, pkg, x.Callee)
		for _, a := range x.Args {
			p.markCallsInExpr(pc, pkg, a.Expr)
		}
	case *ir.NewExpr:
		for _, a := range x.Args {
			p.markCallsInExpr(pc, pkg, a.Expr)
		}
	case *ir.BinaryExpr:
		p.markCallsInExpr(pc, pkg, x.Left)
		p.markCallsInExpr(pc, pkg, x.Right)
	case *ir.UnaryExpr:
		p.markCallsInExpr(pc, pkg, x.Expr)
	case *ir.PrefixUnaryExpr:
		p.markCallsInExpr(pc, pkg, x.Expr)
	case *ir.PostfixUnaryExpr:
		p.markCallsInExpr(pc, pkg, x.Expr)
	case *ir.AssignmentExpr:
		p.markCallsInExpr(pc, pkg, x.Right)
	case *ir.IndexExpr:
		p.markCallsInExpr(pc, pkg, x.Expr)
		p.markCallsInExpr(pc, pkg, x.Index)
	case *ir.FieldAccessExpr:
		p.markCallsInExpr(pc, pkg, x.Expr)
	case *ir.RangeExpr:
		p.markCallsInExpr(pc, pkg, x.Start)
		p.markCallsInExpr(pc, pkg, x.End)
		p.markCallsInExpr(pc, pkg, x.Inc)
	case *ir.TenaryExpr:
		p.markCallsInExpr(pc, pkg, x.Cond)
		p.markCallsInExpr(pc, pkg, x.Then)
		p.markCallsInExpr(pc, pkg, x.Else)
	case *ir.CoalesceExpr:
		p.markCallsInExpr(pc, pkg, x.Left)
		p.markCallsInExpr(pc, pkg, x.Default)
	case *ir.GroupedExpr:
		p.markCallsInExpr(pc, pkg, x.Expr)
	case *ir.WhenExpr:
		p.markCallsInExpr(pc, pkg, x.Expr)
		p.markCallsInExpr(pc, pkg, x.Default)
		for _, c := range x.Cases {
			for _, val := range c.Values {
				p.markCallsInExpr(pc, pkg, val)
			}
			p.markCallsInExpr(pc, pkg, c.Then)
		}
	case *ir.StringTemplateExpr:
		for _, part := range x.Parts {
			p.markCallsInExpr(pc, pkg, part.Expr)
		}
	case *ir.ArrayLiteral:
		for _, el := range x.Elems {
			p.markCallsInExpr(pc, pkg, el)
		}
	case *ir.MapLiteral:
		for _, en := range x.Entries {
			p.markCallsInExpr(pc, pkg, en.Key)
			p.markCallsInExpr(pc, pkg, en.Value)
		}
	case *ir.TupleLiteral:
		for _, el := range x.Elems {
			p.markCallsInExpr(pc, pkg, el)
		}
	case *ir.FuncLitExpr:
		for _, ss := range ir.BlockStmts(x.Body) {
			p.markCallsInStmt(pc, pkg, ss)
		}
	case *ir.AsExpr:
		p.markCallsInExpr(pc, pkg, x.Expr)
	}
}

func (p *PassPropagateAsync) bodyCallsAsync(pc *PassContext, pkg *ir.PackageContext, b *ir.BlockStmt) bool {
	if b == nil {
		return false
	}
	for _, s := range b.Stmts {
		if p.stmtCallsAsync(pc, pkg, s) {
			return true
		}
	}
	return false
}

func (p *PassPropagateAsync) stmtCallsAsync(pc *PassContext, pkg *ir.PackageContext, st ir.Stmt) bool {
	switch s := st.(type) {
	case *ir.BlockStmt:
		return p.bodyCallsAsync(pc, pkg, s)
	case *ir.VarDeclStmt:
		return s.Init != nil && p.exprCallsAsync(pc, pkg, s.Init)
	case *ir.ExprStmt:
		return p.exprCallsAsync(pc, pkg, s.Expr)
	case *ir.FieldAssignmentStmt:
		return p.exprCallsAsync(pc, pkg, s.Value)
	case *ir.MultiAssignmentStmt:
		return p.exprCallsAsync(pc, pkg, s.Value)
	case *ir.IfStmt:
		if p.exprCallsAsync(pc, pkg, s.Cond) {
			return true
		}
		if p.bodyCallsAsync(pc, pkg, s.Then) {
			return true
		}
		for _, eb := range s.ElseIfs {
			if p.exprCallsAsync(pc, pkg, eb.Cond) || p.bodyCallsAsync(pc, pkg, eb.Then) {
				return true
			}
		}
		return p.bodyCallsAsync(pc, pkg, s.Else)
	case *ir.SwitchStmt:
		if p.exprCallsAsync(pc, pkg, s.Expr) {
			return true
		}
		for _, c := range s.Cases {
			for _, v := range c.Values {
				if p.exprCallsAsync(pc, pkg, v) {
					return true
				}
			}
			for _, ss := range c.Stmts {
				if p.stmtCallsAsync(pc, pkg, ss) {
					return true
				}
			}
		}
		for _, ss := range s.Default {
			if p.stmtCallsAsync(pc, pkg, ss) {
				return true
			}
		}
	case *ir.ReturnStmt:
		for _, r := range s.Results {
			if p.exprCallsAsync(pc, pkg, r) {
				return true
			}
		}
	case *ir.GuardStmt:
		if p.exprCallsAsync(pc, pkg, s.Cond) {
			return true
		}
		for _, r := range s.Returns {
			if p.exprCallsAsync(pc, pkg, r) {
				return true
			}
		}
	case *ir.ForStmt:
		if s.CondInt != nil {
			if s.CondInt.Init != nil && p.exprCallsAsync(pc, pkg, s.CondInt.Init.Init) {
				return true
			}
			if p.exprCallsAsync(pc, pkg, s.CondInt.Cond) || p.exprCallsAsync(pc, pkg, s.CondInt.Post) {
				return true
			}
		}
		if s.CondRange != nil {
			if p.exprCallsAsync(pc, pkg, s.CondRange.RangeStart) || p.exprCallsAsync(pc, pkg, s.CondRange.RangeEnd) {
				return true
			}
		}
		if s.CondIn != nil && p.exprCallsAsync(pc, pkg, s.CondIn.IterExpr) {
			return true
		}
		return p.bodyCallsAsync(pc, pkg, s.Body)
	case *ir.WhileStmt:
		if p.exprCallsAsync(pc, pkg, s.Cond) {
			return true
		}
		return p.bodyCallsAsync(pc, pkg, s.Body)
	case *ir.SelectStmt:
		return true
	case *ir.GoStmt:
		return false
	case *ir.DeferStmt:
		if s.Call != nil && p.exprCallsAsync(pc, pkg, s.Call) {
			return true
		}
		if s.Body != nil {
			return p.bodyCallsAsync(pc, pkg, s.Body)
		}
	}
	return false
}

func (p *PassPropagateAsync) exprCallsAsync(pc *PassContext, pkg *ir.PackageContext, e ir.Expr) bool {
	if ir.IsNilExpr(e) {
		return false
	}
	switch x := e.(type) {
	case *ir.FuncCallExpr:
		if p.calleeIsAsync(pc, pkg, x) {
			return true
		}
		if p.exprCallsAsync(pc, pkg, x.Callee) {
			return true
		}
		for _, a := range x.Args {
			if p.exprCallsAsync(pc, pkg, a.Expr) {
				return true
			}
		}
	case *ir.NewExpr:
		for _, a := range x.Args {
			if p.exprCallsAsync(pc, pkg, a.Expr) {
				return true
			}
		}
	case *ir.BinaryExpr:
		return p.exprCallsAsync(pc, pkg, x.Left) || p.exprCallsAsync(pc, pkg, x.Right)
	case *ir.UnaryExpr:
		return p.exprCallsAsync(pc, pkg, x.Expr)
	case *ir.PrefixUnaryExpr:
		return p.exprCallsAsync(pc, pkg, x.Expr)
	case *ir.PostfixUnaryExpr:
		return p.exprCallsAsync(pc, pkg, x.Expr)
	case *ir.AssignmentExpr:
		return p.exprCallsAsync(pc, pkg, x.Right)
	case *ir.IndexExpr:
		return p.exprCallsAsync(pc, pkg, x.Expr) || p.exprCallsAsync(pc, pkg, x.Index)
	case *ir.FieldAccessExpr:
		return p.exprCallsAsync(pc, pkg, x.Expr)
	case *ir.RangeExpr:
		return p.exprCallsAsync(pc, pkg, x.Start) || p.exprCallsAsync(pc, pkg, x.End) || p.exprCallsAsync(pc, pkg, x.Inc)
	case *ir.TenaryExpr:
		return p.exprCallsAsync(pc, pkg, x.Cond) || p.exprCallsAsync(pc, pkg, x.Then) || p.exprCallsAsync(pc, pkg, x.Else)
	case *ir.CoalesceExpr:
		return p.exprCallsAsync(pc, pkg, x.Left) || p.exprCallsAsync(pc, pkg, x.Default)
	case *ir.GroupedExpr:
		return p.exprCallsAsync(pc, pkg, x.Expr)
	case *ir.WhenExpr:
		if p.exprCallsAsync(pc, pkg, x.Expr) || p.exprCallsAsync(pc, pkg, x.Default) {
			return true
		}
		for _, c := range x.Cases {
			for _, v := range c.Values {
				if p.exprCallsAsync(pc, pkg, v) {
					return true
				}
			}
			if p.exprCallsAsync(pc, pkg, c.Then) {
				return true
			}
		}
	case *ir.StringTemplateExpr:
		for _, part := range x.Parts {
			if p.exprCallsAsync(pc, pkg, part.Expr) {
				return true
			}
		}
	case *ir.ArrayLiteral:
		for _, el := range x.Elems {
			if p.exprCallsAsync(pc, pkg, el) {
				return true
			}
		}
	case *ir.MapLiteral:
		for _, en := range x.Entries {
			if p.exprCallsAsync(pc, pkg, en.Key) || p.exprCallsAsync(pc, pkg, en.Value) {
				return true
			}
		}
	case *ir.TupleLiteral:
		for _, el := range x.Elems {
			if p.exprCallsAsync(pc, pkg, el) {
				return true
			}
		}
	}
	return false
}

func (p *PassPropagateAsync) calleeIsAsync(pc *PassContext, pkg *ir.PackageContext, call *ir.FuncCallExpr) bool {
	sym := p.calleeSym(call.Callee)
	if sym == 0 {
		if ft, ok := pc.Types.GetByID(call.Callee.GetType()); ok && ft.Kind == ir.TK_Function && ft.IsAsync {
			call.IsAsync = true
			return true
		}
		return false
	}
	for _, target := range pc.Pkgs {
		s, ok := target.Syms.GetByID(sym)
		if !ok {
			continue
		}
		ft, ok := pc.Types.GetByID(s.Typ)
		if !ok || ft.Kind != ir.TK_Function {
			return false
		}
		if ft.IsAsync {
			call.IsAsync = true
		}
		return ft.IsAsync
	}
	return false
}

func (p *PassPropagateAsync) calleeSym(e ir.Expr) ir.SymID {
	switch c := e.(type) {
	case *ir.VarRef:
		return c.Ref.Sym
	case *ir.FieldAccessExpr:
		if c.ResolvedSym != 0 {
			return c.ResolvedSym
		}
	}
	return 0
}

func (p *PassPropagateAsync) checkNoAsync(pc *PassContext, pkg *ir.PackageContext, e ir.Expr, context string) {
	if p.exprCallsAsync(pc, pkg, e) {
		pc.Diag.Report(diag.ErrAsyncInSyncContext, e.Span(), context)
	}
}
