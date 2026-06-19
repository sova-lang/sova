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
