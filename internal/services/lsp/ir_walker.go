package lsp

import (
	"sova/internal/diag"
	"sova/internal/ir"
)

func walkStmt(t *cursorTarget, s ir.Stmt, p position) bool {
	if s == nil {
		return false
	}

	switch n := s.(type) {
	case *ir.BlockStmt:
		for _, ss := range n.Stmts {
			if walkStmt(t, ss, p) {
				return true
			}
		}

	case *ir.VarDeclStmt:
		for _, tgt := range n.Targets {
			if tgt.Name != nil && p.inSpan(tgt.Name.Span) {
				setSymTarget(t, tgt.Name.Sym, tgt.Name.Span, cursorKindDecl)
				return true
			}

			if tgt.TypeAnn != nil && walkTypeRef(t, tgt.TypeAnn, p) {
				return true
			}
		}

		if n.Init != nil && walkExpr(t, n.Init, p) {
			return true
		}

	case *ir.ExprStmt:
		return walkExpr(t, n.Expr, p)
	case *ir.FieldAssignmentStmt:
		if n.Receiver.Span.StartLn != 0 && p.inSpan(n.Receiver.Span) {
			setSymTarget(t, n.Receiver.Sym, n.Receiver.Span, cursorKindSymbol)
			return true
		}

		recvTyp := ir.TypID(0)
		if t.pkg != nil && n.Receiver.Sym != 0 {
			if recvSym, ok := t.pkg.Syms.GetByID(n.Receiver.Sym); ok {
				recvTyp = recvSym.Typ
			}
		}

		cur := recvTyp
		for _, fld := range n.Fields {
			if p.inSpan(fld.Span) {
				t.kind = cursorKindMember
				t.span = fld.Span
				t.fieldName = fld.Name
				t.memberOf = cur
				t.typ = fieldTypeOnStruct(t.pkg, cur, fld.Name)
				return true
			}

			cur = fieldTypeOnStruct(t.pkg, cur, fld.Name)
		}

		if walkExpr(t, n.Value, p) {
			return true
		}

	case *ir.MultiAssignmentStmt:
		for _, tgt := range n.Targets {
			if tgt.Name != nil && p.inSpan(tgt.Name.Span) {
				setSymTarget(t, tgt.Name.Sym, tgt.Name.Span, cursorKindDecl)
				return true
			}
		}

		if walkExpr(t, n.Value, p) {
			return true
		}

	case *ir.IfStmt:
		if walkExpr(t, n.Cond, p) {
			return true
		}

		if n.Then != nil {
			for _, ss := range ir.BlockStmts(n.Then) {
				if walkStmt(t, ss, p) {
					return true
				}
			}
		}

		for _, eb := range n.ElseIfs {
			if walkExpr(t, eb.Cond, p) {
				return true
			}

			if eb.Then != nil {
				for _, ss := range ir.BlockStmts(eb.Then) {
					if walkStmt(t, ss, p) {
						return true
					}
				}
			}
		}

		if n.Else != nil {
			for _, ss := range ir.BlockStmts(n.Else) {
				if walkStmt(t, ss, p) {
					return true
				}
			}
		}

	case *ir.ReturnStmt:
		for _, r := range n.Results {
			if walkExpr(t, r, p) {
				return true
			}
		}

	case *ir.GuardStmt:
		if walkExpr(t, n.Cond, p) {
			return true
		}

		for _, r := range n.Returns {
			if walkExpr(t, r, p) {
				return true
			}
		}

	case *ir.ForStmt:
		if n.CondInt != nil && n.CondInt.Init != nil {
			if n.CondInt.Init.Init != nil && walkExpr(t, n.CondInt.Init.Init, p) {
				return true
			}
		}

		if n.CondInt != nil {
			if walkExpr(t, n.CondInt.Cond, p) {
				return true
			}

			if walkExpr(t, n.CondInt.Post, p) {
				return true
			}
		}

		if n.CondIn != nil && walkExpr(t, n.CondIn.IterExpr, p) {
			return true
		}

		if n.CondRange != nil {
			if walkExpr(t, n.CondRange.RangeStart, p) {
				return true
			}

			if walkExpr(t, n.CondRange.RangeEnd, p) {
				return true
			}
		}

		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				if walkStmt(t, ss, p) {
					return true
				}
			}
		}

	case *ir.WhileStmt:
		if walkExpr(t, n.Cond, p) {
			return true
		}

		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				if walkStmt(t, ss, p) {
					return true
				}
			}
		}

	case *ir.FuncDeclStmt:
		if p.inSpan(n.Name.Span) {
			setSymTarget(t, n.Name.Sym, n.Name.Span, cursorKindDecl)
			return true
		}

		for _, param := range n.Params {
			if p.inSpan(param.Name.Span) {
				setSymTarget(t, param.Name.Sym, param.Name.Span, cursorKindDecl)
				return true
			}

			if param.Type != nil && walkTypeRef(t, param.Type, p) {
				return true
			}
		}

		if n.ReturnType != nil && walkTypeRef(t, n.ReturnType, p) {
			return true
		}

		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				if walkStmt(t, ss, p) {
					return true
				}
			}
		}

	case *ir.TypeDeclStmt:
		if p.inSpan(n.Name.Span) {
			setSymTarget(t, n.Name.Sym, n.Name.Span, cursorKindDecl)
			return true
		}

		for _, fld := range n.Fields {
			if p.inSpan(fld.Name.Span) {
				setSymTarget(t, fld.Name.Sym, fld.Name.Span, cursorKindDecl)
				return true
			}

			if fld.Type != nil && walkTypeRef(t, fld.Type, p) {
				return true
			}

			if fld.Default != nil && walkExpr(t, fld.Default, p) {
				return true
			}
		}

		for _, ctor := range n.Ctors {
			for _, param := range ctor.Params {
				if p.inSpan(param.Name.Span) {
					setSymTarget(t, param.Name.Sym, param.Name.Span, cursorKindDecl)
					return true
				}

				if param.Type != nil && walkTypeRef(t, param.Type, p) {
					return true
				}
			}

			if ctor.Body != nil {
				for _, ss := range ir.BlockStmts(ctor.Body) {
					if walkStmt(t, ss, p) {
						return true
					}
				}
			}
		}

		for _, m := range n.Methods {
			if walkStmt(t, m.Func, p) {
				return true
			}
		}

	case *ir.ImportStmt:
		if p.inSpan(n.Span()) {
			t.kind = cursorKindImportPath
			t.span = n.Span()
			t.importPath = n.Path.String()
			return true
		}

	case *ir.InterfaceDeclStmt:
		if p.inSpan(n.Name.Span) {
			setSymTarget(t, n.Name.Sym, n.Name.Span, cursorKindDecl)
			return true
		}

		for _, sig := range n.Methods {
			if p.inSpan(sig.Name.Span) {
				setSymTarget(t, sig.Name.Sym, sig.Name.Span, cursorKindDecl)
				return true
			}
		}

	case *ir.EnumDeclStmt:
		if p.inSpan(n.Name.Span) {
			setSymTarget(t, n.Name.Sym, n.Name.Span, cursorKindDecl)
			return true
		}

		for _, c := range n.Cases {
			if p.inSpan(c.Name.Span) {
				setSymTarget(t, 0, c.Name.Span, cursorKindDecl)
				return true
			}
		}

		for _, m := range n.Methods {
			if walkStmt(t, m, p) {
				return true
			}
		}

	case *ir.GoStmt:
		if n.Call != nil && walkExpr(t, n.Call, p) {
			return true
		}

		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				if walkStmt(t, ss, p) {
					return true
				}
			}
		}

	case *ir.DeferStmt:
		if n.Call != nil && walkExpr(t, n.Call, p) {
			return true
		}

		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				if walkStmt(t, ss, p) {
					return true
				}
			}
		}

	case *ir.SelectStmt:
		for _, cc := range n.Cases {
			if cc.ChanExpr != nil && walkExpr(t, cc.ChanExpr, p) {
				return true
			}

			if cc.SendValue != nil && walkExpr(t, cc.SendValue, p) {
				return true
			}

			if cc.Body != nil {
				for _, ss := range ir.BlockStmts(cc.Body) {
					if walkStmt(t, ss, p) {
						return true
					}
				}
			}
		}

		if n.Default != nil {
			for _, ss := range n.Default.Stmts {
				if walkStmt(t, ss, p) {
					return true
				}
			}
		}

	case *ir.AssertStmt:
		return walkExpr(t, n.Expr, p)
	case *ir.TestDeclStmt:
		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				if walkStmt(t, ss, p) {
					return true
				}
			}
		}
	}

	return false
}

