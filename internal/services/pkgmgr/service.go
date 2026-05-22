package pkgmgr

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Service is the high-level facade the CLI talks to. It composes the manifest loader, lockfile, cache, index, resolver, and materialiser into one ergonomic surface, hiding the per-call wiring from command handlers.
type Service struct {
	ProjectRoot string
	Verbose     bool
}

// NewService binds a Service to a project root. The project root is the directory containing `sova.toml`; CLI commands resolve it via cwd-walk before constructing the service.
func NewService(projectRoot string) *Service {
	return &Service{ProjectRoot: projectRoot}
}

// InstallOptions controls a single `Install` invocation. `Frozen` is CI mode: hard-fail if resolution would change the lockfile.
type InstallOptions struct {
	IncludeDev bool
	Frozen     bool
	Offline    bool
	LinkMode   LinkMode
}

// Install reads the manifest, resolves the graph (unless a fresh lockfile already matches), materialises `.sova/deps/`, and writes the lockfile. Returns the resolution used so callers can render summaries.
func (s *Service) Install(opts InstallOptions) (*Resolution, error) {
	manifest, ok, err := LoadManifest(filepath.Join(s.ProjectRoot, ManifestFilename))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("no %s found at %s", ManifestFilename, s.ProjectRoot)
	}
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	git := NewGit(s.Verbose)
	cache := NewCache(cfg.CacheRoot(), git)
	index := NewIndexSet(cfg.IndexRoot(), cfg.IndexURLs(), git)
	index.SetOffline(opts.Offline)
	resolver := &Resolver{Index: index, Cache: cache, Git: git}

	if opts.Frozen {
		lock, lockOk, err := LoadLockfile(filepath.Join(s.ProjectRoot, LockfileFilename))
		if err != nil {
			return nil, err
		}
		if !lockOk {
			return nil, errors.New("--frozen requires an existing sova.lock")
		}
		res, err := s.reproduceFromLockfile(lock, cache, manifest)
		if err != nil {
			return nil, err
		}
		mat := &Materialiser{Mode: opts.LinkMode}
		if err := s.applyMaterialisation(mat, res); err != nil {
			return nil, err
		}
		return res, nil
	}

	res, err := resolver.Resolve(manifest, opts.IncludeDev)
	if err != nil {
		return nil, err
	}

	lock := buildLockfile(res)
	if err := lock.Save(filepath.Join(s.ProjectRoot, LockfileFilename)); err != nil {
		return nil, err
	}
	mat := &Materialiser{Mode: opts.LinkMode}
	if err := s.applyMaterialisation(mat, res); err != nil {
		return nil, err
	}
	return res, nil
}

// applyMaterialisation runs the materialiser against the resolution, then applies any local-link overrides on top. Overrides are applied last so they always win over the resolved view.
func (s *Service) applyMaterialisation(mat *Materialiser, res *Resolution) error {
	if err := mat.Apply(s.ProjectRoot, res); err != nil {
		return err
	}
	links, err := LoadLocalLinks(s.ProjectRoot)
	if err != nil {
		return err
	}
	for name, path := range links.Links {
		abs := path
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(s.ProjectRoot, abs)
		}
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			continue
		}
		dest := filepath.Join(s.ProjectRoot, DepsDirname, name)
		_ = os.RemoveAll(dest)
		if err := mat.stageDirect(abs, dest); err != nil {
			return fmt.Errorf("apply local-link %q: %w", name, err)
		}
	}
	return nil
}

