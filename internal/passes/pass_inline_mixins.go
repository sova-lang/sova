package passes

import (
	"sova/internal/diag"
	"sova/internal/ir"
)

type PassInlineMixins struct{}

func (p *PassInlineMixins) Name() string       { return "inline_mixins" }

func (p *PassInlineMixins) Scope() PassScope   { return PerPackage }

func (p *PassInlineMixins) Requires() []string { return []string{"resolve_libs"} }

func (p *PassInlineMixins) NoErrors() bool     { return false }

func (p *PassInlineMixins) Run(pc *PassContext) error {
	mixins := map[string]*ir.MixinDeclStmt{}

	knownTypes := map[string]bool{}

	for _, f := range pc.Pkg.Files {
		for _, st := range f.Hir.Statements {
			switch v := st.(type) {
			case *ir.MixinDeclStmt:
				mixins[v.Name.Name] = v
			case *ir.TypeDeclStmt:
				knownTypes[v.Name.Name] = true
			case *ir.ExternDeclStmt:
				for _, t := range v.Types {
					knownTypes[t.Name.Name] = true
				}
			}
		}
	}

	for _, f := range pc.Pkg.Files {
		usingImports := []*ir.ImportStmt{}

		for _, st := range f.Hir.Statements {
			if imp, ok := st.(*ir.ImportStmt); ok && (imp.UsingAll || len(imp.UsingList) > 0) {
				usingImports = append(usingImports, imp)
			}
		}

		for _, st := range f.Hir.Statements {
			td, ok := st.(*ir.TypeDeclStmt)
			if !ok {
				continue
			}

			for i := range td.MixedIn {
				ref := &td.MixedIn[i]
				if ref.Qualifier != "" {
					continue
				}

				if mixin, found := mixins[ref.Name]; found {
					p.applyMixin(pc, td, mixin)
					continue
				}

				if knownTypes[ref.Name] {
					continue
				}

				if mixin, alias, found := p.findMixinViaUsing(pc, usingImports, ref.Name); found {
					p.applyMixin(pc, td, mixin)
					ref.Qualifier = alias
					continue
				}

				if p.knownTypeViaUsing(pc, usingImports, ref.Name) {
					continue
				}

				pc.Diag.Report(diag.ErrMixinNotFound, ref.Span, ref.Name)
			}
		}
	}

	return nil
}

func (p *PassInlineMixins) findMixinViaUsing(pc *PassContext, imports []*ir.ImportStmt, name string) (*ir.MixinDeclStmt, string, bool) {
	type hit struct {
		mixin *ir.MixinDeclStmt
		alias string
	}

	var hits []hit
	for _, imp := range imports {
		if !imp.UsingAll && !containsString(imp.UsingList, name) {
			continue
		}

		target := findPackageByPath(pc, imp.Path.String())
		if target == nil {
			continue
		}

		for _, f := range target.Files {
			for _, st := range f.Hir.Statements {
				if m, ok := st.(*ir.MixinDeclStmt); ok && m.Name.Name == name {
					hits = append(hits, hit{mixin: m, alias: imp.Alias})
				}
			}
		}
	}

	if len(hits) == 1 {
		return hits[0].mixin, hits[0].alias, true
	}

	return nil, "", false
}

func (p *PassInlineMixins) knownTypeViaUsing(pc *PassContext, imports []*ir.ImportStmt, name string) bool {
	for _, imp := range imports {
		if !imp.UsingAll && !containsString(imp.UsingList, name) {
			continue
		}

		target := findPackageByPath(pc, imp.Path.String())
		if target == nil {
			continue
		}

		for _, f := range target.Files {
			for _, st := range f.Hir.Statements {
				switch v := st.(type) {
				case *ir.TypeDeclStmt:
					if v.Name.Name == name {
						return true
					}

				case *ir.ExternDeclStmt:
					for _, t := range v.Types {
						if t.Name.Name == name {
							return true
						}
					}
				}
			}
		}
	}

	return false
}

func (p *PassInlineMixins) applyMixin(pc *PassContext, td *ir.TypeDeclStmt, mixin *ir.MixinDeclStmt) {
	existingFields := map[string]*ir.TypeField{}

	for _, fld := range td.Fields {
		existingFields[fld.Name.Name] = fld
	}

	var newFields []*ir.TypeField
	for _, mfld := range mixin.Fields {
		if cur, hit := existingFields[mfld.Name.Name]; hit {
			if !sameTypeRef(cur.Type, mfld.Type) {
				pc.Diag.Report(diag.ErrMixinFieldTypeConflict, cur.Name.Span, cur.Name.Name, td.Name.Name, mixin.Name.Name)
			}

			continue
		}

		cloned := ir.CloneTypeField(mfld, pc.NodeAlloc)
		td.Fields = append(td.Fields, cloned)
		existingFields[mfld.Name.Name] = cloned
		newFields = append(newFields, cloned)
	}

	if len(newFields) > 0 {
		ir.ExtendSyntheticCtor(td, newFields, pc.NodeAlloc)
	}

	existingMethods := map[string]bool{}

	for _, m := range td.Methods {
		existingMethods[m.Func.Name.Name] = true
	}

	for _, mm := range mixin.Methods {
		if existingMethods[mm.Func.Name.Name] {
			continue
		}

		cloned := ir.CloneTypeMethodDecl(mm, pc.NodeAlloc)
		td.Methods = append(td.Methods, cloned)
		existingMethods[mm.Func.Name.Name] = true
	}
}

func sameTypeRef(a, b *ir.TypeRef) bool {
	if a == nil || b == nil {
		return a == b
	}

	if a.Kind != b.Kind {
		return false
	}

	if a.CustomName != b.CustomName {
		return false
	}

	if a.Dim != b.Dim {
		return false
	}

	if !sameTypeRef(a.Elem, b.Elem) || !sameTypeRef(a.Key, b.Key) || !sameTypeRef(a.Value, b.Value) {
		return false
	}

	if len(a.Tuple) != len(b.Tuple) {
		return false
	}

	for i := range a.Tuple {
		if a.Tuple[i].Name != b.Tuple[i].Name {
			return false
		}

		if !sameTypeRef(a.Tuple[i].Type, b.Tuple[i].Type) {
			return false
		}
	}

	return true
}
