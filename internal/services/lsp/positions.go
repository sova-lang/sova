package lsp

import (
	"sova/internal/diag"

	"go.lsp.dev/protocol"
)

func offsetToPosition(src string, offset int) protocol.Position {
	if offset < 0 {
		offset = 0
	}

	if offset > len(src) {
		offset = len(src)
	}

	line, col := uint32(0), uint32(0)
	for i := 0; i < offset; i++ {
		if src[i] == '\n' {
			line++
			col = 0
			continue
		}

		col++
	}

	return protocol.Position{Line: line, Character: col}
}

func positionToOffset(src string, pos protocol.Position) int {
	line, col := uint32(0), uint32(0)
	for i := 0; i < len(src); i++ {
		if line == pos.Line && col == pos.Character {
			return i
		}

		if src[i] == '\n' {
			line++
			col = 0
			if line > pos.Line {
				return i
			}

			continue
		}

		col++
	}

	return len(src)
}

func lineColToOffset(s string, line, col uint32) int {
	return positionToOffset(s, protocol.Position{Line: line, Character: col})
}

func spanToRange(s diag.TextSpan) protocol.Range {
	startLn := uint32(0)
	startCol := uint32(0)
	endLn := uint32(0)
	endCol := uint32(0)
	if s.StartLn > 0 {
		startLn = uint32(s.StartLn - 1)
	}

	if s.StartCol > 0 {
		startCol = uint32(s.StartCol - 1)
	}

	if s.EndLn > 0 {
		endLn = uint32(s.EndLn - 1)
	}

	if s.EndCol > 0 {
		endCol = uint32(s.EndCol - 1)
	}

	return protocol.Range{
		Start: protocol.Position{Line: startLn, Character: startCol},
		End:   protocol.Position{Line: endLn, Character: endCol},
	}
}

func isZeroRange(r protocol.Range) bool {
	return r.Start.Line == 0 && r.Start.Character == 0 && r.End.Line == 0 && r.End.Character == 0
}

func applyRangeEdit(current string, r protocol.Range, newText string) string {
	startOff := lineColToOffset(current, r.Start.Line, r.Start.Character)
	endOff := lineColToOffset(current, r.End.Line, r.End.Character)
	if startOff > endOff {
		startOff, endOff = endOff, startOff
	}

	if startOff < 0 {
		startOff = 0
	}

	if endOff > len(current) {
		endOff = len(current)
	}

	return current[:startOff] + newText + current[endOff:]
}
