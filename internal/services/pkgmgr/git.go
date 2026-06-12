package pkgmgr

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Git wraps the `git` CLI for the operations the package manager needs: bare clones, remote tag enumeration, and SHA-keyed worktree materialisation. We deliberately shell out instead of pulling in a Go git library - git itself handles credential helpers, SSH agents, and shallow protocols correctly across every platform, and we don't need fancy git operations.
type Git struct {
	verbose bool
}

// NewGit builds a Git facade. When verbose, all git invocations stream stderr to the host's stderr for debugging.
func NewGit(verbose bool) *Git { return &Git{verbose: verbose} }

// NormaliseURL canonicalises a git URL to a stable form suitable for cache-dir hashing. Adds `https://` when the scheme is missing AND the input looks remote (e.g. `github.com/me/foo`); local file paths (absolute or starting with `.`) and SSH-style URLs (`git@host:...`) are left alone. Strips a trailing `.git` so the same repo via two URL styles maps to one cache slot.
func NormaliseURL(raw string) string {
	s := strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(s, "/"), strings.HasPrefix(s, "."), strings.HasPrefix(s, "~"):
		// local path; leave alone
	case strings.HasPrefix(s, "git@"), strings.Contains(s, "://"):
		// already a fully-qualified URL or SSH
	default:
		s = "https://" + s
	}
	s = strings.TrimSuffix(s, "/")
	s = strings.TrimSuffix(s, ".git")
	return s
}

// SlugFor builds a deterministic short directory name for a git URL, used as the cache key for that repo's bare clone. Combines a parsed `<host>/<path>` tail with a sha256 prefix of the full URL so two distinct URLs that happen to share a tail don't collide.
func SlugFor(rawURL string) string {
	norm := NormaliseURL(rawURL)
	h := sha256.Sum256([]byte(norm))
	prefix := hex.EncodeToString(h[:6])
	tail := "repo"
	if u, err := url.Parse(norm); err == nil && u.Path != "" {
		segs := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(segs) > 0 {
			tail = sanitiseSlug(segs[len(segs)-1])
		}
	} else if strings.HasPrefix(norm, "git@") {
		if i := strings.Index(norm, ":"); i >= 0 {
			tail = sanitiseSlug(filepath.Base(norm[i+1:]))
		}
	}
	return tail + "-" + prefix
}

func sanitiseSlug(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return "repo"
	}
	return string(out)
}

// EnsureBareClone makes sure a bare clone for `repoURL` exists at `bareDir`. Re-uses an existing clone (without fetching) when it's already there; the caller decides separately whether to call FetchAll.
func (g *Git) EnsureBareClone(repoURL, bareDir string) error {
	if isGitDir(bareDir) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(bareDir), 0o755); err != nil {
		return err
	}
	return g.run("", "clone", "--bare", "--filter=blob:none", repoURL, bareDir)
}

// FetchAll updates a bare clone with all tags and branches. Safe to call repeatedly; git de-duplicates.
func (g *Git) FetchAll(bareDir string) error {
	if !isGitDir(bareDir) {
		return fmt.Errorf("not a git directory: %s", bareDir)
	}
	return g.run(bareDir, "fetch", "--all", "--tags", "--force", "--prune", "--prune-tags")
}

// RemoteTag is one entry from `git ls-remote --tags`: the tag name and the commit SHA it points at. Annotated tags expose both the tag object and the dereferenced commit (`^{}` suffix); we keep the dereferenced form when present so SemVer-resolve maps directly to a commit SHA.
type RemoteTag struct {
	Name   string
	Commit string
}

// LsRemoteTags lists tags upstream without cloning. Used by the resolver during SemVer range evaluation to enumerate candidate versions for `version = "^1.0"`-style entries before deciding whether to fetch.
func (g *Git) LsRemoteTags(repoURL string) ([]RemoteTag, error) {
	out, err := g.runOutput("", "ls-remote", "--tags", repoURL)
	if err != nil {
		return nil, err
	}
	dereferenced := map[string]string{}
	plain := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		sha := fields[0]
		ref := fields[1]
		const prefix = "refs/tags/"
		if !strings.HasPrefix(ref, prefix) {
			continue
		}
		name := strings.TrimPrefix(ref, prefix)
		if strings.HasSuffix(name, "^{}") {
			dereferenced[strings.TrimSuffix(name, "^{}")] = sha
		} else if _, exists := plain[name]; !exists {
			plain[name] = sha
		}
	}
	tags := make([]RemoteTag, 0, len(plain))
	for name, sha := range plain {
		if deref, ok := dereferenced[name]; ok {
			tags = append(tags, RemoteTag{Name: name, Commit: deref})
		} else {
			tags = append(tags, RemoteTag{Name: name, Commit: sha})
		}
	}
	return tags, nil
}

