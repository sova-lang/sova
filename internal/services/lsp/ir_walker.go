package lsp

import (
	"sova/internal/diag"
	"sova/internal/ir"
)

func walkStmt(t *cursorTarget, s ir.Stmt, p position) bool {
	return findCursor(t, s, p)
}

func walkExpr(t *cursorTarget, e ir.Expr, p position) bool {
	return findCursor(t, e, p)
}

func findCursor(t *cursorTarget, root ir.Node, p position) bool {
	if root == nil {
		return false
	}

	hit := false
	ir.Walk(root, func(n ir.Node) ir.WalkAction {
		if visitCursor(t, n, p) {
			hit = true
			return ir.WalkStop
		}

		return ir.WalkContinue
	})

	return hit
}

func visitCursor(t *cursorTarget, n ir.Node, p position) bool {
	switch x := n.(type) {

	case *ir.VarDeclStmt:
		for _, tgt := range x.Targets {
			if tgt.Name != nil && p.inSpan(tgt.Name.Span) {
				setSymTarget(t, tgt.Name.Sym, tgt.Name.Span, cursorKindDecl)
				return true
			}

			if tgt.TypeAnn != nil && walkTypeRef(t, tgt.TypeAnn, p) {
				return true
			}
		}

	case *ir.FieldAssignmentStmt:
		if x.Receiver.Span.StartLn != 0 && p.inSpan(x.Receiver.Span) {
			setSymTarget(t, x.Receiver.Sym, x.Receiver.Span, cursorKindSymbol)
			return true
		}

		recvTyp := ir.TypID(0)
		if t.pkg != nil && x.Receiver.Sym != 0 {
			if recvSym, ok := t.pkg.Syms.GetByID(x.Receiver.Sym); ok {
				recvTyp = recvSym.Typ
			}
		}

		cur := recvTyp
		for _, fld := range x.Fields {
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

	case *ir.MultiAssignmentStmt:
		for _, tgt := range x.Targets {
			if tgt.Name != nil && p.inSpan(tgt.Name.Span) {
				setSymTarget(t, tgt.Name.Sym, tgt.Name.Span, cursorKindDecl)
				return true
			}
		}

	case *ir.FuncDeclStmt:
		if p.inSpan(x.Name.Span) {
			setSymTarget(t, x.Name.Sym, x.Name.Span, cursorKindDecl)
			return true
		}

		for _, param := range x.Params {
			if p.inSpan(param.Name.Span) {
				setSymTarget(t, param.Name.Sym, param.Name.Span, cursorKindDecl)
				return true
			}

			if param.Type != nil && walkTypeRef(t, param.Type, p) {
				return true
			}
		}

		if x.ReturnType != nil && walkTypeRef(t, x.ReturnType, p) {
			return true
		}

	case *ir.TypeDeclStmt:
		if p.inSpan(x.Name.Span) {
			setSymTarget(t, x.Name.Sym, x.Name.Span, cursorKindDecl)
			return true
		}

		for _, fld := range x.Fields {
			if p.inSpan(fld.Name.Span) {
				setSymTarget(t, fld.Name.Sym, fld.Name.Span, cursorKindDecl)
				return true
			}

			if fld.Type != nil && walkTypeRef(t, fld.Type, p) {
				return true
			}
		}

		for _, ctor := range x.Ctors {
			for _, param := range ctor.Params {
				if p.inSpan(param.Name.Span) {
					setSymTarget(t, param.Name.Sym, param.Name.Span, cursorKindDecl)
					return true
				}

				if param.Type != nil && walkTypeRef(t, param.Type, p) {
					return true
				}
			}
		}

	case *ir.ImportStmt:
		if p.inSpan(x.Span()) {
			t.kind = cursorKindImportPath
			t.span = x.Span()
			t.importPath = x.Path.String()
			return true
		}

	case *ir.InterfaceDeclStmt:
		if p.inSpan(x.Name.Span) {
			setSymTarget(t, x.Name.Sym, x.Name.Span, cursorKindDecl)
			return true
		}

		for _, sig := range x.Methods {
			if p.inSpan(sig.Name.Span) {
				setSymTarget(t, sig.Name.Sym, sig.Name.Span, cursorKindDecl)
				return true
			}
		}

	case *ir.EnumDeclStmt:
		if p.inSpan(x.Name.Span) {
			setSymTarget(t, x.Name.Sym, x.Name.Span, cursorKindDecl)
			return true
		}

		for _, c := range x.Cases {
			if p.inSpan(c.Name.Span) {
				setSymTarget(t, 0, c.Name.Span, cursorKindDecl)
				return true
			}
		}

	case *ir.VarRef:
		if p.inSpan(x.Ref.Span) {
			setSymTarget(t, x.Ref.Sym, x.Ref.Span, cursorKindSymbol)
			t.typ = x.GetType()
			return true
		}

	case *ir.FieldAccessExpr:
		recvTyp := ir.TypID(0)
		if x.Expr != nil {
			recvTyp = x.Expr.GetType()
		}

		for i, fld := range x.Fields {
			if p.inSpan(fld.Span) {
				t.kind = cursorKindMember
				t.span = fld.Span
				t.fieldName = fld.Name
				t.typ = x.GetType()
				t.memberOf = recvTyp
				if i == len(x.Fields)-1 && x.ResolvedSym != 0 {
					t.sym = x.ResolvedSym
				}

				return true
			}

			recvTyp = 0
		}

	case *ir.AssignmentExpr:
		if p.inSpan(x.Left.Span) {
			setSymTarget(t, x.Left.Sym, x.Left.Span, cursorKindSymbol)
			return true
		}

	case *ir.FuncLitExpr:
		for _, param := range x.Params {
			if p.inSpan(param.Name.Span) {
				setSymTarget(t, param.Name.Sym, param.Name.Span, cursorKindDecl)
				return true
			}
		}

	case *ir.NewExpr:
		if p.inSpan(x.TypeName.Span) {
			setSymTarget(t, x.TypeName.Sym, x.TypeName.Span, cursorKindSymbol)
			t.typ = x.GetType()
			return true
		}

	case *ir.SessionExpr:
		if p.inSpan(x.Span()) {
			t.kind = cursorKindSymbol
			t.span = x.Span()
			t.typ = x.GetType()
			t.fieldName = "@"
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
		for _, f := range ty.Struct.Fields {
			if f.Name == fieldName {
				return f.Type
			}
		}

		for _, m := range ty.Struct.Methods {
			if m.Name == fieldName {
				return m.FuncTyp
			}
		}
	}

	return 0
}
