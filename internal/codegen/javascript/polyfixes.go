package javascript

import "sova/internal/codegen"

// BuildPolyfixes initializes a new PolyfixRegistry for CodeEmitter, registers all polyfixes, and returns the registry.
func BuildPolyfixes() *codegen.PolyfixRegistry[*CodeEmitter] {
	pfr := codegen.NewPolyfixRegistry[*CodeEmitter]()

	return pfr
}
