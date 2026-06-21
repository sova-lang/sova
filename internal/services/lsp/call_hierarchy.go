package lsp

import (
	"context"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"sova/internal/diag"
	"sova/internal/ir"
	"sova/internal/services/compiler"
)

func (s *Server) PrepareCallHierarchy(ctx context.Context, params *protocol.CallHierarchyPrepareParams) ([]protocol.CallHierarchyItem, error) {
	return withCursor(s, params.TextDocument.URI, params.Position, func(snap *Snapshot, c *compiler.CompilerContext, target *cursorTarget) ([]protocol.CallHierarchyItem, error) {
		if target.sym == 0 {
			return nil, nil
		}

		sym, _ := lookupSymbol(c, target.sym)
		if sym == nil || sym.Kind != ir.SK_Function {
			return nil, nil
		}

		item := buildCallHierarchyItem(c, snap, target.sym)
		if item == nil {
			return nil, nil
		}

		return []protocol.CallHierarchyItem{*item}, nil
	})
}

func (s *Server) IncomingCalls(ctx context.Context, params *protocol.CallHierarchyIncomingCallsParams) ([]protocol.CallHierarchyIncomingCall, error) {
	return withSnapshot(s, func(snap *Snapshot, c *compiler.CompilerContext) ([]protocol.CallHierarchyIncomingCall, error) {
		targetSym, ok := decodeCallHierarchySym(params.Item.Data)
		if !ok {
			return nil, nil
		}

		type acc struct {
			ranges    []protocol.Range
			callerSym ir.SymID
		}

		byCaller := map[ir.SymID]*acc{}

		for _, pkg := range c.Packages {
			for _, f := range pkg.Files {
				if f.Hir == nil {
					continue
				}

				for _, st := range f.Hir.Statements {
					fn, ok := st.(*ir.FuncDeclStmt)
					if !ok || fn.Body == nil {
						continue
					}

					if fn.Name.Sym == targetSym {
						continue
					}

					var sites []diag.TextSpan
					collectCallSites(fn.Body, targetSym, &sites)
					if len(sites) == 0 {
						continue
					}

					ranges := make([]protocol.Range, len(sites))
					for i, s := range sites {
						ranges[i] = spanToRange(s)
					}

					entry, exists := byCaller[fn.Name.Sym]
					if !exists {
						entry = &acc{callerSym: fn.Name.Sym}

						byCaller[fn.Name.Sym] = entry
					}

					entry.ranges = append(entry.ranges, ranges...)
				}
			}
		}

		var out []protocol.CallHierarchyIncomingCall
		for _, entry := range byCaller {
			item := buildCallHierarchyItem(c, snap, entry.callerSym)
			if item == nil {
				continue
			}

			out = append(out, protocol.CallHierarchyIncomingCall{
				From:       *item,
				FromRanges: entry.ranges,
			})
		}

		return out, nil
	})
}

type callSiteAcc struct {
	ranges []protocol.Range
}

func (s *Server) OutgoingCalls(ctx context.Context, params *protocol.CallHierarchyOutgoingCallsParams) ([]protocol.CallHierarchyOutgoingCall, error) {
	return withSnapshot(s, func(snap *Snapshot, c *compiler.CompilerContext) ([]protocol.CallHierarchyOutgoingCall, error) {
		targetSym, ok := decodeCallHierarchySym(params.Item.Data)
		if !ok {
			return nil, nil
		}

		fn := findFuncDeclForSym(c, targetSym)
		if fn == nil || fn.Body == nil {
			return nil, nil
		}

		byCallee := map[ir.SymID]*callSiteAcc{}

		collectOutgoingCalls(fn.Body, c, byCallee)
		var out []protocol.CallHierarchyOutgoingCall
		for sym, entry := range byCallee {
			item := buildCallHierarchyItem(c, snap, sym)
			if item == nil {
				continue
			}

			out = append(out, protocol.CallHierarchyOutgoingCall{
				To:         *item,
				FromRanges: entry.ranges,
			})
		}

		return out, nil
	})
}

