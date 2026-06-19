package codegen

type PolyfixKey string

type PolyfixRegistry[TGen Emitter] struct {
	set     map[PolyfixKey]Polyfix[TGen]
	enabled map[PolyfixKey]bool
}

func NewPolyfixRegistry[TGen Emitter]() *PolyfixRegistry[TGen] {
	return &PolyfixRegistry[TGen]{
		set:     make(map[PolyfixKey]Polyfix[TGen]),
		enabled: make(map[PolyfixKey]bool),
	}
}

func (r *PolyfixRegistry[TGen]) Register(polyfix Polyfix[TGen]) {
	if polyfix == nil {
		return
	}

	key := polyfix.Key()
	r.set[key] = polyfix
	r.enabled[key] = true
}

func (r *PolyfixRegistry[TGen]) Require(polyfix ...PolyfixKey) {
	for _, key := range polyfix {
		r.enabled[key] = true
	}
}

func (r *PolyfixRegistry[TGen]) Generate(generator TGen) {
	for key, polyfix := range r.set {
		if enabled, ok := r.enabled[key]; ok && enabled {
			polyfix.Generate(generator)
		}
	}
}

type Polyfix[TGen any] interface {
	Key() PolyfixKey
	Generate(generator TGen)
}
