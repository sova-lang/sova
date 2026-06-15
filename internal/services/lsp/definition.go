package lsp

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"sova/internal/diag"
	"sova/internal/ir"
	"sova/internal/services/compiler"
)

// Definition jumps to the declaration site of the symbol under the cursor. Returns nil when the cursor isn't on a resolved symbol - the editor responds by leaving the cursor where it is. Multi-location results are supported by the protocol (interface methods with multiple implementations, etc.); v1 emits one Location.
func (s *Server) Definition(ctx context.Context, params *protocol.DefinitionParams) ([]protocol.Location, error) {
	return s.findDefinitionLocations(params.TextDocument.URI, params.Position, false)
}

// TypeDefinition jumps to the declaration of the symbol's *type* - e.g. for `let m = new Mutex()`, jumping on `m` takes you to `type Mutex { ... }`, not to the `let` line. Falls back gracefully (returns nil) when the type isn't a user-defined nominal type.
func (s *Server) TypeDefinition(ctx context.Context, params *protocol.TypeDefinitionParams) ([]protocol.Location, error) {
	return s.findDefinitionLocations(params.TextDocument.URI, params.Position, true)
}

func (s *Server) findDefinitionLocations(docURI uri.URI, pos protocol.Position, viaType bool) ([]protocol.Location, error) {
	snap := s.session.Snapshot()
	if snap == nil {
		return nil, nil
	}
	c, _, err := snap.Compile(s.compileSnapshot)
	if err != nil || c == nil {
		return nil, nil
	}
	if !viaType {
		if src, ok := snap.ReadFile(docURI); ok {
			if name, _, ok := annotationAtCursor(src, pos); ok {
				if loc := synthDefinitionLocation(c, snap, name); loc != nil {
					return []protocol.Location{*loc}, nil
				}
			}
			if locs := cssClassDefinition(c, src, pos); len(locs) > 0 {
				return locs, nil
			}
		}
	}
	target := findCursorTarget(c, docURI, pos.Line, pos.Character)
	if target == nil {
		return nil, nil
	}

	span := declarationSpan(c, target, viaType)
	if span.StartLn == 0 && span.EndLn == 0 {
		return nil, nil
	}
	declURI := uriForSpan(c, snap, span)
	if declURI == "" {
		return nil, nil
	}
	return []protocol.Location{{URI: declURI, Range: spanToLSPRange(span)}}, nil
}

// declarationSpan resolves a cursor target to the span where its referent was declared. Handles four cases: a plain symbol reference (jumps to the declaring NameRef), a `TypeDefinition` request (jumps to the type's declaration even for value-level identifiers), a `TypeRef` cursor hit on a custom-type name (resolves the name+qualifier through the compiler's package index), and an `ImportStmt` cursor hit on the path literal (jumps to the imported package's first source file).
func declarationSpan(c *compiler.CompilerContext, target *cursorTarget, viaType bool) diag.TextSpan {
	if target == nil {
		return diag.TextSpan{}
	}
	switch target.kind {
	case cursorKindImportPath:
		return importPathDeclSpan(c, target.pkg, target.importPath)
	case cursorKindTypeRef:
		if span := typeRefDeclSpan(c, target.pkg, target.typeRefName, target.typeRefQualifier); span.StartLn != 0 {
			return span
		}
		if target.typ != 0 {
			if span := typeDeclarationSpan(c, target.typ); span.StartLn != 0 {
				return span
			}
		}
		return diag.TextSpan{}
	}
	if target.sym == 0 {
		return diag.TextSpan{}
	}
	symInfo, _ := lookupSymbol(c, target.sym)
	if symInfo == nil {
		return diag.TextSpan{}
	}
	if viaType {
		if span := typeDeclarationSpan(c, symInfo.Typ); span.StartLn != 0 {
			return span
		}
	}
	if span, ok := findDeclSpanForSym(c, target.sym); ok {
		return span
	}
	return diag.TextSpan{}
}

// importPathDeclSpan returns the span of the first statement in the first file of the package at `path`. The editor jumps there when the user ctrl+clicks an import path literal.
func importPathDeclSpan(c *compiler.CompilerContext, currentPkg *ir.PackageContext, path string) diag.TextSpan {
	if path == "" {
		return diag.TextSpan{}
	}
	pkg, ok := c.Packages[path]
	if !ok {
		return diag.TextSpan{}
	}
	_ = currentPkg
	for _, f := range pkg.Files {
		if f.Hir == nil || f.Filename == "" {
			continue
		}
		return diag.TextSpan{File: f.Filename, StartLn: 1, StartCol: 1, EndLn: 1, EndCol: 1}
	}
	return diag.TextSpan{}
}

