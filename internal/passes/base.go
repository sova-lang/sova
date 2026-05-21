package passes

import (
	"sova/internal/diag"
	"sova/internal/ir"
)

// PassScope defines the scope of a pass in the compilation process.
type PassScope int

const (
	PerPackage PassScope = iota // PerPackage defines the pass scope to run for each package.
	PerFile                     // PerFile defines the pass scope to run for each file.
	PerBuild                    // PerBuild defines the pass scope to run once for the entire build.
)

// Pass defines the base interface for all compiler passes.
type Pass interface {
	Name() string               // Name returns the name of the pass.
	Scope() PassScope           // Scope returns the scope of the pass.
	Requires() []string         // Requires returns the names of passes that must run before this one.
	NoErrors() bool             // NoErrors returns true if the pass can only start, if there are no errors in the diagnostics bag.
	Run(ctx *PassContext) error // Run executes the pass.
}

// PassContext holds contextual information for a pass.
type PassContext struct {
	Diag      *diag.DiagnosticsBag
	Pkgs      []*ir.PackageContext
	Pkg       *ir.PackageContext
	File      *ir.PreparsedFile
	Types     *ir.TypeTable
	SymAlloc  *ir.IdAlloc
	ScAlloc   *ir.IdAlloc
	NodeAlloc *ir.IdAlloc
	Names     *ir.NameMap
	Cache     map[string]any
}
