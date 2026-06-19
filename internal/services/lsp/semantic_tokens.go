package lsp

import (
	"context"
	"sort"

	"go.lsp.dev/protocol"

	"sova/internal/diag"
	"sova/internal/ir"
)

func (s *Server) SemanticTokensFull(ctx context.Context, params *protocol.SemanticTokensParams) (*protocol.SemanticTokens, error) {
	snap := s.session.Snapshot()
	if snap == nil {
		return nil, nil
	}

	c, _, err := snap.Compile(s.compileSnapshot)
	if err != nil || c == nil {
		return nil, nil
	}

	_, file, _ := lookupFileByURI(c, params.TextDocument.URI)
	if file == nil {
		return nil, nil
	}

	tokens := collectSemanticTokens(file)
	return &protocol.SemanticTokens{Data: encodeSemanticTokens(tokens)}, nil
}

func (s *Server) SemanticTokensRange(ctx context.Context, params *protocol.SemanticTokensRangeParams) (*protocol.SemanticTokens, error) {
	return s.SemanticTokensFull(ctx, &protocol.SemanticTokensParams{TextDocument: params.TextDocument})
}

type semToken struct {
	line   uint32
	start  uint32
	length uint32
	typ    uint32
}

const (
	tokFunction      = 0
	tokMethod        = 1
	tokVariable      = 2
	tokParameter     = 3
	tokProperty      = 4
	tokType          = 5
	tokClass         = 6
	tokInterface     = 7
	tokEnum          = 8
	tokEnumMember    = 9
	tokTypeParameter = 10
	tokNamespace     = 11
)

var semanticTokenLegend = []string{
	"function", "method", "variable", "parameter", "property",
	"type", "class", "interface", "enum", "enumMember", "typeParameter", "namespace",
}

func semanticTokenLegendList() []protocol.SemanticTokenTypes {
	out := make([]protocol.SemanticTokenTypes, len(semanticTokenLegend))
	for i, n := range semanticTokenLegend {
		out[i] = protocol.SemanticTokenTypes(n)
	}

	return out
}

func collectSemanticTokens(f *ir.File) []semToken {
	var tokens []semToken
	for _, st := range f.Statements {
		semStmt(st, &tokens)
	}

	sort.Slice(tokens, func(a, b int) bool {
		if tokens[a].line == tokens[b].line {
			return tokens[a].start < tokens[b].start
		}

		return tokens[a].line < tokens[b].line
	})
	return tokens
}

func emitSem(out *[]semToken, span diag.TextSpan, typ uint32) {
	if span.StartLn == 0 {
		return
	}

	*out = append(*out, semToken{
		line:   uint32(span.StartLn - 1),
		start:  uint32(span.StartCol - 1),
		length: uint32(span.EndCol - span.StartCol),
		typ:    typ,
	})
}

