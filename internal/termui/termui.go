package termui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ANSI colour helpers. Functions return the wrapped text unchanged when the
// destination terminal doesn't support colour (detected lazily on first use)
// or when the `NO_COLOR` env var is set, so the call sites stay clean.

var colorEnabled atomic.Bool

func init() {
	colorEnabled.Store(detectColor())
}

func detectColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("SOVA_NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// DisableColor turns colour output off for the rest of the process. Tests use
// this to keep snapshots deterministic.
func DisableColor() { colorEnabled.Store(false) }

// EnableColor forces colour output on regardless of TTY detection. Used by
// `--color=always` flags and the like.
func EnableColor() { colorEnabled.Store(true) }

// ColorEnabled reports whether colour escapes will be emitted by the wrap
// helpers below. Useful for skipping work (e.g. spinner frames) when output
// is being piped.
func ColorEnabled() bool { return colorEnabled.Load() }

const (
	ansiReset   = "\x1b[0m"
	ansiBold    = "\x1b[1m"
	ansiDim     = "\x1b[2m"
	ansiRed     = "\x1b[31m"
	ansiGreen   = "\x1b[32m"
	ansiYellow  = "\x1b[33m"
	ansiBlue    = "\x1b[34m"
	ansiMagenta = "\x1b[35m"
	ansiCyan    = "\x1b[36m"
	ansiGray    = "\x1b[90m"
)

func wrap(prefix, s string) string {
	if !colorEnabled.Load() || s == "" {
		return s
	}
	return prefix + s + ansiReset
}

// Bold / Dim / colour wrappers used throughout the CLI surface. Each is a
// no-op when colour is disabled, so callers can use them unconditionally.
func Bold(s string) string    { return wrap(ansiBold, s) }
func Dim(s string) string     { return wrap(ansiDim, s) }
func Red(s string) string     { return wrap(ansiRed, s) }
func Green(s string) string   { return wrap(ansiGreen, s) }
func Yellow(s string) string  { return wrap(ansiYellow, s) }
func Blue(s string) string    { return wrap(ansiBlue, s) }
func Magenta(s string) string { return wrap(ansiMagenta, s) }
func Cyan(s string) string    { return wrap(ansiCyan, s) }
func Gray(s string) string    { return wrap(ansiGray, s) }

// Symbols used by the various status helpers. Falls back to ASCII when colour
// is off so plain logs don't accumulate `?`s.
func Tick() string    { return iff(colorEnabled.Load(), Green("✓"), "ok") }
func Cross() string   { return iff(colorEnabled.Load(), Red("✗"), "x") }
func Warn() string    { return iff(colorEnabled.Load(), Yellow("!"), "!") }
func Arrow() string   { return iff(colorEnabled.Load(), Blue("→"), "->") }
func Bullet() string  { return iff(colorEnabled.Load(), Gray("•"), "-") }
func InfoIcon() string { return iff(colorEnabled.Load(), Cyan("ℹ"), "i") }

func iff(b bool, t, f string) string {
	if b {
		return t
	}
	return f
}

// Header prints a bold uppercase banner used at the start of long-running
// commands (`sova build`, `sova dev`, `sova test`).
func Header(label string) {
	fmt.Fprintln(os.Stderr, Bold(label))
}

// Step prints a single `→ text` line. Used by every phase of a long command
// to make the timeline visible.
func Step(msg string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", Arrow(), msg)
}

// Success prints a green checkmark with `msg`. Used as the closer of
// successful sub-phases.
func Success(msg string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", Tick(), msg)
}

// Failure prints a red cross with `msg`. Used when a phase aborts.
func Failure(msg string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", Cross(), msg)
}

// WarnMsg prints a yellow `!` with `msg`. Used for non-fatal anomalies.
func WarnMsg(msg string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", Warn(), msg)
}

// Info prints an `ℹ` with `msg`. Used for ambient status updates that aren't
// progress steps in their own right.
func Info(msg string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", InfoIcon(), msg)
}

// Spinner is a simple braille-frame spinner that renders to stderr until
// Stop is called. Safe across goroutines; calling Stop more than once is a
// no-op so callers can chain `defer spinner.Stop()` without ceremony.
type Spinner struct {
	mu      sync.Mutex
	frames  []string
	label   string
	w       io.Writer
	stop    chan struct{}
	done    chan struct{}
	stopped atomic.Bool
}

// StartSpinner begins emitting a braille spinner with `label`. When colour is
// disabled (non-TTY) the spinner is a silent no-op so log scrapers don't see
// a stream of escape sequences. The returned Spinner must be `Stop`ed.
func StartSpinner(label string) *Spinner {
	s := &Spinner{
		frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		label:  label,
		w:      os.Stderr,
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	if !colorEnabled.Load() {
		close(s.done)
		return s
	}
	go s.run()
	return s
}

func (s *Spinner) run() {
	defer close(s.done)
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()
	i := 0
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.mu.Lock()
			frame := s.frames[i%len(s.frames)]
			fmt.Fprintf(s.w, "\r%s %s ", Cyan(frame), s.label)
			s.mu.Unlock()
			i++
		}
	}
}

// SetLabel swaps the label rendered next to the spinner. Useful for showing
// per-step progress (`compiling…` → `linking…`).
func (s *Spinner) SetLabel(label string) {
	s.mu.Lock()
	s.label = label
	s.mu.Unlock()
}

// Stop halts the spinner and clears its line. Subsequent CLI output starts
// on a clean line. Safe to call more than once.
func (s *Spinner) Stop() {
	if s.stopped.Swap(true) {
		return
	}
	close(s.stop)
	<-s.done
	if colorEnabled.Load() {
		fmt.Fprint(s.w, "\r\x1b[K")
	}
}

// Progress draws a single-line filled bar. Used by `sova upgrade` while
// downloading the release archive. When colour is off, the bar collapses to a
// percentage so non-TTY logs stay readable.
func Progress(label string, current, total int64) {
	if !colorEnabled.Load() {
		if total > 0 {
			fmt.Fprintf(os.Stderr, "%s %d%%\n", label, int(100*current/total))
		}
		return
	}
	const width = 30
	var pct float64
	if total > 0 {
		pct = float64(current) / float64(total)
		if pct > 1 {
			pct = 1
		}
	}
	filled := int(pct * width)
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	fmt.Fprintf(os.Stderr, "\r%s [%s] %3d%%", label, Cyan(bar), int(pct*100))
}

// EndProgress finishes a Progress sequence with a newline so the next line of
// output starts cleanly.
func EndProgress() {
	if colorEnabled.Load() {
		fmt.Fprintln(os.Stderr)
	}
}
