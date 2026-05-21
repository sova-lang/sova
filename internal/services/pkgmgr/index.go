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

// IndexRefreshInterval is how stale an index clone may be before `Resolve` triggers an automatic `git fetch`. Explicit `sova index update` bypasses this threshold.
const IndexRefreshInterval = 24 * time.Hour

// IndexEntry is the parsed payload of a single name→URL alias file inside an index repo. Only `Git` is required; the rest is human-facing metadata that surfaces in `sova outdated` / `sova add` UX.
type IndexEntry struct {
	Git         string `toml:"git"`
	Description string `toml:"description,omitempty"`
}

// IndexSet is one or more cloned index repos, queried in order with later entries overriding earlier ones (private index after public lets you shadow a public package with your own fork). Index repos are cloned lazily on the first `Lookup` so projects that only use path/git/workspace sources never hit the network.
type IndexSet struct {
	root      string
	urls      []string
	git       *Git
	indexDirs []string
	prepared  bool
	offline   bool
}

// NewIndexSet binds an IndexSet to a set of index URLs, all materialised under `root`. Empty `urls` falls back to the DefaultIndex.
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

// SetOffline disables all network operations on this IndexSet. Lookups can still hit on-disk clones; missing clones simply report misses instead of attempting to fetch.
func (idx *IndexSet) SetOffline(offline bool) { idx.offline = offline }

// EnsureReady clones any missing index repos and refreshes stale ones (older than IndexRefreshInterval). `force` bypasses the staleness check.
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

// Lookup resolves a name (`http`) or scope-qualified name (`@org/widgets`) to a git URL by querying each configured index. Later indexes win on collision. Returns ok=false when no index has an entry for the name. Triggers a one-time lazy refresh of the index clones on first call; subsequent calls reuse the cached clones.
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

// readIndexFile resolves the on-disk path for a name within one index repo and parses the TOML entry. The layout matches `.docs/Packages.md`: `packages/<a>/<ab>/<name>.toml` for plain names, `scopes/<@org>/<name>.toml` for scoped names.
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
