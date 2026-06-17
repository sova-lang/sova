package passes

// PassPrecomputeSignatures lifts the per-package "pre-compute every type's signature surface" step out of `infer_types` and runs it as its own pass across ALL packages before `infer_types` starts. Without this lift, a package's `infer_types` body would walk while another (still-untyped) package's structs had nil StructFields / StructCtors / StructMethods — fine for an acyclic build because the topo order guarantees deps are typed first, but a hard failure for cyclic packages like `std/list` + `std/streams` where each side legitimately references the other's types. With this pass in place, every package sees every other package's struct surface populated before any body resolution runs, so circular imports work.
//
// The pre-compute methods themselves still live on `PassInferTypes`; this pass just calls them. That keeps the implementation in one place (the methods know exactly which IR slots they fill in and how to mirror the body-walker's later overwrites) while letting the scheduler interleave them at the correct point.
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
