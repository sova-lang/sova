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

type Snapshot struct {
	ID       int64
	Root     uri.URI
	overlays map[uri.URI]string

	compileOnce sync.Once
	compileCtx  *compiler.CompilerContext
	compileDiag []diag.Diagnostic
	compileErr  error
}

func (s *Snapshot) Compile(build CompileFunc) (*compiler.CompilerContext, []diag.Diagnostic, error) {
	if s == nil {
		return nil, nil, nil
	}

	s.compileOnce.Do(func() {
		s.compileCtx, s.compileDiag, s.compileErr = build(s)
	})
	return s.compileCtx, s.compileDiag, s.compileErr
}

type CompileFunc func(s *Snapshot) (*compiler.CompilerContext, []diag.Diagnostic, error)

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

func (s *Snapshot) HasOverlay(u uri.URI) bool {
	if s == nil {
		return false
	}

	_, ok := s.overlays[u]
	return ok
}

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

func uriToPath(u uri.URI) string {
	s := string(u)
	if strings.HasPrefix(s, "file://") {
		raw := strings.TrimPrefix(s, "file://")
		return filepath.FromSlash(raw)
	}

	return ""
}

func pathToURI(path string) uri.URI {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}

	return uri.URI("file://" + filepath.ToSlash(abs))
}
