package pkgmgr

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// Resolution is the complete dependency graph computed by `Resolver.Resolve`. It's the in-memory representation of what eventually gets written to the lockfile and materialised under `.sova/deps`.
type Resolution struct {
	Packages map[string]*ResolvedPackage
}

// ResolvedPackage is one node in the resolved graph. `Dir` is the on-disk location the materialiser will hardlink from (cache slot for git/index sources, the literal path for path/workspace).
type ResolvedPackage struct {
	Name         string
	Version      string
	Source       string
	Commit       string
	Dir          string
	Dependencies []string
}

// Resolver drives `manifest -> resolution`. Stateless per-instance, safe to reuse across multiple `Resolve` calls within one CLI invocation.
type Resolver struct {
	Index *IndexSet
	Cache *Cache
	Git   *Git
}

// Resolve walks the root workspace's dependency graph and returns a fully-determined Resolution. The walk visits every member's [dependencies] (and the root's, if it is also a package), constructs constraints per package name, and selects the highest version of each that satisfies every constraint seen. Path/workspace sources resolve immediately; git/index sources go through the cache.
func (r *Resolver) Resolve(workspace *Manifest, includeDev bool) (*Resolution, error) {
	res := &Resolution{Packages: map[string]*ResolvedPackage{}}
	state := newResolveState()
	if err := state.seedFromWorkspace(workspace, includeDev); err != nil {
		return nil, err
	}
	for state.hasWork() {
		pending := state.takeNext()
		if pending == nil {
			break
		}
		if existing, ok := res.Packages[pending.Name]; ok {
			if err := state.reconcile(pending, existing); err != nil {
				return nil, err
			}
			continue
		}
		pkg, err := r.resolveOne(workspace, pending, state)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", pending.Name, err)
		}
		res.Packages[pkg.Name] = pkg
		if err := state.enqueueDependencies(pkg, workspace); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// pendingDep is one constraint added to the work queue by either the root workspace seeding or a transitive dep walk.
type pendingDep struct {
	Name   string
	Spec   DependencySpec
	Origin string
}

// resolveState tracks the in-flight work queue plus the cumulative per-name constraints seen so far. Constraints accumulate across the entire walk so that two consumers of the same package each get a version that satisfies both.
type resolveState struct {
	queue       []*pendingDep
	constraints map[string][]constraintEntry
}

type constraintEntry struct {
	Spec   DependencySpec
	Origin string
}

func newResolveState() *resolveState {
	return &resolveState{constraints: map[string][]constraintEntry{}}
}

func (s *resolveState) hasWork() bool { return len(s.queue) > 0 }

func (s *resolveState) takeNext() *pendingDep {
	if len(s.queue) == 0 {
		return nil
	}
	next := s.queue[0]
	s.queue = s.queue[1:]
	return next
}

// reconcile validates that `pending` is compatible with the already-selected `existing` package. The constraint is recorded either way; the conflict check fires when the new constraint excludes the picked version. For source-kind mismatches (e.g. one consumer wants the registry version, another a local path), the existing pick wins for the v1 resolver - the user can hoist the dep into `[overrides]` if that's wrong.
func (s *resolveState) reconcile(pending *pendingDep, existing *ResolvedPackage) error {
	s.constraints[pending.Name] = append(s.constraints[pending.Name], constraintEntry{Spec: pending.Spec, Origin: pending.Origin})
	if existing.Version == "" {
		return nil
	}
	if pending.Spec.Kind() == SourceKindIndex || (pending.Spec.Kind() == SourceKindGit && pending.Spec.Version != "") {
		picked, err := semver.NewVersion(existing.Version)
		if err != nil {
			return nil
		}
		if pending.Spec.Version == "" {
			return nil
		}
		constraint, err := semver.NewConstraint(pending.Spec.Version)
		if err != nil {
			return fmt.Errorf("constraint %q for %s (from %s): %w", pending.Spec.Version, pending.Name, pending.Origin, err)
		}
		if !constraint.Check(picked) {
			return fmt.Errorf("dependency %s: version %s already selected (by earlier constraint) does not satisfy %q (from %s)", pending.Name, existing.Version, pending.Spec.Version, pending.Origin)
		}
	}
	return nil
}

// seedFromWorkspace builds the initial work queue from the workspace's member packages. When the workspace root is itself a package, its direct deps are added; every member's deps are added too. Workspace-member resolution happens by name lookup against the member set, not by network.
func (s *resolveState) seedFromWorkspace(ws *Manifest, includeDev bool) error {
	queueFromManifest := func(pkg *Manifest, origin string) error {
		deps, err := pkg.EffectiveDependencies(ws, false)
		if err != nil {
			return err
		}
		for name, spec := range deps {
			s.queue = append(s.queue, &pendingDep{Name: name, Spec: spec, Origin: origin})
			s.constraints[name] = append(s.constraints[name], constraintEntry{Spec: spec, Origin: origin})
		}
		if includeDev {
			devDeps, err := pkg.EffectiveDependencies(ws, true)
			if err != nil {
				return err
			}
			for name, spec := range devDeps {
				s.queue = append(s.queue, &pendingDep{Name: name, Spec: spec, Origin: origin + " (dev)"})
				s.constraints[name] = append(s.constraints[name], constraintEntry{Spec: spec, Origin: origin + " (dev)"})
			}
		}
		return nil
	}
	if ws.Package != nil {
		if err := queueFromManifest(ws, "workspace root"); err != nil {
			return err
		}
	}
	members, err := ws.ResolveWorkspaceMembers()
	if err != nil {
		return err
	}
	for _, dir := range members {
		mPath := filepath.Join(dir, ManifestFilename)
		member, ok, err := LoadManifest(mPath)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		origin := member.PackageName()
		if origin == "" {
			origin = "workspace:" + dir
		}
		if err := queueFromManifest(member, origin); err != nil {
			return err
		}
	}
	return nil
}

// enqueueDependencies reads a freshly-resolved package's own manifest and adds its `[dependencies]` to the work queue. `[dev-dependencies]` of transitive deps are deliberately skipped (Cargo's rule).
func (s *resolveState) enqueueDependencies(pkg *ResolvedPackage, ws *Manifest) error {
	manifestPath := filepath.Join(pkg.Dir, ManifestFilename)
	depManifest, ok, err := LoadManifest(manifestPath)
	if err != nil {
		return fmt.Errorf("read manifest of dep %s: %w", pkg.Name, err)
	}
	if !ok {
		return nil
	}
	deps, err := depManifest.EffectiveDependencies(ws, false)
	if err != nil {
		return err
	}
	for name, spec := range deps {
		if spec.Workspace {
			return fmt.Errorf("dep %s declares workspace=true for %q, but workspace propagation across deps isn't supported", pkg.Name, name)
		}
		s.queue = append(s.queue, &pendingDep{Name: name, Spec: spec, Origin: pkg.Name})
		s.constraints[name] = append(s.constraints[name], constraintEntry{Spec: spec, Origin: pkg.Name})
		pkg.Dependencies = append(pkg.Dependencies, name)
	}
	sort.Strings(pkg.Dependencies)
	return nil
}

// resolveOne turns one pending dep into a ResolvedPackage. The branch logic dispatches on source kind: index does alias lookup then falls through to git; git resolves the ref; path/workspace skip the cache entirely.
func (r *Resolver) resolveOne(workspace *Manifest, pending *pendingDep, state *resolveState) (*ResolvedPackage, error) {
	if override, ok := workspace.Overrides[pending.Name]; ok {
		pending = &pendingDep{Name: pending.Name, Spec: override, Origin: "override"}
	}
	switch pending.Spec.Kind() {
	case SourceKindWorkspace:
		return r.resolveWorkspaceMember(workspace, pending.Name)
	case SourceKindPath:
		return r.resolvePath(workspace, pending)
	case SourceKindGit:
		return r.resolveGit(pending, state, pending.Spec.Git)
	case SourceKindIndex:
		if r.Index == nil {
			return nil, fmt.Errorf("no index configured but %q has no explicit source", pending.Name)
		}
		entry, ok, err := r.Index.Lookup(pending.Name)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("package %q not found in any configured index", pending.Name)
		}
		return r.resolveGit(pending, state, entry.Git)
	}
	return nil, fmt.Errorf("unknown source kind for %q", pending.Name)
}

