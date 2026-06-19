package fmtsrv

import (
	"strings"
)

type Printer struct {
	buf    strings.Builder
	indent int

	atLineStart bool
	comments    *commentStream
}

func newPrinter() *Printer {
	return &Printer{atLineStart: true}
}

func (p *Printer) String() string {
	return p.buf.String()
}

func (p *Printer) attachComments(cs *commentStream) {
	p.comments = cs
}

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

func (p *Printer) writeNewline() {
	p.buf.WriteByte('\n')
	p.atLineStart = true
}

func (p *Printer) writeLine(s string) {
	p.write(s)
	p.writeNewline()
}

func (p *Printer) writeIndent() {
	for i := 0; i < p.indent; i++ {
		p.buf.WriteString("    ")
	}

	p.atLineStart = false
}

func (p *Printer) withIndent(body func()) {
	p.indent++
	body()
	p.indent--
}

func (p *Printer) blankLine() {
	if !p.atLineStart {
		p.writeNewline()
	}

	p.writeNewline()
}
