package lsp

import (
	"sova/internal/ir"
	"sova/internal/passes"
	"sova/internal/services/compiler"
	"strings"

	"go.lsp.dev/protocol"
)

// annotationAtCursor walks the source line at `pos` looking for an `@<Identifier>` token under the cursor. Returns the identifier (without the `@`), the LSP range covering the full `@<Identifier>` span (so editors can highlight the whole token), and ok=true when the cursor sits anywhere inside that span. The check intentionally ignores `@.session` access (no identifier-character sequence after the `@`) — that path is handled by the regular cursor lookup.
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

// splitLines splits raw source into newline-terminated lines for column-based scans. Mirrors the line indexing the rest of the LSP uses; standalone helper so the annotation hover/definition paths don't depend on the broader cursor infrastructure.
func splitLines(src string) []string {
	out := strings.SplitAfter(src, "\n")
	for i, l := range out {
		out[i] = strings.TrimRight(l, "\n")
	}
	return out
}

// synthAnnotationHover renders the markdown hover content for an `@SynthName` token at the cursor. Returns nil when `name` is not a registered synth so the regular cursor-target lookup can run (and surface, e.g., `@reactive` / `@structTag` docs from the static built-in list).
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

// synthDefinitionLocation returns the LSP location of the `synth` declaration that backs `@name` at the cursor. Returns nil when no synth matches so the regular definition path can fall through to the cursor-target lookup. The URI is resolved through the compiler's package files (which the LSP snapshot also indexes), so jumps work whether the synth is in the same project or a sibling `on synth` package.
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

// lookupSynth fetches the SynthDeclStmt for `name` from the registry cache populated by `pass_expand_synths`. Returns nil for built-in annotations or unknown names.
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
