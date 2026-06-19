package golang

import (
	"sova/internal/codegen"
	"sova/internal/ir"
)

func init() {
	codegen.Register(goBackend{})
}

type goBackend struct{}

func (goBackend) Name() string { return "go" }

func (goBackend) FileExt() string { return ".go" }

func (goBackend) Side() ir.SideKind { return ir.SideBackend }

func (goBackend) Build() (codegen.Emitter, func(...codegen.PolyfixKey)) {
	pfr := BuildPolyfixes()
	return &CodeEmitter{}, pfr.Require
}