// reproduceFromLockfile materialises an existing lockfile without re-resolving. Used by `--frozen` / CI flows where any drift from the committed lockfile is a hard error.
func (s *Service) reproduceFromLockfile(lock *Lockfile, cache *Cache, _ *Manifest) (*Resolution, error) {
	res := &Resolution{Packages: map[string]*ResolvedPackage{}}
	for _, lp := range lock.Packages {
		pkg := &ResolvedPackage{
			Name:    lp.Name,
			Version: lp.Version,
			Source:  lp.Source,
			Commit:  lp.Commit,
			Subdir:  lp.Subdir,
		}
		applySubdir := func(base string) (string, error) {
			if lp.Subdir == "" {
				return base, nil
			}
			joined := filepath.Clean(filepath.Join(base, filepath.FromSlash(lp.Subdir)))
			if info, err := os.Stat(joined); err != nil || !info.IsDir() {
				return "", fmt.Errorf("lockfile entry %s: subdir %q does not exist in %s", lp.Name, lp.Subdir, base)
			}
			return joined, nil
		}
		switch {
		case strings.HasPrefix(lp.Source, "path+"):
			base := strings.TrimPrefix(lp.Source, "path+")
			dir, err := applySubdir(base)
			if err != nil {
				return nil, err
			}
			pkg.Dir = dir
		case lp.Source == "workspace":
			members, err := loadWorkspaceMemberByName(s.ProjectRoot, lp.Name)
			if err != nil {
				return nil, err
			}
			pkg.Dir = members
		case strings.HasPrefix(lp.Source, "git+"):
			repoURL := strings.TrimPrefix(lp.Source, "git+")
			if lp.Commit == "" {
				return nil, fmt.Errorf("lockfile entry %s has no commit; cannot reproduce", lp.Name)
			}
			_, base, err := cache.ResolveAndMaterialise(repoURL, lp.Commit, false)
			if err != nil {
				return nil, err
			}
			dir, err := applySubdir(base)
			if err != nil {
				return nil, err
			}
			pkg.Dir = dir
		}
		for _, raw := range lp.Dependencies {
			parts := strings.SplitN(raw, " ", 2)
			pkg.Dependencies = append(pkg.Dependencies, parts[0])
		}
		res.Packages[pkg.Name] = pkg
	}
	return res, nil
}

// AddOptions configures `Add`. `Spec` is the raw user input (e.g. "^1.2.3" or "github.com/x/y@v1.0.0" or a git URL); `Dev` routes the entry into `[dev-dependencies]`.
type AddOptions struct {
	Name string
	Spec string
	Dev  bool
}

// Add writes a new entry to the manifest and triggers an install. The spec parser accepts: a bare version range, a `github.com/owner/repo@<tag>` form, or a git URL with `#tag=...`/`#branch=...`/`#rev=...` fragments.
func (s *Service) Add(opts AddOptions) (*Resolution, error) {
	manifestPath := filepath.Join(s.ProjectRoot, ManifestFilename)
	mPath, _, err := LoadManifest(manifestPath)
	if err != nil {
		return nil, err
	}
	if mPath == nil {
		return nil, fmt.Errorf("no %s at %s", ManifestFilename, s.ProjectRoot)
	}
	spec, err := parseAddSpec(opts.Spec)
	if err != nil {
		return nil, err
	}
	section := "dependencies"
	if opts.Dev {
		section = "dev-dependencies"
	}
	if err := mergeIntoManifestFile(manifestPath, section, opts.Name, spec); err != nil {
		return nil, err
	}
	return s.Install(InstallOptions{IncludeDev: opts.Dev})
}

// Remove strips a named entry from the manifest's `[dependencies]` and `[dev-dependencies]`, then re-installs to drop it from the lockfile and `.sova/deps/`.
func (s *Service) Remove(name string) (*Resolution, error) {
	manifestPath := filepath.Join(s.ProjectRoot, ManifestFilename)
	if err := stripFromManifestFile(manifestPath, name); err != nil {
		return nil, err
	}
	return s.Install(InstallOptions{})
}

