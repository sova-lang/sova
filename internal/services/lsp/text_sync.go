package lsp

import (
	"context"

	"go.lsp.dev/protocol"
	"go.uber.org/zap"
)

// DidOpen records the editor's initial content for a document and triggers diagnostics. Always full-text in v1 (incremental sync is a v6 follow-up per the design doc).
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

// DidChange applies the editor's text changes to our in-memory overlay. With `TextDocumentSyncKindIncremental` (the kind we advertise) each change is a `Range`-scoped splice - the client never sends full-document replacements via didChange. As a defensive fallback we treat a zero-valued `Range` (`{0,0}-{0,0}`) on a change whose `Text` covers the whole document as a full replace; this catches editors that disagree on the sync mode during the first few keystrokes after `didOpen`. Applies changes in order - the protocol guarantees they're already serialised by the client.
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

// applyRangeEdit splices `newText` into `current` over `r`. Computes byte offsets for the range's start and end by walking the source line-by-line; LSP's `character` field counts UTF-16 code units, but Sova source is ASCII-ish enough in practice that we treat it as bytes - full UTF-16 awareness is a polish item once we hit a real bug.
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

// lineColToOffset returns the byte offset into `s` for `(line, col)`. Treats both as 0-based, matching LSP positions. Returns len(s) when the position is past EOF so callers don't have to range-check before splicing.
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

// DidSave is mostly a no-op for us - we get the live buffer on every keystroke via `didChange`, so the save event carries no new information. Logged for visibility; could later trigger formatting or extra-aggressive analysis.
func (s *Server) DidSave(ctx context.Context, params *protocol.DidSaveTextDocumentParams) error {
	s.logger.Debug("didSave", zap.String("uri", string(params.TextDocument.URI)))
	return nil
}

// DidClose drops the overlay for the document. Subsequent reads fall back to disk; diagnostics for this URI are cleared so the editor doesn't keep stale errors around.
func (s *Server) DidClose(ctx context.Context, params *protocol.DidCloseTextDocumentParams) error {
	snap := s.session.RemoveOverlay(params.TextDocument.URI)
	s.logger.Debug("didClose", zap.String("uri", string(params.TextDocument.URI)), zap.Int64("snapshot", snap.ID))
	_ = s.client.PublishDiagnostics(ctx, &protocol.PublishDiagnosticsParams{
		URI:         params.TextDocument.URI,
		Diagnostics: []protocol.Diagnostic{},
	})
	return nil
}
