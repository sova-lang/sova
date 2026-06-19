package passes

import "sova/internal/ir"

const TestRegistryCacheKey = "test_registry"

type TestEntry struct {
	Pkg               *ir.PackageContext
	File              *ir.PreparsedFile
	Decl              *ir.TestDeclStmt
	GroupPath         []string
	Setups            []*ir.SetupStmt
	Teardowns         []*ir.TeardownStmt
	SetupAlls         []*ir.SetupStmt
	SetupAllOwners    []string
	TeardownAlls      []*ir.TeardownStmt
	TeardownAllOwners []string
	Parallel          bool
	Tags              []string
}

type PassTestDiscovery struct{}

func (p *PassTestDiscovery) Name() string       { return "test_discovery" }

func (p *PassTestDiscovery) Scope() PassScope   { return PerBuild }

func (p *PassTestDiscovery) Requires() []string { return []string{"resolve_typerefs"} }

func (p *PassTestDiscovery) NoErrors() bool     { return false }

func (p *PassTestDiscovery) Run(pc *PassContext) error {
	var entries []TestEntry
	for _, pkg := range pc.Pkgs {
		for _, f := range pkg.Files {
			if f.Hir == nil || f.Hir.Side.Kind != ir.SideTest {
				continue
			}

			entries = p.walkStmts(entries, pkg, f, f.Hir.Statements, nil, nil, nil, nil, nil, nil, nil, false, nil)
		}
	}

	pc.Cache[TestRegistryCacheKey] = entries
	return nil
}

func (p *PassTestDiscovery) walkStmts(
	entries []TestEntry,
	pkg *ir.PackageContext,
	f *ir.PreparsedFile,
	stmts []ir.Stmt,
	groupPath []string,
	setups []*ir.SetupStmt,
	teardowns []*ir.TeardownStmt,
	setupAlls []*ir.SetupStmt,
	setupAllOwners []string,
	teardownAlls []*ir.TeardownStmt,
	teardownAllOwners []string,
	parallelInherit bool,
	tagsInherit []string,
) []TestEntry {
	localSetups := append([]*ir.SetupStmt(nil), setups...)
	localTeardowns := append([]*ir.TeardownStmt(nil), teardowns...)
	localSetupAlls := append([]*ir.SetupStmt(nil), setupAlls...)
	localSetupAllOwners := append([]string(nil), setupAllOwners...)
	localTeardownAlls := append([]*ir.TeardownStmt(nil), teardownAlls...)
	localTeardownAllOwners := append([]string(nil), teardownAllOwners...)
	ownerKey := joinGroupPath(groupPath)
	for _, st := range stmts {
		switch s := st.(type) {
		case *ir.SetupStmt:
			if s.IsAll {
				localSetupAlls = append(localSetupAlls, s)
				localSetupAllOwners = append(localSetupAllOwners, ownerKey)
			} else {
				localSetups = append(localSetups, s)
			}

		case *ir.TeardownStmt:
			if s.IsAll {
				localTeardownAlls = append(localTeardownAlls, s)
				localTeardownAllOwners = append(localTeardownAllOwners, ownerKey)
			} else {
				localTeardowns = append(localTeardowns, s)
			}
		}
	}

	for _, st := range stmts {
		switch s := st.(type) {
		case *ir.TestDeclStmt:
			tags := append([]string(nil), tagsInherit...)
			tags = append(tags, s.Tags...)
			entries = append(entries, TestEntry{
				Pkg:               pkg,
				File:              f,
				Decl:              s,
				GroupPath:         append([]string(nil), groupPath...),
				Setups:            append([]*ir.SetupStmt(nil), localSetups...),
				Teardowns:         append([]*ir.TeardownStmt(nil), localTeardowns...),
				SetupAlls:         append([]*ir.SetupStmt(nil), localSetupAlls...),
				SetupAllOwners:    append([]string(nil), localSetupAllOwners...),
				TeardownAlls:      append([]*ir.TeardownStmt(nil), localTeardownAlls...),
				TeardownAllOwners: append([]string(nil), localTeardownAllOwners...),
				Parallel:          parallelInherit || s.Parallel,
				Tags:              tags,
			})
		case *ir.GroupDeclStmt:
			nestedPath := append(append([]string(nil), groupPath...), s.Name)
			nestedTags := append([]string(nil), tagsInherit...)
			nestedTags = append(nestedTags, s.Tags...)
			entries = p.walkStmts(entries, pkg, f, s.Body, nestedPath, localSetups, localTeardowns, localSetupAlls, localSetupAllOwners, localTeardownAlls, localTeardownAllOwners, parallelInherit || s.Parallel, nestedTags)
		}
	}

	return entries
}

func joinGroupPath(path []string) string {
	if len(path) == 0 {
		return "<root>"
	}

	out := ""
	for i, p := range path {
		if i > 0 {
			out += "/"
		}

		out += p
	}

	return out
}
