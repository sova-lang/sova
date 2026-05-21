package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go.lsp.dev/uri"

	"sova/internal/diag"
	"sova/internal/services/compiler"
)

// Snapshot is the immutable view of the workspace at the point in time when it was created. Built by the Session in response to file-overlay changes. Multiple snapshots may be alive simultaneously: long-running LSP requests keep a reference to the snapshot they were issued against so they always see a consistent file set.
type Snapshot struct {
	ID       int64
	Root     uri.URI
	overlays map[uri.URI]string

	compileOnce sync.Once
	compileCtx  *compiler.CompilerContext
	compileDiag []diag.Diagnostic
	compileErr  error
}

// Compile lazily runs the compiler's check pipeline against this snapshot and caches the resulting CompilerContext, diagnostics, and any pipeline error. Subsequent calls return the same result - guaranteed by `sync.Once`. This is what every navigation handler (Hover, Definition, DocumentSymbol) calls to get a fully-resolved view of the workspace; diagnostics callers reuse the same cache so per-snapshot work runs at most once.
func (s *Snapshot) Compile(build CompileFunc) (*compiler.CompilerContext, []diag.Diagnostic, error) {
	if s == nil {
		return nil, nil, nil
	}
	s.compileOnce.Do(func() {
		s.compileCtx, s.compileDiag, s.compileErr = build(s)
	})
	return s.compileCtx, s.compileDiag, s.compileErr
}

// CompileFunc is the function the LSP server passes into `Snapshot.Compile` to actually populate the cached compile state. It exists so the snapshot itself stays decoupled from how files are gathered (workspace walk, dep crawl, overlay merge) - that logic lives in the server's `diagnostics.go`.
type CompileFunc func(s *Snapshot) (*compiler.CompilerContext, []diag.Diagnostic, error)

// ReadFile returns the current content of `u`. Overlay (editor buffer) wins over disk; an unsaved edit is what the snapshot sees. Returns ok=false when neither overlay nor disk has the file.
func (s *Snapshot) ReadFile(u uri.URI) (string, bool) {
	if s == nil {
		return "", false
	}
	if v, ok := s.overlays[u]; ok {
		return v, true
	}
	path := uriToPath(u)
	if path == "" {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(data), true
}

// HasOverlay reports whether `u` has an in-memory editor overlay. Used by diagnostics scheduling to distinguish "the user is editing this file" from "this file exists on disk only".
func (s *Snapshot) HasOverlay(u uri.URI) bool {
	if s == nil {
		return false
	}
	_, ok := s.overlays[u]
	return ok
}

// OverlayURIs returns a sorted-stable list of URIs that have editor overlays in this snapshot. Diagnostics fan out across exactly this set per recompute cycle.
func (s *Snapshot) OverlayURIs() []uri.URI {
	if s == nil {
		return nil
	}
	out := make([]uri.URI, 0, len(s.overlays))
	for u := range s.overlays {
		out = append(out, u)
	}
	return out
}

// uriToPath converts an LSP `file://` URI to an OS path. Returns "" for non-file schemes (we don't yet support virtual documents).
func uriToPath(u uri.URI) string {
	s := string(u)
	if strings.HasPrefix(s, "file://") {
		raw := strings.TrimPrefix(s, "file://")
		return filepath.FromSlash(raw)
	}
	return ""
}

// pathToURI converts an OS path to an LSP `file://` URI. Used when emitting diagnostics for files we found via disk walk.
func pathToURI(path string) uri.URI {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return uri.URI("file://" + filepath.ToSlash(abs))
}
