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

const ManifestFilename = "sova.toml"

type Manifest struct {
	Package         *PackageMeta              `toml:"package"`
	Dependencies    map[string]DependencySpec `toml:"dependencies"`
	DevDependencies map[string]DependencySpec `toml:"dev-dependencies"`
	NPM             map[string]NPMDepSpec     `toml:"npm-dependencies"`
	Workspace       *WorkspaceSection         `toml:"workspace"`
	Overrides       map[string]DependencySpec `toml:"overrides"`

	root string
}

type NPMDepSpec struct {
	Version string `toml:"version,omitempty"`
	Package string `toml:"package,omitempty"`
	Default *bool  `toml:"default,omitempty"`
}

func (n *NPMDepSpec) UnmarshalTOML(data any) error {
	switch v := data.(type) {
	case string:
		n.Version = v
		return nil
	case map[string]any:
		if s, ok := v["version"].(string); ok {
			n.Version = s
		}

		if s, ok := v["package"].(string); ok {
			n.Package = s
		}

		if b, ok := v["default"].(bool); ok {
			n.Default = &b
		}

		return nil
	}

	return nil
}

type PackageMeta struct {
	Name        string   `toml:"name"`
	Version     string   `toml:"version"`
	Description string   `toml:"description,omitempty"`
	Authors     []string `toml:"authors,omitempty"`
	License     string   `toml:"license,omitempty"`
}

type WorkspaceSection struct {
	Members        []string                  `toml:"members"`
	DefaultMembers []string                  `toml:"default-members,omitempty"`
	Dependencies   map[string]DependencySpec `toml:"dependencies"`
}

type DependencySpec struct {
	Version   string `toml:"version,omitempty"`
	Git       string `toml:"git,omitempty"`
	Tag       string `toml:"tag,omitempty"`
	Branch    string `toml:"branch,omitempty"`
	Rev       string `toml:"rev,omitempty"`
	Path      string `toml:"path,omitempty"`
	Subdir    string `toml:"subdir,omitempty"`
	Workspace bool   `toml:"workspace,omitempty"`
	Optional  bool   `toml:"optional,omitempty"`
}

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

		if s, ok := v["subdir"].(string); ok {
			d.Subdir = s
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

func (d DependencySpec) NormalisedSubdir() (string, error) {
	raw := strings.TrimSpace(d.Subdir)
	if raw == "" {
		return "", nil
	}

	clean := strings.TrimPrefix(strings.ReplaceAll(raw, "\\", "/"), "./")
	clean = strings.TrimPrefix(clean, "/")
	if clean == "" {
		return "", fmt.Errorf("subdir %q resolves to the repository root, drop the field instead", d.Subdir)
	}

	cleaned := filepath.ToSlash(filepath.Clean(clean))
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", fmt.Errorf("subdir %q must stay within the repository (no parent traversal allowed)", d.Subdir)
	}

	return cleaned, nil
}

type SourceKind int

const (
	SourceKindIndex SourceKind = iota
	SourceKindGit
	SourceKindPath
	SourceKindWorkspace
)

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

type RefSelector int

const (
	RefSelectorNone RefSelector = iota
	RefSelectorRev
	RefSelectorTag
	RefSelectorBranch
	RefSelectorRange
)

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

func (m *Manifest) Root() string { return m.root }

func (m *Manifest) IsWorkspaceRoot() bool {
	return m.Workspace != nil && len(m.Workspace.Members) > 0
}

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

func (m *Manifest) PackageName() string {
	if m.Package == nil {
		return ""
	}

	return strings.TrimSpace(m.Package.Name)
}
