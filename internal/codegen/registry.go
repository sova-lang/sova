package codegen

import "sova/internal/ir"

type Backend interface {
	Name() string
	FileExt() string
	Side() ir.SideKind
	Build() (Emitter, func(...PolyfixKey))
}

var backends = map[string]Backend{}

func Register(b Backend) {
	if b == nil {
		return
	}

	backends[b.Name()] = b
}

func Get(name string) (Backend, bool) {
	b, ok := backends[name]
	return b, ok
}

func All() []Backend {
	out := make([]Backend, 0, len(backends))
	for _, b := range backends {
		out = append(out, b)
	}

	return out
}
