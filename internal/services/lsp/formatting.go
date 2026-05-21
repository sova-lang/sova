package lsp

import (
	"context"
	"strings"

	"go.lsp.dev/protocol"

	"sova/internal/services/fmtsrv"
)

// Formatting runs the canonical Sova formatter against the document under the cursor and returns a single full-replacement TextEdit. The editor swaps that range for the formatted output. We don't compute minimal edits today; the formatter is fast enough that "replace the whole file" works at the scale of real Sova files and avoids drift between minimal-edit logic and the formatter's idea of canonical output.
func (s *Server) Formatting(ctx context.Context, params *protocol.DocumentFormattingParams) ([]protocol.TextEdit, error) {
	snap := s.session.Snapshot()
	if snap == nil {
		return nil, nil
	}
	src, ok := snap.ReadFile(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}
	formatted, err := fmtsrv.Source(src)
	if err != nil {
		s.logger.Debug("formatting: source has parse errors; skipping")
		return nil, nil
	}
	if formatted == src {
		return nil, nil
	}
	endLine, endChar := documentEndPosition(src)
	return []protocol.TextEdit{{
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: endLine, Character: endChar},
		},
		NewText: formatted,
	}}, nil
}

// documentEndPosition returns the (0-based line, 0-based char) of the document's end-of-text - the LSP-position cursor would land here if you `End`-keyed the buffer. Needed for the full-replacement TextEdit range.
func documentEndPosition(src string) (uint32, uint32) {
	lines := strings.Split(src, "\n")
	last := len(lines) - 1
	return uint32(last), uint32(len(lines[last]))
}
