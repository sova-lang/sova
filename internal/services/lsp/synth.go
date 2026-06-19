package lsp

import (
	"sova/internal/ir"
	"sova/internal/passes"
	"sova/internal/services/compiler"
	"strings"

	"go.lsp.dev/protocol"
)

func annotationAtCursor(src string, pos protocol.Position) (string, protocol.Range, bool) {
	lines := splitLines(src)
	if int(pos.Line) >= len(lines) {
		return "", protocol.Range{}, false
	}

	line := lines[int(pos.Line)]
	col := int(pos.Character)
	if col > len(line) {
		col = len(line)
	}

	for i := 0; i < len(line); i++ {
		if line[i] != '@' {
			continue
		}

		j := i + 1
		for j < len(line) && isIdentChar(line[j]) {
			j++
		}

		if j == i+1 {
			continue
		}

		if col >= i && col <= j {
			rng := protocol.Range{
				Start: protocol.Position{Line: pos.Line, Character: uint32(i)},
				End:   protocol.Position{Line: pos.Line, Character: uint32(j)},
			}

			return line[i+1 : j], rng, true
		}

		i = j - 1
	}

	return "", protocol.Range{}, false
}

func splitLines(src string) []string {
	out := strings.SplitAfter(src, "\n")
	for i, l := range out {
		out[i] = strings.TrimRight(l, "\n")
	}

	return out
}

func synthAnnotationHover(c *compiler.CompilerContext, name string, rng protocol.Range) *protocol.Hover {
	sd := lookupSynth(c, name)
	if sd == nil {
		return nil
	}

	body := "```sova\n" + synthSignature(sd) + "\n```\n\n" + synthDoc(sd)
	return &protocol.Hover{
		Contents: protocol.MarkupContent{Kind: protocol.Markdown, Value: body},
		Range:    &rng,
	}
}

func synthDefinitionLocation(c *compiler.CompilerContext, snap *Snapshot, name string) *protocol.Location {
	sd := lookupSynth(c, name)
	if sd == nil {
		return nil
	}

	span := sd.Name.Span
	u := uriForSpan(c, snap, span)
	if u == "" {
		return nil
	}

	return &protocol.Location{URI: u, Range: spanToLSPRange(span)}
}

func lookupSynth(c *compiler.CompilerContext, name string) *ir.SynthDeclStmt {
	if c == nil {
		return nil
	}

	reg, ok := c.Cache[passes.SynthRegistryCacheKey].(map[string]*ir.SynthDeclStmt)
	if !ok {
		return nil
	}

	return reg[name]
}