func (r *Resolver) resolveWorkspaceMember(workspace *Manifest, name string) (*ResolvedPackage, error) {
	members, err := workspace.ResolveWorkspaceMembers()
	if err != nil {
		return nil, err
	}
	for _, dir := range members {
		m, ok, err := LoadManifest(filepath.Join(dir, ManifestFilename))
		if err != nil || !ok {
			continue
		}
		if m.PackageName() == name {
			version := "0.0.0"
			if m.Package != nil && m.Package.Version != "" {
				version = m.Package.Version
			}
			return &ResolvedPackage{
				Name:    name,
				Version: version,
				Source:  "workspace",
				Dir:     dir,
			}, nil
		}
	}
	return nil, fmt.Errorf("workspace member %q not found", name)
}

func (r *Resolver) resolvePath(workspace *Manifest, pending *pendingDep) (*ResolvedPackage, error) {
	abs := pending.Spec.Path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(workspace.Root(), abs)
	}
	abs = filepath.Clean(abs)
	m, ok, err := LoadManifest(filepath.Join(abs, ManifestFilename))
	if err != nil {
		return nil, err
	}
	version := "0.0.0"
	name := pending.Name
	if ok && m.Package != nil {
		if m.Package.Name != "" {
			name = m.Package.Name
		}
		if m.Package.Version != "" {
			version = m.Package.Version
		}
	}
	return &ResolvedPackage{
		Name:    name,
		Version: version,
		Source:  "path+" + abs,
		Dir:     abs,
	}, nil
}

