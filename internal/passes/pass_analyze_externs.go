package passes

import (
	"sova/internal/diag"
	"sova/internal/ir"
)

// PassAnalyzeExterns enforces the side-boundary rules around extern declarations: native type and interface bindings are inherently side-specific (a Go struct lives on the backend, a JS class on the frontend) and therefore cannot appear in a shared file. After the shared check, the pass stamps each declared extern type with its origin side so the wrong-side reference pass can fault use-sites that cross the boundary.
type PassAnalyzeExterns struct{}

func (p *PassAnalyzeExterns) Name() string       { return "analyze_externs" }
func (p *PassAnalyzeExterns) Scope() PassScope   { return PerPackage }
func (p *PassAnalyzeExterns) Requires() []string { return []string{"infer_types"} }
func (p *PassAnalyzeExterns) NoErrors() bool     { return false }

func (p *PassAnalyzeExterns) Run(pc *PassContext) error {
	for _, f := range pc.Pkg.Files {
		side := f.Hir.Side.Kind
		for _, st := range f.Hir.Statements {
			ext, ok := st.(*ir.ExternDeclStmt)
			if !ok {
				continue
			}
			if side == ir.SideShared {
				for _, t := range ext.Types {
					pc.Diag.Report(diag.ErrExternTypeInSharedFile, t.Span(), t.Name.Name)
				}
				for _, iface := range ext.Interfaces {
					pc.Diag.Report(diag.ErrExternTypeInSharedFile, iface.Span(), iface.Name.Name)
				}
				continue
			}
			for _, t := range ext.Types {
				p.stampExternSide(pc, t.Name.Sym, side)
				if hasValueAnnotation(t.Annotations) {
					p.stampExternValue(pc, t.Name.Sym)
				}
			}
			for _, iface := range ext.Interfaces {
				p.stampExternSide(pc, iface.Name.Sym, side)
			}
		}
	}
	for _, f := range pc.Pkg.Files {
		side := f.Hir.Side.Kind
		if side == ir.SideShared {
			continue
		}
		for _, st := range f.Hir.Statements {
			p.checkStmtRefs(pc, st, side)
		}
	}
	return nil
}

// hasValueAnnotation returns true when the declaration carries an `@value` marker. Used to flip an extern type from the default pointer-receiver Go rendering to a value-type rendering (e.g. `time.Time` instead of `*time.Time`).
func hasValueAnnotation(annos []ir.Annotation) bool {
	for _, a := range annos {
		if a.Name.Name == "value" {
			return true
		}
	}
	return false
}

func (p *PassAnalyzeExterns) stampExternValue(pc *PassContext, sym ir.SymID) {
	if sym == 0 {
		return
	}
	s, ok := pc.Pkg.Syms.GetByID(sym)
	if !ok || s.Typ == 0 {
		return
	}
	if typ, ok := pc.Types.GetByID(s.Typ); ok {
		typ.ExternValue = true
	}
}

func (p *PassAnalyzeExterns) stampExternSide(pc *PassContext, sym ir.SymID, side ir.SideKind) {
	if sym == 0 {
		return
	}
	s, ok := pc.Pkg.Syms.GetByID(sym)
	if !ok || s.Typ == 0 {
		return
	}
	if typ, ok := pc.Types.GetByID(s.Typ); ok {
		typ.ExternSide = side
	}
}

// checkStmtRefs walks declaration-level type references in a top-level statement and faults any that cross the side boundary. We deliberately scope this to declaration shapes (func params/returns, var type annotations, type fields) where the reference is unambiguous; expression-internal type uses go through type inference which will surface the same problem through a downstream type-mismatch.
func (p *PassAnalyzeExterns) checkStmtRefs(pc *PassContext, st ir.Stmt, fileSide ir.SideKind) {
	switch s := st.(type) {
	case *ir.FuncDeclStmt:
		for _, param := range s.Params {
			p.checkTypeRef(pc, param.Type, fileSide)
		}
		p.checkTypeRef(pc, s.ReturnType, fileSide)
	case *ir.VarDeclStmt:
		for _, target := range s.Targets {
			p.checkTypeRef(pc, target.TypeAnn, fileSide)
		}
	case *ir.TypeDeclStmt:
		if s.IsExtern {
			return
		}
		for _, field := range s.Fields {
			p.checkTypeRef(pc, field.Type, fileSide)
		}
		for _, m := range s.Methods {
			if m.Func == nil {
				continue
			}
			for _, param := range m.Func.Params {
				p.checkTypeRef(pc, param.Type, fileSide)
			}
			p.checkTypeRef(pc, m.Func.ReturnType, fileSide)
		}
	}
}

func (p *PassAnalyzeExterns) checkTypeRef(pc *PassContext, tr *ir.TypeRef, fileSide ir.SideKind) {
	if tr == nil || tr.Typ == 0 {
		return
	}
	typ, ok := pc.Types.GetByID(tr.Typ)
	if !ok || !typ.IsExtern {
		return
	}
	if typ.ExternSide == ir.SideShared || typ.ExternSide == 0 || typ.ExternSide == fileSide {
		return
	}
	pc.Diag.Report(diag.ErrExternTypeWrongSide, tr.Span(), tr.CustomName, sideLabel(typ.ExternSide), sideLabel(fileSide))
}

func sideLabel(s ir.SideKind) string {
	switch s {
	case ir.SideBackend:
		return "backend"
	case ir.SideFrontend:
		return "frontend"
	case ir.SideShared:
		return "shared"
	default:
		return "unknown"
	}
}
