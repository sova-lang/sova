package lsp

import (
	"context"

	"go.lsp.dev/protocol"

	"sova/internal/diag"
	"sova/internal/ir"
)

// FoldingRanges returns one folding region per multi-line construct in the file: function/type/enum/interface/mixin bodies, `extern { ... }` blocks, group bodies, control-flow blocks, comments, and import groups. Editors render these as the +/− gutter widgets that collapse code.
func (s *Server) FoldingRanges(ctx context.Context, params *protocol.FoldingRangeParams) ([]protocol.FoldingRange, error) {
	snap := s.session.Snapshot()
	if snap == nil {
		return nil, nil
	}
	c, _, err := snap.Compile(s.compileSnapshot)
	if err != nil || c == nil {
		return nil, nil
	}
	_, file, _ := lookupFileByURI(c, params.TextDocument.URI)
	if file == nil {
		return nil, nil
	}
	var ranges []protocol.FoldingRange
	for _, st := range file.Statements {
		collectFoldingForStmt(st, &ranges)
	}
	ranges = append(ranges, foldingForImports(file)...)
	src, ok := snap.ReadFile(params.TextDocument.URI)
	if ok {
		ranges = append(ranges, foldingForComments(src)...)
	}
	return ranges, nil
}

// collectFoldingForStmt emits folding ranges for one statement (and recurses into nested blocks). Multi-line spans become regular foldings; single-line constructs are skipped - the LSP defines folding ranges as spanning at least two lines.
func collectFoldingForStmt(s ir.Stmt, out *[]protocol.FoldingRange) {
	if s == nil {
		return
	}
	switch n := s.(type) {
	case *ir.FuncDeclStmt:
		emitFolding(out, n.Span(), protocol.RegionFoldingRange)
		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				collectFoldingForStmt(ss, out)
			}
		}
	case *ir.TypeDeclStmt:
		emitFolding(out, n.Span(), protocol.RegionFoldingRange)
		for _, ctor := range n.Ctors {
			if ctor.Body != nil {
				for _, ss := range ir.BlockStmts(ctor.Body) {
					collectFoldingForStmt(ss, out)
				}
			}
		}
		for _, m := range n.Methods {
			if m.Func != nil {
				collectFoldingForStmt(m.Func, out)
			}
		}
	case *ir.EnumDeclStmt:
		emitFolding(out, n.Span(), protocol.RegionFoldingRange)
		for _, m := range n.Methods {
			collectFoldingForStmt(m, out)
		}
	case *ir.InterfaceDeclStmt:
		emitFolding(out, n.Span(), protocol.RegionFoldingRange)
	case *ir.MixinDeclStmt:
		emitFolding(out, n.Span(), protocol.RegionFoldingRange)
	case *ir.ExternDeclStmt:
		emitFolding(out, n.Span(), protocol.RegionFoldingRange)
	case *ir.GroupDeclStmt:
		emitFolding(out, n.Span(), protocol.RegionFoldingRange)
		for _, ss := range n.Body {
			collectFoldingForStmt(ss, out)
		}
	case *ir.TestDeclStmt:
		emitFolding(out, n.Span(), protocol.RegionFoldingRange)
		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				collectFoldingForStmt(ss, out)
			}
		}
	case *ir.IfStmt:
		if n.Then != nil {
			emitFolding(out, n.Then.Span(), protocol.RegionFoldingRange)
			for _, ss := range ir.BlockStmts(n.Then) {
				collectFoldingForStmt(ss, out)
			}
		}
		for _, eb := range n.ElseIfs {
			if eb.Then != nil {
				emitFolding(out, eb.Then.Span(), protocol.RegionFoldingRange)
				for _, ss := range ir.BlockStmts(eb.Then) {
					collectFoldingForStmt(ss, out)
				}
			}
		}
		if n.Else != nil {
			emitFolding(out, n.Else.Span(), protocol.RegionFoldingRange)
			for _, ss := range ir.BlockStmts(n.Else) {
				collectFoldingForStmt(ss, out)
			}
		}
	case *ir.ForStmt:
		if n.Body != nil {
			emitFolding(out, n.Body.Span(), protocol.RegionFoldingRange)
			for _, ss := range ir.BlockStmts(n.Body) {
				collectFoldingForStmt(ss, out)
			}
		}
	case *ir.WhileStmt:
		if n.Body != nil {
			emitFolding(out, n.Body.Span(), protocol.RegionFoldingRange)
			for _, ss := range ir.BlockStmts(n.Body) {
				collectFoldingForStmt(ss, out)
			}
		}
	case *ir.SelectStmt:
		emitFolding(out, n.Span(), protocol.RegionFoldingRange)
	case *ir.GoStmt:
		if n.Body != nil {
			emitFolding(out, n.Body.Span(), protocol.RegionFoldingRange)
		}
	case *ir.DeferStmt:
		if n.Body != nil {
			emitFolding(out, n.Body.Span(), protocol.RegionFoldingRange)
		}
	case *ir.AsSessionStmt:
		if n.Body != nil {
			emitFolding(out, n.Body.Span(), protocol.RegionFoldingRange)
		}
	case *ir.BlockStmt:
		emitFolding(out, n.Span(), protocol.RegionFoldingRange)
		for _, ss := range n.Stmts {
			collectFoldingForStmt(ss, out)
		}
	}
}

