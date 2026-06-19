package passes

import (
	"fmt"
	"path/filepath"

	"sova/internal/codegen"
	_ "sova/internal/codegen/golang"
	_ "sova/internal/codegen/javascript"
	"sova/internal/ir"
)

const buildConfigCacheKey = "build_config"

type buildPaths struct {
	OutputDir  string
	OutputName string
}

type buildConfigGetter interface {
	OutputDirectory() string
	OutputBaseName() string
	SourceDirectory() string
}

type scssConfigGetter interface {
	SCSSCommandValue() string
	SCSSDisabledValue() bool
}

func resolveBuildPaths(pc *PassContext) buildPaths {
	paths := buildPaths{OutputDir: ".output", OutputName: "output"}

	if raw, ok := pc.Cache[buildConfigCacheKey]; ok {
		if cfg, ok := raw.(buildConfigGetter); ok {
			paths.OutputDir = cfg.OutputDirectory()
			paths.OutputName = cfg.OutputBaseName()
		}
	}

	return paths
}

type PassEmitGo struct{}

func (p *PassEmitGo) Name() string { return "emit_go" }

func (p *PassEmitGo) Scope() PassScope { return PerBuild }

func (p *PassEmitGo) Requires() []string { return []string{"mangle"} }

func (p *PassEmitGo) NoErrors() bool { return true }

func (p *PassEmitGo) Run(pc *PassContext) error {
	publishTestRegistryView(pc)
	return runBackend(pc, "go")
}

type PassEmitJS struct{}

func (p *PassEmitJS) Name() string { return "emit_js" }

func (p *PassEmitJS) Scope() PassScope { return PerBuild }

func (p *PassEmitJS) Requires() []string { return []string{"mangle"} }

func (p *PassEmitJS) NoErrors() bool { return true }

func (p *PassEmitJS) Run(pc *PassContext) error {
	return runBackend(pc, "js")
}

func runBackend(pc *PassContext, name string) error {
	backend, ok := codegen.Get(name)
	if !ok {
		return fmt.Errorf("codegen backend %q is not registered", name)
	}

	paths := resolveBuildPaths(pc)
	outFile := filepath.Join(paths.OutputDir, paths.OutputName+backend.FileExt())
	if err := codegen.EnsureOutputDir(outFile); err != nil {
		return err
	}

	pkgs, transPkgs := resolvePkgSlices(pc, backend.Side())

	var initPlan []*codegen.InitPlanEntry
	if arr, ok := pc.Cache["init_plan"]; ok {
		if ipe, ok := arr.([]*codegen.InitPlanEntry); ok {
			initPlan = ipe
		}
	}

	emitter, polyfixRequire := backend.Build()
	ctx := codegen.NewEmitContext(true, outFile, pkgs, transPkgs, pc.Names, pc.Types, initPlan, polyfixRequire, pc.Cache)

	if err := emitter.Init(ctx); err != nil {
		return err
	}

	return emitter.Emit(ctx)
}

func publishTestRegistryView(pc *PassContext) {
	raw, ok := pc.Cache[TestRegistryCacheKey]
	if !ok {
		return
	}

	entries, ok := raw.([]TestEntry)
	if !ok {
		return
	}

	view := make([]codegen.TestRegistryEntryView, 0, len(entries))
	for _, e := range entries {
		ev := codegen.TestRegistryEntryView{
			Pkg:       e.Pkg,
			File:      e.File,
			Decl:      e.Decl,
			GroupPath: append([]string(nil), e.GroupPath...),
			Parallel:  e.Parallel,
		}

		for _, s := range e.Setups {
			if s.Body != nil {
				ev.SetupBodies = append(ev.SetupBodies, ir.BlockStmts(s.Body))
			}
		}

		for _, t := range e.Teardowns {
			if t.Body != nil {
				ev.TeardownBodies = append(ev.TeardownBodies, ir.BlockStmts(t.Body))
			}
		}

		for i, s := range e.SetupAlls {
			if s.Body != nil {
				ev.SetupAlls = append(ev.SetupAlls, s)
				ev.SetupAllOwners = append(ev.SetupAllOwners, e.SetupAllOwners[i])
			}
		}

		for i, t := range e.TeardownAlls {
			if t.Body != nil {
				ev.TeardownAlls = append(ev.TeardownAlls, t)
				ev.TeardownAllOwners = append(ev.TeardownAllOwners, e.TeardownAllOwners[i])
			}
		}

		view = append(view, ev)
	}

	pc.Cache[codegen.TestRegistryViewCacheKey] = view
}

func resolvePkgSlices(pc *PassContext, side ir.SideKind) (
	pkgs []*ir.PackageContext,
	transPkgs []*ir.PackageContext,
) {
	pkgs = []*ir.PackageContext{}

	transPkgs = []*ir.PackageContext{}

	testMode := false
	if raw, ok := pc.Cache[buildConfigCacheKey]; ok {
		if cfg, ok := raw.(interface{ TestModeValue() bool }); ok && cfg.TestModeValue() {
			testMode = true
		}
	}

	for _, pkg := range pc.Pkgs {
		mainPkg := &ir.PackageContext{
			Path:   pkg.Path,
			Files:  []*ir.PreparsedFile{},
			Syms:   pkg.Syms,
			Scopes: pkg.Scopes,
			Types:  pkg.Types,
			Root:   pkg.Root,
		}

		transPkg := &ir.PackageContext{
			Path:   pkg.Path,
			Files:  []*ir.PreparsedFile{},
			Syms:   pkg.Syms,
			Scopes: pkg.Scopes,
			Types:  pkg.Types,
			Root:   pkg.Root,
		}

		for _, f := range pkg.Files {
			fileSide := f.Hir.Side.Kind
			if fileSide == side || fileSide == ir.SideShared || fileSide == ir.SideUnknown {
				mainPkg.Files = append(mainPkg.Files, f)
			} else if testMode && fileSide == ir.SideTest {
				mainPkg.Files = append(mainPkg.Files, f)
			} else {
				transPkg.Files = append(transPkg.Files, f)
			}
		}

		if len(mainPkg.Files) > 0 {
			pkgs = append(pkgs, mainPkg)
		}

		if len(transPkg.Files) > 0 {
			transPkgs = append(transPkgs, transPkg)
		}
	}

	return
}
