package passes

// PassResolveLibs is a pass that resolves library dependencies. The pass triggers library/extern resolution; the actual package loading happens via the CompilerContext's import resolver before the pipeline runs, so today the pass is mostly a placeholder reserved for future cross-package preparation work.
type PassResolveLibs struct{}

func (p *PassResolveLibs) Name() string       { return "resolve_libs" }
func (p *PassResolveLibs) Scope() PassScope   { return PerBuild }
func (p *PassResolveLibs) Requires() []string { return nil }
func (p *PassResolveLibs) NoErrors() bool     { return false }

func (p *PassResolveLibs) Run(pc *PassContext) error {
	return nil
}
