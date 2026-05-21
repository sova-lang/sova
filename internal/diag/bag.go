package diag

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"sova/internal/termui"
)

// NoSpan is a zero-length TextSpan used to indicate that no specific span is associated with a diagnostic.
var NoSpan = TextSpan{}

// DiagnosticsBag is a collection point for diagnostic information like errors and warnings
// which occur during compilation.
type DiagnosticsBag struct {
	arr []Diagnostic
}

// NewBag creates a new DiagnosticsBag instance with an initial capacity.
func NewBag() *DiagnosticsBag {
	return &DiagnosticsBag{
		arr: make([]Diagnostic, 0, 16), // initial capacity of 16
	}
}

// Diagnostics returns the slice of diagnostics collected in the bag.
func (bag *DiagnosticsBag) Diagnostics() []Diagnostic {
	return bag.arr
}

// Report adds a new diagnostic message to the bag.
func (bag *DiagnosticsBag) Report(code DiagnosticCode, span TextSpan, args ...any) {
	msg := Diagnostic{
		code,
		span,
		fmt.Sprintf(code.Format, args...),
	}
	bag.arr = append(bag.arr, msg)
}

// Errored returns true if the bag contains any error diagnostics.
func (bag *DiagnosticsBag) Errored() bool {
	hasErrors, _, _ := bag.Results()
	return hasErrors
}

// Warnings returns true if the bag contains any warning diagnostics.
func (bag *DiagnosticsBag) Warnings() bool {
	_, hasWarnings, _ := bag.Results()
	return hasWarnings
}

// Results returns a tuple indicating whether the bag contains any errors or warnings.
func (bag *DiagnosticsBag) Results() (
	hasErrors bool,
	hasWarnings bool,
	sorted map[DiagnosticLevel][]Diagnostic,
) {
	sorted = map[DiagnosticLevel][]Diagnostic{
		LevelError:   {},
		LevelWarning: {},
		LevelInfo:    {},
	}

	for _, d := range bag.arr {
		if d.Level == LevelError {
			hasErrors = true
			sorted[LevelError] = append(sorted[LevelError], d)
		} else if d.Level == LevelWarning {
			hasWarnings = true
			sorted[LevelWarning] = append(sorted[LevelWarning], d)
		} else if d.Level == LevelInfo {
			sorted[LevelInfo] = append(sorted[LevelInfo], d)
		}
	}
	return hasErrors, hasWarnings, sorted
}

