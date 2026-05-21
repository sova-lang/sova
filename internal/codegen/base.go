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
