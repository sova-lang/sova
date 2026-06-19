package pkgmgr

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

const LocalLinksFilename = ".sova/local-links.toml"

type LocalLinks struct {
	Links map[string]string `toml:"links"`
}

func LoadLocalLinks(root string) (*LocalLinks, error) {
	path := filepath.Join(root, LocalLinksFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &LocalLinks{Links: map[string]string{}}, nil
		}

		return nil, err
	}

	var ll LocalLinks
	if err := toml.Unmarshal(data, &ll); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	if ll.Links == nil {
		ll.Links = map[string]string{}
	}

	return &ll, nil
}

func (l *LocalLinks) Save(root string) error {
	path := filepath.Join(root, LocalLinksFilename)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	names := make([]string, 0, len(l.Links))
	for name := range l.Links {
		names = append(names, name)
	}

	sort.Strings(names)
	var b strings.Builder
	b.WriteString("# Local development overrides. Not committed.\n")
	b.WriteString("# Managed via `sova link` / `sova unlink`.\n\n")
	if len(names) > 0 {
		b.WriteString("[links]\n")
		for _, n := range names {
			fmt.Fprintf(&b, "%s = %s\n", tomlQuote(n), tomlQuote(l.Links[n]))
		}
	}

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func (l *LocalLinks) Lookup(name, projectRoot string) (string, bool) {
	raw, ok := l.Links[name]
	if !ok {
		return "", false
	}

	if filepath.IsAbs(raw) {
		return raw, true
	}

	return filepath.Join(projectRoot, raw), true
}

func (l *LocalLinks) Set(name, path string) {
	if l.Links == nil {
		l.Links = map[string]string{}
	}

	l.Links[name] = path
}

func (l *LocalLinks) Remove(name string) bool {
	if _, ok := l.Links[name]; !ok {
		return false
	}

	delete(l.Links, name)
	return true
}
