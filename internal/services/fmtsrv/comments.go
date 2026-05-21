package fmtsrv

import (
	"sort"
	"strings"

	"sova/internal/diag"
)

// commentKind classifies a captured comment so the printer can pick the right shape when re-emitting (`//` vs `/* */`).
type commentKind int

const (
	commentLine commentKind = iota
	commentBlock
)

// extractedComment records one comment captured from raw source: the kind (line/block), the bytes of the comment as it appeared, and the start position so the printer can interleave it with HIR nodes that have spans.
type extractedComment struct {
	kind  commentKind
	text  string
	start position
}

// position is a 1-based line/column pair matching `diag.TextSpan`'s coordinate system. Built fresh as we scan raw source byte-by-byte.
type position struct {
	line, col int
}

// commentStream is the ordered (by source position) list of comments extracted from a file, plus a cursor pointing at the next un-emitted comment. The printer calls `flushBefore(span)` to drain comments that should appear before the span.
type commentStream struct {
	all    []extractedComment
	cursor int
}

// extractComments scans raw source and collects every `//` and `/* */` comment along with its 1-based start position. Skips comments inside string and char literals so `"http://..."` doesn't get mis-parsed as a comment.
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

// flushBefore drains every comment whose start position is before `span` into `p`. Each line comment lands on its own line; block comments preserve their internal newlines. After the drain the cursor advances past the emitted entries.
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

// flushTrailing drains every remaining comment after the last HIR node has been emitted. Catches comments at end-of-file.
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

// scanStringLiteral returns the index just past a `"..."` literal that starts at i. Handles `\"` escapes.
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

// scanCharLiteral returns the index just past a `'.'` literal (single-quoted char) at i. Tolerant of escape sequences.
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

// scanTemplateLiteral returns the index just past a backtick template literal. Sova template strings allow `${expr}` interpolations whose contents can contain anything, including other strings; we don't recurse into them (no nested comments inside `${...}` for v1) - we just skip to the closing backtick.
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
