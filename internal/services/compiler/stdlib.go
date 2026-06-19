package compiler

import (
	"os"
	"path/filepath"
	"strings"
)

func IsStdImport(path string) bool {
	return strings.HasPrefix(path, "std/")
}

func StdlibSearchPaths() []string {
	return stdlibSearchPaths()
}

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