// Update re-resolves the graph from scratch (ignoring lockfile pins) and writes a new lockfile. `names` empty → update everything; otherwise only the listed packages and their transitive deps get re-resolved.
func (s *Service) Update(names []string) (*Resolution, error) {
	lockPath := filepath.Join(s.ProjectRoot, LockfileFilename)
	if len(names) == 0 {
		_ = os.Remove(lockPath)
		return s.Install(InstallOptions{})
	}
	existing, ok, err := LoadLockfile(lockPath)
	if err != nil || !ok {
		_ = os.Remove(lockPath)
		return s.Install(InstallOptions{})
	}
	dropSet := map[string]bool{}
	for _, n := range names {
		dropSet[n] = true
	}
	walked := map[string]bool{}
	var queue []string
	for n := range dropSet {
		queue = append(queue, n)
	}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		if walked[next] {
			continue
		}
		walked[next] = true
		dropSet[next] = true
		if lp, found := existing.FindByName(next); found {
			for _, raw := range lp.Dependencies {
				parts := strings.SplitN(raw, " ", 2)
				if !walked[parts[0]] {
					queue = append(queue, parts[0])
				}
			}
		}
	}
	kept := make([]LockedPackage, 0, len(existing.Packages))
	for _, p := range existing.Packages {
		if !dropSet[p.Name] {
			kept = append(kept, p)
		}
	}
	existing.Packages = kept
	if err := existing.Save(lockPath); err != nil {
		return nil, err
	}
	return s.Install(InstallOptions{})
}

// Link registers a local-path override for the package whose `[package].name` lives at `srcPath`. The src manifest is read to determine the override key, so the user only types `sova link <path>` and the name resolves automatically.
func (s *Service) Link(srcPath string) (string, error) {
	abs, err := filepath.Abs(srcPath)
	if err != nil {
		return "", err
	}
	m, ok, err := LoadManifest(filepath.Join(abs, ManifestFilename))
	if err != nil {
		return "", err
	}
	if !ok || m.Package == nil || m.Package.Name == "" {
		return "", fmt.Errorf("path %s does not contain a package with [package].name", abs)
	}
	links, err := LoadLocalLinks(s.ProjectRoot)
	if err != nil {
		return "", err
	}
	links.Set(m.Package.Name, abs)
	if err := links.Save(s.ProjectRoot); err != nil {
		return "", err
	}
	return m.Package.Name, nil
}

// Unlink removes a local-path override by package name. Returns whether anything was removed.
func (s *Service) Unlink(name string) (bool, error) {
	links, err := LoadLocalLinks(s.ProjectRoot)
	if err != nil {
		return false, err
	}
	if !links.Remove(name) {
		return false, nil
	}
	if err := links.Save(s.ProjectRoot); err != nil {
		return false, err
	}
	return true, nil
}

// OutdatedReport is what `sova outdated` renders. For every package whose source is a git URL with semver tags, the report shows the currently-selected version and the highest tag available upstream.
type OutdatedReport struct {
	Entries []OutdatedEntry
}

// OutdatedEntry is one row in an OutdatedReport.
type OutdatedEntry struct {
	Name    string
	Current string
	Latest  string
	Source  string
}

// Outdated walks the lockfile, queries each git-source package's latest semver tag upstream, and returns the deltas. Skips path/workspace sources (no upstream to query).
func (s *Service) Outdated() (*OutdatedReport, error) {
	lock, ok, err := LoadLockfile(filepath.Join(s.ProjectRoot, LockfileFilename))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("no sova.lock found; run `sova install` first")
	}
	git := NewGit(s.Verbose)
	out := &OutdatedReport{}
	for _, p := range lock.Packages {
		if !strings.HasPrefix(p.Source, "git+") {
			continue
		}
		repoURL := strings.TrimPrefix(p.Source, "git+")
		tags, err := git.LsRemoteTags(repoURL)
		if err != nil {
			out.Entries = append(out.Entries, OutdatedEntry{Name: p.Name, Current: p.Version, Latest: "error: " + err.Error(), Source: p.Source})
			continue
		}
		latest, _, _, _ := selectBestTag(tags, nil)
		out.Entries = append(out.Entries, OutdatedEntry{Name: p.Name, Current: p.Version, Latest: latest, Source: p.Source})
	}
	sort.Slice(out.Entries, func(i, j int) bool { return out.Entries[i].Name < out.Entries[j].Name })
	return out, nil
}

