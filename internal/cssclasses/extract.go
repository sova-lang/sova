package cssclasses

type ClassEntry struct {
	Name   string
	Offset int
	Line   int
	Char   int
}

func Extract(source string) []ClassEntry {
	var out []ClassEntry
	line, char := 0, 0

	advance := func(b byte) {
		if b == '\n' {
			line++
			char = 0
		} else {
			char++
		}
	}

	for i := 0; i < len(source); {
		c := source[i]
		switch {
		case c == '/' && i+1 < len(source) && source[i+1] == '/':
			end := skipLineComment(source, i)
			for ; i < end; i++ {
				advance(source[i])
			}

		case c == '/' && i+1 < len(source) && source[i+1] == '*':
			end := skipBlockComment(source, i)
			for ; i < end; i++ {
				advance(source[i])
			}

		case c == '"' || c == '\'':
			end := skipString(source, i, c)
			for ; i < end; i++ {
				advance(source[i])
			}

		case c == '.':
			dotLine, dotChar := line, char
			advance(c)
			i++
			name, next := readClassName(source, i)
			for j := i; j < next; j++ {
				advance(source[j])
			}

			i = next
			if name == "" {
				continue
			}

			out = append(out, ClassEntry{
				Name:   name,
				Offset: i - len(name) - 1,
				Line:   dotLine,
				Char:   dotChar,
			})
		default:
			advance(c)
			i++
		}
	}

	return out
}

func Imports(source string) []string {
	var out []string
	i := 0
	for i < len(source) {
		c := source[i]
		switch {
		case c == '/' && i+1 < len(source) && source[i+1] == '/':
			i = skipLineComment(source, i)
		case c == '/' && i+1 < len(source) && source[i+1] == '*':
			i = skipBlockComment(source, i)
		case c == '"' || c == '\'':
			i = skipString(source, i, c)
		case c == '@':
			name, next := readAtRule(source, i+1)
			if name == "use" || name == "import" {
				out = append(out, readImportArgs(source, next)...)
				i = skipToStatementEnd(source, next)
				continue
			}

			i = next
		default:
			i++
		}
	}

	return out
}

func readAtRule(s string, i int) (string, int) {
	start := i
	for i < len(s) && isIdentTail(s[i]) {
		i++
	}

	return s[start:i], i
}

func readImportArgs(s string, i int) []string {
	var out []string
	depth := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == '/' && i+1 < len(s) && s[i+1] == '/':
			i = skipLineComment(s, i)
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			i = skipBlockComment(s, i)
		case c == '"' || c == '\'':
			end := skipString(s, i, c)
			if end > i+1 {
				out = append(out, s[i+1:end-1])
			}

			i = end
		case c == '(':
			depth++
			i++
		case c == ')':
			if depth > 0 {
				depth--
			}

			i++
		case c == ';' && depth == 0, c == '{' && depth == 0:
			return out
		default:
			i++
		}
	}

	return out
}

func skipToStatementEnd(s string, i int) int {
	for i < len(s) {
		c := s[i]
		switch {
		case c == '/' && i+1 < len(s) && s[i+1] == '/':
			i = skipLineComment(s, i)
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			i = skipBlockComment(s, i)
		case c == '"' || c == '\'':
			i = skipString(s, i, c)
		case c == ';', c == '{':
			return i + 1
		default:
			i++
		}
	}

	return i
}

func Names(source string) []string {
	entries := Extract(source)
	seen := map[string]struct{}{}

	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if _, dup := seen[e.Name]; dup {
			continue
		}

		seen[e.Name] = struct{}{}

		out = append(out, e.Name)
	}

	return out
}

func RuleAt(source string, offset int) string {
	i := offset
	for i < len(source) {
		c := source[i]
		switch {
		case c == '/' && i+1 < len(source) && source[i+1] == '/':
			i = skipLineComment(source, i)
		case c == '/' && i+1 < len(source) && source[i+1] == '*':
			i = skipBlockComment(source, i)
		case c == '"' || c == '\'':
			i = skipString(source, i, c)
		case c == '{':
			end := matchingClose(source, i)
			if end < 0 {
				return ""
			}

			return source[offset : end+1]
		case c == ';' || c == '}':
			return ""
		default:
			i++
		}
	}

	return ""
}

func matchingClose(source string, openIdx int) int {
	depth := 1
	i := openIdx + 1
	for i < len(source) {
		c := source[i]
		switch {
		case c == '/' && i+1 < len(source) && source[i+1] == '/':
			i = skipLineComment(source, i)
		case c == '/' && i+1 < len(source) && source[i+1] == '*':
			i = skipBlockComment(source, i)
		case c == '"' || c == '\'':
			i = skipString(source, i, c)
		case c == '{':
			depth++
			i++
		case c == '}':
			depth--
			if depth == 0 {
				return i
			}

			i++
		default:
			i++
		}
	}

	return -1
}

func skipLineComment(s string, i int) int {
	for i < len(s) && s[i] != '\n' {
		i++
	}

	return i
}

func skipBlockComment(s string, i int) int {
	i += 2
	for i+1 < len(s) {
		if s[i] == '*' && s[i+1] == '/' {
			return i + 2
		}

		i++
	}

	return len(s)
}

func skipString(s string, i int, quote byte) int {
	i++
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			i += 2
			continue
		}

		if s[i] == quote {
			return i + 1
		}

		i++
	}

	return len(s)
}

func readClassName(s string, i int) (string, int) {
	start := i
	if i < len(s) && s[i] == '-' {
		i++
	}

	if i >= len(s) || !isIdentHead(s[i]) {
		return "", start
	}

	i++
	for i < len(s) && isIdentTail(s[i]) {
		i++
	}

	return s[start:i], i
}

func isIdentHead(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isIdentTail(c byte) bool {
	return isIdentHead(c) || (c >= '0' && c <= '9') || c == '-'
}