// typeRefDeclSpan resolves a `name` (with optional `qualifier`) appearing inside a `TypeRef` to the declaring `type` / `interface` / `enum` statement. The qualifier path goes through the file's import-alias map to find the right package; an empty qualifier searches the current package first, then any other loaded package whose root scope exports the name.
func typeRefDeclSpan(c *compiler.CompilerContext, currentPkg *ir.PackageContext, name, qualifier string) diag.TextSpan {
	if name == "" {
		return diag.TextSpan{}
	}
	var target *ir.PackageContext
	if qualifier == "" {
		target = currentPkg
	} else if currentPkg != nil {
		for _, f := range currentPkg.Files {
			if f.Hir == nil {
				continue
			}
			for _, st := range f.Hir.Statements {
				imp, ok := st.(*ir.ImportStmt)
				if !ok {
					continue
				}
				alias := imp.Alias
				if alias == "" && len(imp.Path) > 0 {
					alias = imp.Path[len(imp.Path)-1]
				}
				if alias == qualifier {
					if pkg, found := c.Packages[imp.Path.String()]; found {
						target = pkg
					}
					break
				}
			}
			if target != nil {
				break
			}
		}
	}
	if target != nil {
		if span := findTypeDeclSpanInPkg(target, name); span.StartLn != 0 {
			return span
		}
	}
	if qualifier == "" {
		for _, pkg := range c.Packages {
			if pkg == currentPkg {
				continue
			}
			if span := findTypeDeclSpanInPkg(pkg, name); span.StartLn != 0 {
				return span
			}
		}
	}
	return diag.TextSpan{}
}

// findTypeDeclSpanInPkg returns the declaration-name span of a `type` / `interface` / `enum` named `name` within `pkg`, or zero if no match.
func findTypeDeclSpanInPkg(pkg *ir.PackageContext, name string) diag.TextSpan {
	if pkg == nil {
		return diag.TextSpan{}
	}
	for _, f := range pkg.Files {
		if f.Hir == nil {
			continue
		}
		if span, ok := findTypeDeclByName(f.Hir, name); ok {
			return span
		}
	}
	return diag.TextSpan{}
}

// typeDeclarationSpan returns the source span of the type's defining declaration, when the type is a nominal user type (struct/enum/interface). Other type kinds (primitives, generated function types) have no Sova-side declaration we could navigate to, so we return zero.
func typeDeclarationSpan(c *compiler.CompilerContext, typ ir.TypID) diag.TextSpan {
	ty, ok := c.TypeUniverse.GetByID(typ)
	if !ok {
		return diag.TextSpan{}
	}
	var name string
	switch ty.Kind {
	case ir.TK_Struct:
		name = ty.StructName
	case ir.TK_Enum:
		name = ty.EnumName
	case ir.TK_Interface:
		name = ty.InterfaceName
	default:
		return diag.TextSpan{}
	}
	if name == "" {
		return diag.TextSpan{}
	}
	for _, pkg := range c.Packages {
		if ty.PackagePath != "" && pkg.Path.String() != ty.PackagePath {
			continue
		}
		for _, f := range pkg.Files {
			if f.Hir == nil {
				continue
			}
			if span, found := findTypeDeclByName(f.Hir, name); found {
				return span
			}
		}
	}
	return diag.TextSpan{}
}

// findTypeDeclByName walks a file's top-level declarations looking for a TypeDeclStmt (or EnumDeclStmt / InterfaceDeclStmt) whose Name matches `name`. Returns the Name reference's span - that's what we want to highlight, not the entire decl body.
func findTypeDeclByName(f *ir.File, name string) (diag.TextSpan, bool) {
	for _, st := range f.Statements {
		switch n := st.(type) {
		case *ir.TypeDeclStmt:
			if n.Name.Name == name {
				return n.Name.Span, true
			}
		case *ir.EnumDeclStmt:
			if n.Name.Name == name {
				return n.Name.Span, true
			}
		case *ir.InterfaceDeclStmt:
			if n.Name.Name == name {
				return n.Name.Span, true
			}
		}
	}
	return diag.TextSpan{}, false
}

// findDeclSpanForSym walks every package's HIR until it finds a top-level (or method-level) declaration whose Name.Sym matches `sym`. Returns the Name span on hit. The walk is O(total declarations) per call; this is fine for the LSP since navigation is single-cursor, not bulk.
func findDeclSpanForSym(c *compiler.CompilerContext, sym ir.SymID) (diag.TextSpan, bool) {
	for _, pkg := range c.Packages {
		for _, f := range pkg.Files {
			if f.Hir == nil {
				continue
			}
			if span, ok := scanFileForDeclSym(f.Hir, sym); ok {
				return span, true
			}
		}
	}
	return diag.TextSpan{}, false
}

func scanFileForDeclSym(f *ir.File, sym ir.SymID) (diag.TextSpan, bool) {
	for _, st := range f.Statements {
		if span, ok := scanStmtForDeclSym(st, sym); ok {
			return span, true
		}
	}
	return diag.TextSpan{}, false
}

