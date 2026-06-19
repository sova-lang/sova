package passes

import (
	"sova/internal/diag"
	"sova/internal/ir"
)

type PassCheckImports struct{}

func (p *PassCheckImports) Name() string       { return "check_imports" }

func (p *PassCheckImports) Scope() PassScope   { return PerBuild }

func (p *PassCheckImports) Requires() []string { return []string{"resolve_libs"} }

func (p *PassCheckImports) NoErrors() bool     { return false }

func (p *PassCheckImports) Run(pc *PassContext) error {
	sideByPkg := map[string]ir.SideKind{}

	for _, pkg := range pc.Pkgs {
		sideByPkg[pkg.Path.String()] = packageEffectiveSide(pkg)
	}

	for _, pkg := range pc.Pkgs {
		for _, f := range pkg.Files {
			if f.Hir == nil {
				continue
			}

			fileSide := f.Hir.Side.Kind
			if fileSide == ir.SideUnknown {
				fileSide = ir.SideShared
			}

			for _, st := range f.Hir.Statements {
				imp, ok := st.(*ir.ImportStmt)
				if !ok {
					continue
				}

				target := imp.Path.String()
				targetSide, known := sideByPkg[target]
				if !known {
					continue
				}

				if targetSide == ir.SideUnknown {
					targetSide = ir.SideShared
				}

				if importAllowed(fileSide, targetSide) {
					continue
				}

				pc.Diag.Report(diag.ErrImportSideMismatch, imp.Span(),
					sideName(targetSide), target, sideName(fileSide), sideName(fileSide), allowedTargets(fileSide))
			}
		}
	}

	return nil
}

func importAllowed(fileSide, targetSide ir.SideKind) bool {
	switch fileSide {
	case ir.SideShared:
		return targetSide == ir.SideShared
	case ir.SideBackend:
		return targetSide == ir.SideShared || targetSide == ir.SideBackend
	case ir.SideFrontend:
		return targetSide == ir.SideShared || targetSide == ir.SideFrontend
	}

	return true
}

func allowedTargets(fileSide ir.SideKind) string {
	switch fileSide {
	case ir.SideShared:
		return "shared"
	case ir.SideBackend:
		return "shared or backend"
	case ir.SideFrontend:
		return "shared or frontend"
	}

	return "shared"
}

func packageEffectiveSide(pkg *ir.PackageContext) ir.SideKind {
	hasFrontend := false
	hasBackend := false
	for _, f := range pkg.Files {
		if f.Hir == nil {
			continue
		}

		switch f.Hir.Side.Kind {
		case ir.SideShared, ir.SideUnknown:
			return ir.SideShared
		case ir.SideFrontend:
			hasFrontend = true
		case ir.SideBackend:
			hasBackend = true
		}
	}

	if packageHasWireSymbol(pkg) {
		return ir.SideShared
	}

	if hasFrontend && hasBackend {
		return ir.SideShared
	}

	if hasFrontend {
		return ir.SideFrontend
	}

	if hasBackend {
		return ir.SideBackend
	}

	return ir.SideShared
}

func packageHasWireSymbol(pkg *ir.PackageContext) bool {
	for _, f := range pkg.Files {
		if f.Hir == nil {
			continue
		}

		for _, st := range f.Hir.Statements {
			switch s := st.(type) {
			case *ir.FuncDeclStmt:
				if s.IsWired {
					return true
				}

			case *ir.VarDeclStmt:
				if s.IsWired {
					return true
				}
			}
		}
	}

	return false
}

func sideName(k ir.SideKind) string {
	switch k {
	case ir.SideFrontend:
		return "frontend"
	case ir.SideBackend:
		return "backend"
	case ir.SideShared:
		return "shared"
	}

	return "unknown"
}
