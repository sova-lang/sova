package lsp

import (
	"strings"

	"go.lsp.dev/protocol"
)

func classifyCompletion(src string, pos protocol.Position) (completionContextKind, string) {
	offset := positionToOffset(src, pos)
	if offset <= 0 {
		return completionIdentifier, ""
	}

	if isInsideImportString(src, offset) {
		return completionImportPath, ""
	}

	if isInsideWireOptions(src, offset) {
		return completionWireOption, ""
	}

	if isInsideGenericString(src, offset) {
		return completionCSSClass, ""
	}

	end := offset
	for end > 0 && isIdentChar(src[end-1]) {
		end--
	}

	if end > 0 && src[end-1] == '@' {
		return completionAnnotation, ""
	}

	if end == 0 || src[end-1] != '.' {
		return completionIdentifier, ""
	}

	dotEnd := end - 1
	recvEnd := dotEnd
	if recvEnd > 0 && src[recvEnd-1] == '@' {
		return completionAfterDot, "@"
	}

	if recvEnd > 0 && src[recvEnd-1] == ')' {
		return completionAfterDot, "()"
	}

	recvStart := recvEnd
	for recvStart > 0 && isIdentChar(src[recvStart-1]) {
		recvStart--
	}

	return completionAfterDot, src[recvStart:recvEnd]
}

func isInsideWireOptions(src string, offset int) bool {
	depthParen, depthBracket, depthBrace := 0, 0, 0
	inString := byte(0)
	for i := offset - 1; i >= 0; i-- {
		ch := src[i]
		if inString != 0 {
			if ch == inString && (i == 0 || src[i-1] != '\\') {
				inString = 0
			}

			continue
		}

		switch ch {
		case '"', '\'', '`':
			inString = ch
		case ')':
			depthParen++
		case ']':
			depthBracket++
		case '}':
			depthBrace++
		case '[':
			if depthBracket > 0 {
				depthBracket--
			}

		case '{':
			if depthBrace > 0 {
				depthBrace--
			}

		case '(':
			if depthParen == 0 && depthBracket == 0 && depthBrace == 0 {

				end := i
				for end > 0 && (src[end-1] == ' ' || src[end-1] == '\t') {
					end--
				}

				start := end
				for start > 0 && isIdentChar(src[start-1]) {
					start--
				}

				return src[start:end] == "wire"
			}

			depthParen--
		}
	}

	return false
}

func isInsideImportString(src string, offset int) bool {
	lineStart := offset
	for lineStart > 0 && src[lineStart-1] != '\n' {
		lineStart--
	}

	line := src[lineStart:offset]
	quoteCount := 0
	for i := 0; i < len(line); i++ {
		if line[i] == '"' {
			quoteCount++
		}
	}

	if quoteCount%2 != 1 {
		return false
	}

	trimmed := strings.TrimLeft(line, " \t")
	return strings.HasPrefix(trimmed, "import")
}

func isInsideGenericString(src string, offset int) bool {
	lineStart := offset
	for lineStart > 0 && src[lineStart-1] != '\n' {
		lineStart--
	}

	inDouble := false
	inSingle := false
	i := lineStart
	for i < offset {
		c := src[i]
		if c == '\\' && i+1 < offset {
			i += 2
			continue
		}

		switch {
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		}

		i++
	}

	return inDouble || inSingle
}

func isIdentChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

func isClassCharForKind(b byte, kind completionContextKind) bool {
	if kind == completionCSSClass && b == '-' {
		return true
	}

	return isIdentChar(b)
}
