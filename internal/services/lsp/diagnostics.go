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
)

// scheduleDiagnostics kicks off an async recompute against `snap`. Each scheduled job is keyed by the snapshot ID; if a newer snapshot supersedes this one before the recompute starts, the older job exits early so we don't churn on rapid keystrokes. No debouncing yet - that's a v6 polish item; the snapshot-supersede check is sufficient for the typical "edit, pause, edit, pause" cadence.
func (s *Server) scheduleDiagnostics(ctx context.Context, snap *Snapshot) {
	go s.runDiagnostics(ctx, snap)
}

// runDiagnostics drives one full recompute. Walks the project root to gather source files, layers in editor overlays, runs the compiler's check pipeline, maps each diagnostic onto an LSP `Diagnostic`, and publishes one `PublishDiagnostics` per URI that had any output (plus empty arrays for URIs that were previously errored but are now clean).
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

	byURI := s.bucketDiagnosticsByURI(root, diags)
	for u := range snap.overlays {
		if _, ok := byURI[u]; !ok {
			byURI[u] = nil
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

// compileSnapshot is the CompileFunc the LSP installs on every snapshot it creates. Lazily invoked by the first navigation request (or by runDiagnostics) - gathers sources, runs the check pipeline, returns the populated CompilerContext + flat diagnostics list. Cached on the snapshot so a hover + a diagnostics publish for the same snapshot share one compile pass.
func (s *Server) compileSnapshot(snap *Snapshot) (retCtx *compiler.CompilerContext, retDiags []diag.Diagnostic, retErr error) {
	root := uriToPath(snap.Root)
	if root == "" {
		root = "."
	}
	c := compiler.New()
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

// collectSources merges on-disk `.sova` files under `root` with any in-memory overlays from the snapshot. Overlays always win for paths they cover. Hidden directories (`.git`, etc.) are skipped - except `.sova/deps/` which IS walked so cross-package imports resolve against the package-manager's materialised view.
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
	for u, text := range snap.overlays {
		p := uriToPath(u)
		if p == "" {
			continue
		}
		out[p] = text
	}
	return out
}

// walkSovaTree adds every `.sova` file under `root` into `out` (keyed by absolute path). When `skipHidden` is true, directories whose name starts with `.` are pruned - the project tree walk uses this to keep `.git`, `.sova`, `.vscode` etc. out. The dep walk passes false because it starts INSIDE `.sova/deps/` and needs to see the materialised tree.
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

// bucketDiagnosticsByURI maps a flat list of compiler diagnostics into a per-URI bucket suitable for `PublishDiagnostics`. Diagnostics without a known file are skipped - the LSP protocol can't render them anyway.
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

// toLSPDiagnostic converts one Sova compiler diagnostic into the protocol shape: spans become zero-indexed ranges, severities map onto LSP levels, the diagnostic code goes into the `Code` field so editors can show "ERR.TYP.0001"-style identifiers and filter by them.
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
