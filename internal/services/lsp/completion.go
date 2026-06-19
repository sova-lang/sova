package lsp

import (
	"context"

	"go.lsp.dev/protocol"
)

func (s *Server) Completion(ctx context.Context, params *protocol.CompletionParams) (*protocol.CompletionList, error) {
	snap := s.session.Snapshot()
	if snap == nil {
		return nil, nil
	}

	c, _, err := snap.Compile(s.compileSnapshot)
	if err != nil || c == nil {
		return nil, nil
	}

	pkg, file, _ := lookupFileByURI(c, params.TextDocument.URI)
	if file == nil {
		return nil, nil
	}

	src, ok := snap.ReadFile(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}

	ctxKind, dotPrefix := classifyCompletion(src, params.Position)
	var items []protocol.CompletionItem
	switch ctxKind {
	case completionAfterDot:
		items = memberCompletions(c, file, params.Position, dotPrefix)
	case completionImportPath:
		items = importPathCompletions(s, snap, c, params.TextDocument.URI)
	case completionWireOption:
		items = wireOptionCompletions()
	case completionAnnotation:
		items = annotationCompletions(c)
	case completionCSSClass:
		offset := positionToOffset(src, params.Position)
		var slot *callContext
		if ctx, ok := cssClassSlotAt(c, src, offset); ok {
			slot = &ctx
		}

		items = cssClassCompletions(c, slot)
	default:
		items = identifierCompletions(c, pkg, file)
		items = append(items, localScopeCompletions(c, file, params.Position)...)
	}

	applyWordReplaceRange(items, src, params.Position, ctxKind)
	return &protocol.CompletionList{Items: items, IsIncomplete: false}, nil
}

func applyWordReplaceRange(items []protocol.CompletionItem, src string, pos protocol.Position, kind completionContextKind) {
	if len(items) == 0 {
		return
	}

	if kind == completionImportPath || kind == completionWireOption {
		return
	}

	offset := positionToOffset(src, pos)
	wordStart := offset
	for wordStart > 0 && isClassCharForKind(src[wordStart-1], kind) {
		wordStart--
	}

	wordEnd := offset
	for wordEnd < len(src) && isClassCharForKind(src[wordEnd], kind) {
		wordEnd++
	}

	startPos := offsetToPosition(src, wordStart)
	endPos := offsetToPosition(src, wordEnd)
	rng := protocol.Range{Start: startPos, End: endPos}

	for i := range items {
		newText := items[i].InsertText
		if newText == "" {
			newText = items[i].Label
		}

		items[i].TextEdit = &protocol.TextEdit{Range: rng, NewText: newText}
	}
}

type completionContextKind int

const (
	completionUnknown completionContextKind = iota
	completionIdentifier
	completionAfterDot
	completionImportPath
	completionWireOption
	completionAnnotation
	completionCSSClass
)

func dedupeCompletionItems(items []protocol.CompletionItem) []protocol.CompletionItem {
	type key struct {
		label string
		kind  protocol.CompletionItemKind
	}

	seen := map[key]bool{}

	out := make([]protocol.CompletionItem, 0, len(items))
	for _, it := range items {
		k := key{label: it.Label, kind: it.Kind}

		if seen[k] {
			continue
		}

		seen[k] = true
		out = append(out, it)
	}

	return out
}
