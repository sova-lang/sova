package passes

type PassResolveLibs struct{}

func (p *PassResolveLibs) Name() string       { return "resolve_libs" }

func (p *PassResolveLibs) Scope() PassScope   { return PerBuild }

func (p *PassResolveLibs) Requires() []string { return nil }

func (p *PassResolveLibs) NoErrors() bool     { return false }

func (p *PassResolveLibs) Run(pc *PassContext) error {
	return nil
}
