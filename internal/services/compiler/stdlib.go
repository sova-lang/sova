package compiler

import (
	"os"
	"path/filepath"
	"strings"
)

// IsStdImport returns true when the given Sova import path targets a built-in stdlib module (any path that starts with "std/"). The loader bypass uses this to resolve from the on-disk `std/` directory shipped alongside the Sova binary instead of the project-local source root.
func IsStdImport(path string) bool {
	return strings.HasPrefix(path, "std/")
}

// StdlibSearchPaths is the exported wrapper around stdlibSearchPaths so external tooling (the LSP, dev tools) can resolve `std/...` to on-disk locations using the same probe order the compiler uses internally.
func StdlibSearchPaths() []string {
	return stdlibSearchPaths()
}

// stdlibSearchPaths returns the candidate directories the compiler probes when resolving a `std/...` import, in priority order:
//
//  1. `$SOVA_HOME/std` if `SOVA_HOME` is set - explicit override, used for tests and custom installs.
//  2. `<dir-of-binary>/std` - production layout (binary + std folder side by side).
//  3. `<dir-of-binary>/../std` - when the binary lives in a `bin/` subfolder of an installation prefix.
//  4. The current working directory's `std/` - repo-development convenience so `go run .` from the project root finds the in-tree stdlib.
//
// New stdlib modules ship by dropping `.sova` files into `<install>/std/<name>/` (or `<install>/std/<name>.sova` for single-file modules). No compiler rebuild required.
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

// loadStdPackage materialises the stdlib package at `importPath` (e.g. "std/strings") into the CompilerContext by reading from the first stdlib search directory that contains the requested module. Two layouts are supported per module: a single `<name>.sova` file directly under `std/` for tiny modules, or a subdirectory `<name>/` with one or more `.sova` files for larger ones. Returns true when at least one file was loaded; false leaves the regular filesystem loader to run (and surface a clean "no such package" diagnostic).
func loadStdPackage(c *CompilerContext, importPath string) (bool, error) {
	if !IsStdImport(importPath) {
		return false, nil
	}
	rel := strings.TrimPrefix(importPath, "std/")
	if rel == "" {
		return false, nil
	}
	for _, base := range stdlibSearchPaths() {
		dir := filepath.Join(base, filepath.FromSlash(rel))
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			entries, err := os.ReadDir(dir)
			if err != nil {
				return false, err
			}
			loaded := false
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sova") {
					continue
				}
				full := filepath.Join(dir, entry.Name())
				data, err := os.ReadFile(full)
				if err != nil {
					return false, err
				}
				virtualPath := filepath.ToSlash(filepath.Join("std", rel, entry.Name()))
				c.AddSource(virtualPath, string(data))
				loaded = true
			}
			if loaded {
				return true, nil
			}
		}
		flat := filepath.Join(base, filepath.FromSlash(rel)+".sova")
		if data, err := os.ReadFile(flat); err == nil {
			virtualPath := filepath.ToSlash(filepath.Join("std", rel) + ".sova")
			c.AddSource(virtualPath, string(data))
			return true, nil
		}
	}
	return false, nil
}