func buildCallHierarchyItem(c *compiler.CompilerContext, snap *Snapshot, symID ir.SymID) *protocol.CallHierarchyItem {
	sym, _ := lookupSymbol(c, symID)
	if sym == nil {
		return nil
	}

	fn := findFuncDeclForSym(c, symID)
	if fn == nil {
		return nil
	}

	span := fn.Span()
	nameSpan := fn.Name.Span
	u := uriForSpan(c, snap, span)
	if u == "" {
		return nil
	}

	detail := ""
	if fnTy, ok := c.TypeUniverse.GetByID(sym.Typ); ok && fnTy.Kind == ir.TK_Function {
		detail = "func " + sym.Name + "(" + funcTypeParamList(c.TypeUniverse, fnTy) + ")"
		if fnTy.Func.ReturnType != 0 {
			detail += ": " + formatType(c.TypeUniverse, fnTy.Func.ReturnType)
		}
	}

	return &protocol.CallHierarchyItem{
		Name:           sym.Name,
		Kind:           protocol.SymbolKindFunction,
		Detail:         detail,
		URI:            uri.URI(u),
		Range:          spanToRange(span),
		SelectionRange: spanToRange(nameSpan),
		Data:           map[string]interface{}{"sym": float64(symID)},
	}
}

func decodeCallHierarchySym(data interface{}) (ir.SymID, bool) {
	m, ok := data.(map[string]interface{})
	if !ok {
		return 0, false
	}

	raw, ok := m["sym"]
	if !ok {
		return 0, false
	}

	switch v := raw.(type) {
	case float64:
		return ir.SymID(v), true
	case int:
		return ir.SymID(v), true
	case int64:
		return ir.SymID(v), true
	}

	return 0, false
}

func findFuncDeclForSym(c *compiler.CompilerContext, sym ir.SymID) *ir.FuncDeclStmt {
	for _, pkg := range c.Packages {
		for _, f := range pkg.Files {
			if f.Hir == nil {
				continue
			}

			for _, st := range f.Hir.Statements {
				switch n := st.(type) {
				case *ir.FuncDeclStmt:
					if n.Name.Sym == sym {
						return n
					}

				case *ir.TypeDeclStmt:
					for _, m := range n.Methods {
						if m.Func != nil && m.Func.Name.Sym == sym {
							return m.Func
						}
					}
				}
			}
		}
	}

	return nil
}

func collectCallSites(b *ir.BlockStmt, target ir.SymID, out *[]diag.TextSpan) {
	if b == nil {
		return
	}

	for _, st := range b.Stmts {
		callSitesInStmt(st, target, out)
	}
}

func callSitesInStmt(s ir.Stmt, target ir.SymID, out *[]diag.TextSpan) {
	if s == nil {
		return
	}

	switch n := s.(type) {
	case *ir.BlockStmt:
		for _, ss := range n.Stmts {
			callSitesInStmt(ss, target, out)
		}

	case *ir.VarDeclStmt:
		callSitesInExpr(n.Init, target, out)
	case *ir.ExprStmt:
		callSitesInExpr(n.Expr, target, out)
	case *ir.FieldAssignmentStmt:
		callSitesInExpr(n.Value, target, out)
	case *ir.MultiAssignmentStmt:
		callSitesInExpr(n.Value, target, out)
	case *ir.IfStmt:
		callSitesInExpr(n.Cond, target, out)
		collectCallSites(n.Then, target, out)
		for _, eb := range n.ElseIfs {
			callSitesInExpr(eb.Cond, target, out)
			collectCallSites(eb.Then, target, out)
		}

		collectCallSites(n.Else, target, out)
	case *ir.ReturnStmt:
		for _, r := range n.Results {
			callSitesInExpr(r, target, out)
		}

	case *ir.ForStmt:
		collectCallSites(n.Body, target, out)
	case *ir.WhileStmt:
		callSitesInExpr(n.Cond, target, out)
		collectCallSites(n.Body, target, out)
	case *ir.GoStmt:
		callSitesInExpr(n.Call, target, out)
		collectCallSites(n.Body, target, out)
	case *ir.DeferStmt:
		callSitesInExpr(n.Call, target, out)
		collectCallSites(n.Body, target, out)
	case *ir.SelectStmt:
		for _, cc := range n.Cases {
			callSitesInExpr(cc.ChanExpr, target, out)
			callSitesInExpr(cc.SendValue, target, out)
			collectCallSites(cc.Body, target, out)
		}

		collectCallSites(n.Default, target, out)
	case *ir.AssertStmt:
		callSitesInExpr(n.Expr, target, out)
	}
}

