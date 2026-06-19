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

	typeRefName      string
	typeRefQualifier string

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
