package lsp

import (
	"sova/internal/diag"
	"sova/internal/ir"
	"sova/internal/services/compiler"
)

type referenceHit struct {
	file   string
	span   diag.TextSpan
	isDecl bool
}

func collectReferences(c *compiler.CompilerContext, sym ir.SymID) []referenceHit {
	var hits []referenceHit
	for _, pkg := range c.Packages {
		for _, f := range pkg.Files {
			if f.Hir == nil {
				continue
			}

			collectFileReferences(f.Hir, sym, &hits)
		}
	}

	return hits
}

func collectFileReferences(f *ir.File, sym ir.SymID, hits *[]referenceHit) {
	for _, st := range f.Statements {
		collectStmtRefs(st, sym, f.Path, hits)
	}
}

func collectStmtRefs(s ir.Stmt, sym ir.SymID, file string, hits *[]referenceHit) {
	if s == nil {
		return
	}

	switch n := s.(type) {
	case *ir.BlockStmt:
		for _, ss := range n.Stmts {
			collectStmtRefs(ss, sym, file, hits)
		}

	case *ir.VarDeclStmt:
		for _, tgt := range n.Targets {
			if tgt.Name != nil && tgt.Name.Sym == sym {
				*hits = append(*hits, referenceHit{file: file, span: tgt.Name.Span, isDecl: true})
			}
		}

		collectExprRefs(n.Init, sym, file, hits)
	case *ir.ExprStmt:
		collectExprRefs(n.Expr, sym, file, hits)
	case *ir.FieldAssignmentStmt:
		if n.Receiver.Sym == sym {
			*hits = append(*hits, referenceHit{file: file, span: n.Receiver.Span})
		}

		collectExprRefs(n.Value, sym, file, hits)
	case *ir.MultiAssignmentStmt:
		for _, tgt := range n.Targets {
			if tgt.Name != nil && tgt.Name.Sym == sym {
				*hits = append(*hits, referenceHit{file: file, span: tgt.Name.Span, isDecl: true})
			}
		}

		collectExprRefs(n.Value, sym, file, hits)
	case *ir.IfStmt:
		collectExprRefs(n.Cond, sym, file, hits)
		if n.Then != nil {
			for _, ss := range ir.BlockStmts(n.Then) {
				collectStmtRefs(ss, sym, file, hits)
			}
		}

		for _, eb := range n.ElseIfs {
			collectExprRefs(eb.Cond, sym, file, hits)
			if eb.Then != nil {
				for _, ss := range ir.BlockStmts(eb.Then) {
					collectStmtRefs(ss, sym, file, hits)
				}
			}
		}

		if n.Else != nil {
			for _, ss := range ir.BlockStmts(n.Else) {
				collectStmtRefs(ss, sym, file, hits)
			}
		}

	case *ir.ReturnStmt:
		for _, r := range n.Results {
			collectExprRefs(r, sym, file, hits)
		}

	case *ir.GuardStmt:
		collectExprRefs(n.Cond, sym, file, hits)
		for _, r := range n.Returns {
			collectExprRefs(r, sym, file, hits)
		}

	case *ir.ForStmt:
		if n.CondInt != nil {
			if n.CondInt.Init != nil {
				collectExprRefs(n.CondInt.Init.Init, sym, file, hits)
			}

			collectExprRefs(n.CondInt.Cond, sym, file, hits)
			collectExprRefs(n.CondInt.Post, sym, file, hits)
		}

		if n.CondIn != nil {
			collectExprRefs(n.CondIn.IterExpr, sym, file, hits)
		}

		if n.CondRange != nil {
			collectExprRefs(n.CondRange.RangeStart, sym, file, hits)
			collectExprRefs(n.CondRange.RangeEnd, sym, file, hits)
		}

		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				collectStmtRefs(ss, sym, file, hits)
			}
		}

	case *ir.WhileStmt:
		collectExprRefs(n.Cond, sym, file, hits)
		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				collectStmtRefs(ss, sym, file, hits)
			}
		}

	case *ir.FuncDeclStmt:
		if n.Name.Sym == sym {
			*hits = append(*hits, referenceHit{file: file, span: n.Name.Span, isDecl: true})
		}

		for _, param := range n.Params {
			if param.Name.Sym == sym {
				*hits = append(*hits, referenceHit{file: file, span: param.Name.Span, isDecl: true})
			}
		}

		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				collectStmtRefs(ss, sym, file, hits)
			}
		}

	case *ir.TypeDeclStmt:
		if n.Name.Sym == sym {
			*hits = append(*hits, referenceHit{file: file, span: n.Name.Span, isDecl: true})
		}

		for _, ctor := range n.Ctors {
			for _, param := range ctor.Params {
				if param.Name.Sym == sym {
					*hits = append(*hits, referenceHit{file: file, span: param.Name.Span, isDecl: true})
				}
			}

			if ctor.Body != nil {
				for _, ss := range ir.BlockStmts(ctor.Body) {
					collectStmtRefs(ss, sym, file, hits)
				}
			}
		}

		for _, m := range n.Methods {
			collectStmtRefs(m.Func, sym, file, hits)
		}

	case *ir.EnumDeclStmt:
		if n.Name.Sym == sym {
			*hits = append(*hits, referenceHit{file: file, span: n.Name.Span, isDecl: true})
		}

	case *ir.InterfaceDeclStmt:
		if n.Name.Sym == sym {
			*hits = append(*hits, referenceHit{file: file, span: n.Name.Span, isDecl: true})
		}

	case *ir.GoStmt:
		if n.Call != nil {
			collectExprRefs(n.Call, sym, file, hits)
		}

		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				collectStmtRefs(ss, sym, file, hits)
			}
		}

	case *ir.DeferStmt:
		if n.Call != nil {
			collectExprRefs(n.Call, sym, file, hits)
		}

		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				collectStmtRefs(ss, sym, file, hits)
			}
		}

	case *ir.SelectStmt:
		for _, cc := range n.Cases {
			collectExprRefs(cc.ChanExpr, sym, file, hits)
			collectExprRefs(cc.SendValue, sym, file, hits)
			if cc.Body != nil {
				for _, ss := range ir.BlockStmts(cc.Body) {
					collectStmtRefs(ss, sym, file, hits)
				}
			}
		}

		if n.Default != nil {
			for _, ss := range n.Default.Stmts {
				collectStmtRefs(ss, sym, file, hits)
			}
		}

	case *ir.AssertStmt:
		collectExprRefs(n.Expr, sym, file, hits)
	case *ir.TestDeclStmt:
		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				collectStmtRefs(ss, sym, file, hits)
			}
		}
	}
}