func walkExpr(t *cursorTarget, e ir.Expr, p position) bool {
	if e == nil {
		return false
	}

	switch n := e.(type) {
	case *ir.VarRef:
		if p.inSpan(n.Ref.Span) {
			setSymTarget(t, n.Ref.Sym, n.Ref.Span, cursorKindSymbol)
			t.typ = n.GetType()
			return true
		}

	case *ir.FieldAccessExpr:
		if walkExpr(t, n.Expr, p) {
			return true
		}

		recvTyp := n.Expr.GetType()
		for i, fld := range n.Fields {
			if p.inSpan(fld.Span) {
				t.kind = cursorKindMember
				t.span = fld.Span
				t.fieldName = fld.Name
				t.typ = n.GetType()
				t.memberOf = recvTyp
				if i == len(n.Fields)-1 && n.ResolvedSym != 0 {
					t.sym = n.ResolvedSym
				}

				return true
			}

			recvTyp = 0
		}

	case *ir.FuncCallExpr:
		if walkExpr(t, n.Callee, p) {
			return true
		}

		for _, arg := range n.Args {
			if walkExpr(t, arg.Expr, p) {
				return true
			}
		}

	case *ir.BinaryExpr:
		if walkExpr(t, n.Left, p) {
			return true
		}

		if walkExpr(t, n.Right, p) {
			return true
		}

	case *ir.UnaryExpr:
		if walkExpr(t, n.Expr, p) {
			return true
		}

	case *ir.PrefixUnaryExpr:
		if walkExpr(t, n.Expr, p) {
			return true
		}

	case *ir.PostfixUnaryExpr:
		if walkExpr(t, n.Expr, p) {
			return true
		}

	case *ir.GroupedExpr:
		if walkExpr(t, n.Expr, p) {
			return true
		}

	case *ir.AssignmentExpr:
		if p.inSpan(n.Left.Span) {
			setSymTarget(t, n.Left.Sym, n.Left.Span, cursorKindSymbol)
			return true
		}

		if walkExpr(t, n.Right, p) {
			return true
		}

	case *ir.IndexExpr:
		if walkExpr(t, n.Expr, p) {
			return true
		}

		if walkExpr(t, n.Index, p) {
			return true
		}

	case *ir.TenaryExpr:
		if walkExpr(t, n.Cond, p) {
			return true
		}

		if walkExpr(t, n.Then, p) {
			return true
		}

		if walkExpr(t, n.Else, p) {
			return true
		}

	case *ir.CoalesceExpr:
		if walkExpr(t, n.Left, p) {
			return true
		}

		if walkExpr(t, n.Default, p) {
			return true
		}

	case *ir.FuncLitExpr:
		for _, param := range n.Params {
			if p.inSpan(param.Name.Span) {
				setSymTarget(t, param.Name.Sym, param.Name.Span, cursorKindDecl)
				return true
			}
		}

		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				if walkStmt(t, ss, p) {
					return true
				}
			}
		}

	case *ir.NewExpr:
		if p.inSpan(n.TypeName.Span) {
			setSymTarget(t, n.TypeName.Sym, n.TypeName.Span, cursorKindSymbol)
			t.typ = n.GetType()
			return true
		}

		for _, arg := range n.Args {
			if arg.Expr != nil && walkExpr(t, arg.Expr, p) {
				return true
			}
		}

	case *ir.ArrayLiteral:
		for _, el := range n.Elems {
			if walkExpr(t, el, p) {
				return true
			}
		}

	case *ir.MapLiteral:
		for _, kv := range n.Entries {
			if walkExpr(t, kv.Key, p) {
				return true
			}

			if walkExpr(t, kv.Value, p) {
				return true
			}
		}

	case *ir.TupleLiteral:
		for _, el := range n.Elems {
			if walkExpr(t, el, p) {
				return true
			}
		}

	case *ir.RangeExpr:
		if walkExpr(t, n.Start, p) {
			return true
		}

		if walkExpr(t, n.End, p) {
			return true
		}

		if n.Inc != nil && walkExpr(t, n.Inc, p) {
			return true
		}

	case *ir.WhenExpr:
		if walkExpr(t, n.Expr, p) {
			return true
		}

		for _, c := range n.Cases {
			for _, v := range c.Values {
				if walkExpr(t, v, p) {
					return true
				}
			}

			if walkExpr(t, c.Then, p) {
				return true
			}
		}

		if walkExpr(t, n.Default, p) {
			return true
		}

	case *ir.SessionExpr:
		if p.inSpan(n.Span()) {
			t.kind = cursorKindSymbol
			t.span = n.Span()
			t.typ = n.GetType()
			t.fieldName = "@"
			return true
		}

	case *ir.AsExpr:
		if walkExpr(t, n.Expr, p) {
			return true
		}
	}

	return false
}

