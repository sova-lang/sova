// Package loader assembles a `compiler.PackageLoader` that resolves
// `import "..."` directives against a Sova project root the same way
// the CLI's build/dev/test/check commands do — consulting
// `sova.toml` `[dependencies]`, workspace members, `.sova/deps/`,
// `.sova/local-links.toml`, then falling back to a project-internal
// `package X` index scan.
//
// Exists so both the CLI and the long-running LSP share one
// resolution strategy. Before this package existed, the LSP wired a
// nil loader and `import "strix"` (a path-dep in `sova.toml`)
// silently failed to load — every reference to the missing package
// then cascaded into hundreds of "undefined" diagnostics in the
// editor while `sova build` (which DOES wire the loader) ran clean.
// The LSP/compiler asymmetry was the whole story.
package loader

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"sova/internal/services/compiler"
	"sova/internal/services/pkgmgr"
)

// SourceFile is a (path-relative-to-root, content) pair gathered by
// `CrawlSova`. Exported so external callers building their own load
// flows can use the same shape the loader internally consumes.
type SourceFile struct {
	RelPath string
	Content string
}

// New returns a `compiler.PackageLoader` rooted at `root` (the
// directory holding the project's `sova.toml`). Resolution order
// per package path:
//
//  1. `[dependencies]` `path = "..."` (and workspace members + local
//     links) via `ResolveDepPath`.
//  2. A directory at `<root>/<pkgPath>` — the convention for
//     in-project packages laid out by path.
//  3. A scan of every `.sova` file under the project for a matching
//     `package <name>` declaration — the fallback for projects
//     whose directory layout doesn't mirror package names
//     (`src/back/backend.sova` declaring `package testBackend`).
//
// The package index is built lazily on first need and cached for
// subsequent imports.
func New(root string) compiler.PackageLoader {
	var indexOnce sync.Once
	var pkgIndex map[string][]string
	return func(c *compiler.CompilerContext, pkgPath string) error {
		if dir, ok := ResolveDepPath(root, pkgPath); ok {
			return loadFromDir(c, root, dir)
		}
		dir := filepath.Join(root, filepath.FromSlash(pkgPath))
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return loadFromDir(c, root, dir)
		}
		indexOnce.Do(func() {
			pkgIndex = buildPackageIndex(root)
		})
		if files, ok := pkgIndex[pkgPath]; ok && len(files) > 0 {
			for _, abs := range files {
				data, err := os.ReadFile(abs)
				if err != nil {
					return fmt.Errorf("import %q: %w", pkgPath, err)
				}
				rel, rerr := filepath.Rel(root, abs)
				if rerr != nil {
					rel = abs
				}
				c.AddSource(rel, string(data))
			}
			return nil
		}
		return fmt.Errorf("import %q: package not found (no directory %s and no file declaring `package %s` under the project)", pkgPath, dir, pkgPath)
	}
}

// ResolveDepPath consults the package-manager layers in order:
//  1. local-link override at `<projectRoot>/.sova/local-links.toml`
//  2. workspace member listed in `<projectRoot>/sova.toml`
//  3. inline `path = "..."` dep in the manifest's `[dependencies]`
//  4. resolved dep view at `<projectRoot>/.sova/deps/<name>/`
//
// `projectRoot` is found by walking upward from `sourceRoot` looking
// for the nearest `sova.toml`. Returns the on-disk directory and
// `true` on the first hit, `("", false)` when no layer matches.
func ResolveDepPath(sourceRoot, pkgPath string) (string, bool) {
	projectRoot := FindUpwardManifest(sourceRoot)
	if projectRoot == "" {
		return "", false
	}
	if links, err := pkgmgr.LoadLocalLinks(projectRoot); err == nil {
		if dir, ok := links.Lookup(pkgPath, projectRoot); ok {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				return dir, true
			}
		}
	}
	m, manifestOK, _ := pkgmgr.LoadManifest(filepath.Join(projectRoot, pkgmgr.ManifestFilename))
	if manifestOK && m.Workspace != nil {
		if members, err := m.ResolveWorkspaceMembers(); err == nil {
			for _, dir := range members {
				if mm, ok, err := pkgmgr.LoadManifest(filepath.Join(dir, pkgmgr.ManifestFilename)); err == nil && ok && mm.PackageName() == pkgPath {
					return dir, true
				}
			}
		}
	}
	if manifestOK {
		if dep, ok := m.Dependencies[pkgPath]; ok && dep.Path != "" {
			depDir := dep.Path
			if !filepath.IsAbs(depDir) {
				depDir = filepath.Join(projectRoot, filepath.FromSlash(depDir))
			}
			if sub, err := dep.NormalisedSubdir(); err == nil && sub != "" {
				depDir = filepath.Join(depDir, filepath.FromSlash(sub))
			}
			if info, err := os.Stat(depDir); err == nil && info.IsDir() {
				return depDir, true
			}
		}
	}
	candidate := filepath.Join(projectRoot, pkgmgr.DepsDirname, filepath.FromSlash(pkgPath))
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate, true
	}
	return "", false
}

