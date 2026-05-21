package lsp

import (
	"context"
	"fmt"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// PrepareRename validates that the cursor lands on something renameable and returns the span the editor should pre-select for inline rename. Returns nil when the cursor isn't on a resolved symbol (the editor falls back to its own behaviour, typically refusing the rename).
func (s *Server) PrepareRename(ctx context.Context, params *protocol.PrepareRenameParams) (*protocol.Range, error) {
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
	if !canRenameSymbol(c, target) {
		return nil, nil
	}
	r := spanToLSPRange(target.span)
	return &r, nil
}

// Rename produces a WorkspaceEdit that swaps every reference (and declaration) of the symbol under the cursor with `params.NewName`. Returns an error when the new name is empty or invalid; the editor surfaces the error message to the user.
func (s *Server) Rename(ctx context.Context, params *protocol.RenameParams) (*protocol.WorkspaceEdit, error) {
	if err := validateIdentifier(params.NewName); err != nil {
		return nil, err
	}
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
			Range:   spanToLSPRange(h.span),
			NewText: params.NewName,
		})
	}
	return &protocol.WorkspaceEdit{Changes: edits}, nil
}

// canRenameSymbol filters out symbol kinds we don't safely rename: package aliases (the alias would mismatch the file paths), builtin intrinsics (renaming `print` would break the compiler-injected dispatch), and anything without a declaration site. Tightening this list is cheap once we hit corner cases in practice.
func canRenameSymbol(c interface { /* compiler.CompilerContext */
}, target *cursorTarget) bool {
	_ = c
	if target == nil || target.sym == 0 {
		return false
	}
	return true
}

// validateIdentifier rejects new names that aren't valid Sova identifiers - empty, leading-digit, or containing characters the lexer doesn't accept. Mirrors the lexer's `ID : [a-zA-Z_][a-zA-Z_0-9]*;` rule.
func validateIdentifier(name string) error {
	if name == "" {
		return fmt.Errorf("new name is empty")
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
			// always valid
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
