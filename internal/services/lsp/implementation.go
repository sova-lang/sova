package lsp

import (
	"context"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"sova/internal/diag"
	"sova/internal/ir"
	"sova/internal/services/compiler"
)

func (s *Server) Implementation(ctx context.Context, params *protocol.ImplementationParams) ([]protocol.Location, error) {
	return withCursor(s, params.TextDocument.URI, params.Position, func(snap *Snapshot, c *compiler.CompilerContext, target *cursorTarget) ([]protocol.Location, error) {
		if target.sym == 0 {
			return nil, nil
		}

		sym, _ := lookupSymbol(c, target.sym)
		if sym == nil {
			return nil, nil
		}

		if ifaceType, ok := interfaceTypeForSymbol(c, sym); ok {
			return implementersOfInterface(c, snap, ifaceType), nil
		}

		if methodName, ifaceTyp, ok := interfaceMethodForSymbol(c, sym); ok {
			return methodImplementationsFor(c, snap, ifaceTyp, methodName), nil
		}

		return nil, nil
	})
}

func interfaceTypeForSymbol(c *compiler.CompilerContext, sym *ir.Symbol) (ir.TypID, bool) {
	if sym == nil {
		return 0, false
	}

	if sym.Typ != 0 {
		if ty, ok := c.TypeUniverse.GetByID(sym.Typ); ok && ty.Kind == ir.TK_Interface {
			return sym.Typ, true
		}
	}

	for id, ty := range walkAllTypes(c) {
		if ty.Kind != ir.TK_Interface {
			continue
		}

		if ty.Interface.Name == sym.Name {
			return id, true
		}
	}

	return 0, false
}

func interfaceMethodForSymbol(c *compiler.CompilerContext, sym *ir.Symbol) (string, ir.TypID, bool) {
	if sym == nil {
		return "", 0, false
	}

	for id, ty := range walkAllTypes(c) {
		if ty.Kind != ir.TK_Interface {
			continue
		}

		for _, m := range ty.Interface.Methods {
			if m.Name == sym.Name {
				return sym.Name, id, true
			}
		}
	}

	return "", 0, false
}

func implementersOfInterface(c *compiler.CompilerContext, snap *Snapshot, ifaceTyp ir.TypID) []protocol.Location {
	var out []protocol.Location
	for _, ty := range walkAllTypes(c) {
		if ty.Kind != ir.TK_Struct {
			continue
		}

		if !sliceContainsTypID(ty.StructImplements, ifaceTyp) {
			continue
		}

		if span, ok := findTypeDeclSpanByName(c, ty.StructName, ty.PackagePath); ok {
			u := uriForSpan(c, snap, span)
			if u == "" {
				continue
			}

			out = append(out, protocol.Location{URI: u, Range: spanToRange(span)})
		}
	}

	return out
}

func methodImplementationsFor(c *compiler.CompilerContext, snap *Snapshot, ifaceTyp ir.TypID, methodName string) []protocol.Location {
	var out []protocol.Location
	for _, ty := range walkAllTypes(c) {
		if ty.Kind != ir.TK_Struct {
			continue
		}

		if !sliceContainsTypID(ty.StructImplements, ifaceTyp) {
			continue
		}

		if span, ok := findMethodSpanOnType(c, ty.StructName, ty.PackagePath, methodName); ok {
			u := uriForSpan(c, snap, span)
			if u == "" {
				continue
			}

			out = append(out, protocol.Location{URI: u, Range: spanToRange(span)})
		}
	}

	return out
}

func walkAllTypes(c *compiler.CompilerContext) map[ir.TypID]*ir.Type {
	out := map[ir.TypID]*ir.Type{}

	for id := ir.TypID(1); ; id++ {
		ty, ok := c.TypeUniverse.GetByID(id)
		if !ok {
			break
		}

		out[id] = ty
	}

	return out
}

func sliceContainsTypID(xs []ir.TypID, target ir.TypID) bool {
	for _, x := range xs {
		if x == target {
			return true
		}
	}

	return false
}

func findTypeDeclSpanByName(c *compiler.CompilerContext, name, pkgPath string) (diag.TextSpan, bool) {
	for _, pkg := range c.Packages {
		if pkgPath != "" && pkg.Path.String() != pkgPath {
			continue
		}

		for _, f := range pkg.Files {
			if f.Hir == nil {
				continue
			}

			for _, st := range f.Hir.Statements {
				if td, ok := st.(*ir.TypeDeclStmt); ok && td.Name.Name == name {
					return td.Name.Span, true
				}
			}
		}
	}

	return diag.TextSpan{}, false
}

func findMethodSpanOnType(c *compiler.CompilerContext, typeName, pkgPath, methodName string) (diag.TextSpan, bool) {
	for _, pkg := range c.Packages {
		if pkgPath != "" && pkg.Path.String() != pkgPath {
			continue
		}

		for _, f := range pkg.Files {
			if f.Hir == nil {
				continue
			}

			for _, st := range f.Hir.Statements {
				td, ok := st.(*ir.TypeDeclStmt)
				if !ok || td.Name.Name != typeName {
					continue
				}

				for _, m := range td.Methods {
					if m.Func != nil && m.Func.Name.Name == methodName {
						return m.Func.Name.Span, true
					}
				}
			}
		}
	}

	return diag.TextSpan{}, false
}

var _ uri.URI
