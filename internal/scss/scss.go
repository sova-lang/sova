// Package scss wraps an external `sass` / `dart-sass` binary as a one-shot
// preprocessor: input a path to a `.scss` / `.sass` file, output the
// compiled CSS bytes. Sova doesn't ship a Sass compiler in-process — the
// language ecosystem standardised on dart-sass long ago and re-implementing
// it would be a non-trivial dependency that most users don't need. Instead
// the compiler looks for a `sass` (or `dart-sass`) binary on PATH (or at
// the explicit path the user pins in `sova.toml`), shells out, and uses
// the resulting CSS.
//
// The dependency is opt-in: a `.scss`/`.sass` file referenced from `@embed`
// without an available preprocessor produces a clear diagnostic instead of
// silently inlining the raw SCSS source.
package scss

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Config selects the preprocessor binary the resolver should use. `Command`
// is either an absolute path or a name resolvable on PATH; when empty the
// preprocessor performs auto-discovery (`sass` first, then `dart-sass`).
// `Disabled` short-circuits the whole feature so users on systems without
// Sass installed never accidentally pay the auto-discovery cost.
type Config struct {
	Command  string
	Disabled bool
}

// Preprocessor is the resolved view of a Config: the absolute command the
// resolver will exec. The empty string signals "no preprocessor available"
// — every method on a zero-value Preprocessor returns a clean diagnostic
// rather than silently failing.
type Preprocessor struct {
	command string
}

// New resolves a Config into a Preprocessor by locating the binary. Returns
// a zero-value Preprocessor when Disabled or when no binary can be found —
// the caller decides whether that's fatal (`.scss` referenced) or fine
// (only `.css` paths in play).
func New(cfg Config) Preprocessor {
	if cfg.Disabled {
		return Preprocessor{}
	}
	if cfg.Command != "" {
		if path, err := exec.LookPath(cfg.Command); err == nil {
			return Preprocessor{command: path}
		}
		return Preprocessor{command: cfg.Command}
	}
	for _, candidate := range []string{"sass", "dart-sass"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return Preprocessor{command: path}
		}
	}
	return Preprocessor{}
}

// Available reports whether a preprocessor binary was found. The embed
// resolver uses this to decide between "preprocess the file" and "report a
// clean missing-preprocessor diagnostic" — callers should not invoke
// Compile when Available is false, but if they do, Compile returns an
// error rather than panicking.
func (p Preprocessor) Available() bool {
	return p.command != ""
}

// Command returns the absolute path of the preprocessor binary the
// resolver will exec, or "" when no binary was found. Useful for
// diagnostics ("sass binary at /usr/local/bin/sass failed: …").
func (p Preprocessor) Command() string {
	return p.command
}

// Compile runs the preprocessor against the file at `path` and returns
// the compiled CSS as bytes. The file's extension determines the syntax
// flag (`.sass` uses the indented syntax). Standard error output from the
// preprocessor is folded into the returned error so the user sees the real
// Sass diagnostic in their build output.
func (p Preprocessor) Compile(path string) ([]byte, error) {
	if !p.Available() {
		return nil, fmt.Errorf("no SCSS preprocessor configured (`sass` not on PATH; set [build.scss] command in sova.toml or disable @embed on .scss files)")
	}
	args := []string{path}
	if strings.EqualFold(filepath.Ext(path), ".sass") {
		args = append([]string{"--indented"}, args...)
	}
	cmd := exec.Command(p.command, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		trimmed := strings.TrimSpace(stderr.String())
		if trimmed == "" {
			trimmed = err.Error()
		}
		return nil, fmt.Errorf("%s: %s", filepath.Base(p.command), trimmed)
	}
	return stdout.Bytes(), nil
}

// IsSassPath reports whether `path` looks like a Sass source file by
// extension. Case-insensitive so `Button.SCSS` works the same way; the
// embed resolver delegates to this rather than duplicating the check.
func IsSassPath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".scss" || ext == ".sass"
}
