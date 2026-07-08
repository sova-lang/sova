package ir

type WalkAction int

const (
	WalkContinue WalkAction = iota
	WalkSkip
	WalkStop
)

type VisitFunc func(Node) WalkAction

func Walk(root Node, visit VisitFunc) {
	walkNode(root, visit)
}

func WalkStmts(stmts []Stmt, visit VisitFunc) {
	for _, s := range stmts {
		if walkNode(s, visit) {
			return
		}
	}
}

func walkNode(n Node, visit VisitFunc) bool {
	if isNil(n) {
		return false
	}

	switch visit(n) {
	case WalkStop:
		return true
	case WalkSkip:
		return false
	}

	return walkChildren(n, visit)
}

func walkChildren(n Node, visit VisitFunc) bool {
	switch x := n.(type) {

	case *BlockStmt:
		for _, s := range x.Stmts {
			if walkNode(s, visit) {
				return true
			}
		}

	case *VarDeclStmt:
		return walkNode(x.Init, visit)
	case *ExprStmt:
		return walkNode(x.Expr, visit)
	case *FieldAssignmentStmt:
		return walkNode(x.Value, visit)
	case *IndexAssignmentStmt:
		if walkNode(x.Receiver, visit) || walkNode(x.Index, visit) {
			return true
		}

		return walkNode(x.Value, visit)
	case *MultiAssignmentStmt:
		return walkNode(x.Value, visit)
	case *IfStmt:
		if walkNode(x.Cond, visit) || walkNode(x.Then, visit) {
			return true
		}

		for _, ei := range x.ElseIfs {
			if walkNode(ei.Cond, visit) || walkNode(ei.Then, visit) {
				return true
			}
		}

		return walkNode(x.Else, visit)
	case *SwitchStmt:
		if walkNode(x.Expr, visit) {
			return true
		}

		for _, c := range x.Cases {
			for _, v := range c.Values {
				if walkNode(v, visit) {
					return true
				}
			}

			for _, s := range c.Stmts {
				if walkNode(s, visit) {
					return true
				}
			}
		}

		for _, s := range x.Default {
			if walkNode(s, visit) {
				return true
			}
		}

	case *BreakStmt, *ContinueStmt, *ImportStmt, *TypeAliasStmt, *WireRulesetStmt:

	case *ReturnStmt:
		for _, r := range x.Results {
			if walkNode(r, visit) {
				return true
			}
		}

	case *GuardStmt:
		if walkNode(x.Cond, visit) {
			return true
		}

		for _, r := range x.Returns {
			if walkNode(r, visit) {
				return true
			}
		}

	case *ForStmt:
		if x.CondInt != nil {
			if x.CondInt.Init != nil && walkNode(x.CondInt.Init, visit) {
				return true
			}

			if walkNode(x.CondInt.Cond, visit) || walkNode(x.CondInt.Post, visit) {
				return true
			}
		}

		if x.CondRange != nil {
			if walkNode(x.CondRange.RangeStart, visit) || walkNode(x.CondRange.RangeEnd, visit) {
				return true
			}
		}

		if x.CondIn != nil && walkNode(x.CondIn.IterExpr, visit) {
			return true
		}

		return walkNode(x.Body, visit)
	case *WhileStmt:
		if walkNode(x.Cond, visit) {
			return true
		}

		return walkNode(x.Body, visit)
	case *TestDeclStmt:
		return walkNode(x.Body, visit)
	case *GroupDeclStmt:
		for _, s := range x.Body {
			if walkNode(s, visit) {
				return true
			}
		}

	case *SetupStmt:
		return walkNode(x.Body, visit)
	case *TeardownStmt:
		return walkNode(x.Body, visit)
	case *AssertStmt:
		return walkNode(x.Expr, visit)
	case *AsSessionStmt:
		return walkNode(x.Body, visit)
	case *GoStmt:
		if walkNode(x.Call, visit) {
			return true
		}

		return walkNode(x.Body, visit)
	case *DeferStmt:
		if walkNode(x.Call, visit) {
			return true
		}

		return walkNode(x.Body, visit)
	case *SelectStmt:
		for _, c := range x.Cases {
			if c == nil {
				continue
			}

			if walkNode(c.ChanExpr, visit) || walkNode(c.SendValue, visit) || walkNode(c.Body, visit) {
				return true
			}
		}

		return walkNode(x.Default, visit)
	case *WireGroupStmt:
		for _, s := range x.Stmts {
			if walkNode(s, visit) {
				return true
			}
		}

	case *FuncDeclStmt:
		for _, p := range x.Params {
			if p != nil && walkNode(p.Default, visit) {
				return true
			}
		}

		return walkNode(x.Body, visit)
	case *TypeDeclStmt:
		for _, f := range x.Fields {
			if f != nil && walkNode(f.Default, visit) {
				return true
			}
		}

		for _, c := range x.Ctors {
			if c == nil {
				continue
			}

			for _, p := range c.Params {
				if p != nil && walkNode(p.Default, visit) {
					return true
				}
			}

			if walkNode(c.Body, visit) {
				return true
			}
		}

		for _, m := range x.Methods {
			if m != nil && m.Func != nil && walkNode(m.Func, visit) {
				return true
			}
		}

		for _, c := range x.Casts {
			if c == nil {
				continue
			}

			if c.Param != nil && walkNode(c.Param.Default, visit) {
				return true
			}

			if walkNode(c.Body, visit) {
				return true
			}
		}

	case *EnumDeclStmt:
		for _, f := range x.Fields {
			if f != nil && walkNode(f.Default, visit) {
				return true
			}
		}

		for _, c := range x.Cases {
			if c == nil {
				continue
			}

			for _, a := range c.Args {
				if walkNode(a, visit) {
					return true
				}
			}
		}

		for _, m := range x.Methods {
			if m != nil && walkNode(m, visit) {
				return true
			}
		}

	case *InterfaceDeclStmt:
		for _, m := range x.Methods {
			if m == nil {
				continue
			}

			for _, p := range m.Params {
				if p != nil && walkNode(p.Default, visit) {
					return true
				}
			}
		}

	case *MixinDeclStmt:
		for _, f := range x.Fields {
			if f != nil && walkNode(f.Default, visit) {
				return true
			}
		}

		for _, m := range x.Methods {
			if m != nil && m.Func != nil && walkNode(m.Func, visit) {
				return true
			}
		}

	case *ExternDeclStmt:
		for _, fn := range x.Funcs {
			if fn == nil {
				continue
			}

			for _, p := range fn.Params {
				if p != nil && walkNode(p.Default, visit) {
					return true
				}
			}
		}

		for _, t := range x.Types {
			if t != nil && walkNode(t, visit) {
				return true
			}
		}

		for _, i := range x.Interfaces {
			if i != nil && walkNode(i, visit) {
				return true
			}
		}

	case *CastDecl:
		if x.Param != nil && walkNode(x.Param.Default, visit) {
			return true
		}

		return walkNode(x.Body, visit)
	case *FuncParam:
		return walkNode(x.Default, visit)

	case *WhenExpr:
		if walkNode(x.Expr, visit) {
			return true
		}

		for _, c := range x.Cases {
			for _, v := range c.Values {
				if walkNode(v, visit) {
					return true
				}
			}

			if walkNode(c.Then, visit) {
				return true
			}
		}

		return walkNode(x.Default, visit)
	case *UnaryExpr:
		return walkNode(x.Expr, visit)
	case *PrefixUnaryExpr:
		return walkNode(x.Expr, visit)
	case *PostfixUnaryExpr:
		return walkNode(x.Expr, visit)
	case *BinaryExpr:
		if walkNode(x.Left, visit) {
			return true
		}

		return walkNode(x.Right, visit)
	case *CoalesceExpr:
		if walkNode(x.Left, visit) {
			return true
		}

		return walkNode(x.Default, visit)
	case *TenaryExpr:
		if walkNode(x.Cond, visit) || walkNode(x.Then, visit) {
			return true
		}

		return walkNode(x.Else, visit)
	case *GroupedExpr:
		return walkNode(x.Expr, visit)
	case *AsExpr:
		return walkNode(x.Expr, visit)
	case *InstanceofExpr:
		return walkNode(x.Expr, visit)
	case *OptionUnwrapExpr:
		return walkNode(x.Expr, visit)
	case *AssignmentExpr:
		return walkNode(x.Right, visit)
	case *IndexExpr:
		if walkNode(x.Expr, visit) {
			return true
		}

		return walkNode(x.Index, visit)
	case *SliceRangeExpr:
		if walkNode(x.Expr, visit) || walkNode(x.Low, visit) {
			return true
		}

		return walkNode(x.High, visit)
	case *FieldAccessExpr:
		return walkNode(x.Expr, visit)
	case *RangeExpr:
		if walkNode(x.Start, visit) || walkNode(x.End, visit) {
			return true
		}

		return walkNode(x.Inc, visit)
	case *FuncCallExpr:
		if walkNode(x.Callee, visit) {
			return true
		}

		for _, a := range x.Args {
			if walkNode(a.Expr, visit) {
				return true
			}
		}

	case *ComposableCallExpr:
		if walkNode(x.Callee, visit) {
			return true
		}

		for _, a := range x.Args {
			if walkNode(a.Expr, visit) {
				return true
			}
		}

		for _, ch := range x.Children {
			if walkNode(ch.Expr, visit) || walkNode(ch.Stmt, visit) {
				return true
			}
		}

	case *NewExpr:
		for _, a := range x.Args {
			if walkNode(a.Expr, visit) {
				return true
			}
		}

	case *ChanInitExpr:
		return walkNode(x.Capacity, visit)
	case *FuncLitExpr:
		for _, p := range x.Params {
			if p != nil && walkNode(p.Default, visit) {
				return true
			}
		}

		return walkNode(x.Body, visit)
	case *ArrayLiteral:
		for _, e := range x.Elems {
			if walkNode(e, visit) {
				return true
			}
		}

	case *MapLiteral:
		for _, en := range x.Entries {
			if walkNode(en.Key, visit) || walkNode(en.Value, visit) {
				return true
			}
		}

	case *TupleLiteral:
		for _, e := range x.Elems {
			if walkNode(e, visit) {
				return true
			}
		}

	case *StringTemplateExpr:
		for _, p := range x.Parts {
			if walkNode(p.Expr, visit) {
				return true
			}
		}

	case *VarRef,
		*SessionExpr,
		*EnumValueExpr,
		*LitInt,
		*LitFloat,
		*LitString,
		*LitChar,
		*LitBool,
		*LitNone:

	}

	return false
}

func isNil(n Node) bool {
	if n == nil {
		return true
	}

	switch x := n.(type) {
	case Expr:
		return IsNilExpr(x)
	case Stmt:
		return IsNilStmt(x)
	}

	return false
}
