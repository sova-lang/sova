// Package fmtsrv is Sova's source-code formatter (think `gofmt` for Sova). The pretty-printer walks an already-parsed HIR and emits canonical Sova source: deterministic indentation, single space around binary operators, one statement per line, fields and methods inside a `type` block separated by blank lines, and so on. Callers can drive it through `Source(...)` (the convenience entry point: source text → formatted source text) or by handing it a pre-built `*ir.File` for cases where the caller already has the HIR.
package fmtsrv

import (
	"strings"
)

// Printer is the per-call formatter state: an output buffer, the current indent depth, and a lookahead for comment interleaving. Construct with `newPrinter()`; emit via the `print_*.go` files; pull the final string with `String()`.
type Printer struct {
	buf    strings.Builder
	indent int

	atLineStart bool
	comments    *commentStream
}

// newPrinter builds an empty printer. The caller wires up the comment stream separately via `attachComments` so files without comments stay cheap (no buffer manipulation, no per-emit lookups).
func newPrinter() *Printer {
	return &Printer{atLineStart: true}
}

// String returns the formatted output accumulated so far. Typically called once at the end of a top-level `printFile`.
func (p *Printer) String() string {
	return p.buf.String()
}

// attachComments registers a sorted comment stream for the printer to interleave. Subsequent `flushCommentsBefore(span)` calls drain comments whose position falls before `span` into the output before the next emit.
func (p *Printer) attachComments(cs *commentStream) {
	p.comments = cs
}

// write appends literal text to the output, expanding the indentation prefix the first time per line. Internal callers should prefer `writeLine`, `writeNewline`, etc. for higher-level operations.
func (p *Printer) write(s string) {
	if s == "" {
		return
	}
	if p.atLineStart {
		p.writeIndent()
	}
	p.buf.WriteString(s)
	p.atLineStart = false
}

// writeNewline emits `\n` and marks the next write as a line-start so the indent prefix gets applied.
func (p *Printer) writeNewline() {
	p.buf.WriteByte('\n')
	p.atLineStart = true
}

// writeLine writes `s` followed by a newline. The most common emit-and-end-line pattern in the statement printer.
func (p *Printer) writeLine(s string) {
	p.write(s)
	p.writeNewline()
}

// writeIndent emits the current indentation prefix. Called by `write` when at line-start; safe to invoke directly only when the caller knows the cursor is at column 0.
func (p *Printer) writeIndent() {
	for i := 0; i < p.indent; i++ {
		p.buf.WriteString("    ")
	}
	p.atLineStart = false
}

// withIndent increases the indent depth, runs `body`, then restores. Used by every block-emission path so nesting follows the source structure 1:1.
func (p *Printer) withIndent(body func()) {
	p.indent++
	body()
	p.indent--
}

// blankLine writes a single `\n` (a blank line if we were already at line-start, otherwise a line break followed by a blank line). Used between top-level declarations.
func (p *Printer) blankLine() {
	if !p.atLineStart {
		p.writeNewline()
	}
	p.writeNewline()
}
