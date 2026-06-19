package passes

import (
	"sova/internal/diag"
	"sova/internal/ir"
	"strings"
)

const ExternGoModulesCacheKey = "extern_go_modules"

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

	VisitStatements(pc.Pkgs, StmtVisitOpts{IncludeSynth: true}, func(_ *ir.PackageContext, f *ir.PreparsedFile, st ir.Stmt) {
		if f.Hir.Side.Kind == ir.SideFrontend {
			return
		}

		ext, ok := st.(*ir.ExternDeclStmt)
		if !ok {
			return
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
	})

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
