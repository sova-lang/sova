package pkgmgr

import (
	"fmt"
	"path/filepath"
)

type Cache struct {
	root string
	git  *Git
}

func NewCache(root string, git *Git) *Cache {
	return &Cache{root: root, git: git}
}

func (c *Cache) BareDir(repoURL string) string {
	return filepath.Join(c.root, "git", SlugFor(repoURL), "bare.git")
}

func (c *Cache) CommitDir(repoURL, sha string) string {
	return filepath.Join(c.root, "git", SlugFor(repoURL), "commits", sha)
}

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

func (c *Cache) MaterialiseCommit(repoURL, sha string) (string, error) {
	bare := c.BareDir(repoURL)
	dest := c.CommitDir(repoURL, sha)
	if err := c.git.Materialise(bare, sha, dest); err != nil {
		return "", err
	}

	return dest, nil
}

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
