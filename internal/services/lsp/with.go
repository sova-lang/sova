package lsp

import (
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"sova/internal/services/compiler"
)

func withSnapshot[T any](s *Server, fn func(snap *Snapshot, c *compiler.CompilerContext) (T, error)) (T, error) {
	var zero T
	snap := s.session.Snapshot()
	if snap == nil {
		return zero, nil
	}

	c, _, err := snap.Compile(s.compileSnapshot)
	if err != nil || c == nil {
		return zero, nil
	}

	return fn(snap, c)
}

func withCursor[T any](s *Server, docURI uri.URI, pos protocol.Position, fn func(snap *Snapshot, c *compiler.CompilerContext, target *cursorTarget) (T, error)) (T, error) {
	return withSnapshot(s, func(snap *Snapshot, c *compiler.CompilerContext) (T, error) {
		target := findCursorTarget(c, docURI, pos.Line, pos.Character)
		if target == nil {
			var zero T
			return zero, nil
		}

		return fn(snap, c, target)
	})
}
