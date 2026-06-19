package javascript

import (
	"sova/internal/codegen"
	"sova/internal/ir"
)

func init() {
	codegen.Register(jsBackend{})
}

type jsBackend struct{}

func (jsBackend) Name() string { return "js" }

func (jsBackend) FileExt() string { return ".js" }

func (jsBackend) Side() ir.SideKind { return ir.SideFrontend }

func (jsBackend) Build() (codegen.Emitter, func(...codegen.PolyfixKey)) {
	pfr := BuildPolyfixes()
	return &CodeEmitter{}, pfr.Require
}