func semStmt(s ir.Stmt, out *[]semToken) {
	if s == nil {
		return
	}

	switch n := s.(type) {
	case *ir.FuncDeclStmt:
		emitSem(out, n.Name.Span, tokFunction)
		for _, tp := range n.TypeParams {
			_ = tp
		}

		for _, p := range n.Params {
			emitSem(out, p.Name.Span, tokParameter)
			if p.Default != nil {
				semExpr(p.Default, out)
			}
		}

		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				semStmt(ss, out)
			}
		}

	case *ir.TypeDeclStmt:
		emitSem(out, n.Name.Span, tokClass)
		for _, fld := range n.Fields {
			emitSem(out, fld.Name.Span, tokProperty)
			if fld.Default != nil {
				semExpr(fld.Default, out)
			}
		}

		for _, ctor := range n.Ctors {
			for _, p := range ctor.Params {
				emitSem(out, p.Name.Span, tokParameter)
				if p.Default != nil {
					semExpr(p.Default, out)
				}
			}

			if ctor.Body != nil {
				for _, ss := range ir.BlockStmts(ctor.Body) {
					semStmt(ss, out)
				}
			}
		}

		for _, m := range n.Methods {
			if m.Func == nil {
				continue
			}

			emitSem(out, m.Func.Name.Span, tokMethod)
			for _, p := range m.Func.Params {
				emitSem(out, p.Name.Span, tokParameter)
			}

			if m.Func.Body != nil {
				for _, ss := range ir.BlockStmts(m.Func.Body) {
					semStmt(ss, out)
				}
			}
		}

	case *ir.EnumDeclStmt:
		emitSem(out, n.Name.Span, tokEnum)
		for _, c := range n.Cases {
			emitSem(out, c.Name.Span, tokEnumMember)
		}

		for _, m := range n.Methods {
			semStmt(m, out)
		}

	case *ir.InterfaceDeclStmt:
		emitSem(out, n.Name.Span, tokInterface)
		for _, sig := range n.Methods {
			emitSem(out, sig.Name.Span, tokMethod)
			for _, p := range sig.Params {
				emitSem(out, p.Name.Span, tokParameter)
			}
		}

	case *ir.MixinDeclStmt:
		emitSem(out, n.Name.Span, tokClass)
		for _, fld := range n.Fields {
			emitSem(out, fld.Name.Span, tokProperty)
		}

		for _, m := range n.Methods {
			if m.Func != nil {
				emitSem(out, m.Func.Name.Span, tokMethod)
				if m.Func.Body != nil {
					for _, ss := range ir.BlockStmts(m.Func.Body) {
						semStmt(ss, out)
					}
				}
			}
		}

	case *ir.ImportStmt:

		if n.Alias != "" {
			emitSem(out, n.Span(), tokNamespace)
		}

	case *ir.VarDeclStmt:
		for _, tgt := range n.Targets {
			if tgt.Name != nil {
				emitSem(out, tgt.Name.Span, tokVariable)
			}
		}

		if n.Init != nil {
			semExpr(n.Init, out)
		}

	case *ir.ExprStmt:
		semExpr(n.Expr, out)
	case *ir.FieldAssignmentStmt:
		emitSem(out, n.Receiver.Span, tokVariable)
		for _, fld := range n.Fields {
			emitSem(out, fld.Span, tokProperty)
		}

		semExpr(n.Value, out)
	case *ir.MultiAssignmentStmt:
		for _, tgt := range n.Targets {
			if tgt.Name != nil {
				emitSem(out, tgt.Name.Span, tokVariable)
			}
		}

		semExpr(n.Value, out)
	case *ir.IfStmt:
		semExpr(n.Cond, out)
		if n.Then != nil {
			for _, ss := range ir.BlockStmts(n.Then) {
				semStmt(ss, out)
			}
		}

		for _, eb := range n.ElseIfs {
			semExpr(eb.Cond, out)
			if eb.Then != nil {
				for _, ss := range ir.BlockStmts(eb.Then) {
					semStmt(ss, out)
				}
			}
		}

		if n.Else != nil {
			for _, ss := range ir.BlockStmts(n.Else) {
				semStmt(ss, out)
			}
		}

	case *ir.ReturnStmt:
		for _, r := range n.Results {
			semExpr(r, out)
		}

	case *ir.GuardStmt:
		semExpr(n.Cond, out)
		for _, r := range n.Returns {
			semExpr(r, out)
		}

	case *ir.ForStmt:
		if n.CondInt != nil {
			if n.CondInt.Init != nil {
				for _, tgt := range n.CondInt.Init.Targets {
					if tgt.Name != nil {
						emitSem(out, tgt.Name.Span, tokVariable)
					}
				}

				semExpr(n.CondInt.Init.Init, out)
			}

			semExpr(n.CondInt.Cond, out)
			semExpr(n.CondInt.Post, out)
		}

		if n.CondIn != nil {
			emitSem(out, n.CondIn.InFirstVar.Span, tokVariable)
			if n.CondIn.InSecondVar != nil {
				emitSem(out, n.CondIn.InSecondVar.Span, tokVariable)
			}

			semExpr(n.CondIn.IterExpr, out)
		}

		if n.CondRange != nil {
			emitSem(out, n.CondRange.RangeVar.Span, tokVariable)
			semExpr(n.CondRange.RangeStart, out)
			semExpr(n.CondRange.RangeEnd, out)
		}

		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				semStmt(ss, out)
			}
		}

	case *ir.WhileStmt:
		semExpr(n.Cond, out)
		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				semStmt(ss, out)
			}
		}

	case *ir.GoStmt:
		if n.Call != nil {
			semExpr(n.Call, out)
		}

		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				semStmt(ss, out)
			}
		}

	case *ir.DeferStmt:
		if n.Call != nil {
			semExpr(n.Call, out)
		}

		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				semStmt(ss, out)
			}
		}

	case *ir.SelectStmt:
		for _, cc := range n.Cases {
			semExpr(cc.ChanExpr, out)
			semExpr(cc.SendValue, out)
			if cc.Body != nil {
				for _, ss := range ir.BlockStmts(cc.Body) {
					semStmt(ss, out)
				}
			}
		}

		if n.Default != nil {
			for _, ss := range n.Default.Stmts {
				semStmt(ss, out)
			}
		}

	case *ir.AssertStmt:
		semExpr(n.Expr, out)
	case *ir.TestDeclStmt:
		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				semStmt(ss, out)
			}
		}

	case *ir.BlockStmt:
		for _, ss := range n.Stmts {
			semStmt(ss, out)
		}
	}
}

