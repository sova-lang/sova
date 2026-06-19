package lsp

import (
	"context"
	"fmt"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"sova/internal/services/compiler"
)

func (s *Server) PrepareRename(ctx context.Context, params *protocol.PrepareRenameParams) (*protocol.Range, error) {
	return withCursor(s, params.TextDocument.URI, params.Position, func(snap *Snapshot, c *compiler.CompilerContext, target *cursorTarget) (*protocol.Range, error) {
		if target.sym == 0 || !canRenameSymbol(c, target) {
			return nil, nil
		}

		r := spanToRange(target.span)
		return &r, nil
	})
}

func (s *Server) Rename(ctx context.Context, params *protocol.RenameParams) (*protocol.WorkspaceEdit, error) {
	if err := validateIdentifier(params.NewName); err != nil {
		return nil, err
	}

	return withCursor(s, params.TextDocument.URI, params.Position, func(snap *Snapshot, c *compiler.CompilerContext, target *cursorTarget) (*protocol.WorkspaceEdit, error) {
		if target.sym == 0 {
			return nil, nil
		}

		if !canRenameSymbol(c, target) {
			return nil, fmt.Errorf("symbol is not renameable")
		}

		hits := collectReferences(c, target.sym)
		if len(hits) == 0 {
			return nil, nil
		}

		edits := map[uri.URI][]protocol.TextEdit{}

		for _, h := range hits {
			u := uriForSpan(c, snap, h.span)
			if u == "" {
				continue
			}

			edits[u] = append(edits[u], protocol.TextEdit{
				Range:   spanToRange(h.span),
				NewText: params.NewName,
			})
		}

		return &protocol.WorkspaceEdit{Changes: edits}, nil
	})
}

func canRenameSymbol(c interface {
}, target *cursorTarget) bool {
	_ = c
	if target == nil || target.sym == 0 {
		return false
	}

	return true
}

func validateIdentifier(name string) error {
	if name == "" {
		return fmt.Errorf("new name is empty")
	}

	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':

		case r >= '0' && r <= '9':
			if i == 0 {
				return fmt.Errorf("identifier %q must not start with a digit", name)
			}

		default:
			return fmt.Errorf("identifier %q contains invalid character %q", name, r)
		}
	}

	return nil
}
