package passes

import (
	"sova/internal/diag"
	"sova/internal/ir"
)

type PassScope int

const (
	PerPackage PassScope = iota
	PerFile
	PerBuild
)

type Pass interface {
	Name() string
	Scope() PassScope
	Requires() []string
	NoErrors() bool
	Run(ctx *PassContext) error
}

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