func semExpr(e ir.Expr, out *[]semToken) {
	if e == nil {
		return
	}

	switch n := e.(type) {
	case *ir.VarRef:
		kind := uint32(tokVariable)
		if n.Ref.Qualifier != "" {
			kind = tokNamespace
		}

		emitSem(out, n.Ref.Span, kind)
	case *ir.FieldAccessExpr:
		semExpr(n.Expr, out)
		for _, f := range n.Fields {
			emitSem(out, f.Span, tokProperty)
		}

	case *ir.FuncCallExpr:
		semExpr(n.Callee, out)
		for _, arg := range n.Args {
			semExpr(arg.Expr, out)
		}

	case *ir.BinaryExpr:
		semExpr(n.Left, out)
		semExpr(n.Right, out)
	case *ir.GroupedExpr:
		semExpr(n.Expr, out)
	case *ir.UnaryExpr:
		semExpr(n.Expr, out)
	case *ir.PrefixUnaryExpr:
		semExpr(n.Expr, out)
	case *ir.PostfixUnaryExpr:
		semExpr(n.Expr, out)
	case *ir.AssignmentExpr:
		emitSem(out, n.Left.Span, tokVariable)
		semExpr(n.Right, out)
	case *ir.IndexExpr:
		semExpr(n.Expr, out)
		semExpr(n.Index, out)
	case *ir.TenaryExpr:
		semExpr(n.Cond, out)
		semExpr(n.Then, out)
		semExpr(n.Else, out)
	case *ir.CoalesceExpr:
		semExpr(n.Left, out)
		semExpr(n.Default, out)
	case *ir.FuncLitExpr:
		for _, p := range n.Params {
			emitSem(out, p.Name.Span, tokParameter)
		}

		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				semStmt(ss, out)
			}
		}

	case *ir.NewExpr:
		emitSem(out, n.TypeName.Span, tokClass)
		for _, arg := range n.Args {
			semExpr(arg.Expr, out)
		}

	case *ir.ChanInitExpr:
		semExpr(n.Capacity, out)
	case *ir.ArrayLiteral:
		for _, el := range n.Elems {
			semExpr(el, out)
		}

	case *ir.MapLiteral:
		for _, kv := range n.Entries {
			semExpr(kv.Key, out)
			semExpr(kv.Value, out)
		}

	case *ir.TupleLiteral:
		for _, el := range n.Elems {
			semExpr(el, out)
		}

	case *ir.RangeExpr:
		semExpr(n.Start, out)
		semExpr(n.End, out)
		semExpr(n.Inc, out)
	case *ir.WhenExpr:
		semExpr(n.Expr, out)
		for _, c := range n.Cases {
			for _, v := range c.Values {
				semExpr(v, out)
			}

			semExpr(c.Then, out)
		}

		semExpr(n.Default, out)
	case *ir.StringTemplateExpr:
		for _, p := range n.Parts {
			semExpr(p.Expr, out)
		}

	case *ir.EnumValueExpr:

		emitSem(out, n.Span(), tokEnumMember)
	}
}

func encodeSemanticTokens(tokens []semToken) []uint32 {
	out := make([]uint32, 0, len(tokens)*5)
	prevLine := uint32(0)
	prevStart := uint32(0)
	for _, t := range tokens {
		deltaLine := t.line - prevLine
		deltaStart := t.start
		if deltaLine == 0 {
			deltaStart = t.start - prevStart
		}

		out = append(out, deltaLine, deltaStart, t.length, t.typ, 0)
		prevLine = t.line
		prevStart = t.start
	}

	return out
}
