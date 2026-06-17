package codegen

import "sova/internal/ir"

type InitPlanEntry struct {
	Stmt ir.Stmt            // Stmt is the initialization statement to be executed.
	Pkg  *ir.PackageContext // Pkg is the package context where the statement belongs.
	File *ir.File           // File is the file context where the statement belongs.
}

// EmitContext is the context in which code generation occurs. It holds the current state of the code generation process.
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

// NewEmitContext creates a new EmitContext with the provided parameters.
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

// EnablePolyfixes enables the specified polyfixes in the EmitContext.
func (outCtx *EmitContext) EnablePolyfixes(keys ...PolyfixKey) {
	outCtx.EnablePolyfix(keys...)
}

// Emitter is the interface that all code emitters must implement. The emitters should work one at a time.
type Emitter interface {
	Init(ctx *EmitContext) error // Init initializes the emitter, preparing it for code generation.
	Emit(*EmitContext) error     // Emit generates code based on the provided EmitContext.
}

// TopLevelStmtPrunable reports whether a top-level statement was marked unreachable by the compute_reachability pass and may be skipped during emit. Returns false for any decl flagged IsWired or IsExtern (those are entry points or runtime-instantiated), for VarDeclStmts (their initializers run at module load and may have side effects), and for non-decl statements (only top-level decls participate in DCE).
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
