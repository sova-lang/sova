package passes

import (
	"sova/internal/diag"
	"sova/internal/ir"
)

// PassCheckImports reports a diagnostic when a file's `import` clause crosses
// the frontend/backend boundary the wrong way. The rule mirrors Sova's tier
// model: a file may only import packages whose effective side is compatible
// with where the file itself runs.
//
//   - `on shared` files run on both tiers, so they may only import shared
//     packages - pulling in backend- or frontend-only code would leave the
//     other tier unable to compile it.
//   - `on backend` files may import shared or backend packages.
//   - `on frontend` files may import shared or frontend packages.
//
// A package whose only cross-tier surface is `wire` symbols is treated as
// shared, since the wire mechanism is exactly the supported way to call
// across tiers.
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

// importAllowed implements the side-compatibility matrix for `import`. A
// shared file runs on both tiers and can only depend on packages that also
// run on both. A backend/frontend file may pull in shared packages plus
// packages on its own side.
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

// allowedTargets renders the human-readable list of import targets a file
// on `fileSide` is allowed to pull in. Used to fill the trailing `%s` in
// `ErrImportSideMismatch` so the diagnostic explains the exact rule the
// import broke.
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

// packageEffectiveSide is the union of the sides declared by the package's
// files. A package with any `on shared` file (or no side declaration at all)
// is treated as shared, since at least one of its files is freely callable
// from either tier. A package whose files all live on one side is bound to
// that side - UNLESS it exports a `wire` symbol, in which case the package
// is still importable from the opposite side (calling a wire is the
// intended cross-tier mechanism, e.g. a frontend file importing a backend
// package to invoke its `wire func`).
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

// packageHasWireSymbol reports whether any top-level statement in `pkg` is a
// `wire` declaration (func, var, or const). Wire symbols are inherently
// cross-tier - they're the mechanism Sova provides for one side to call into
// the other - so a package that exposes any wire is importable from either
// side regardless of the file-level `on backend` / `on frontend` annotation.
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
