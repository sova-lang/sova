package lsp

import (
	"context"
	"fmt"

	"go.lsp.dev/protocol"

	"sova/internal/diag"
	"sova/internal/ir"
	"sova/internal/services/compiler"
)

func (s *Server) CodeLens(ctx context.Context, params *protocol.CodeLensParams) ([]protocol.CodeLens, error) {
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

	var lenses []protocol.CodeLens
	for _, st := range file.Statements {
		switch n := st.(type) {
		case *ir.FuncDeclStmt:
			lenses = append(lenses, refsCountLens(c, n.Name.Sym, n.Name.Span))
			if n.Name.Name == "main" {
				lenses = append(lenses, protocol.CodeLens{
					Range: spanToRange(n.Name.Span),
					Command: &protocol.Command{
						Title:     "▶ Run",
						Command:   "sova.runMain",
						Arguments: []interface{}{string(params.TextDocument.URI)},
					},
				})
			}

		case *ir.TypeDeclStmt:
			lenses = append(lenses, refsCountLens(c, n.Name.Sym, n.Name.Span))
		case *ir.EnumDeclStmt:
			lenses = append(lenses, refsCountLens(c, n.Name.Sym, n.Name.Span))
		case *ir.InterfaceDeclStmt:
			lenses = append(lenses, refsCountLens(c, n.Name.Sym, n.Name.Span))
		case *ir.VarDeclStmt:
			for _, tgt := range n.Targets {
				if tgt.Name == nil {
					continue
				}

				lenses = append(lenses, refsCountLens(c, tgt.Name.Sym, tgt.Name.Span))
			}

		case *ir.TestDeclStmt:
			lenses = append(lenses, protocol.CodeLens{
				Range: spanToRange(n.Span()),
				Command: &protocol.Command{
					Title:     "▶ Run test",
					Command:   "sova.runTest",
					Arguments: []interface{}{string(params.TextDocument.URI), n.Name},
				},
			})
		}
	}

	return lenses, nil
}

func refsCountLens(c *compiler.CompilerContext, sym ir.SymID, span diag.TextSpan) protocol.CodeLens {
	count := 0
	if sym != 0 {
		for _, h := range collectReferences(c, sym) {
			if !h.isDecl {
				count++
			}
		}
	}

	return protocol.CodeLens{
		Range: spanToRange(span),
		Command: &protocol.Command{
			Title: fmt.Sprintf("%d references", count),
		},
	}
}
