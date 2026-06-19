package passes

import "sova/internal/ir"

type StmtVisitOpts struct {
	IncludeSynth bool
	OnlySide     ir.SideKind
	SkipPkg      func(*ir.PackageContext) bool
}

func VisitStatements(pkgs []*ir.PackageContext, opts StmtVisitOpts, fn func(pkg *ir.PackageContext, f *ir.PreparsedFile, st ir.Stmt)) {
	VisitFiles(pkgs, opts, func(pkg *ir.PackageContext, f *ir.PreparsedFile) {
		for _, st := range f.Hir.Statements {
			fn(pkg, f, st)
		}
	})
}

func VisitFiles(pkgs []*ir.PackageContext, opts StmtVisitOpts, fn func(pkg *ir.PackageContext, f *ir.PreparsedFile)) {
	for _, pkg := range pkgs {
		if pkg == nil {
			continue
		}

		if opts.SkipPkg != nil && opts.SkipPkg(pkg) {
			continue
		}

		for _, f := range pkg.Files {
			if f == nil || f.Hir == nil {
				continue
			}

			side := f.Hir.Side.Kind
			if !opts.IncludeSynth && side == ir.SideSynth {
				continue
			}

			if opts.OnlySide != ir.SideUnknown && side != opts.OnlySide {
				continue
			}

			fn(pkg, f)
		}
	}
}