func callSitesInExpr(e ir.Expr, target ir.SymID, out *[]diag.TextSpan) {
	if e == nil {
		return
	}

	switch n := e.(type) {
	case *ir.FuncCallExpr:
		if sym := calleeSymbolFor(n.Callee); sym == target {
			*out = append(*out, calleeNameSpan(n.Callee))
		}

		callSitesInExpr(n.Callee, target, out)
		for _, arg := range n.Args {
			callSitesInExpr(arg.Expr, target, out)
		}

	case *ir.BinaryExpr:
		callSitesInExpr(n.Left, target, out)
		callSitesInExpr(n.Right, target, out)
	case *ir.GroupedExpr:
		callSitesInExpr(n.Expr, target, out)
	case *ir.UnaryExpr:
		callSitesInExpr(n.Expr, target, out)
	case *ir.PrefixUnaryExpr:
		callSitesInExpr(n.Expr, target, out)
	case *ir.PostfixUnaryExpr:
		callSitesInExpr(n.Expr, target, out)
	case *ir.AssignmentExpr:
		callSitesInExpr(n.Right, target, out)
	case *ir.FieldAccessExpr:
		callSitesInExpr(n.Expr, target, out)
	case *ir.IndexExpr:
		callSitesInExpr(n.Expr, target, out)
		callSitesInExpr(n.Index, target, out)
	case *ir.TenaryExpr:
		callSitesInExpr(n.Cond, target, out)
		callSitesInExpr(n.Then, target, out)
		callSitesInExpr(n.Else, target, out)
	case *ir.CoalesceExpr:
		callSitesInExpr(n.Left, target, out)
		callSitesInExpr(n.Default, target, out)
	case *ir.NewExpr:
		for _, arg := range n.Args {
			callSitesInExpr(arg.Expr, target, out)
		}

	case *ir.FuncLitExpr:
		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				callSitesInStmt(ss, target, out)
			}
		}

	case *ir.ArrayLiteral:
		for _, el := range n.Elems {
			callSitesInExpr(el, target, out)
		}

	case *ir.MapLiteral:
		for _, kv := range n.Entries {
			callSitesInExpr(kv.Key, target, out)
			callSitesInExpr(kv.Value, target, out)
		}

	case *ir.TupleLiteral:
		for _, el := range n.Elems {
			callSitesInExpr(el, target, out)
		}

	case *ir.WhenExpr:
		callSitesInExpr(n.Expr, target, out)
		for _, c := range n.Cases {
			for _, v := range c.Values {
				callSitesInExpr(v, target, out)
			}

			callSitesInExpr(c.Then, target, out)
		}

		callSitesInExpr(n.Default, target, out)
	}
}

func calleeNameSpan(callee ir.Expr) diag.TextSpan {
	switch c := callee.(type) {
	case *ir.VarRef:
		return c.Ref.Span
	case *ir.FieldAccessExpr:
		if len(c.Fields) > 0 {
			return c.Fields[len(c.Fields)-1].Span
		}
	}

	return callee.Span()
}

func collectOutgoingCalls(b *ir.BlockStmt, c *compiler.CompilerContext, byCallee map[ir.SymID]*callSiteAcc) {
	if b == nil {
		return
	}

	for _, st := range b.Stmts {
		outgoingInStmt(st, c, byCallee)
	}
}

func outgoingInStmt(s ir.Stmt, c *compiler.CompilerContext, byCallee map[ir.SymID]*callSiteAcc) {
	if s == nil {
		return
	}

	switch n := s.(type) {
	case *ir.BlockStmt:
		for _, ss := range n.Stmts {
			outgoingInStmt(ss, c, byCallee)
		}

	case *ir.VarDeclStmt:
		outgoingInExpr(n.Init, c, byCallee)
	case *ir.ExprStmt:
		outgoingInExpr(n.Expr, c, byCallee)
	case *ir.FieldAssignmentStmt:
		outgoingInExpr(n.Value, c, byCallee)
	case *ir.MultiAssignmentStmt:
		outgoingInExpr(n.Value, c, byCallee)
	case *ir.IfStmt:
		outgoingInExpr(n.Cond, c, byCallee)
		collectOutgoingCalls(n.Then, c, byCallee)
		for _, eb := range n.ElseIfs {
			outgoingInExpr(eb.Cond, c, byCallee)
			collectOutgoingCalls(eb.Then, c, byCallee)
		}

		collectOutgoingCalls(n.Else, c, byCallee)
	case *ir.ReturnStmt:
		for _, r := range n.Results {
			outgoingInExpr(r, c, byCallee)
		}

	case *ir.ForStmt:
		collectOutgoingCalls(n.Body, c, byCallee)
	case *ir.WhileStmt:
		outgoingInExpr(n.Cond, c, byCallee)
		collectOutgoingCalls(n.Body, c, byCallee)
	case *ir.GoStmt:
		outgoingInExpr(n.Call, c, byCallee)
		collectOutgoingCalls(n.Body, c, byCallee)
	case *ir.DeferStmt:
		outgoingInExpr(n.Call, c, byCallee)
		collectOutgoingCalls(n.Body, c, byCallee)
	case *ir.SelectStmt:
		for _, cc := range n.Cases {
			outgoingInExpr(cc.ChanExpr, c, byCallee)
			outgoingInExpr(cc.SendValue, c, byCallee)
			collectOutgoingCalls(cc.Body, c, byCallee)
		}

		collectOutgoingCalls(n.Default, c, byCallee)
	case *ir.AssertStmt:
		outgoingInExpr(n.Expr, c, byCallee)
	}
}

