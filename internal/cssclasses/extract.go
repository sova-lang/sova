// Package cssclasses extracts class names from CSS or SCSS source. The
// implementation is deliberately simple: a single linear pass that skips
// strings (`"..."`, `'...'`) and comments (`//`, `/* */`) and captures every
// `.<ident>` token whose identifier shape matches a CSS class. Declaration
// values like `padding: .5rem;` are not false positives because the lead
// character after the dot (`5`) is not a valid class-identifier head.
//
// The sole consumer today is the LSP, which feeds in the file contents of
// any `.css` / `.scss` referenced by an `@embed` or `@StyleFile` declaration
// in the current build and surfaces the result as completion candidates
// inside string literals in component method bodies. Phase 3 of that LSP
// work also uses ClassEntry / RuleAt to power hover and go-to-definition.
package cssclasses

// ClassEntry is one occurrence of a class selector in the source. The
// `Offset` is the byte index of the leading `.`; `Line`/`Char` are 0-based
// line + column (column counted in bytes, matching the LSP's byte-position
// convention for ASCII content). One class name can appear in multiple
// entries when the source defines it more than once or in multiple
// selector groups — the LSP's index keys on the name and stores every
// occurrence so hover/jump pick the first one and (eventually) "list
// implementations" can show all.
type ClassEntry struct {
	Name   string
	Offset int
	Line   int
	Char   int
}

// Extract returns every class-name occurrence in `source` with its byte
// offset and 0-based line+char position. Order matches the source — first
// occurrence wins for any per-name lookup the caller performs.
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

// Imports returns the bare path arguments of every `@use` and `@import`
// at-rule in `source`. Paths are returned verbatim — extension stripping,
// `_partial` prefix probing, and directory resolution all live at the
// caller (the LSP, which knows the host file's directory and can probe
// the four standard SCSS spellings: `name.scss`, `_name.scss`,
// `name.sass`, `_name.sass`). Returns nil when no imports are present
// or when the source has no `@use`/`@import` shape.
//
// Recognises the modern Sass surface (`@use "variables";`,
// `@use "variables" as v;`, `@use "variables" with (...);`) and the
// legacy `@import "variables";` form. Single-quote and double-quote
// string forms both work. Multi-path imports
// (`@import "a", "b";`) yield one entry per path.
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

// readImportArgs walks forward from the start of the at-rule's arguments and pulls every quoted string before the next `;`, `{`, or end-of-input. Stops on `{` so we don't misread the body of a multi-line at-rule like `@use "foo" with ($x: 1);` — the closing `;` terminates the import argument list cleanly because `with (...)` is balanced.
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

// Names is a convenience wrapper that returns the unique class names of
// `source` in first-occurrence order. Equivalent to Extract + dedup by
// `Name`; kept as a separate function so callers that only need names
// (the completion list) don't have to allocate the per-entry positional
// data.
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

// RuleAt returns the full rule block (selector group + `{ ... }`) the class
// at `offset` is part of, as source text. Scans forward from the leading
// `.`, skipping intermediate selector pieces (`.foo.bar`, `, .other`,
// `:hover`, descendant combinators), strings, and comments, until it
// finds the opening `{`. The matching `}` is found by depth-counting that
// also skips strings, comments, and nested blocks. Returns "" when no
// brace block follows (e.g. `@extend .foo;`) or when the source is
// malformed.
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

// matchingClose returns the index of the `}` that closes the block opened at `openIdx`. Tracks nested `{}` blocks and skips strings + comments inside the body so a `content: "}"` declaration value doesn't prematurely terminate the rule. Returns -1 when no matching `}` is found.
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

// readClassName reads the identifier portion of a class name starting at index i (the position just AFTER the leading `.`). Returns the name and the index of the first byte not consumed. Class names follow the standard CSS identifier rules: an optional `-` prefix, then `[a-zA-Z_]` followed by `[a-zA-Z0-9_-]*`. Returns ("", i) when the position does not start a valid identifier — that covers numbers (`.5rem`), interpolation markers (`.#{...}`), and lone dots.
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
