package lsp

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
	"go.uber.org/zap"

	"sova/internal/diag"
	"sova/internal/services/compiler"
	"sova/internal/services/loader"
)

func (s *Server) scheduleDiagnostics(ctx context.Context, snap *Snapshot) {
	go s.runDiagnostics(ctx, snap)
}

func (s *Server) runDiagnostics(ctx context.Context, snap *Snapshot) {
	if s.session.Snapshot() != snap {
		return
	}

	_, diags, err := snap.Compile(s.compileSnapshot)
	if err != nil {
		s.logger.Warn("compile snapshot failed", zap.Error(err))
		return
	}

	if s.session.Snapshot() != snap {
		return
	}

	root := uriToPath(snap.Root)
	if root == "" {
		root = "."
	}

	c, _, _ := snap.Compile(s.compileSnapshot)
	byURI := s.bucketDiagnosticsByURI(root, diags)
	for u := range snap.overlays {
		if _, ok := byURI[u]; !ok {
			byURI[u] = nil
		}
	}

	if c != nil {
		for u := range byURI {
			_, file, _ := lookupFileByURI(c, u)
			if file == nil {
				continue
			}

			if extra := cssClassDiagnostics(c, file); len(extra) > 0 {
				byURI[u] = append(byURI[u], extra...)
			}
		}

	}

	s.diagMu.Lock()
	if s.publishedURIs == nil {
		s.publishedURIs = map[string]struct{}{}
	}

	for prev := range s.publishedURIs {
		u := uri.URI(prev)
		if _, ok := byURI[u]; !ok {
			byURI[u] = nil
		}
	}

	nextPublished := make(map[string]struct{}, len(byURI))
	for u, items := range byURI {
		if len(items) > 0 {
			nextPublished[string(u)] = struct{}{}
		}
	}

	s.publishedURIs = nextPublished
	s.diagMu.Unlock()

	for u, items := range byURI {
		if items == nil {
			items = []protocol.Diagnostic{}
		}

		params := &protocol.PublishDiagnosticsParams{
			URI:         u,
			Version:     uint32(s.session.OverlayVersion(u)),
			Diagnostics: items,
		}

		if err := s.client.PublishDiagnostics(ctx, params); err != nil {
			s.logger.Warn("publishDiagnostics failed", zap.Error(err), zap.String("uri", string(u)))
		}
	}
}

type lspBuildConfig struct {
	root string
}

func (c lspBuildConfig) OutputDirectory() string  { return ".output" }

func (c lspBuildConfig) OutputBaseName() string   { return "output" }

func (c lspBuildConfig) SourceDirectory() string  { return c.root }

func (c lspBuildConfig) SCSSCommandValue() string { return "" }

func (c lspBuildConfig) SCSSDisabledValue() bool  { return false }

func (s *Server) compileSnapshot(snap *Snapshot) (retCtx *compiler.CompilerContext, retDiags []diag.Diagnostic, retErr error) {
	root := uriToPath(snap.Root)
	if root == "" {
		root = "."
	}

	c := compiler.New()
	c.SetBuildConfig("build_config", lspBuildConfig{root: root})
	if root != "" {

		realLoader := loader.New(root)
		c.Loader = func(cc *compiler.CompilerContext, pkgPath string) error {
			if compiler.IsStdImport(pkgPath) {
				return nil
			}

			return realLoader(cc, pkgPath)
		}
	}

	defer func() {
		if r := recover(); r != nil {
			retCtx = c
			retDiags = c.Diag.Diagnostics()
			retErr = nil
		}
	}()
	sourcesByPath := s.collectSources(snap, root)
	for path, content := range sourcesByPath {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}

		c.AddSource(filepath.ToSlash(rel), content)
	}

	_ = c.Check()
	return c, c.Diag.Diagnostics(), nil
}

func (s *Server) collectSources(snap *Snapshot, root string) map[string]string {
	out := map[string]string{}

	if root != "" {
		walkSovaTree(root, out, true)
		depsRoot := filepath.Join(root, ".sova", "deps")
		if entries, err := os.ReadDir(depsRoot); err == nil {
			for _, e := range entries {
				path := filepath.Join(depsRoot, e.Name())
				resolved, err := filepath.EvalSymlinks(path)
				if err == nil {
					path = resolved
				}

				if info, err := os.Stat(path); err == nil && info.IsDir() {
					walkSovaTree(path, out, false)
				}
			}
		}
	}

	rootAbs := ""
	if root != "" {
		if abs, err := filepath.Abs(root); err == nil {
			rootAbs = abs
		}
	}

	for u, text := range snap.overlays {
		p := uriToPath(u)
		if p == "" {
			continue
		}

		if rootAbs != "" {
			if pAbs, err := filepath.Abs(p); err == nil {
				rel, err := filepath.Rel(rootAbs, pAbs)
				if err != nil || strings.HasPrefix(rel, "..") || strings.HasPrefix(rel, "../") {
					continue
				}
			}
		}

		out[p] = text
	}

	return out
}

func walkSovaTree(root string, out map[string]string, skipHidden bool) {
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			if skipHidden && path != root {
				name := d.Name()
				if strings.HasPrefix(name, ".") {
					return fs.SkipDir
				}
			}

			return nil
		}

		if !strings.HasSuffix(path, ".sova") {
			return nil
		}

		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			path = resolved
		}

		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}

		out[path] = string(data)
		return nil
	})
}

func (s *Server) bucketDiagnosticsByURI(root string, items []diag.Diagnostic) map[uri.URI][]protocol.Diagnostic {
	out := map[uri.URI][]protocol.Diagnostic{}

	for _, d := range items {
		if d.S.File == "" {
			continue
		}

		path := d.S.File
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, filepath.FromSlash(path))
		}

		u := pathToURI(path)
		out[u] = append(out[u], toLSPDiagnostic(d))
	}

	return out
}

func toLSPDiagnostic(d diag.Diagnostic) protocol.Diagnostic {
	startLn := uint32(0)
	startCol := uint32(0)
	endLn := uint32(0)
	endCol := uint32(0)
	if d.S.StartLn > 0 {
		startLn = uint32(d.S.StartLn - 1)
	}

	if d.S.StartCol > 0 {
		startCol = uint32(d.S.StartCol - 1)
	}

	if d.S.EndLn > 0 {
		endLn = uint32(d.S.EndLn - 1)
	}

	if d.S.EndCol > 0 {
		endCol = uint32(d.S.EndCol - 1)
	}

	if endLn < startLn || (endLn == startLn && endCol <= startCol) {
		endLn = startLn
		endCol = startCol + 1
	}

	severity := protocol.DiagnosticSeverityError
	switch d.Level {
	case diag.LevelWarning:
		severity = protocol.DiagnosticSeverityWarning
	case diag.LevelInfo:
		severity = protocol.DiagnosticSeverityInformation
	}

	return protocol.Diagnostic{
		Range: protocol.Range{
			Start: protocol.Position{Line: startLn, Character: startCol},
			End:   protocol.Position{Line: endLn, Character: endCol},
		},
		Severity: severity,
		Code:     d.ID(),
		Source:   "sova",
		Message:  d.Msg,
	}
}