func scanStmtForDeclSym(s ir.Stmt, sym ir.SymID) (diag.TextSpan, bool) {
	switch n := s.(type) {
	case *ir.VarDeclStmt:
		for _, tgt := range n.Targets {
			if tgt.Name != nil && tgt.Name.Sym == sym {
				return tgt.Name.Span, true
			}
		}
	case *ir.FuncDeclStmt:
		if n.Name.Sym == sym {
			return n.Name.Span, true
		}
		for _, param := range n.Params {
			if param.Name.Sym == sym {
				return param.Name.Span, true
			}
		}
		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				if span, ok := scanStmtForDeclSym(ss, sym); ok {
					return span, true
				}
			}
		}
	case *ir.TypeDeclStmt:
		if n.Name.Sym == sym {
			return n.Name.Span, true
		}
		for _, ctor := range n.Ctors {
			for _, param := range ctor.Params {
				if param.Name.Sym == sym {
					return param.Name.Span, true
				}
			}
			if ctor.Body != nil {
				for _, ss := range ir.BlockStmts(ctor.Body) {
					if span, ok := scanStmtForDeclSym(ss, sym); ok {
						return span, true
					}
				}
			}
		}
		for _, m := range n.Methods {
			if span, ok := scanStmtForDeclSym(m.Func, sym); ok {
				return span, true
			}
		}
	case *ir.EnumDeclStmt:
		if n.Name.Sym == sym {
			return n.Name.Span, true
		}
	case *ir.InterfaceDeclStmt:
		if n.Name.Sym == sym {
			return n.Name.Span, true
		}
	case *ir.BlockStmt:
		for _, ss := range n.Stmts {
			if span, ok := scanStmtForDeclSym(ss, sym); ok {
				return span, true
			}
		}
	case *ir.IfStmt:
		if n.Then != nil {
			for _, ss := range ir.BlockStmts(n.Then) {
				if span, ok := scanStmtForDeclSym(ss, sym); ok {
					return span, true
				}
			}
		}
		for _, eb := range n.ElseIfs {
			if eb.Then != nil {
				for _, ss := range ir.BlockStmts(eb.Then) {
					if span, ok := scanStmtForDeclSym(ss, sym); ok {
						return span, true
					}
				}
			}
		}
		if n.Else != nil {
			for _, ss := range ir.BlockStmts(n.Else) {
				if span, ok := scanStmtForDeclSym(ss, sym); ok {
					return span, true
				}
			}
		}
	case *ir.ForStmt:
		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				if span, ok := scanStmtForDeclSym(ss, sym); ok {
					return span, true
				}
			}
		}
	case *ir.WhileStmt:
		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				if span, ok := scanStmtForDeclSym(ss, sym); ok {
					return span, true
				}
			}
		}
	case *ir.TestDeclStmt:
		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				if span, ok := scanStmtForDeclSym(ss, sym); ok {
					return span, true
				}
			}
		}
	}
	return diag.TextSpan{}, false
}

// uriForSpan resolves a `TextSpan.File` (which the compiler stores as the path passed to AddSource - typically workspace-relative) into an LSP `file://` URI anchored to the snapshot's workspace root. Absolute paths pass through unchanged. Stdlib paths (`std/...`) are probed across the compiler's `stdlibSearchPaths` before falling back to the workspace root - the editor then opens the actual on-disk stdlib file shipped alongside the binary.
func uriForSpan(c *compiler.CompilerContext, snap *Snapshot, span diag.TextSpan) uri.URI {
	if span.File == "" {
		return ""
	}
	_ = c
	if filepath.IsAbs(span.File) {
		return pathToURI(span.File)
	}
	if abs := resolveStdlibPath(span.File); abs != "" {
		return pathToURI(abs)
	}
	root := uriToPath(snap.Root)
	if root == "" {
		return pathToURI(span.File)
	}
	return pathToURI(filepath.Join(root, filepath.FromSlash(span.File)))
}

// resolveStdlibPath returns the absolute on-disk path of a stdlib source file when `relPath` looks like `std/...` and exists in one of the stdlib search directories the compiler probes at load time. Returns empty when the path isn't stdlib-shaped or can't be located - the caller then falls back to its normal workspace-anchored resolution.
func resolveStdlibPath(relPath string) string {
	if !compiler.IsStdImport(filepath.ToSlash(filepath.Dir(relPath))) && !compiler.IsStdImport(filepath.ToSlash(relPath)) {
		return ""
	}
	rel := filepath.FromSlash(relPath)
	for _, base := range compiler.StdlibSearchPaths() {
		candidate := filepath.Join(base, strings.TrimPrefix(rel, "std"+string(filepath.Separator)))
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}
