package lsp

import (
	"path/filepath"
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"sova/internal/diag"
	"sova/internal/ir"
	"sova/internal/services/compiler"
)

// cursorTarget is the result of a position-to-IR lookup: the file the cursor is in, the innermost node whose span contains the cursor, and (when the cursor is on a name reference) the symbol it resolves to. Returned by `findCursorTarget` for use by Hover, Definition, etc.
type cursorTarget struct {
	pkg       *ir.PackageContext
	file      *ir.File
	filePath  string
	span      diag.TextSpan
	sym       ir.SymID
	typ       ir.TypID
	kind      cursorKind
	memberOf  ir.TypID
	fieldName string
	// typeRefName / typeRefQualifier are populated for cursorKindTypeRef hits - the cursor is on a `CustomName` (and optional qualifier) inside a `TypeRef`, e.g. `let x: pkg.Foo<T>` or a parameter / field / return type. Resolution looks them up in the type table at definition time.
	typeRefName      string
	typeRefQualifier string
	// importPath is populated for cursorKindImportPath hits - the cursor is on the `"pkg/path"` literal of an `import` statement.
	importPath string
}

type cursorKind int

const (
	cursorKindNone cursorKind = iota
	cursorKindSymbol
	cursorKindMember
	cursorKindDecl
	cursorKindTypeRef
	cursorKindImportPath
)

// findCursorTarget locates the deepest IR node that contains the given LSP position and reports what's there: a resolved symbol, a member-access (sym=0, fieldName set), or nothing. Returns nil when the URI isn't part of any package or when no node contains the position.
func findCursorTarget(c *compiler.CompilerContext, target uri.URI, line, col uint32) *cursorTarget {
	pkg, file, path := lookupFileByURI(c, target)
	if file == nil {
		return nil
	}
	cursor := position{line: int(line) + 1, col: int(col) + 1}
	t := &cursorTarget{pkg: pkg, file: file, filePath: path}
	for _, st := range file.Statements {
		if hit := walkStmt(t, st, cursor); hit {
			return t
		}
	}
	if t.kind != cursorKindNone {
		return t
	}
	return nil
}

// position is the 1-based row/col cursor coordinate used internally; LSP positions are 0-based, the conversion happens at the entry point.
type position struct{ line, col int }

func (p position) inSpan(s diag.TextSpan) bool {
	if s.StartLn == 0 && s.EndLn == 0 {
		return false
	}
	if p.line < s.StartLn || p.line > s.EndLn {
		return false
	}
	if p.line == s.StartLn && p.col < s.StartCol {
		return false
	}
	if p.line == s.EndLn && p.col > s.EndCol {
		return false
	}
	return true
}

// lookupFileByURI scans every package's files looking for one whose Filename matches `target` (compared by absolute path). Returns the owning package + the file's HIR + the absolute path we matched on. The LSP gives us file URIs from the editor; the compiler stores files by the relative path we used at AddSource time, so we have to compare via absolute paths.
func lookupFileByURI(c *compiler.CompilerContext, target uri.URI) (*ir.PackageContext, *ir.File, string) {
	targetPath := uriToPath(target)
	if targetPath == "" {
		return nil, nil, ""
	}
	targetAbs, err := filepath.Abs(targetPath)
	if err != nil {
		targetAbs = targetPath
	}
	resolved, err := filepath.EvalSymlinks(targetAbs)
	if err == nil {
		targetAbs = resolved
	}
	for _, pkg := range c.Packages {
		for _, f := range pkg.Files {
			if f.Hir == nil {
				continue
			}
			candidates := []string{f.Filename, f.Hir.Path}
			for _, cand := range candidates {
				if cand == "" {
					continue
				}
				abs := cand
				if !filepath.IsAbs(abs) {
					if absJoined, err := filepath.Abs(cand); err == nil {
						abs = absJoined
					}
				}
				if r, err := filepath.EvalSymlinks(abs); err == nil {
					abs = r
				}
				if filepath.ToSlash(abs) == filepath.ToSlash(targetAbs) || strings.HasSuffix(filepath.ToSlash(targetAbs), filepath.ToSlash(cand)) {
					return pkg, f.Hir, abs
				}
			}
		}
	}
	return nil, nil, ""
}

// spanToLSPRange converts a 1-based Sova TextSpan into a 0-based LSP Range. Used by every navigation handler that wants to point at a source location.
func spanToLSPRange(s diag.TextSpan) protocol.Range {
	startLn := uint32(0)
	startCol := uint32(0)
	endLn := uint32(0)
	endCol := uint32(0)
	if s.StartLn > 0 {
		startLn = uint32(s.StartLn - 1)
	}
	if s.StartCol > 0 {
		startCol = uint32(s.StartCol - 1)
	}
	if s.EndLn > 0 {
		endLn = uint32(s.EndLn - 1)
	}
	if s.EndCol > 0 {
		endCol = uint32(s.EndCol - 1)
	}
	return protocol.Range{
		Start: protocol.Position{Line: startLn, Character: startCol},
		End:   protocol.Position{Line: endLn, Character: endCol},
	}
}