// FindUpwardManifest walks from `start` up the filesystem looking
// for `sova.toml`. Returns the containing directory or "" when no
// manifest is found.
func FindUpwardManifest(start string) string {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, pkgmgr.ManifestFilename)); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// CrawlSova returns every `.sova` file under `root`, keyed by path
// relative to `root`. Stops at sub-manifests so a workspace member
// is only crawled when explicitly listed in the workspace's
// `[workspace.members]` (see `workspaceMemberSet`). Skips hidden
// directories (`.git`, `.sova`, etc.) except `root` itself.
func CrawlSova(root string) ([]SourceFile, error) {
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	memberAllowed := workspaceMemberSet(root)
	var out []SourceFile
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name != "." && strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			if path != root {
				if _, statErr := os.Stat(filepath.Join(path, pkgmgr.ManifestFilename)); statErr == nil {
					rel, _ := filepath.Rel(root, path)
					rel = filepath.ToSlash(rel)
					if memberAllowed != nil {
						if _, ok := memberAllowed[rel]; !ok {
							return fs.SkipDir
						}
					} else {
						return fs.SkipDir
					}
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, ".sova") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		out = append(out, SourceFile{RelPath: rel, Content: string(data)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func loadFromDir(c *compiler.CompilerContext, root, dir string) error {
	files, err := CrawlSova(dir)
	if err != nil {
		return err
	}
	for _, src := range files {
		rel, rerr := filepath.Rel(root, filepath.Join(dir, src.RelPath))
		if rerr != nil {
			rel = src.RelPath
		}
		c.AddSource(rel, src.Content)
	}
	return nil
}

func workspaceMemberSet(root string) map[string]bool {
	m, ok, err := pkgmgr.LoadManifest(filepath.Join(root, pkgmgr.ManifestFilename))
	if err != nil || !ok || m == nil || !m.IsWorkspaceRoot() {
		return nil
	}
	abs, err := m.ResolveWorkspaceMembers()
	if err != nil {
		return nil
	}
	out := map[string]bool{}
	for _, dir := range abs {
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			continue
		}
		out[filepath.ToSlash(rel)] = true
	}
	return out
}

func buildPackageIndex(loaderRoot string) map[string][]string {
	scanRoot := FindUpwardManifest(loaderRoot)
	if scanRoot == "" {
		scanRoot = loaderRoot
	}
	out := map[string][]string{}
	_ = filepath.WalkDir(scanRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path != scanRoot && strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			if name == ".output" || name == "node_modules" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".sova") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if pkg := extractPackageDecl(string(data)); pkg != "" {
			out[pkg] = append(out[pkg], path)
		}
		return nil
	})
	return out
}

func extractPackageDecl(content string) string {
	i := 0
	n := len(content)
	for i < n {
		for i < n && (content[i] == ' ' || content[i] == '\t' || content[i] == '\r' || content[i] == '\n') {
			i++
		}
		if i+1 < n && content[i] == '/' && content[i+1] == '/' {
			for i < n && content[i] != '\n' {
				i++
			}
			continue
		}
		if i+1 < n && content[i] == '/' && content[i+1] == '*' {
			i += 2
			for i+1 < n && !(content[i] == '*' && content[i+1] == '/') {
				i++
			}
			if i+1 < n {
				i += 2
			}
			continue
		}
		break
	}
	const kw = "package"
	if i+len(kw) > n || content[i:i+len(kw)] != kw {
		return ""
	}
	j := i + len(kw)
	if j >= n || (content[j] != ' ' && content[j] != '\t') {
		return ""
	}
	for j < n && (content[j] == ' ' || content[j] == '\t') {
		j++
	}
	start := j
	for j < n {
		ch := content[j]
		if ch == '_' || ch == '/' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			j++
			continue
		}
		break
	}
	if j == start {
		return ""
	}
	return content[start:j]
}
