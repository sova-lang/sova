package pkgmgr

import (
	"fmt"
	"path/filepath"
)

// Cache is the on-disk store for bare git clones and SHA-keyed materialised worktrees, rooted at the configured cache directory (`~/.sova/cache` by default). Every commit SHA maps to an immutable directory: hardlinks from project views into these directories are safe forever.
type Cache struct {
	root string
	git  *Git
}

// NewCache binds a Cache to its root directory and the git helper. Caller is responsible for ensuring `root` exists; the cache lazily creates subdirectories as needed.
func NewCache(root string, git *Git) *Cache {
	return &Cache{root: root, git: git}
}

// BareDir returns the directory that holds (or will hold) the bare clone for `repoURL`.
func (c *Cache) BareDir(repoURL string) string {
	return filepath.Join(c.root, "git", SlugFor(repoURL), "bare.git")
}

// CommitDir returns the directory that holds the materialised worktree for `repoURL` at `sha`. The path is content-addressed by SHA so multiple project views can hardlink from the same source.
func (c *Cache) CommitDir(repoURL, sha string) string {
	return filepath.Join(c.root, "git", SlugFor(repoURL), "commits", sha)
}

// EnsureBare clones `repoURL` if its bare slot is empty, then refreshes refs unless `skipFetch` is set. Returns the bare directory path.
func (c *Cache) EnsureBare(repoURL string, skipFetch bool) (string, error) {
	bare := c.BareDir(repoURL)
	if err := c.git.EnsureBareClone(repoURL, bare); err != nil {
		return "", err
	}
	if !skipFetch {
		if err := c.git.FetchAll(bare); err != nil {
			return "", fmt.Errorf("refresh %s: %w", repoURL, err)
		}
	}
	return bare, nil
}

// MaterialiseCommit ensures the SHA-keyed worktree exists in the cache and returns its path. Idempotent - repeated calls with the same SHA do no work after the first.
func (c *Cache) MaterialiseCommit(repoURL, sha string) (string, error) {
	bare := c.BareDir(repoURL)
	dest := c.CommitDir(repoURL, sha)
	if err := c.git.Materialise(bare, sha, dest); err != nil {
		return "", err
	}
	return dest, nil
}

// ResolveAndMaterialise is the common path the installer takes per dep: ensure the bare clone, refresh tags/branches, resolve the symbolic ref (tag/branch/short-sha) to a full SHA, then materialise. Returns the full SHA and the on-disk path.
func (c *Cache) ResolveAndMaterialise(repoURL, ref string, skipFetch bool) (sha, dir string, err error) {
	bare, err := c.EnsureBare(repoURL, skipFetch)
	if err != nil {
		return "", "", err
	}
	sha, err = c.git.ResolveRef(bare, ref)
	if err != nil {
		return "", "", err
	}
	dir, err = c.MaterialiseCommit(repoURL, sha)
	if err != nil {
		return "", "", err
	}
	return sha, dir, nil
}
