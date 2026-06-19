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

var NoSpan = TextSpan{}

type DiagnosticsBag struct {
	arr []Diagnostic
}

func NewBag() *DiagnosticsBag {
	return &DiagnosticsBag{
		arr: make([]Diagnostic, 0, 16),
	}
}

func (bag *DiagnosticsBag) Diagnostics() []Diagnostic {
	return bag.arr
}

func (bag *DiagnosticsBag) Report(code DiagnosticCode, span TextSpan, args ...any) {
	msg := Diagnostic{
		code,
		span,
		fmt.Sprintf(code.Format, args...),
	}

	bag.arr = append(bag.arr, msg)
}

func (bag *DiagnosticsBag) Errored() bool {
	hasErrors, _, _ := bag.Results()
	return hasErrors
}

func (bag *DiagnosticsBag) Warnings() bool {
	_, hasWarnings, _ := bag.Results()
	return hasWarnings
}

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

func (bag *DiagnosticsBag) Print() {
	hasErrors, hasWarnings, _ := bag.Results()

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
