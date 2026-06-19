package pkgmgr

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const IndexRefreshInterval = 24 * time.Hour

type IndexEntry struct {
	Git         string `toml:"git"`
	Description string `toml:"description,omitempty"`
}

type IndexSet struct {
	root      string
	urls      []string
	git       *Git
	indexDirs []string
	prepared  bool
	offline   bool
}

func NewIndexSet(root string, urls []string, git *Git) *IndexSet {
	if len(urls) == 0 {
		urls = []string{DefaultIndex}
	}

	dirs := make([]string, len(urls))
	for i, u := range urls {
		dirs[i] = filepath.Join(root, indexSlug(u))
	}

	return &IndexSet{root: root, urls: urls, git: git, indexDirs: dirs}
}

func (idx *IndexSet) SetOffline(offline bool) { idx.offline = offline }

func (idx *IndexSet) EnsureReady(force bool) error {
	if err := os.MkdirAll(idx.root, 0o755); err != nil {
		return err
	}

	for i, u := range idx.urls {
		dir := idx.indexDirs[i]
		if isGitDir(filepath.Join(dir, ".git")) {
			if force || idx.isStale(dir) {
				if err := idx.git.run(dir, "pull", "--ff-only"); err != nil {
					return fmt.Errorf("refresh index %s: %w", u, err)
				}

				_ = os.Chtimes(filepath.Join(dir, ".sova-fetched"), time.Now(), time.Now())
				_ = os.WriteFile(filepath.Join(dir, ".sova-fetched"), []byte(time.Now().Format(time.RFC3339)), 0o644)
			}

			continue
		}

		if err := idx.git.run("", "clone", "--depth=1", u, dir); err != nil {
			return fmt.Errorf("clone index %s: %w", u, err)
		}

		_ = os.WriteFile(filepath.Join(dir, ".sova-fetched"), []byte(time.Now().Format(time.RFC3339)), 0o644)
	}

	return nil
}

func (idx *IndexSet) Lookup(name string) (IndexEntry, bool, error) {
	if !idx.prepared {
		idx.prepared = true
		if !idx.offline {
			if err := idx.EnsureReady(false); err != nil {
				if !idx.anyClonePresent() {
					return IndexEntry{}, false, fmt.Errorf("index unavailable and package %q needs index lookup: %w", name, err)
				}
			}
		}
	}

	var result IndexEntry
	found := false
	for _, dir := range idx.indexDirs {
		entry, ok, err := readIndexFile(dir, name)
		if err != nil {
			return IndexEntry{}, false, err
		}

		if ok {
			result = entry
			found = true
		}
	}

	return result, found, nil
}

func (idx *IndexSet) anyClonePresent() bool {
	for _, dir := range idx.indexDirs {
		if isGitDir(filepath.Join(dir, ".git")) {
			return true
		}
	}

	return false
}

func (idx *IndexSet) isStale(dir string) bool {
	stamp, err := os.ReadFile(filepath.Join(dir, ".sova-fetched"))
	if err != nil {
		return true
	}

	t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(stamp)))
	if err != nil {
		return true
	}

	return time.Since(t) > IndexRefreshInterval
}

func readIndexFile(dir, name string) (IndexEntry, bool, error) {
	path := indexEntryPath(dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return IndexEntry{}, false, nil
		}

		return IndexEntry{}, false, err
	}

	var entry IndexEntry
	if err := toml.Unmarshal(data, &entry); err != nil {
		return IndexEntry{}, false, fmt.Errorf("parse index entry %s: %w", path, err)
	}

	if strings.TrimSpace(entry.Git) == "" {
		return IndexEntry{}, false, fmt.Errorf("index entry %s: missing `git` field", path)
	}

	return entry, true, nil
}

func indexEntryPath(dir, name string) string {
	if strings.HasPrefix(name, "@") {
		parts := strings.SplitN(strings.TrimPrefix(name, "@"), "/", 2)
		if len(parts) != 2 {
			return filepath.Join(dir, "scopes", "_invalid", name+".toml")
		}

		return filepath.Join(dir, "scopes", "@"+parts[0], parts[1]+".toml")
	}

	if len(name) == 0 {
		return ""
	}

	first := strings.ToLower(string(name[0]))
	two := first
	if len(name) >= 2 {
		two = strings.ToLower(name[:2])
	}

	return filepath.Join(dir, "packages", first, two, name+".toml")
}

func indexSlug(url string) string {
	h := sha256.Sum256([]byte(NormaliseURL(url)))
	return hex.EncodeToString(h[:8])
}