func outgoingInExpr(e ir.Expr, c *compiler.CompilerContext, byCallee map[ir.SymID]*callSiteAcc) {
	if e == nil {
		return
	}

	switch n := e.(type) {
	case *ir.FuncCallExpr:
		if sym := calleeSymbolFor(n.Callee); sym != 0 {
			if s, _ := lookupSymbol(c, sym); s != nil && s.Kind == ir.SK_Function {
				if findFuncDeclForSym(c, sym) != nil {
					entry, ok := byCallee[sym]
					if !ok {
						entry = &callSiteAcc{}

						byCallee[sym] = entry
					}

					entry.ranges = append(entry.ranges, spanToRange(calleeNameSpan(n.Callee)))
				}
			}
		}

		outgoingInExpr(n.Callee, c, byCallee)
		for _, arg := range n.Args {
			outgoingInExpr(arg.Expr, c, byCallee)
		}

	case *ir.BinaryExpr:
		outgoingInExpr(n.Left, c, byCallee)
		outgoingInExpr(n.Right, c, byCallee)
	case *ir.GroupedExpr:
		outgoingInExpr(n.Expr, c, byCallee)
	case *ir.UnaryExpr:
		outgoingInExpr(n.Expr, c, byCallee)
	case *ir.PrefixUnaryExpr:
		outgoingInExpr(n.Expr, c, byCallee)
	case *ir.PostfixUnaryExpr:
		outgoingInExpr(n.Expr, c, byCallee)
	case *ir.AssignmentExpr:
		outgoingInExpr(n.Right, c, byCallee)
	case *ir.FieldAccessExpr:
		outgoingInExpr(n.Expr, c, byCallee)
	case *ir.IndexExpr:
		outgoingInExpr(n.Expr, c, byCallee)
		outgoingInExpr(n.Index, c, byCallee)
	case *ir.TenaryExpr:
		outgoingInExpr(n.Cond, c, byCallee)
		outgoingInExpr(n.Then, c, byCallee)
		outgoingInExpr(n.Else, c, byCallee)
	case *ir.CoalesceExpr:
		outgoingInExpr(n.Left, c, byCallee)
		outgoingInExpr(n.Default, c, byCallee)
	case *ir.NewExpr:
		for _, arg := range n.Args {
			outgoingInExpr(arg.Expr, c, byCallee)
		}

	case *ir.FuncLitExpr:
		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				outgoingInStmt(ss, c, byCallee)
			}
		}

	case *ir.ArrayLiteral:
		for _, el := range n.Elems {
			outgoingInExpr(el, c, byCallee)
		}

	case *ir.MapLiteral:
		for _, kv := range n.Entries {
			outgoingInExpr(kv.Key, c, byCallee)
			outgoingInExpr(kv.Value, c, byCallee)
		}

	case *ir.TupleLiteral:
		for _, el := range n.Elems {
			outgoingInExpr(el, c, byCallee)
		}

	case *ir.WhenExpr:
		outgoingInExpr(n.Expr, c, byCallee)
		for _, cc := range n.Cases {
			for _, v := range cc.Values {
				outgoingInExpr(v, c, byCallee)
			}

			outgoingInExpr(cc.Then, c, byCallee)
		}

		outgoingInExpr(n.Default, c, byCallee)
	}
}
