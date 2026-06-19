package lsp

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"sova/internal/cssclasses"
	"sova/internal/passes"
	"sova/internal/services/compiler"
)

func projectCSSClasses(c *compiler.CompilerContext) []string {
	index := projectCSSClassIndex(c)
	if len(index) == 0 {
		return nil
	}

	out := make([]string, 0, len(index))
	for name := range index {
		out = append(out, name)
	}

	sort.Strings(out)
	return out
}

type classRef struct {
	cssclasses.ClassEntry
	File   string
	Source string
}

func projectCSSClassIndex(c *compiler.CompilerContext) map[string][]classRef {
	if c == nil {
		return nil
	}

	raw, ok := c.Cache[passes.EmbedAssetsCacheKey]
	if !ok {
		return nil
	}

	records, ok := raw.([]*passes.EmbedRecord)
	if !ok || len(records) == 0 {
		return nil
	}

	type fileEntry struct {
		path    string
		source  string
		entries []cssclasses.ClassEntry
	}

	byPath := map[string]fileEntry{}

	visit := func(path string) {}

	visit = func(path string) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return
		}

		if _, done := byPath[abs]; done {
			return
		}

		content, err := readEmbeddedSource(abs)
		if err != nil {
			byPath[abs] = fileEntry{}

			return
		}

		byPath[abs] = fileEntry{
			path:    abs,
			source:  content,
			entries: cssclasses.Extract(content),
		}

		baseDir := filepath.Dir(abs)
		for _, importRef := range cssclasses.Imports(content) {
			if partial := resolveSassPartial(baseDir, importRef); partial != "" {
				visit(partial)
			}
		}
	}

	for _, rec := range records {
		if rec == nil || rec.Info == nil {
			continue
		}

		if !isStylesheetPath(rec.Info.SourcePath) {
			continue
		}

		visit(rec.Info.SourcePath)
	}

	index := map[string][]classRef{}

	for _, fe := range byPath {
		if fe.path == "" {
			continue
		}

		for _, e := range fe.entries {
			index[e.Name] = append(index[e.Name], classRef{
				ClassEntry: e,
				File:       fe.path,
				Source:     fe.source,
			})
		}
	}

	return index
}

func resolveSassPartial(baseDir, importRef string) string {
	if filepath.IsAbs(importRef) {
		return ""
	}

	clean := strings.TrimSuffix(strings.TrimSuffix(importRef, ".scss"), ".sass")
	dir := filepath.Join(baseDir, filepath.FromSlash(filepath.Dir(clean)))
	base := filepath.Base(clean)
	candidates := []string{
		filepath.Join(dir, base+".scss"),
		filepath.Join(dir, "_"+base+".scss"),
		filepath.Join(dir, base+".sass"),
		filepath.Join(dir, "_"+base+".sass"),
	}

	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}

	return ""
}

func isStylesheetPath(path string) bool {
	switch filepath.Ext(path) {
	case ".css", ".scss", ".sass":
		return true
	}

	return false
}

func readEmbeddedSource(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return string(data), nil
}