func setSymTarget(t *cursorTarget, sym ir.SymID, span diag.TextSpan, kind cursorKind) {
	t.sym = sym
	t.span = span
	t.kind = kind
}

func walkTypeRef(t *cursorTarget, tr *ir.TypeRef, p position) bool {
	if tr == nil {
		return false
	}

	if tr.Kind == ir.TK_Enum || tr.Kind == ir.TK_Struct || tr.Kind == ir.TK_Interface {
		if tr.CustomName != "" && p.inSpan(tr.Span()) {
			t.kind = cursorKindTypeRef
			t.span = tr.Span()
			t.typeRefName = tr.CustomName
			t.typeRefQualifier = tr.CustomQualifier
			t.typ = tr.Typ
			return true
		}
	}

	if tr.Elem != nil && walkTypeRef(t, tr.Elem, p) {
		return true
	}

	if tr.Key != nil && walkTypeRef(t, tr.Key, p) {
		return true
	}

	if tr.Value != nil && walkTypeRef(t, tr.Value, p) {
		return true
	}

	for _, tf := range tr.Tuple {
		if walkTypeRef(t, tf.Type, p) {
			return true
		}
	}

	for _, fp := range tr.FuncParams {
		if walkTypeRef(t, fp.Type, p) {
			return true
		}
	}

	if tr.FuncReturn != nil && walkTypeRef(t, tr.FuncReturn, p) {
		return true
	}

	for _, ta := range tr.TypeArgs {
		if walkTypeRef(t, ta, p) {
			return true
		}
	}

	return false
}

func fieldTypeOnStruct(pkg *ir.PackageContext, recvTyp ir.TypID, fieldName string) ir.TypID {
	if pkg == nil || recvTyp == 0 || fieldName == "" {
		return 0
	}

	ty, ok := pkg.Types.GetByID(recvTyp)
	if !ok || ty == nil {
		return 0
	}

	if ty.Kind == ir.TK_Struct {
		for _, f := range ty.StructFields {
			if f.Name == fieldName {
				return f.Type
			}
		}

		for _, m := range ty.StructMethods {
			if m.Name == fieldName {
				return m.FuncTyp
			}
		}
	}

	return 0
}