func collectExprRefs(e ir.Expr, sym ir.SymID, file string, hits *[]referenceHit) {
	if e == nil {
		return
	}

	switch n := e.(type) {
	case *ir.VarRef:
		if n.Ref.Sym == sym {
			*hits = append(*hits, referenceHit{file: file, span: n.Ref.Span})
		}

	case *ir.FieldAccessExpr:
		collectExprRefs(n.Expr, sym, file, hits)
		if n.ResolvedSym == sym && len(n.Fields) > 0 {
			*hits = append(*hits, referenceHit{file: file, span: n.Fields[len(n.Fields)-1].Span})
		}

	case *ir.FuncCallExpr:
		collectExprRefs(n.Callee, sym, file, hits)
		for _, arg := range n.Args {
			collectExprRefs(arg.Expr, sym, file, hits)
		}

	case *ir.BinaryExpr:
		collectExprRefs(n.Left, sym, file, hits)
		collectExprRefs(n.Right, sym, file, hits)
	case *ir.UnaryExpr:
		collectExprRefs(n.Expr, sym, file, hits)
	case *ir.PrefixUnaryExpr:
		collectExprRefs(n.Expr, sym, file, hits)
	case *ir.PostfixUnaryExpr:
		collectExprRefs(n.Expr, sym, file, hits)
	case *ir.GroupedExpr:
		collectExprRefs(n.Expr, sym, file, hits)
	case *ir.AssignmentExpr:
		if n.Left.Sym == sym {
			*hits = append(*hits, referenceHit{file: file, span: n.Left.Span})
		}

		collectExprRefs(n.Right, sym, file, hits)
	case *ir.IndexExpr:
		collectExprRefs(n.Expr, sym, file, hits)
		collectExprRefs(n.Index, sym, file, hits)
	case *ir.TenaryExpr:
		collectExprRefs(n.Cond, sym, file, hits)
		collectExprRefs(n.Then, sym, file, hits)
		collectExprRefs(n.Else, sym, file, hits)
	case *ir.CoalesceExpr:
		collectExprRefs(n.Left, sym, file, hits)
		collectExprRefs(n.Default, sym, file, hits)
	case *ir.FuncLitExpr:
		for _, param := range n.Params {
			if param.Name.Sym == sym {
				*hits = append(*hits, referenceHit{file: file, span: param.Name.Span, isDecl: true})
			}
		}

		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				collectStmtRefs(ss, sym, file, hits)
			}
		}

	case *ir.NewExpr:
		if n.TypeName.Sym == sym {
			*hits = append(*hits, referenceHit{file: file, span: n.TypeName.Span})
		}

		for _, arg := range n.Args {
			collectExprRefs(arg.Expr, sym, file, hits)
		}

	case *ir.ArrayLiteral:
		for _, el := range n.Elems {
			collectExprRefs(el, sym, file, hits)
		}

	case *ir.MapLiteral:
		for _, kv := range n.Entries {
			collectExprRefs(kv.Key, sym, file, hits)
			collectExprRefs(kv.Value, sym, file, hits)
		}

	case *ir.TupleLiteral:
		for _, el := range n.Elems {
			collectExprRefs(el, sym, file, hits)
		}

	case *ir.RangeExpr:
		collectExprRefs(n.Start, sym, file, hits)
		collectExprRefs(n.End, sym, file, hits)
		collectExprRefs(n.Inc, sym, file, hits)
	case *ir.WhenExpr:
		collectExprRefs(n.Expr, sym, file, hits)
		for _, cc := range n.Cases {
			for _, v := range cc.Values {
				collectExprRefs(v, sym, file, hits)
			}

			collectExprRefs(cc.Then, sym, file, hits)
		}

		collectExprRefs(n.Default, sym, file, hits)
	}
}
