package lsp

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"sova/internal/services/compiler"
)

// importPathCompletions builds the list shown inside an `import "..."` literal.
// It surfaces three layers: stdlib modules under `std/`, packages declared
// anywhere in the project's source tree (matched against their `package <name>`
// declaration so `src/back/backend.sova` declaring `package testBackend` shows
// up as `testBackend`), and dependencies the package manager has materialised
// under `<projectRoot>/.sova/deps/`. Each entry is annotated with a Kind +
// Detail so editors render category hints next to the name.
func importPathCompletions(s *Server, snap *Snapshot, c *compiler.CompilerContext, docURI uri.URI) []protocol.CompletionItem {
	root := uriToPath(snap.Root)
	if root == "" {
		root = filepath.Dir(uriToPath(docURI))
	}
	projectRoot := findProjectRoot(root)
	var items []protocol.CompletionItem
	seen := map[string]struct{}{}
	add := func(label, detail string, kind protocol.CompletionItemKind) {
		if label == "" {
			return
		}
		if _, dup := seen[label]; dup {
			return
		}
		seen[label] = struct{}{}
		items = append(items, protocol.CompletionItem{
			Label:      label,
			Kind:       kind,
			Detail:     detail,
			InsertText: label,
		})
	}
	for _, p := range stdlibImportCandidates() {
		add(p, "stdlib", protocol.CompletionItemKindModule)
	}
	for _, p := range builtinPackageCandidates(c) {
		add(p, "built-in", protocol.CompletionItemKindModule)
	}
	for name := range collectLocalPackageNames(projectRoot) {
		add(name, "local package", protocol.CompletionItemKindModule)
	}
	for _, p := range collectDepImportCandidates(projectRoot) {
		add(p, "dependency", protocol.CompletionItemKindModule)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Label < items[j].Label })
	return items
}

// builtinPackageCandidates returns the import paths of every compiler-built
// package whose contents are synthesised in code (no source files on disk),
// e.g. the `sessions` package registered by registerBuiltinPackages. They
// don't show up in the stdlib search-path walk because they have no `.sova`
// file, so we surface them here so import-completion lists them alongside
// the rest of the stdlib.
func builtinPackageCandidates(c *compiler.CompilerContext) []string {
	if c == nil {
		return nil
	}
	var out []string
	for path, pkg := range c.Packages {
		if pkg == nil {
			continue
		}
		hasFile := false
		for _, f := range pkg.Files {
			if f != nil && f.Hir != nil {
				hasFile = true
				break
			}
		}
		if !hasFile {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

// findProjectRoot walks upward from `start` looking for `sova.toml` so import
// candidates can be scoped to the active project. Returns "" when no manifest
// is found, leaving the caller to fall back to whatever directory it knows.
func findProjectRoot(start string) string {
	dir := start
	for {
		if dir == "" {
			return ""
		}
		if _, err := os.Stat(filepath.Join(dir, "sova.toml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// stdlibImportCandidates enumerates the available `std/<name>` (or
// `std/<group>/<name>`) modules by probing the same search paths the compiler
// uses at link time. Each `.sova` file becomes a `std/...` path; subdirectories
// containing `.sova` files become module roots as well.
func stdlibImportCandidates() []string {
	var out []string
	seen := map[string]struct{}{}
	add := func(p string) {
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	for _, base := range stdlibSearchPaths() {
		_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil || path == base {
				return nil
			}
			if d.IsDir() {
				if strings.HasPrefix(d.Name(), ".") {
					return fs.SkipDir
				}
				if hasSovaFile(path) {
					rel, err := filepath.Rel(base, path)
					if err == nil {
						add("std/" + filepath.ToSlash(rel))
					}
				}
				return nil
			}
			if !strings.HasSuffix(path, ".sova") {
				return nil
			}
			rel, err := filepath.Rel(base, path)
			if err != nil {
				return nil
			}
			rel = strings.TrimSuffix(rel, ".sova")
			add("std/" + filepath.ToSlash(rel))
			return nil
		})
	}
	sort.Strings(out)
	return out
}

// stdlibSearchPaths mirrors compiler.stdlibSearchPaths (which is unexported).
// We re-derive the same probe order here so completion stays consistent with
// what the compiler will actually accept at build time.
func stdlibSearchPaths() []string {
	var paths []string
	if home := os.Getenv("SOVA_HOME"); home != "" {
		paths = append(paths, filepath.Join(home, "std"))
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		paths = append(paths, filepath.Join(exeDir, "std"))
		paths = append(paths, filepath.Join(exeDir, "..", "std"))
	}
	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(cwd, "std"))
	}
	return paths
}

// hasSovaFile reports whether `dir` contains at least one `.sova` file directly
// (non-recursive). Used to decide whether a directory is itself an importable
// stdlib module root.
func hasSovaFile(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sova") {
			return true
		}
	}
	return false
}

// collectLocalPackageNames walks the project source tree and returns the set of
// `package <name>` declarations encountered. The names are exactly what an
// `import "..."` clause would write, so completion can offer them verbatim.
func collectLocalPackageNames(projectRoot string) map[string]struct{} {
	out := map[string]struct{}{}
	if projectRoot == "" {
		return out
	}
	_ = filepath.WalkDir(projectRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path != projectRoot && strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			if name == ".output" || name == "node_modules" || name == "dist" {
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
		if pkg := extractPackageDeclFromContent(string(data)); pkg != "" && pkg != "main" {
			out[pkg] = struct{}{}
		}
		return nil
	})
	return out
}

// extractPackageDeclFromContent is the LSP-local copy of the CLI helper. We
// duplicate the few lines instead of exposing it across package boundaries.
func extractPackageDeclFromContent(content string) string {
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

// collectDepImportCandidates lists the package paths the package manager has
// materialised under `<projectRoot>/.sova/deps/`. Each direct child directory
// corresponds to one import path; we don't recurse, since deps register one
// top-level name each.
func collectDepImportCandidates(projectRoot string) []string {
	if projectRoot == "" {
		return nil
	}
	depsDir := filepath.Join(projectRoot, ".sova", "deps")
	entries, err := os.ReadDir(depsDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}
