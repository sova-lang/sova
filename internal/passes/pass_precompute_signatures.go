package passes

type PassPrecomputeSignatures struct {
	delegate *PassInferTypes
}

func (p *PassPrecomputeSignatures) Name() string       { return "precompute_signatures" }

func (p *PassPrecomputeSignatures) Scope() PassScope   { return PerPackage }

func (p *PassPrecomputeSignatures) Requires() []string { return []string{"resolve_typerefs"} }

func (p *PassPrecomputeSignatures) NoErrors() bool     { return false }

func (p *PassPrecomputeSignatures) Run(pc *PassContext) error {
	if p.delegate == nil {
		p.delegate = &PassInferTypes{}
	}

	for _, f := range pc.Pkg.Files {
		p.delegate.preComputeExternSignatures(pc, f.Hir.Statements)
	}

	for _, f := range pc.Pkg.Files {
		p.delegate.preComputeFuncSignatures(pc, f.Hir.Statements)
	}

	for _, f := range pc.Pkg.Files {
		p.delegate.preComputeTopLevelVarSignatures(pc, f.Hir.Statements)
	}

	for _, f := range pc.Pkg.Files {
		p.delegate.preComputeStructFields(pc, f.Hir.Statements)
	}

	for _, f := range pc.Pkg.Files {
		p.delegate.preComputeStructCtors(pc, f.Hir.Statements)
	}

	for _, f := range pc.Pkg.Files {
		p.delegate.preComputeStructMethods(pc, f.Hir.Statements)
	}

	for _, f := range pc.Pkg.Files {
		p.delegate.preComputeEnumCases(pc, f.Hir.Statements)
	}

	return nil
}
