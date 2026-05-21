package passes

import "sova/internal/ir"

// PassAnalyzeComposables decides which user types are composable. A type counts as composable when its `with`-chain - including transitively-applied mixins - eventually reaches the built-in `Composable` mixin. The resulting `IsComposable` flag is stamped on the corresponding TypeTable entry so later passes and the codegen can branch cheaply.
type PassAnalyzeComposables struct{}

func (p *PassAnalyzeComposables) Name() string       { return "analyze_composables" }
func (p *PassAnalyzeComposables) Scope() PassScope   { return PerPackage }
func (p *PassAnalyzeComposables) Requires() []string { return []string{"infer_types"} }
func (p *PassAnalyzeComposables) NoErrors() bool     { return false }

func (p *PassAnalyzeComposables) Run(pc *PassContext) error {
	for _, f := range pc.Pkg.Files {
		for _, st := range f.Hir.Statements {
			td, ok := st.(*ir.TypeDeclStmt)
			if !ok {
				continue
			}
			if !mentionsComposable(td.MixedIn) {
				continue
			}
			if td.Name.Sym == 0 {
				continue
			}
			sym, ok := pc.Pkg.Syms.GetByID(td.Name.Sym)
			if !ok || sym.Typ == 0 {
				continue
			}
			if typ, ok := pc.Types.GetByID(sym.Typ); ok {
				typ.IsComposable = true
			}
		}
	}
	return nil
}

// mentionsComposable returns true if any entry in the `with`-list is the built-in `Composable` mixin. Accepts the unqualified `Composable` (legacy direct-injection path) or the qualifier `__globals__` (resolved via the synthetic `using *` import of `std/__globals__`). Transitive composability (mixin chains that themselves `with Composable`) is a Phase 2 polish item; it requires grammar support for `mixin X with Y` which Sova does not have yet.
func mentionsComposable(refs []ir.NameRef) bool {
	for _, r := range refs {
		if r.Name == "Composable" && (r.Qualifier == "" || r.Qualifier == "__globals__") {
			return true
		}
	}
	return false
}
