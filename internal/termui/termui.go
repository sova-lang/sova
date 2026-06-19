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

func DisableColor() { colorEnabled.Store(false) }

func EnableColor() { colorEnabled.Store(true) }

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

func Bold(s string) string    { return wrap(ansiBold, s) }

func Dim(s string) string     { return wrap(ansiDim, s) }

func Red(s string) string     { return wrap(ansiRed, s) }

func Green(s string) string   { return wrap(ansiGreen, s) }

func Yellow(s string) string  { return wrap(ansiYellow, s) }

func Blue(s string) string    { return wrap(ansiBlue, s) }

func Magenta(s string) string { return wrap(ansiMagenta, s) }

func Cyan(s string) string    { return wrap(ansiCyan, s) }

func Gray(s string) string    { return wrap(ansiGray, s) }

func Tick() string     { return iff(colorEnabled.Load(), Green("✓"), "ok") }

func Cross() string    { return iff(colorEnabled.Load(), Red("✗"), "x") }

func Warn() string     { return iff(colorEnabled.Load(), Yellow("!"), "!") }

func Arrow() string    { return iff(colorEnabled.Load(), Blue("→"), "->") }

func Bullet() string   { return iff(colorEnabled.Load(), Gray("•"), "-") }

func InfoIcon() string { return iff(colorEnabled.Load(), Cyan("ℹ"), "i") }

func iff(b bool, t, f string) string {
	if b {
		return t
	}

	return f
}

func Header(label string) {
	fmt.Fprintln(os.Stderr, Bold(label))
}

func Step(msg string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", Arrow(), msg)
}

func Success(msg string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", Tick(), msg)
}

func Failure(msg string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", Cross(), msg)
}

func WarnMsg(msg string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", Warn(), msg)
}

func Info(msg string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", InfoIcon(), msg)
}

type Spinner struct {
	mu      sync.Mutex
	frames  []string
	label   string
	w       io.Writer
	stop    chan struct{}

	done    chan struct{}

	stopped atomic.Bool
}

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

func (s *Spinner) SetLabel(label string) {
	s.mu.Lock()
	s.label = label
	s.mu.Unlock()
}

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

func EndProgress() {
	if colorEnabled.Load() {
		fmt.Fprintln(os.Stderr)
	}
}
