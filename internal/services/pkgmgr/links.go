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

// LocalLinksFilename is the per-project file that records local-path overrides registered via `sova link`. Gitignored by default - these are dev-machine specific.
const LocalLinksFilename = ".sova/local-links.toml"

// LocalLinks is the parsed view of `.sova/local-links.toml`: a name → absolute-path map that overrides whatever the lockfile would otherwise pick.
type LocalLinks struct {
	Links map[string]string `toml:"links"`
}

// LoadLocalLinks reads `<root>/.sova/local-links.toml`. Missing file → empty links, no error.
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

// Save writes the link map back to disk in a deterministic, git-friendly form: header comment, then sorted `[links]` table.
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

// Lookup returns the override path for `name` if one is registered, normalised to absolute. Empty string + false when no override applies.
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

// Set installs (or replaces) a link for `name` pointing at `path`. The path is stored verbatim - `Save` then `Lookup` apply the absolute-path normalisation.
func (l *LocalLinks) Set(name, path string) {
	if l.Links == nil {
		l.Links = map[string]string{}
	}
	l.Links[name] = path
}

// Remove drops the override for `name`, returning whether anything was removed.
func (l *LocalLinks) Remove(name string) bool {
	if _, ok := l.Links[name]; !ok {
		return false
	}
	delete(l.Links, name)
	return true
}
