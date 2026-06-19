package lsp

import (
	"context"
	"path/filepath"
	"strings"

	"go.lsp.dev/protocol"

	"sova/internal/diag"
	"sova/internal/ir"
)

func (s *Server) References(ctx context.Context, params *protocol.ReferenceParams) ([]protocol.Location, error) {
	snap := s.session.Snapshot()
	if snap == nil {
		return nil, nil
	}

	c, _, err := snap.Compile(s.compileSnapshot)
	if err != nil || c == nil {
		return nil, nil
	}

	if src, ok := snap.ReadFile(params.TextDocument.URI); ok {
		if locs := cssClassDefinition(c, src, params.Position); len(locs) > 0 {
			return locs, nil
		}
	}

	target := findCursorTarget(c, params.TextDocument.URI, params.Position.Line, params.Position.Character)
	if target == nil || target.sym == 0 {
		return nil, nil
	}

	hits := collectReferences(c, target.sym)
	out := make([]protocol.Location, 0, len(hits))
	for _, h := range hits {
		if h.isDecl && !params.Context.IncludeDeclaration {
			continue
		}

		u := uriForSpan(c, snap, h.span)
		if u == "" {
			continue
		}

		out = append(out, protocol.Location{URI: u, Range: spanToLSPRange(h.span)})
	}

	return out, nil
}

func (s *Server) DocumentHighlight(ctx context.Context, params *protocol.DocumentHighlightParams) ([]protocol.DocumentHighlight, error) {
	snap := s.session.Snapshot()
	if snap == nil {
		return nil, nil
	}

	c, _, err := snap.Compile(s.compileSnapshot)
	if err != nil || c == nil {
		return nil, nil
	}

	target := findCursorTarget(c, params.TextDocument.URI, params.Position.Line, params.Position.Character)
	if target == nil || target.sym == 0 {
		return nil, nil
	}

	hits := collectReferences(c, target.sym)
	var out []protocol.DocumentHighlight
	for _, h := range hits {
		u := uriForSpan(c, snap, h.span)
		if u != params.TextDocument.URI {
			continue
		}

		kind := protocol.DocumentHighlightKindRead
		if h.isDecl {
			kind = protocol.DocumentHighlightKindWrite
		}

		out = append(out, protocol.DocumentHighlight{Range: spanToLSPRange(h.span), Kind: kind})
	}

	return out, nil
}

func (s *Server) Symbols(ctx context.Context, params *protocol.WorkspaceSymbolParams) ([]protocol.SymbolInformation, error) {
	snap := s.session.Snapshot()
	if snap == nil {
		return nil, nil
	}

	c, _, err := snap.Compile(s.compileSnapshot)
	if err != nil || c == nil {
		return nil, nil
	}

	query := strings.ToLower(strings.TrimSpace(params.Query))
	const cap = 256
	var out []protocol.SymbolInformation
	for _, pkg := range c.Packages {
		for _, f := range pkg.Files {
			if f.Hir == nil {
				continue
			}

			collectWorkspaceSymbols(c, snap, f.Hir, query, &out)
			if len(out) >= cap {
				return out[:cap], nil
			}
		}
	}

	return out, nil
}

func collectWorkspaceSymbols(_ interface{}, snap *Snapshot, f *ir.File, query string, out *[]protocol.SymbolInformation) {
	for _, st := range f.Statements {
		switch n := st.(type) {
		case *ir.FuncDeclStmt:
			emitWorkspaceSymbol(snap, n.Name.Name, "", protocol.SymbolKindFunction, n.Name.Span, query, out)
		case *ir.VarDeclStmt:
			for _, tgt := range n.Targets {
				if tgt.Name == nil {
					continue
				}

				kind := protocol.SymbolKindVariable
				if n.IsConst {
					kind = protocol.SymbolKindConstant
				}

				emitWorkspaceSymbol(snap, tgt.Name.Name, "", kind, tgt.Name.Span, query, out)
			}

		case *ir.TypeDeclStmt:
			emitWorkspaceSymbol(snap, n.Name.Name, "", protocol.SymbolKindClass, n.Name.Span, query, out)
			for _, m := range n.Methods {
				if m.Func == nil {
					continue
				}

				emitWorkspaceSymbol(snap, m.Func.Name.Name, n.Name.Name, protocol.SymbolKindMethod, m.Func.Name.Span, query, out)
			}

		case *ir.EnumDeclStmt:
			emitWorkspaceSymbol(snap, n.Name.Name, "", protocol.SymbolKindEnum, n.Name.Span, query, out)
		case *ir.InterfaceDeclStmt:
			emitWorkspaceSymbol(snap, n.Name.Name, "", protocol.SymbolKindInterface, n.Name.Span, query, out)
		}
	}
}

func emitWorkspaceSymbol(snap *Snapshot, name, container string, kind protocol.SymbolKind, span diag.TextSpan, query string, out *[]protocol.SymbolInformation) {
	if !matchSymbolName(name, query) {
		return
	}

	u := workspaceSpanToURI(snap, span)
	*out = append(*out, protocol.SymbolInformation{
		Name:          name,
		Kind:          kind,
		Location:      protocol.Location{URI: u, Range: spanToLSPRange(span)},
		ContainerName: container,
	})
}

func workspaceSpanToURI(snap *Snapshot, span diag.TextSpan) protocol.DocumentURI {
	path := span.File
	if path == "" {
		return ""
	}

	if !filepath.IsAbs(path) {
		if root := uriToPath(snap.Root); root != "" {
			path = filepath.Join(root, filepath.FromSlash(path))
		}
	}

	return protocol.DocumentURI(pathToURI(path))
}

func matchSymbolName(name, query string) bool {
	if query == "" {
		return true
	}

	return strings.Contains(strings.ToLower(name), query)
}