// RefreshIndex forces a `git pull --ff-only` on every configured index repo. Equivalent to `sova index update`.
func (s *Service) RefreshIndex() error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	git := NewGit(s.Verbose)
	idx := NewIndexSet(cfg.IndexRoot(), cfg.IndexURLs(), git)
	return idx.EnsureReady(true)
}

func buildLockfile(res *Resolution) *Lockfile {
	lock := &Lockfile{Version: LockfileVersion}
	for _, pkg := range res.Packages {
		entry := LockedPackage{
			Name:    pkg.Name,
			Version: pkg.Version,
			Source:  pkg.Source,
			Commit:  pkg.Commit,
			Subdir:  pkg.Subdir,
		}
		if checksum, err := ComputeChecksum(pkg.Dir); err == nil {
			entry.Checksum = checksum
		}
		for _, dep := range pkg.Dependencies {
			depPkg, ok := res.Packages[dep]
			if !ok {
				entry.Dependencies = append(entry.Dependencies, dep)
				continue
			}
			entry.Dependencies = append(entry.Dependencies, fmt.Sprintf("%s %s", dep, depPkg.Version))
		}
		lock.Packages = append(lock.Packages, entry)
	}
	return lock
}

func loadWorkspaceMemberByName(projectRoot, name string) (string, error) {
	m, ok, err := LoadManifest(filepath.Join(projectRoot, ManifestFilename))
	if err != nil || !ok {
		return "", fmt.Errorf("workspace member %s: project manifest missing", name)
	}
	members, err := m.ResolveWorkspaceMembers()
	if err != nil {
		return "", err
	}
	for _, dir := range members {
		mm, ok, err := LoadManifest(filepath.Join(dir, ManifestFilename))
		if err != nil || !ok {
			continue
		}
		if mm.PackageName() == name {
			return dir, nil
		}
	}
	return "", fmt.Errorf("workspace member %s not found", name)
}

// parseAddSpec normalises the shorthand forms users type at the CLI:
//   - "^1.2.3"                                        → version range
//   - "github.com/owner/repo@v1.0.0"                  → git URL + tag
//   - "github.com/owner/repo#branch=main"             → git URL + branch
//   - "https://x/y.git#rev=abc1234"                   → git URL + rev
//   - "../local-path"                                 → local path
func parseAddSpec(raw string) (DependencySpec, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return DependencySpec{}, errors.New("empty spec")
	}
	if strings.HasPrefix(s, ".") || strings.HasPrefix(s, "/") {
		return DependencySpec{Path: s}, nil
	}
	if strings.Contains(s, "://") || strings.HasPrefix(s, "git@") || strings.HasPrefix(s, "github.com/") {
		spec := DependencySpec{}
		urlPart, fragment, _ := strings.Cut(s, "#")
		if atIdx := strings.LastIndex(urlPart, "@"); atIdx > 0 && !strings.HasPrefix(urlPart, "git@") {
			rest := urlPart[atIdx+1:]
			if isVersionish(rest) {
				spec.Git = urlPart[:atIdx]
				spec.Tag = rest
				return spec, nil
			}
		}
		spec.Git = urlPart
		if fragment != "" {
			for _, frag := range strings.Split(fragment, "&") {
				k, v, _ := strings.Cut(frag, "=")
				switch k {
				case "tag":
					spec.Tag = v
				case "branch":
					spec.Branch = v
				case "rev":
					spec.Rev = v
				case "subdir":
					spec.Subdir = v
				}
			}
		}
		return spec, nil
	}
	return DependencySpec{Version: s}, nil
}

func isVersionish(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r == '.', r == '-', r == '+', r == '~', r == '^', r == 'v', r == 'V':
		default:
			if r >= 'a' && r <= 'z' {
				continue
			}
			return false
		}
	}
	return len(s) > 0
}

