package lsp

import (
	"path/filepath"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"sova/internal/cssclasses"
	"sova/internal/services/compiler"
)

func cssClassHover(c *compiler.CompilerContext, src string, pos protocol.Position) *protocol.Hover {
	offset := positionToOffset(src, pos)
	name, start, end, ok := classNameAtCursor(src, offset)
	if !ok {
		return nil
	}

	index := projectCSSClassIndex(c)
	refs, ok := index[name]
	if !ok || len(refs) == 0 {
		return nil
	}

	ref := refs[0]
	rule := cssclasses.RuleAt(ref.Source, ref.Offset)
	body := "**." + name + "**"
	if rel := filepath.Base(ref.File); rel != "" {
		body += " . `" + rel + "`"
	}

	if len(refs) > 1 {
		body += " . " + itoa(len(refs)) + " occurrences"
	}

	if rule != "" {
		body += "\n\n```css\n" + rule + "\n```"
	}

	startPos := offsetToPosition(src, start)
	endPos := offsetToPosition(src, end)
	rng := protocol.Range{Start: startPos, End: endPos}

	return &protocol.Hover{
		Contents: protocol.MarkupContent{Kind: protocol.Markdown, Value: body},
		Range:    &rng,
	}
}

func cssClassDefinition(c *compiler.CompilerContext, src string, pos protocol.Position) []protocol.Location {
	offset := positionToOffset(src, pos)
	name, _, _, ok := classNameAtCursor(src, offset)
	if !ok {
		return nil
	}

	index := projectCSSClassIndex(c)
	refs, ok := index[name]
	if !ok || len(refs) == 0 {
		return nil
	}

	out := make([]protocol.Location, 0, len(refs))
	for _, ref := range refs {

		startCol := uint32(ref.Char)
		endCol := startCol + uint32(len(ref.Name)) + 1
		out = append(out, protocol.Location{
			URI: uri.URI("file://" + filepath.ToSlash(ref.File)),
			Range: protocol.Range{
				Start: protocol.Position{Line: uint32(ref.Line), Character: startCol},
				End:   protocol.Position{Line: uint32(ref.Line), Character: endCol},
			},
		})
	}

	return out
}
