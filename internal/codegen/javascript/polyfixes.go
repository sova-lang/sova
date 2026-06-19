package javascript

import "sova/internal/codegen"

func BuildPolyfixes() *codegen.PolyfixRegistry[*CodeEmitter] {
	pfr := codegen.NewPolyfixRegistry[*CodeEmitter]()

	return pfr
}
