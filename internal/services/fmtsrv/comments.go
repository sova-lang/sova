package fmtsrv

import (
	"sort"
	"strings"

	"sova/internal/diag"
)

type commentKind int

const (
	commentLine commentKind = iota
	commentBlock
)

type extractedComment struct {
	kind  commentKind
	text  string
	start position
}

type position struct {
	line, col int
}

type commentStream struct {
	all    []extractedComment
	cursor int
}

func extractComments(src string) *commentStream {
	cs := &commentStream{}

	line, col := 1, 1
	i := 0
	for i < len(src) {
		c := src[i]
		switch c {
		case '\n':
			line++
			col = 1
			i++
		case '"':
			end := scanStringLiteral(src, i)
			i = end
			col += end - i
		case '\'':
			end := scanCharLiteral(src, i)
			i = end
			col += end - i
		case '`':
			end := scanTemplateLiteral(src, i)
			i = end
			col += end - i
		case '/':
			if i+1 < len(src) && src[i+1] == '/' {
				start := position{line: line, col: col}
				end := i
				for end < len(src) && src[end] != '\n' {
					end++
				}
				cs.all = append(cs.all, extractedComment{
					kind:  commentLine,
					text:  src[i:end],
					start: start,
				})
				col += end - i
				i = end
				continue
			}
			if i+1 < len(src) && src[i+1] == '*' {
				start := position{line: line, col: col}
				end := i + 2
				for end+1 < len(src) && !(src[end] == '*' && src[end+1] == '/') {
					if src[end] == '\n' {
						line++
						col = 1
					} else {
						col++
					}
					end++
				}
				if end+1 < len(src) {
					end += 2
				}
				cs.all = append(cs.all, extractedComment{
					kind:  commentBlock,
					text:  src[i:end],
					start: start,
				})
				i = end
				continue
			}
			col++
			i++
		default:
			col++
			i++
		}
	}
	sort.SliceStable(cs.all, func(a, b int) bool {
		if cs.all[a].start.line == cs.all[b].start.line {
			return cs.all[a].start.col < cs.all[b].start.col
		}
		return cs.all[a].start.line < cs.all[b].start.line
	})
	return cs
}

func (cs *commentStream) flushBefore(p *Printer, span diag.TextSpan) {
	if cs == nil {
		return
	}
	for cs.cursor < len(cs.all) {
		c := cs.all[cs.cursor]
		if !positionLessThanSpan(c.start, span) {
			return
		}
		cs.emit(p, c)
		cs.cursor++
	}
}

func (cs *commentStream) flushTrailing(p *Printer) {
	if cs == nil {
		return
	}
	for cs.cursor < len(cs.all) {
		cs.emit(p, cs.all[cs.cursor])
		cs.cursor++
	}
}

func (cs *commentStream) emit(p *Printer, c extractedComment) {
	if c.kind == commentLine {
		p.writeLine(strings.TrimRight(c.text, " \t"))
		return
	}
	p.writeLine(c.text)
}

func positionLessThanSpan(p position, s diag.TextSpan) bool {
	if s.StartLn == 0 {
		return false
	}
	if p.line < s.StartLn {
		return true
	}
	if p.line == s.StartLn && p.col < s.StartCol {
		return true
	}
	return false
}

func scanStringLiteral(src string, i int) int {
	i++
	for i < len(src) {
		switch src[i] {
		case '\\':
			i += 2
		case '"':
			return i + 1
		case '\n':
			return i
		default:
			i++
		}
	}
	return i
}

func scanCharLiteral(src string, i int) int {
	i++
	for i < len(src) {
		switch src[i] {
		case '\\':
			i += 2
		case '\'':
			return i + 1
		case '\n':
			return i
		default:
			i++
		}
	}
	return i
}

func scanTemplateLiteral(src string, i int) int {
	i++
	for i < len(src) {
		switch src[i] {
		case '\\':
			i += 2
		case '`':
			return i + 1
		default:
			i++
		}
	}

	return i
}
