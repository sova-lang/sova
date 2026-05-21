package codegen

// PolyfixKey is a unique identifier for a polyfix.
type PolyfixKey string

// PolyfixRegistry is a registry for polyfixes allowing the compiler to apply them as needed.
type PolyfixRegistry[TGen Emitter] struct {
	set     map[PolyfixKey]Polyfix[TGen] // set is a map of polyfixes, keyed by their unique identifier.
	enabled map[PolyfixKey]bool          // enabled is a map indicating whether each polyfix is enabled or not.
}

// NewPolyfixRegistry creates a new PolyfixRegistry instance.
func NewPolyfixRegistry[TGen Emitter]() *PolyfixRegistry[TGen] {
	return &PolyfixRegistry[TGen]{
		set:     make(map[PolyfixKey]Polyfix[TGen]),
		enabled: make(map[PolyfixKey]bool),
	}
}

// Register adds a polyfix to the registry. If the polyfix already exists, it will be replaced.
func (r *PolyfixRegistry[TGen]) Register(polyfix Polyfix[TGen]) {
	if polyfix == nil {
		return // Do not register nil polyfixes.
	}
	key := polyfix.Key()
	r.set[key] = polyfix
	r.enabled[key] = true // Enable the polyfix by default when registered.
}

// Require sets all provided polyfixes as required, enabling them in the registry and marking them for later generation.
func (r *PolyfixRegistry[TGen]) Require(polyfix ...PolyfixKey) {
	for _, key := range polyfix {
		r.enabled[key] = true
	}
}

// Generate generates the polyfixes using the provided generator.
func (r *PolyfixRegistry[TGen]) Generate(generator TGen) {
	for key, polyfix := range r.set {
		if enabled, ok := r.enabled[key]; ok && enabled {
			polyfix.Generate(generator)
		}
	}
}

// Polyfix is the base interface for all polyfixes.
type Polyfix[TGen any] interface {
	Key() PolyfixKey         // Key returns the unique identifier for the polyfix.
	Generate(generator TGen) // Generate applies the polyfix using the provided generator.
}
