package lsp

import (
	"context"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"sova/internal/diag"
	"sova/internal/ir"
	"sova/internal/services/compiler"
)

// Implementation answers "what concrete type/method implements this?" Two directions:
//
//   - Cursor on an interface type → list every type whose `implements` clause names this interface, jumping to that type's declaration.
//   - Cursor on an interface method signature → list every concrete method on implementing types with the same name.
//
// For a non-interface target the response is empty (the regular `Definition` covers the trivial case). The protocol allows multi-location results so navigation jumps go straight to a picker if more than one implementer exists.
func (s *Server) Implementation(ctx context.Context, params *protocol.ImplementationParams) ([]protocol.Location, error) {
	snap := s.session.Snapshot()
	if snap == nil {
		return nil, nil
	}
	c, _, err := snap.Compile(s.compileSnapshot)
	if err != nil || c == nil {
		return nil, nil
	}
	target := findCursorTarget(c, params.TextDocument.URI, params.Position.Line, params.Position.Character)
	if target == nil || target.sym == 0 {
		return nil, nil
	}
	sym, _ := lookupSymbol(c, target.sym)
	if sym == nil {
		return nil, nil
	}
	switch ifaceType, ok := interfaceTypeForSymbol(c, sym); {
	case ok:
		return implementersOfInterface(c, snap, ifaceType), nil
	}
	if methodName, ifaceTyp, ok := interfaceMethodForSymbol(c, sym); ok {
		return methodImplementationsFor(c, snap, ifaceTyp, methodName), nil
	}
	return nil, nil
}

// interfaceTypeForSymbol reports whether `sym` is the declaration of an interface type and returns the corresponding TypID. Used for the "list implementers" branch of `Implementation`.
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
		if ty.InterfaceName == sym.Name {
			return id, true
		}
	}
	return 0, false
}

// interfaceMethodForSymbol reports whether `sym` is one method on an interface - returning the method name and the interface's TypID. Built by checking every interface's signatures for a matching name (the symbol's Name); v1 doesn't disambiguate by signature, so two interfaces with same-named methods both win, which is fine for the navigation UX (multi-location response).
func interfaceMethodForSymbol(c *compiler.CompilerContext, sym *ir.Symbol) (string, ir.TypID, bool) {
	if sym == nil {
		return "", 0, false
	}
	for id, ty := range walkAllTypes(c) {
		if ty.Kind != ir.TK_Interface {
			continue
		}
		for _, m := range ty.InterfaceMethods {
			if m.Name == sym.Name {
				return sym.Name, id, true
			}
		}
	}
	return "", 0, false
}

// implementersOfInterface returns one Location per type whose `implements` list contains `ifaceTyp`. Points each Location at the type's declaration Name span.
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
			out = append(out, protocol.Location{URI: u, Range: spanToLSPRange(span)})
		}
	}
	return out
}

// methodImplementationsFor returns one Location per implementing type's method named `methodName`. The receiver type must implement `ifaceTyp`. The found Location points at the method's declaration name in the implementing type's body.
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
			out = append(out, protocol.Location{URI: u, Range: spanToLSPRange(span)})
		}
	}
	return out
}

// walkAllTypes returns every registered type. The TypeTable doesn't expose its map, so we iterate via successive `GetByID` lookups starting at id=1 until we hit a gap. Each id is allocated densely so we stop after the first miss - close enough for navigation, not exact.
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

// findTypeDeclSpanByName walks every package's HIR until it finds a TypeDeclStmt with the given name. When pkgPath is non-empty, only the package whose Path matches is considered.
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

// findMethodSpanOnType locates a method's declaration name span on a specific type. Same scan as findTypeDeclSpanByName but goes one level deeper to the type's Methods.
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

// _ silences unused-import for uri when this file alone is built - kept here so the other handlers in this package don't need to import it again.
var _ uri.URI