// mergeIntoManifestFile inserts (or replaces) a dependency entry inside the user's `sova.toml` while preserving every other section verbatim. We deliberately do NOT round-trip via BurntSushi/toml's encoder - it doesn't preserve comments, ordering, or whitespace. Instead, locate the target section header and rewrite a single line.
func mergeIntoManifestFile(path, section, name string, spec DependencySpec) error {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	text := string(data)
	body, err := upsertManifestEntry(text, section, name, spec)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), 0o644)
}

func stripFromManifestFile(path, name string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	body := removeManifestEntry(string(data), "dependencies", name)
	body = removeManifestEntry(body, "dev-dependencies", name)
	return os.WriteFile(path, []byte(body), 0o644)
}

func upsertManifestEntry(body, section, name string, spec DependencySpec) (string, error) {
	rendered := renderDependencyValue(spec)
	line := fmt.Sprintf("%s = %s", tomlMaybeQuote(name), rendered)

	lines := strings.Split(body, "\n")
	header := "[" + section + "]"
	headerIdx := -1
	for i, ln := range lines {
		if strings.TrimSpace(ln) == header {
			headerIdx = i
			break
		}
	}
	if headerIdx < 0 {
		if body != "" && !strings.HasSuffix(body, "\n\n") {
			if !strings.HasSuffix(body, "\n") {
				body += "\n"
			}
			body += "\n"
		}
		body += header + "\n" + line + "\n"
		return body, nil
	}
	end := len(lines)
	for i := headerIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			end = i
			break
		}
	}
	for i := headerIdx + 1; i < end; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, _, _ := strings.Cut(trimmed, "=")
		if strings.TrimSpace(strings.Trim(key, "\"")) == name {
			lines[i] = line
			return strings.Join(lines, "\n"), nil
		}
	}
	prefix := lines[:end]
	suffix := lines[end:]
	prefix = append(prefix, line)
	return strings.Join(append(prefix, suffix...), "\n"), nil
}

func removeManifestEntry(body, section, name string) string {
	lines := strings.Split(body, "\n")
	header := "[" + section + "]"
	out := make([]string, 0, len(lines))
	inSection := false
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inSection = trimmed == header
			out = append(out, ln)
			continue
		}
		if inSection && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			key, _, _ := strings.Cut(trimmed, "=")
			if strings.TrimSpace(strings.Trim(key, "\"")) == name {
				continue
			}
		}
		out = append(out, ln)
	}
	return strings.Join(out, "\n")
}

func renderDependencyValue(spec DependencySpec) string {
	if spec.Git == "" && spec.Path == "" && !spec.Workspace && spec.Tag == "" && spec.Branch == "" && spec.Rev == "" && spec.Subdir == "" && !spec.Optional {
		return tomlQuote(spec.Version)
	}
	var parts []string
	if spec.Version != "" {
		parts = append(parts, "version = "+tomlQuote(spec.Version))
	}
	if spec.Git != "" {
		parts = append(parts, "git = "+tomlQuote(spec.Git))
	}
	if spec.Tag != "" {
		parts = append(parts, "tag = "+tomlQuote(spec.Tag))
	}
	if spec.Branch != "" {
		parts = append(parts, "branch = "+tomlQuote(spec.Branch))
	}
	if spec.Rev != "" {
		parts = append(parts, "rev = "+tomlQuote(spec.Rev))
	}
	if spec.Path != "" {
		parts = append(parts, "path = "+tomlQuote(spec.Path))
	}
	if spec.Subdir != "" {
		parts = append(parts, "subdir = "+tomlQuote(spec.Subdir))
	}
	if spec.Workspace {
		parts = append(parts, "workspace = true")
	}
	if spec.Optional {
		parts = append(parts, "optional = true")
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

func tomlMaybeQuote(name string) string {
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return tomlQuote(name)
		}
	}
	return name
}