// resolveGit handles both `{ git = "...", tag/branch/rev = "..." }` and index-resolved entries. The selector priority is rev > tag > branch > version range; range mode enumerates remote tags, semver-filters, picks the highest.
func (r *Resolver) resolveGit(pending *pendingDep, state *resolveState, repoURL string) (*ResolvedPackage, error) {
	selector, value := pending.Spec.SelectRef()
	if selector == RefSelectorNone {
		selector, value = RefSelectorBranch, "HEAD"
	}
	var sha, dir, picked string
	switch selector {
	case RefSelectorRev, RefSelectorTag, RefSelectorBranch:
		ref := value
		if selector == RefSelectorBranch && ref == "HEAD" {
			ref = "HEAD"
		}
		var err error
		sha, dir, err = r.Cache.ResolveAndMaterialise(repoURL, ref, false)
		if err != nil {
			return nil, err
		}
		picked = pickedVersionFromTag(value, sha)
	case RefSelectorRange:
		entries := state.constraints[pending.Name]
		constraints := make([]*semver.Constraints, 0, len(entries))
		for _, c := range entries {
			if c.Spec.Version == "" {
				continue
			}
			parsed, err := semver.NewConstraint(c.Spec.Version)
			if err != nil {
				return nil, fmt.Errorf("constraint %q (from %s): %w", c.Spec.Version, c.Origin, err)
			}
			constraints = append(constraints, parsed)
		}
		tags, err := r.Git.LsRemoteTags(repoURL)
		if err != nil {
			return nil, err
		}
		best, bestVersion, bestSha, err := selectBestTag(tags, constraints)
		if err != nil {
			return nil, err
		}
		if best == "" {
			return nil, fmt.Errorf("no version satisfying %q (origin: %s) in tags of %s", pending.Spec.Version, pending.Origin, repoURL)
		}
		sha, dir, err = r.Cache.ResolveAndMaterialise(repoURL, bestSha, true)
		if err != nil {
			return nil, err
		}
		_ = bestVersion
		picked = bestVersion
	}
	pkg := &ResolvedPackage{
		Name:    pending.Name,
		Version: picked,
		Source:  "git+" + NormaliseURL(repoURL),
		Commit:  sha,
		Dir:     dir,
	}
	if m, ok, err := LoadManifest(filepath.Join(dir, ManifestFilename)); err == nil && ok && m.Package != nil {
		if m.Package.Name != "" {
			pkg.Name = m.Package.Name
		}
		if picked == "" && m.Package.Version != "" {
			pkg.Version = m.Package.Version
		}
	}
	if pkg.Version == "" {
		pkg.Version = "0.0.0-" + shortSha(sha)
	}
	return pkg, nil
}

// selectBestTag enumerates remote tags, filters to SemVer-shaped names, and returns the highest version that satisfies every constraint. The tag's commit SHA travels with it so the materialiser can skip a separate `ls-remote`.
func selectBestTag(tags []RemoteTag, constraints []*semver.Constraints) (tagName, version, sha string, err error) {
	type candidate struct {
		tag string
		v   *semver.Version
		sha string
	}
	cands := make([]candidate, 0, len(tags))
	for _, t := range tags {
		stripped := strings.TrimPrefix(t.Name, "v")
		v, err := semver.NewVersion(stripped)
		if err != nil {
			continue
		}
		cands = append(cands, candidate{tag: t.Name, v: v, sha: t.Commit})
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].v.GreaterThan(cands[j].v) })
	for _, c := range cands {
		ok := true
		for _, cc := range constraints {
			if !cc.Check(c.v) {
				ok = false
				break
			}
		}
		if ok {
			return c.tag, c.v.String(), c.sha, nil
		}
	}
	return "", "", "", nil
}

func pickedVersionFromTag(tag, sha string) string {
	stripped := strings.TrimPrefix(tag, "v")
	if v, err := semver.NewVersion(stripped); err == nil {
		return v.String()
	}
	return "0.0.0-" + shortSha(sha)
}

func shortSha(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}
	return sha
}
