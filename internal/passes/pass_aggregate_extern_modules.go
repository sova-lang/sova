package passes

import (
	"sova/internal/diag"
	"sova/internal/ir"
	"strings"
)

// ExternGoModulesCacheKey is the cache key under which PassAggregateExternModules stores the resolved Go-module pin map (`map[modulePath]version`). The Go emitter reads this map to synthesise `require` lines into the generated `go.mod`. Modules that appear in extern declarations without an explicit `@version` selector are intentionally omitted: the user has opted into "whatever go mod tidy resolves" for that dependency, and forcing a pin here would silently overwrite the lockfile entry.
const ExternGoModulesCacheKey = "extern_go_modules"

// PassAggregateExternModules is a build-scoped pass that walks every backend or shared file in the build, collects the (module-path, version) pairs declared via `extern "path@version"` / `backend("path@version")`, and stores the merged pin set in the pass-context cache for the Go code emitter to read. The pass also enforces two consistency rules: every pin for the same module path must agree on the version, and each pin must look like a Go-toolchain-accepted version selector. Conflicts and malformed pins surface as regular Sova diagnostics rather than late `go mod tidy` failures, which would otherwise blame a generated file the user never wrote.
type PassAggregateExternModules struct{}

func (p *PassAggregateExternModules) Name() string       { return "aggregate_extern_modules" }
func (p *PassAggregateExternModules) Scope() PassScope   { return PerBuild }
func (p *PassAggregateExternModules) Requires() []string { return []string{"analyze_externs"} }
func (p *PassAggregateExternModules) NoErrors() bool     { return true }

type externModulePin struct {
	version string
	span    diag.TextSpan
}

func (p *PassAggregateExternModules) Run(pc *PassContext) error {
	pins := map[string]externModulePin{}
	for _, pkg := range pc.Pkgs {
		for _, f := range pkg.Files {
			side := f.Hir.Side.Kind
			if side == ir.SideFrontend {
				continue
			}
			for _, st := range f.Hir.Statements {
				ext, ok := st.(*ir.ExternDeclStmt)
				if !ok {
					continue
				}
				if ext.Module != nil && *ext.Module != "" {
					p.record(pc, pins, *ext.Module, ext.Version, ext.Span())
				}
				for _, fn := range ext.Funcs {
					if fn.Mapping == nil {
						continue
					}
					p.recordFromMapping(pc, pins, fn.Mapping, fn.Span())
				}
				for _, v := range ext.Vars {
					if v.Mapping == nil {
						continue
					}
					p.recordFromMapping(pc, pins, v.Mapping, v.Span())
				}
			}
		}
	}
	out := make(map[string]string, len(pins))
	for path, pin := range pins {
		if pin.version == "" {
			continue
		}
		out[path] = pin.version
	}
	pc.Cache[ExternGoModulesCacheKey] = out
	return nil
}

func (p *PassAggregateExternModules) recordFromMapping(pc *PassContext, pins map[string]externModulePin, m *ir.ExternMapping, span diag.TextSpan) {
	if m.Shared == nil {
		return
	}
	if be, ok := m.Shared[ir.SideBackend]; ok && be != nil && be.Module != nil && *be.Module != "" {
		p.record(pc, pins, *be.Module, be.Version, span)
	}
}

func (p *PassAggregateExternModules) record(pc *PassContext, pins map[string]externModulePin, modulePath, version string, span diag.TextSpan) {
	modulePath = strings.TrimSpace(modulePath)
	version = strings.TrimSpace(version)
	if version != "" && !isAcceptableGoVersion(version) {
		pc.Diag.Report(diag.ErrExternModuleVersionInvalid, span, modulePath, version)
		return
	}
	existing, seen := pins[modulePath]
	if !seen {
		pins[modulePath] = externModulePin{version: version, span: span}
		return
	}
	if existing.version == "" && version != "" {
		pins[modulePath] = externModulePin{version: version, span: span}
		return
	}
	if version == "" || version == existing.version {
		return
	}
	pc.Diag.Report(diag.ErrExternModuleVersionConflict, span, modulePath, version, existing.version)
}

// isAcceptableGoVersion returns true for version selectors the Go toolchain will accept in a `require` line. The check is intentionally loose: anything starting with `v` followed by a digit, the literal `latest`, or a hex commit SHA of at least seven characters. The Go toolchain itself does the authoritative validation when `go mod tidy` runs against the generated `go.mod`; this pre-check exists only to catch obvious typos like `gorm.io/gorm@1.2.3` (missing the `v`) before they explode three layers deeper.
func isAcceptableGoVersion(version string) bool {
	if version == "" {
		return false
	}
	if version == "latest" {
		return true
	}
	if len(version) >= 2 && version[0] == 'v' && version[1] >= '0' && version[1] <= '9' {
		return true
	}
	if len(version) >= 7 && isAllHex(version) {
		return true
	}
	return false
}

func isAllHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return false
	}
	return true
}
