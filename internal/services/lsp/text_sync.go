package lsp

import (
	"context"

	"go.lsp.dev/protocol"
	"go.uber.org/zap"
)

func (s *Server) DidOpen(ctx context.Context, params *protocol.DidOpenTextDocumentParams) error {
	if params == nil {
		return nil
	}

	td := params.TextDocument
	snap := s.session.UpsertOverlay(td.URI, td.Version, td.Text)
	s.logger.Debug("didOpen", zap.String("uri", string(td.URI)), zap.Int32("version", td.Version), zap.Int64("snapshot", snap.ID))
	s.scheduleDiagnostics(ctx, snap)
	return nil
}

func (s *Server) DidChange(ctx context.Context, params *protocol.DidChangeTextDocumentParams) error {
	if params == nil || len(params.ContentChanges) == 0 {
		return nil
	}

	current, _ := s.session.CurrentOverlayText(params.TextDocument.URI)
	for _, change := range params.ContentChanges {
		if isZeroRange(change.Range) && (current == "" || len(change.Text) >= len(current)) {
			current = change.Text
			continue
		}

		current = applyRangeEdit(current, change.Range, change.Text)
	}

	snap := s.session.UpsertOverlay(params.TextDocument.URI, params.TextDocument.Version, current)
	s.logger.Debug("didChange", zap.String("uri", string(params.TextDocument.URI)), zap.Int32("version", params.TextDocument.Version), zap.Int64("snapshot", snap.ID), zap.Int("changes", len(params.ContentChanges)))
	s.scheduleDiagnostics(ctx, snap)
	return nil
}

func isZeroRange(r protocol.Range) bool {
	return r.Start.Line == 0 && r.Start.Character == 0 && r.End.Line == 0 && r.End.Character == 0
}

func applyRangeEdit(current string, r protocol.Range, newText string) string {
	startOff := lineColToOffset(current, r.Start.Line, r.Start.Character)
	endOff := lineColToOffset(current, r.End.Line, r.End.Character)
	if startOff > endOff {
		startOff, endOff = endOff, startOff
	}

	if startOff < 0 {
		startOff = 0
	}

	if endOff > len(current) {
		endOff = len(current)
	}

	return current[:startOff] + newText + current[endOff:]
}

func lineColToOffset(s string, line, col uint32) int {
	curLine := uint32(0)
	for i := 0; i < len(s); i++ {
		if curLine == line {
			if uint32(0)+col == 0 {
				return i
			}

			rest := s[i:]
			for j := 0; j < len(rest); j++ {
				if curLine == line && uint32(j) == col {
					return i + j
				}

				if rest[j] == '\n' {
					return i + j
				}
			}

			return i + len(rest)
		}

		if s[i] == '\n' {
			curLine++
		}
	}

	return len(s)
}

func (s *Server) DidSave(ctx context.Context, params *protocol.DidSaveTextDocumentParams) error {
	s.logger.Debug("didSave", zap.String("uri", string(params.TextDocument.URI)))
	return nil
}

func (s *Server) DidClose(ctx context.Context, params *protocol.DidCloseTextDocumentParams) error {
	snap := s.session.RemoveOverlay(params.TextDocument.URI)
	s.logger.Debug("didClose", zap.String("uri", string(params.TextDocument.URI)), zap.Int64("snapshot", snap.ID))
	_ = s.client.PublishDiagnostics(ctx, &protocol.PublishDiagnosticsParams{
		URI:         params.TextDocument.URI,
		Diagnostics: []protocol.Diagnostic{},
	})
	return nil
}