// ResolveRef resolves a tag, branch, or short SHA against a bare clone, returning the full 40-char commit SHA. Used after `EnsureBareClone` + `FetchAll` to lock a manifest-level ref selector to an immutable commit before cache materialisation.
func (g *Git) ResolveRef(bareDir, ref string) (string, error) {
	candidates := []string{
		ref,
		"refs/tags/" + ref,
		"refs/heads/" + ref,
		"refs/remotes/origin/" + ref,
	}
	for _, c := range candidates {
		out, err := g.runOutput(bareDir, "rev-parse", "--verify", c+"^{commit}")
		if err == nil {
			sha := strings.TrimSpace(out)
			if len(sha) >= 7 {
				return sha, nil
			}
		}
	}
	return "", fmt.Errorf("ref %q not found in %s", ref, bareDir)
}

// Materialise extracts the tree at `sha` into `destDir`. Atomic: writes to a tempdir alongside `destDir`, then renames. Idempotent: if `destDir` exists and contains a `.sova-commit` sentinel matching `sha`, it's a no-op.
//
// Uses `git archive | tar -x` rather than `git checkout` so the extraction is index-free: a bare repo only has one index file, and parallel `checkout` calls on the same bare (which is the common case when several deps point at the same monorepo via different subdirs) would race on that single index and silently leave the cache slot with shuffled file contents from whichever checkout lost the race. `archive` reads straight from the object store and never touches the index, so the same bare repo can be extracted concurrently for different SHAs without corruption.
func (g *Git) Materialise(bareDir, sha, destDir string) error {
	sentinel := filepath.Join(destDir, ".sova-commit")
	if data, err := os.ReadFile(sentinel); err == nil {
		if strings.TrimSpace(string(data)) == sha {
			return nil
		}
	}
	parent := filepath.Dir(destDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, ".materialise-")
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tmp)
		}
	}()
	if err := g.archiveExtract(bareDir, sha, tmp); err != nil {
		return fmt.Errorf("git archive %s: %w", sha[:8], err)
	}
	if err := os.WriteFile(filepath.Join(tmp, ".sova-commit"), []byte(sha+"\n"), 0o644); err != nil {
		return err
	}
	if _, err := os.Stat(destDir); err == nil {
		_ = os.RemoveAll(destDir)
	}
	if err := os.Rename(tmp, destDir); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// archiveExtract pipes `git archive --format=tar <sha>` from the bare repo into a `tar -xf - -C <destDir>`. Selecting tar over zip keeps Unix file modes intact; the operation reads from the object store only and is safe to run concurrently across multiple destinations.
func (g *Git) archiveExtract(bareDir, sha, destDir string) error {
	archive := exec.Command("git", "--git-dir="+bareDir, "archive", "--format=tar", sha)
	extract := exec.Command("tar", "-x", "-f", "-", "-C", destDir)
	pipe, err := archive.StdoutPipe()
	if err != nil {
		return err
	}
	extract.Stdin = pipe
	var archiveErr, extractErr bytes.Buffer
	if g.verbose {
		archive.Stderr = os.Stderr
		extract.Stderr = os.Stderr
	} else {
		archive.Stderr = &archiveErr
		extract.Stderr = &extractErr
	}
	if err := extract.Start(); err != nil {
		return fmt.Errorf("tar start: %w", err)
	}
	if err := archive.Run(); err != nil {
		_ = extract.Wait()
		msg := strings.TrimSpace(archiveErr.String())
		if msg == "" {
			return err
		}
		return fmt.Errorf("%s", msg)
	}
	if err := extract.Wait(); err != nil {
		msg := strings.TrimSpace(extractErr.String())
		if msg == "" {
			return fmt.Errorf("tar extract: %w", err)
		}
		return fmt.Errorf("tar extract: %s", msg)
	}
	return nil
}

func isGitDir(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "HEAD"))
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func (g *Git) run(workDir string, args ...string) error {
	cmd := exec.Command("git", args...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if g.verbose {
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return nil
}

func (g *Git) runOutput(workDir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}