// foldingForImports groups consecutive `import` statements into a single foldable block - the conventional "imports" widget every gofmt-like editor shows.
func foldingForImports(f *ir.File) []protocol.FoldingRange {
	var out []protocol.FoldingRange
	startLn, endLn := 0, 0
	first := true
	for _, st := range f.Statements {
		imp, ok := st.(*ir.ImportStmt)
		if !ok {
			if !first && endLn > startLn {
				out = append(out, protocol.FoldingRange{
					StartLine: uint32(startLn - 1),
					EndLine:   uint32(endLn - 1),
					Kind:      protocol.ImportsFoldingRange,
				})
			}
			first = true
			continue
		}
		span := imp.Span()
		if first {
			startLn = span.StartLn
			endLn = span.EndLn
			first = false
			continue
		}
		endLn = span.EndLn
	}
	if !first && endLn > startLn {
		out = append(out, protocol.FoldingRange{
			StartLine: uint32(startLn - 1),
			EndLine:   uint32(endLn - 1),
			Kind:      protocol.ImportsFoldingRange,
		})
	}
	return out
}

// foldingForComments emits ranges for runs of consecutive line-comments and for multi-line block comments. A line-comment "run" is two or more `//` comments on adjacent lines; the run is foldable as a unit. Block comments fold when they span more than one line. The scan is byte-level (no parse required) so it works even when the source has parse errors.
func foldingForComments(src string) []protocol.FoldingRange {
	var out []protocol.FoldingRange
	runStart, runEnd := 0, 0
	inRun := false
	flush := func() {
		if inRun && runEnd > runStart {
			out = append(out, protocol.FoldingRange{
				StartLine: uint32(runStart - 1),
				EndLine:   uint32(runEnd - 1),
				Kind:      protocol.CommentFoldingRange,
			})
		}
		inRun = false
	}
	line := 1
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			startLine := line
			for i < len(src) && src[i] != '\n' {
				i++
			}
			if inRun && startLine == runEnd+1 {
				runEnd = startLine
				continue
			}
			flush()
			runStart = startLine
			runEnd = startLine
			inRun = true
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			flush()
			startLine := line
			i += 2
			for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
				if src[i] == '\n' {
					line++
				}
				i++
			}
			if i+1 < len(src) {
				i += 2
			}
			if line > startLine {
				out = append(out, protocol.FoldingRange{
					StartLine: uint32(startLine - 1),
					EndLine:   uint32(line - 1),
					Kind:      protocol.CommentFoldingRange,
				})
			}
		case c == '"':
			i++
			for i < len(src) && src[i] != '"' && src[i] != '\n' {
				if src[i] == '\\' && i+1 < len(src) {
					i++
				}
				i++
			}
			if i < len(src) && src[i] == '"' {
				i++
			}
		default:
			i++
		}
	}
	flush()
	return out
}

func emitFolding(out *[]protocol.FoldingRange, span diag.TextSpan, kind protocol.FoldingRangeKind) {
	if span.EndLn <= span.StartLn {
		return
	}
	*out = append(*out, protocol.FoldingRange{
		StartLine: uint32(span.StartLn - 1),
		EndLine:   uint32(span.EndLn - 1),
		Kind:      kind,
	})
}
