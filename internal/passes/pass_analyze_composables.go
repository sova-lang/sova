package passes

import "sova/internal/ir"

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

func mentionsComposable(refs []ir.NameRef) bool {
	for _, r := range refs {
		if r.Name == "Composable" && (r.Qualifier == "" || r.Qualifier == "__globals__") {
			return true
		}
	}

	return false
}
