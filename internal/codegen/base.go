package codegen

import "sova/internal/ir"

type InitPlanEntry struct {
	Stmt ir.Stmt
	Pkg  *ir.PackageContext
	File *ir.File
}

type EmitContext struct {
	DebugSymbols  bool
	OutPath       string
	Pkgs          []*ir.PackageContext
	TransPkgs     []*ir.PackageContext
	Names         *ir.NameMap
	Types         *ir.TypeTable
	InitPlan      []*InitPlanEntry
	EnablePolyfix func(...PolyfixKey)
	Cache         map[string]any
}

func NewEmitContext(
	debugSymbols bool,
	outPath string,
	pkgs []*ir.PackageContext,
	transPkgs []*ir.PackageContext,
	names *ir.NameMap,
	types *ir.TypeTable,
	initPlan []*InitPlanEntry,
	enablePolyfix func(...PolyfixKey),
	cache map[string]any,
) *EmitContext {
	return &EmitContext{
		DebugSymbols:  debugSymbols,
		OutPath:       outPath,
		Pkgs:          pkgs,
		TransPkgs:     transPkgs,
		Names:         names,
		Types:         types,
		InitPlan:      initPlan,
		EnablePolyfix: enablePolyfix,
		Cache:         cache,
	}
}

func (outCtx *EmitContext) EnablePolyfixes(keys ...PolyfixKey) {
	outCtx.EnablePolyfix(keys...)
}

type Emitter interface {
	Init(ctx *EmitContext) error
	Emit(*EmitContext) error
}

func TopLevelStmtPrunable(ctx *EmitContext, st ir.Stmt) bool {
	switch x := st.(type) {
	case *ir.FuncDeclStmt:
		if x.IsWired || x.Name.Sym == 0 {
			return false
		}

		return !symReachable(ctx, x.Name.Sym)
	case *ir.TypeDeclStmt:
		if x.IsExtern || x.Name.Sym == 0 {
			return false
		}

		return !symReachable(ctx, x.Name.Sym)
	}

	return false
}

func symReachable(ctx *EmitContext, sym ir.SymID) bool {
	if sym == 0 {
		return false
	}

	for _, pkg := range ctx.Pkgs {
		if s, ok := pkg.Syms.GetByID(sym); ok {
			return s.Flags&ir.SF_Reachable != 0
		}
	}

	for _, pkg := range ctx.TransPkgs {
		if s, ok := pkg.Syms.GetByID(sym); ok {
			return s.Flags&ir.SF_Reachable != 0
		}
	}

	return false
}
