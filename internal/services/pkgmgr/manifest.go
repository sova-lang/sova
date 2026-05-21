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

// ManifestFilename is the filename of the project's package-manager manifest. Co-existing with `sova.toml`'s other sections (`[project]`, `[build]`, ...); the package-manager sections (`[package]`, `[dependencies]`, ...) are read from the same file.
const ManifestFilename = "sova.toml"

// Manifest is the package-manager view of `sova.toml`. The compiler-config sections (`[project]`, `[build]`, ...) live in a separate type in `internal/cli` and are deliberately not duplicated here.
type Manifest struct {
	Package         *PackageMeta              `toml:"package"`
	Dependencies    map[string]DependencySpec `toml:"dependencies"`
	DevDependencies map[string]DependencySpec `toml:"dev-dependencies"`
	Workspace       *WorkspaceSection         `toml:"workspace"`
	Overrides       map[string]DependencySpec `toml:"overrides"`

	root string
}

// PackageMeta describes the package's own identity. Only `Name` is required when the manifest declares `[package]`; everything else is optional metadata.
type PackageMeta struct {
	Name        string   `toml:"name"`
	Version     string   `toml:"version"`
	Description string   `toml:"description,omitempty"`
	Authors     []string `toml:"authors,omitempty"`
	License     string   `toml:"license,omitempty"`
}

// WorkspaceSection captures the `[workspace]` table. Member paths can be plain directories (`"pkg-a"`) or glob patterns (`"apps/*"`). Glob expansion happens in `(*Manifest).ResolveWorkspaceMembers`.
type WorkspaceSection struct {
	Members        []string                  `toml:"members"`
	DefaultMembers []string                  `toml:"default-members,omitempty"`
	Dependencies   map[string]DependencySpec `toml:"dependencies"`
}

// DependencySpec is the resolved view of a single dep declaration. The TOML grammar accepts two surface forms - a bare version string (`http = "^1.0"`) or an inline table (`http = { git = "...", tag = "..." }`) - both unmarshal into this struct via a custom UnmarshalTOML method.
type DependencySpec struct {
	Version   string `toml:"version,omitempty"`
	Git       string `toml:"git,omitempty"`
	Tag       string `toml:"tag,omitempty"`
	Branch    string `toml:"branch,omitempty"`
	Rev       string `toml:"rev,omitempty"`
	Path      string `toml:"path,omitempty"`
	Workspace bool   `toml:"workspace,omitempty"`
	Optional  bool   `toml:"optional,omitempty"`
}

// UnmarshalTOML accepts either a bare version-range string (`"^1.0"`) or an inline TOML table. Required because BurntSushi/toml maps a TOML scalar to a struct field only when the field is itself a scalar type; we need the polymorphism handled at the struct level.
func (d *DependencySpec) UnmarshalTOML(data any) error {
	switch v := data.(type) {
	case string:
		d.Version = v
		return nil
	case map[string]any:
		if s, ok := v["version"].(string); ok {
			d.Version = s
		}
		if s, ok := v["git"].(string); ok {
			d.Git = s
		}
		if s, ok := v["tag"].(string); ok {
			d.Tag = s
		}
		if s, ok := v["branch"].(string); ok {
			d.Branch = s
		}
		if s, ok := v["rev"].(string); ok {
			d.Rev = s
		}
		if s, ok := v["path"].(string); ok {
			d.Path = s
		}
		if b, ok := v["workspace"].(bool); ok {
			d.Workspace = b
		}
		if b, ok := v["optional"].(bool); ok {
			d.Optional = b
		}
		return nil
	}
	return fmt.Errorf("dependency: expected string or table, got %T", data)
}

// SourceKind classifies a dependency by what backs it: a git URL (with optional ref), a local path on disk, a workspace sibling, or an index alias that still needs to be resolved against the alias map. The resolver dispatches per kind.
type SourceKind int

const (
	SourceKindIndex SourceKind = iota
	SourceKindGit
	SourceKindPath
	SourceKindWorkspace
)

// Kind classifies the spec into one of the resolver's input shapes. Path and Workspace are direct (no git/network); Git is a direct URL with a ref selector; Index requires alias lookup before resolution can proceed.
func (d DependencySpec) Kind() SourceKind {
	switch {
	case d.Workspace:
		return SourceKindWorkspace
	case d.Path != "":
		return SourceKindPath
	case d.Git != "":
		return SourceKindGit
	default:
		return SourceKindIndex
	}
}

