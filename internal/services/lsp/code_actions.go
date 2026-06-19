package lsp

import (
	"context"
	"sort"
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"sova/internal/ir"
)

func (s *Server) CodeAction(ctx context.Context, params *protocol.CodeActionParams) ([]protocol.CodeAction, error) {
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

	src, ok := snap.ReadFile(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}

	var actions []protocol.CodeAction
	if organize := organizeImportsAction(file, src, params.TextDocument.URI); organize != nil {
		actions = append(actions, *organize)
	}

	for _, d := range params.Context.Diagnostics {
		if strings.Contains(strings.ToLower(d.Message), "unused") {
			if act := removeLineAction(src, params.TextDocument.URI, d); act != nil {
				actions = append(actions, *act)
			}
		}
	}

	return actions, nil
}

func organizeImportsAction(file *ir.File, src string, docURI uri.URI) *protocol.CodeAction {
	type imp struct {
		path  string
		alias string
		using string
		span  protocol.Range
	}

	var imps []imp
	for _, st := range file.Statements {
		i, ok := st.(*ir.ImportStmt)
		if !ok {
			continue
		}

		entry := imp{path: i.Path.String(), alias: i.Alias, span: spanToRange(i.Span())}

		if i.UsingAll {
			entry.using = " using *"
		} else if len(i.UsingList) > 0 {
			entry.using = " using {" + strings.Join(i.UsingList, ", ") + "}"
		}

		imps = append(imps, entry)
	}

	if len(imps) < 2 {
		return nil
	}

	seen := map[string]bool{}

	uniq := make([]imp, 0, len(imps))
	for _, i := range imps {
		if seen[i.path] {
			continue
		}

		seen[i.path] = true
		uniq = append(uniq, i)
	}

	sort.Slice(uniq, func(a, b int) bool { return uniq[a].path < uniq[b].path })

	canonical := true
	for i, x := range imps {
		if i >= len(uniq) || x.path != uniq[i].path {
			canonical = false
			break
		}
	}

	if canonical && len(imps) == len(uniq) {
		return nil
	}

	first := imps[0].span.Start
	last := imps[len(imps)-1].span.End
	var b strings.Builder
	for _, i := range uniq {
		b.WriteString("import \"")
		b.WriteString(i.path)
		b.WriteString("\"")
		b.WriteString(i.using)
		b.WriteString("\n")
	}

	_ = src
	edit := protocol.TextEdit{
		Range:   protocol.Range{Start: first, End: last},
		NewText: strings.TrimRight(b.String(), "\n"),
	}

	return &protocol.CodeAction{
		Title: "Organize imports",
		Kind:  protocol.SourceOrganizeImports,
		Edit:  &protocol.WorkspaceEdit{Changes: map[uri.URI][]protocol.TextEdit{docURI: {edit}}},
	}
}

func removeLineAction(src string, docURI uri.URI, d protocol.Diagnostic) *protocol.CodeAction {
	lineStart := protocol.Position{Line: d.Range.Start.Line, Character: 0}

	nextLine := protocol.Position{Line: d.Range.Start.Line + 1, Character: 0}

	edit := protocol.TextEdit{
		Range:   protocol.Range{Start: lineStart, End: nextLine},
		NewText: "",
	}

	_ = src
	return &protocol.CodeAction{
		Title: "Remove this declaration",
		Kind:  protocol.QuickFix,
		Edit: &protocol.WorkspaceEdit{Changes: map[uri.URI][]protocol.TextEdit{
			docURI: {edit},
		}},
		Diagnostics: []protocol.Diagnostic{d},
	}
}
