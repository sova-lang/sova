package passes

import (
	"path/filepath"
	"sova/internal/codegen"
	"sova/internal/codegen/golang"
	"sova/internal/codegen/javascript"
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

// scssConfigGetter is the optional interface a BuildConfig implements to wire the SCSS preprocessor. Kept separate so the BuildConfig type only needs to satisfy it when SCSS support is wanted; the embed resolver falls back to "no SCSS" when the cache value doesn't satisfy this interface.
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

// PassEmitGo is a pass that emits Go code (for backend).
type PassEmitGo struct{}

func (p *PassEmitGo) Name() string       { return "emit_go" }
func (p *PassEmitGo) Scope() PassScope   { return PerBuild }
func (p *PassEmitGo) Requires() []string { return []string{"mangle"} }
func (p *PassEmitGo) NoErrors() bool     { return true }

func (p *PassEmitGo) Run(pc *PassContext) error {
	paths := resolveBuildPaths(pc)
	outFile := filepath.Join(paths.OutputDir, paths.OutputName+".go")
	if err := codegen.EnsureOutputDir(outFile); err != nil {
		return err
	}

	publishTestRegistryView(pc)

	pkgs, transPkgs := resolvePkgSlices(pc, ir.SideBackend)

	var initPlan []*codegen.InitPlanEntry
	if arr, ok := pc.Cache["init_plan"]; ok {
		if ipe, ok := arr.([]*codegen.InitPlanEntry); ok {
			initPlan = ipe
		}
	}

	pfr := golang.BuildPolyfixes()
	ctx := codegen.NewEmitContext(true, outFile, pkgs, transPkgs, pc.Names, pc.Types, initPlan, pfr.Require, pc.Cache)

	emitter := &golang.CodeEmitter{}
	if err := emitter.Init(ctx); err != nil {
		return err
	}
	return emitter.Emit(ctx)
}

// PassEmitJS is a pass that emits JavaScript code (for frontend).
type PassEmitJS struct{}

func (p *PassEmitJS) Name() string       { return "emit_js" }
func (p *PassEmitJS) Scope() PassScope   { return PerBuild }
func (p *PassEmitJS) Requires() []string { return []string{"mangle"} }
func (p *PassEmitJS) NoErrors() bool     { return true }

func (p *PassEmitJS) Run(pc *PassContext) error {
	paths := resolveBuildPaths(pc)
	outFile := filepath.Join(paths.OutputDir, paths.OutputName+".js")
	if err := codegen.EnsureOutputDir(outFile); err != nil {
		return err
	}

	pkgs, transPkgs := resolvePkgSlices(pc, ir.SideFrontend)

	var initPlan []*codegen.InitPlanEntry
	if arr, ok := pc.Cache["init_plan"]; ok {
		if ipe, ok := arr.([]*codegen.InitPlanEntry); ok {
			initPlan = ipe
		}
	}

	pfr := javascript.BuildPolyfixes()
	ctx := codegen.NewEmitContext(true, outFile, pkgs, transPkgs, pc.Names, pc.Types, initPlan, pfr.Require, pc.Cache)

	emitter := &javascript.CodeEmitter{}
	if err := emitter.Init(ctx); err != nil {
		return err
	}
	return emitter.Emit(ctx)
}

// publishTestRegistryView re-packages the test_discovery output into the codegen-facing view (`codegen.TestRegistryEntryView`) and stores it under `codegen.TestRegistryViewCacheKey` so the Go emitter can consume the registry without importing the `passes` package (which would form an import cycle through pass_codegen.go).
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

// resolvePkgSlices resolves the package slices for the given side (ir.SideKind). When the build is in test mode (cache flag `build_config` carrying `TestModeValue() == true`), files marked `on test` participate in both backend and frontend codegen - the test driver linked into the same binary needs visibility to both sides regardless of the current emit pass.
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