// RefSelector classifies the kind of git ref this spec selects, in priority order: `rev` > `tag` > `branch` > version range.
type RefSelector int

const (
	RefSelectorNone RefSelector = iota
	RefSelectorRev
	RefSelectorTag
	RefSelectorBranch
	RefSelectorRange
)

// SelectRef picks the ref selector form: explicit `rev` / `tag` / `branch` win in that order, falling back to the version-range string. Returns RefSelectorNone only when the spec has neither - invalid for index/git sources, valid only for `workspace = true` and `path = ...`.
func (d DependencySpec) SelectRef() (RefSelector, string) {
	if d.Rev != "" {
		return RefSelectorRev, d.Rev
	}
	if d.Tag != "" {
		return RefSelectorTag, d.Tag
	}
	if d.Branch != "" {
		return RefSelectorBranch, d.Branch
	}
	if d.Version != "" {
		return RefSelectorRange, d.Version
	}
	return RefSelectorNone, ""
}

// LoadManifest reads and parses a project manifest, returning ok=false (and zero Manifest) when the file is absent.
func LoadManifest(path string) (*Manifest, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	m := &Manifest{root: filepath.Dir(path)}
	if err := toml.Unmarshal(data, m); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", path, err)
	}
	return m, true, nil
}

// Root returns the directory the manifest was loaded from. Empty for in-memory manifests.
func (m *Manifest) Root() string { return m.root }

// IsWorkspaceRoot reports whether this manifest's `[workspace]` block has any members, meaning it's a workspace root (it may or may not also be a package; the two are independent).
func (m *Manifest) IsWorkspaceRoot() bool {
	return m.Workspace != nil && len(m.Workspace.Members) > 0
}

// ResolveWorkspaceMembers expands glob patterns in `[workspace] members` into absolute directory paths. Each result is verified to contain a `sova.toml` with a `[package]` section; entries that fail this check are returned as errors but processing continues.
func (m *Manifest) ResolveWorkspaceMembers() ([]string, error) {
	if m.Workspace == nil {
		return nil, nil
	}
	var members []string
	seen := map[string]bool{}
	for _, raw := range m.Workspace.Members {
		full := filepath.Join(m.root, raw)
		matches, err := filepath.Glob(full)
		if err != nil {
			return nil, fmt.Errorf("workspace member %q: %w", raw, err)
		}
		if len(matches) == 0 {
			matches = []string{full}
		}
		sort.Strings(matches)
		for _, p := range matches {
			info, err := os.Stat(p)
			if err != nil || !info.IsDir() {
				continue
			}
			abs, _ := filepath.Abs(p)
			if seen[abs] {
				continue
			}
			seen[abs] = true
			members = append(members, abs)
		}
	}
	return members, nil
}

// EffectiveDependencies returns the merged dependency view for a single package within a workspace context: a workspace member's `name.workspace = true` entries get resolved against `workspaceManifest.Workspace.Dependencies`; other entries pass through unchanged. For a non-workspace project, `workspaceManifest` is the same as the package manifest and only the package-level deps are returned.
func (m *Manifest) EffectiveDependencies(workspaceManifest *Manifest, dev bool) (map[string]DependencySpec, error) {
	src := m.Dependencies
	if dev {
		src = m.DevDependencies
	}
	if src == nil {
		return map[string]DependencySpec{}, nil
	}
	out := make(map[string]DependencySpec, len(src))
	for name, spec := range src {
		if !spec.Workspace {
			out[name] = spec
			continue
		}
		if workspaceManifest == nil || workspaceManifest.Workspace == nil {
			return nil, fmt.Errorf("dependency %q references workspace, but no workspace manifest is in scope", name)
		}
		wsSpec, ok := workspaceManifest.Workspace.Dependencies[name]
		if !ok {
			return nil, fmt.Errorf("dependency %q sets workspace=true, but no matching entry in [workspace.dependencies]", name)
		}
		out[name] = wsSpec
	}
	return out, nil
}

// PackageName returns this manifest's package name when it's a package (has `[package]`); for a virtual workspace root with no `[package]`, returns the empty string.
func (m *Manifest) PackageName() string {
	if m.Package == nil {
		return ""
	}
	return strings.TrimSpace(m.Package.Name)
}