// Print renders the bag's diagnostics in Rust-style: a one-line header per
// diagnostic with code + message, an arrow pointing at the source location,
// and an aligned snippet with caret(s) marking the span. Multiple
// diagnostics from the same file share a deduplicated source-read so the
// repeated I/O stays cheap. Pass names are intentionally never shown -
// they're a compiler-internal detail.
func (bag *DiagnosticsBag) Print() {
	hasErrors, hasWarnings, _ := bag.Results()

	// Sort all diagnostics by file + position so they read top-to-bottom in
	// source order. Errors are not separated from warnings any more - that
	// was driving people to scan the same area twice; the severity icon
	// already calls out the level at a glance.
	all := make([]Diagnostic, len(bag.arr))
	copy(all, bag.arr)
	sort.SliceStable(all, func(i, j int) bool {
		a, b := all[i], all[j]
		if a.S.File != b.S.File {
			return a.S.File < b.S.File
		}
		if a.S.StartLn != b.S.StartLn {
			return a.S.StartLn < b.S.StartLn
		}
		return a.S.StartCol < b.S.StartCol
	})

	sourceCache := map[string][]string{}
	errCount := 0
	warnCount := 0
	for _, d := range all {
		switch d.Level {
		case LevelError:
			errCount++
		case LevelWarning:
			warnCount++
		}
		printDiag(d, sourceCache)
	}
	if errCount+warnCount > 0 {
		fmt.Fprintln(os.Stderr)
	}
	switch {
	case hasErrors:
		fmt.Fprintf(os.Stderr, "%s compilation failed: %d error%s",
			termui.Cross(), errCount, plural(errCount))
		if warnCount > 0 {
			fmt.Fprintf(os.Stderr, ", %d warning%s", warnCount, plural(warnCount))
		}
		fmt.Fprintln(os.Stderr)
	case hasWarnings:
		fmt.Fprintf(os.Stderr, "%s compiled with %d warning%s\n",
			termui.Warn(), warnCount, plural(warnCount))
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// printDiag renders a single diagnostic. The header line uses a coloured
// `error[CODE]:` / `warning[CODE]:` prefix (à la rustc); the location line is
// indented with ` --> file:line:col`; the snippet block shows the offending
// line plus a caret-row underneath. We resolve relative paths against the
// working directory so editors can click through.
func printDiag(d Diagnostic, sourceCache map[string][]string) {
	level := levelStyling(d.Level)
	header := fmt.Sprintf("%s[%s]: %s", level.label, d.ID(), d.Msg)
	fmt.Fprintln(os.Stderr, level.wrap(header))

	if d.S.StartLn == 0 {
		return
	}
	loc := d.S.File
	if loc != "" {
		if abs, err := filepath.Abs(loc); err == nil {
			if rel, err := filepath.Rel(mustGetwd(), abs); err == nil && !strings.HasPrefix(rel, "..") {
				loc = rel
			}
		}
	}
	fmt.Fprintf(os.Stderr, " %s %s:%d:%d\n",
		termui.Gray("-->"),
		loc, d.S.StartLn, d.S.StartCol)

	lines := readSourceLines(d.S.File, sourceCache)
	if len(lines) == 0 {
		return
	}

	gutterWidth := len(fmt.Sprintf("%d", d.S.EndLn))
	if gutterWidth < 2 {
		gutterWidth = 2
	}
	fmt.Fprintf(os.Stderr, " %s %s\n",
		strings.Repeat(" ", gutterWidth),
		termui.Gray("|"))
	for ln := d.S.StartLn; ln <= d.S.EndLn && ln <= len(lines); ln++ {
		text := lines[ln-1]
		fmt.Fprintf(os.Stderr, " %s %s %s\n",
			termui.Gray(fmt.Sprintf("%*d", gutterWidth, ln)),
			termui.Gray("|"),
			text)
		startCol, endCol := 1, len(text)+1
		if ln == d.S.StartLn {
			startCol = d.S.StartCol
		}
		if ln == d.S.EndLn {
			endCol = d.S.EndCol
		}
		if startCol < 1 {
			startCol = 1
		}
		if endCol <= startCol {
			endCol = startCol + 1
		}
		caretWidth := endCol - startCol
		if caretWidth < 1 {
			caretWidth = 1
		}
		pad := strings.Repeat(" ", startCol-1)
		carets := strings.Repeat("^", caretWidth)
		fmt.Fprintf(os.Stderr, " %s %s %s%s\n",
			strings.Repeat(" ", gutterWidth),
			termui.Gray("|"),
			pad,
			level.wrap(carets))
	}
}

type levelStyle struct {
	label string
	wrap  func(string) string
}

func levelStyling(level DiagnosticLevel) levelStyle {
	switch level {
	case LevelError:
		return levelStyle{label: "error", wrap: func(s string) string { return termui.Bold(termui.Red(s)) }}
	case LevelWarning:
		return levelStyle{label: "warning", wrap: func(s string) string { return termui.Bold(termui.Yellow(s)) }}
	default:
		return levelStyle{label: "note", wrap: func(s string) string { return termui.Bold(termui.Cyan(s)) }}
	}
}

func readSourceLines(path string, cache map[string][]string) []string {
	if path == "" {
		return nil
	}
	if cached, ok := cache[path]; ok {
		return cached
	}
	// Candidate paths, in priority order: the diagnostic's path verbatim;
	// the same path resolved against cwd; the path as an absolute /tmp-style
	// path. Covers `build` (cwd-relative), `lsp --check` (rel or abs), and
	// snippets opened from an editor that may already be absolute.
	candidates := []string{path}
	if !filepath.IsAbs(path) {
		if abs, err := filepath.Abs(path); err == nil {
			candidates = append(candidates, abs)
		}
	}
	var lines []string
	for _, c := range candidates {
		f, err := os.Open(c)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		f.Close()
		break
	}
	cache[path] = lines
	return lines
}

func mustGetwd() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return ""
}
