package lsp

import (
	"context"
	"sort"
	"strings"

	"go.lsp.dev/protocol"

	"sova/internal/diag"
	"sova/internal/ir"
	"sova/internal/passes"
	"sova/internal/services/compiler"
)

func (s *Server) Completion(ctx context.Context, params *protocol.CompletionParams) (*protocol.CompletionList, error) {
	snap := s.session.Snapshot()
	if snap == nil {
		return nil, nil
	}

	c, _, err := snap.Compile(s.compileSnapshot)
	if err != nil || c == nil {
		return nil, nil
	}

	pkg, file, _ := lookupFileByURI(c, params.TextDocument.URI)
	if file == nil {
		return nil, nil
	}

	src, ok := snap.ReadFile(params.TextDocument.URI)
	if !ok {
		return nil, nil
	}

	ctxKind, dotPrefix := classifyCompletion(src, params.Position)
	var items []protocol.CompletionItem
	switch ctxKind {
	case completionAfterDot:
		items = memberCompletions(c, file, params.Position, dotPrefix)
	case completionImportPath:
		items = importPathCompletions(s, snap, c, params.TextDocument.URI)
	case completionWireOption:
		items = wireOptionCompletions()
	case completionAnnotation:
		items = annotationCompletions(c)
	case completionCSSClass:
		offset := lspPositionToOffset(src, params.Position)
		var slot *callContext
		if ctx, ok := cssClassSlotAt(c, src, offset); ok {
			slot = &ctx
		}

		items = cssClassCompletions(c, slot)
	default:
		items = identifierCompletions(c, pkg, file)
		items = append(items, localScopeCompletions(c, file, params.Position)...)
	}

	applyWordReplaceRange(items, src, params.Position, ctxKind)
	return &protocol.CompletionList{Items: items, IsIncomplete: false}, nil
}

func applyWordReplaceRange(items []protocol.CompletionItem, src string, pos protocol.Position, kind completionContextKind) {
	if len(items) == 0 {
		return
	}

	if kind == completionImportPath || kind == completionWireOption {
		return
	}

	offset := lspPositionToOffset(src, pos)
	wordStart := offset
	for wordStart > 0 && isClassCharForKind(src[wordStart-1], kind) {
		wordStart--
	}

	wordEnd := offset
	for wordEnd < len(src) && isClassCharForKind(src[wordEnd], kind) {
		wordEnd++
	}

	startPos := offsetToLSPPosition(src, wordStart)
	endPos := offsetToLSPPosition(src, wordEnd)
	rng := protocol.Range{Start: startPos, End: endPos}

	for i := range items {
		newText := items[i].InsertText
		if newText == "" {
			newText = items[i].Label
		}

		items[i].TextEdit = &protocol.TextEdit{Range: rng, NewText: newText}
	}
}

func offsetToLSPPosition(src string, offset int) protocol.Position {
	if offset < 0 {
		offset = 0
	}

	if offset > len(src) {
		offset = len(src)
	}

	line, col := uint32(0), uint32(0)
	for i := 0; i < offset; i++ {
		if src[i] == '\n' {
			line++
			col = 0
			continue
		}

		col++
	}

	return protocol.Position{Line: line, Character: col}
}

type completionContextKind int

const (
	completionUnknown completionContextKind = iota
	completionIdentifier
	completionAfterDot
	completionImportPath
	completionWireOption
	completionAnnotation
	completionCSSClass
)

func classifyCompletion(src string, pos protocol.Position) (completionContextKind, string) {
	offset := lspPositionToOffset(src, pos)
	if offset <= 0 {
		return completionIdentifier, ""
	}

	if isInsideImportString(src, offset) {
		return completionImportPath, ""
	}

	if isInsideWireOptions(src, offset) {
		return completionWireOption, ""
	}

	if isInsideGenericString(src, offset) {
		return completionCSSClass, ""
	}

	end := offset
	for end > 0 && isIdentChar(src[end-1]) {
		end--
	}

	if end > 0 && src[end-1] == '@' {
		return completionAnnotation, ""
	}

	if end == 0 || src[end-1] != '.' {
		return completionIdentifier, ""
	}

	dotEnd := end - 1
	recvEnd := dotEnd
	if recvEnd > 0 && src[recvEnd-1] == '@' {
		return completionAfterDot, "@"
	}

	if recvEnd > 0 && src[recvEnd-1] == ')' {
		return completionAfterDot, "()"
	}

	recvStart := recvEnd
	for recvStart > 0 && isIdentChar(src[recvStart-1]) {
		recvStart--
	}

	return completionAfterDot, src[recvStart:recvEnd]
}

func annotationCompletions(c *compiler.CompilerContext) []protocol.CompletionItem {
	type ann struct{ name, detail, doc string }

	anns := []ann{
		{
			name:   "reactive",
			detail: "@reactive (field | wire let)",
			doc:    "Marks a field on a `type` or a top-level `wire let` as reactive. Reads tracked inside Strix `effect` / `computed` / `view()` subscribe to the field; writes notify every observer, so views re-render automatically.\n\n```sova\ntype Counter with Composable, Component {\n    @reactive count: int = 0\n\n    func view(): Composable {\n        return H1 { \"count: \" + count }\n    }\n}\n\n@reactive wire let ingameTime: int = 0\n```",
		},
		{
			name:   "structTag",
			detail: "@structTag(\"<key>\", \"<value>\") (field)",
			doc:    "Adds a Go struct tag to the Go-side struct field the Sova field compiles to. The first argument is the tag *namespace* (`gorm`, `json`, `validate`, `xml`, ...) - anything the consuming Go library reflects on - and the second is the literal tag value. Multiple `@structTag` entries on the same field stack: same-namespace values are joined with a single space, so two `@structTag(\"gorm\", ...)` annotations produce one `` `gorm:\"...\"` `` tag with both rules.\n\n```sova\ntype User {\n    @structTag(\"gorm\", \"primaryKey;autoIncrement\")\n    @structTag(\"json\", \"id\")\n    id: int = 0\n\n    @structTag(\"gorm\", \"size:200;not null\")\n    @structTag(\"gorm\", \"index\")\n    name: string = \"\"\n}\n```\n\nThe Sova-side `json:\"<fieldname>\"` tag is emitted automatically for every non-`__`-prefixed field; supply your own `@structTag(\"json\", ...)` to override the default. The compiler enforces the exact-two-string-args shape at compile time, so typos surface as clean diagnostics rather than malformed tags.",
		},
		{
			name:   "cssClass",
			detail: "@cssClass (param)",
			doc:    "Marks a function parameter as a *CSS class slot* so the LSP knows that string arguments passed at that position are class names from the project's CSS / SCSS files. The compiler treats this annotation as metadata only - it does not affect codegen or runtime behaviour - but the editor uses it to offer precise class-name completion at the call site instead of falling back to the broad in-string heuristic.\n\n```sova\nfunc Element(tag: string, @cssClass class: string): Composable { ... }\n\n// At the call site:\nElement(\"button\", \"prim<cursor>\") // editor suggests `primary`, `btn-large`, ...\n```\n\nUse this on any string-typed param whose value will become a `class` attribute, an element tag the renderer reads as a class, or a CSS selector argument. The annotation may appear on multiple params; each one becomes its own class slot.",
		},
	}

	out := make([]protocol.CompletionItem, 0, len(anns))
	for _, a := range anns {
		item := protocol.CompletionItem{
			Label:  a.name,
			Kind:   protocol.CompletionItemKindProperty,
			Detail: a.detail,
		}

		attachDocToCompletionItem(&item, a.doc)
		out = append(out, item)
	}

	for _, item := range synthAnnotationCompletions(c) {
		out = append(out, item)
	}

	return out
}

func synthAnnotationCompletions(c *compiler.CompilerContext) []protocol.CompletionItem {
	if c == nil {
		return nil
	}

	reg, ok := c.Cache[passes.SynthRegistryCacheKey].(map[string]*ir.SynthDeclStmt)
	if !ok || len(reg) == 0 {
		return nil
	}

	names := make([]string, 0, len(reg))
	for n := range reg {
		names = append(names, n)
	}

	sort.Strings(names)
	out := make([]protocol.CompletionItem, 0, len(names))
	for _, name := range names {
		sd := reg[name]
		item := protocol.CompletionItem{
			Label:  name,
			Kind:   protocol.CompletionItemKindProperty,
			Detail: synthSignature(sd),
		}

		attachDocToCompletionItem(&item, synthDoc(sd))
		out = append(out, item)
	}

	return out
}

func synthSignature(sd *ir.SynthDeclStmt) string {
	if sd == nil {
		return ""
	}

	out := "@" + sd.Name.Name
	if len(sd.Params) > 0 {
		out += "("
		for i, p := range sd.Params {
			if i > 0 {
				out += ", "
			}

			if p == nil {
				out += "?"
				continue
			}

			out += p.Name.Name
			if p.Type != nil && p.Type.CustomName != "" {
				out += ": " + p.Type.CustomName
			}
		}

		out += ")"
	}

	out += " (on "
	if side := synthSideLabel(sd.RequiredSide); side != "" {
		out += side + " "
	}

	out += sd.Target.Kind.String() + " " + sd.Target.BindName + ")"
	return out
}

func synthSideLabel(s ir.SideKind) string {
	switch s {
	case ir.SideFrontend:
		return "frontend"
	case ir.SideBackend:
		return "backend"
	case ir.SideShared:
		return "shared"
	}

	return ""
}

func synthDoc(sd *ir.SynthDeclStmt) string {
	if sd == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString("Custom annotation. Lowers via the following synth declaration:\n\n```sova\n")
	b.WriteString("synth ")
	b.WriteString(sd.Name.Name)
	if len(sd.Params) > 0 {
		b.WriteString("(")
		for i, p := range sd.Params {
			if i > 0 {
				b.WriteString(", ")
			}

			if p != nil {
				b.WriteString(p.Name.Name)
				if p.Type != nil && p.Type.CustomName != "" {
					b.WriteString(": ")
					b.WriteString(p.Type.CustomName)
				}
			}
		}

		b.WriteString(")")
	}

	b.WriteString(" on ")
	if side := synthSideLabel(sd.RequiredSide); side != "" {
		b.WriteString(side)
		b.WriteString(" ")
	}

	b.WriteString(sd.Target.Kind.String())
	b.WriteString(" ")
	b.WriteString(sd.Target.BindName)
	b.WriteString(" { ... }\n```")
	return b.String()
}

func wireOptionCompletions() []protocol.CompletionItem {
	type opt struct{ key, snippet, detail, doc string }

	opts := []opt{
		{
			key:     "authn",
			snippet: "authn: $0",
			detail:  "authn: bool",
			doc:     "Whether the wire requires an authenticated session. Defaults to `true`; set `false` to expose the wire publicly.\n\n```sova\nwire(authn: false) func login(username: string, password: string): bool { ... }\n```",
		},
		{
			key:     "authz",
			snippet: "authz: [$0]",
			detail:  "authz: []string",
			doc:     "Role names allowed to invoke the wire. The compiler enforces the check at call time. Implies `authn: true`.\n\n```sova\nwire(authz: [\"admin\"]) func deleteUser(id: string) { ... }\n```",
		},
		{
			key:     "transport",
			snippet: "transport: \"$0\"",
			detail:  "transport: \"http\" | \"ws\" | \"sse\"",
			doc:     "Pins the wire to a specific transport. Backend wires accept `http` (default) or `ws`; frontend wires accept `ws` (default) or `sse`. Other combinations are rejected at compile time.",
		},
		{
			key:     "method",
			snippet: "method: \"$0\"",
			detail:  "method: \"GET\" | \"POST\" | \"PUT\" | \"PATCH\" | \"DELETE\"",
			doc:     "Overrides the auto-derived HTTP method for backend wires. The default is derived from the function name (`get*` → GET, otherwise POST).",
		},
		{
			key:     "path",
			snippet: "path: \"$0\"",
			detail:  "path: string",
			doc:     "Overrides the auto-derived URL path for backend wires. Path parameters use `:name` placeholders that bind to function parameters of the same name.\n\n```sova\nwire(path: \"/users/:id\") func getUser(id: string): User { ... }\n```",
		},
		{
			key:     "buffer",
			snippet: "buffer: $0",
			detail:  "buffer: bool | int",
			doc:     "Frontend-wire only. `true` enables a per-session bounded queue (default 100 messages) so pushes survive a brief disconnect; an `int` sets the queue size. Without `buffer`, messages to a disconnected session are dropped.",
		},
		{
			key:     "maxBody",
			snippet: "maxBody: $0",
			detail:  "maxBody: int",
			doc:     "Maximum request body size in bytes accepted by this wire (default `1048576` = 1 MiB). Bodies larger than the cap respond with **400 Bad Request** before the wire body runs, so the handler can't be DoS'd by oversized payloads. Set to `0` to disable the cap entirely - useful for file-upload endpoints that validate size in user code.\n\n```sova\nwire(maxBody: 10485760) func uploadAvatar(image: string) { ... }   // 10 MiB\nwire(maxBody: 0)        func uploadArchive(bytes: string) { ... }  // unbounded\n```",
		},
	}

	out := make([]protocol.CompletionItem, 0, len(opts))
	for _, o := range opts {
		item := protocol.CompletionItem{
			Label:            o.key,
			Kind:             protocol.CompletionItemKindProperty,
			Detail:           o.detail,
			InsertText:       o.snippet,
			InsertTextFormat: protocol.InsertTextFormatSnippet,
		}

		attachDocToCompletionItem(&item, o.doc)
		out = append(out, item)
	}

	return out
}

func isInsideWireOptions(src string, offset int) bool {
	depthParen, depthBracket, depthBrace := 0, 0, 0
	inString := byte(0)
	for i := offset - 1; i >= 0; i-- {
		ch := src[i]
		if inString != 0 {
			if ch == inString && (i == 0 || src[i-1] != '\\') {
				inString = 0
			}

			continue
		}

		switch ch {
		case '"', '\'', '`':
			inString = ch
		case ')':
			depthParen++
		case ']':
			depthBracket++
		case '}':
			depthBrace++
		case '[':
			if depthBracket > 0 {
				depthBracket--
			}

		case '{':
			if depthBrace > 0 {
				depthBrace--
			}

		case '(':
			if depthParen == 0 && depthBracket == 0 && depthBrace == 0 {

				end := i
				for end > 0 && (src[end-1] == ' ' || src[end-1] == '\t') {
					end--
				}

				start := end
				for start > 0 && isIdentChar(src[start-1]) {
					start--
				}

				return src[start:end] == "wire"
			}

			depthParen--
		}
	}

	return false
}

func isInsideImportString(src string, offset int) bool {
	lineStart := offset
	for lineStart > 0 && src[lineStart-1] != '\n' {
		lineStart--
	}

	line := src[lineStart:offset]
	quoteCount := 0
	for i := 0; i < len(line); i++ {
		if line[i] == '"' {
			quoteCount++
		}
	}

	if quoteCount%2 != 1 {
		return false
	}

	trimmed := strings.TrimLeft(line, " \t")
	return strings.HasPrefix(trimmed, "import")
}

func isInsideGenericString(src string, offset int) bool {
	lineStart := offset
	for lineStart > 0 && src[lineStart-1] != '\n' {
		lineStart--
	}

	inDouble := false
	inSingle := false
	i := lineStart
	for i < offset {
		c := src[i]
		if c == '\\' && i+1 < offset {
			i += 2
			continue
		}

		switch {
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		}

		i++
	}

	return inDouble || inSingle
}

func isIdentChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

func isClassCharForKind(b byte, kind completionContextKind) bool {
	if kind == completionCSSClass && b == '-' {
		return true
	}

	return isIdentChar(b)
}

func lspPositionToOffset(src string, pos protocol.Position) int {
	line, col := uint32(0), uint32(0)
	for i := 0; i < len(src); i++ {
		if line == pos.Line && col == pos.Character {
			return i
		}

		if src[i] == '\n' {
			line++
			col = 0
			if line > pos.Line {
				return i
			}

			continue
		}

		col++
	}

	return len(src)
}

func localScopeCompletions(c *compiler.CompilerContext, file *ir.File, pos protocol.Position) []protocol.CompletionItem {
	if file == nil {
		return nil
	}

	cursor := position{line: int(pos.Line) + 1, col: int(pos.Character) + 1}

	enc := findEnclosingFunc(file, cursor)
	if enc == nil {
		return nil
	}

	var items []protocol.CompletionItem
	for _, p := range enc.Params {
		if p == nil {
			continue
		}

		detail := "param " + p.Name.Name
		if p.Type != nil {
			detail += ": " + formatTypeFromRef(p.Type)
		}

		items = append(items, protocol.CompletionItem{
			Label:  p.Name.Name,
			Kind:   protocol.CompletionItemKindVariable,
			Detail: detail,
		})
	}

	collectLocalDecls(c, enc.Body, cursor, &items)
	return items
}

func findEnclosingFunc(file *ir.File, cursor position) *funcShape {
	var found *funcShape
	for _, st := range file.Statements {
		walkForEnclosingFunc(st, cursor, &found)
	}

	return found
}

type funcShape struct {
	Params []*ir.FuncParam
	Body   *ir.BlockStmt
}

func walkForEnclosingFunc(s ir.Stmt, cursor position, out **funcShape) {
	if ir.IsNilStmt(s) {
		return
	}

	switch n := s.(type) {
	case *ir.FuncDeclStmt:
		if n.Body != nil && cursor.inSpan(n.Body.Span()) {
			*out = &funcShape{Params: n.Params, Body: n.Body}

			for _, ss := range ir.BlockStmts(n.Body) {
				walkForEnclosingFunc(ss, cursor, out)
			}
		}

	case *ir.TypeDeclStmt:
		for _, m := range n.Methods {
			if m.Func == nil {
				continue
			}

			if m.Func.Body != nil && cursor.inSpan(m.Func.Body.Span()) {
				*out = &funcShape{Params: m.Func.Params, Body: m.Func.Body}

				for _, ss := range ir.BlockStmts(m.Func.Body) {
					walkForEnclosingFunc(ss, cursor, out)
				}
			}
		}

		for _, ctor := range n.Ctors {
			if ctor.Body != nil && cursor.inSpan(ctor.Body.Span()) {
				*out = &funcShape{Params: ctor.Params, Body: ctor.Body}

				for _, ss := range ir.BlockStmts(ctor.Body) {
					walkForEnclosingFunc(ss, cursor, out)
				}
			}
		}

	case *ir.BlockStmt:
		for _, ss := range n.Stmts {
			walkForEnclosingFunc(ss, cursor, out)
		}

	case *ir.IfStmt:
		for _, ss := range ir.BlockStmts(n.Then) {
			walkForEnclosingFunc(ss, cursor, out)
		}

		for _, eb := range n.ElseIfs {
			for _, ss := range ir.BlockStmts(eb.Then) {
				walkForEnclosingFunc(ss, cursor, out)
			}
		}

		for _, ss := range ir.BlockStmts(n.Else) {
			walkForEnclosingFunc(ss, cursor, out)
		}

	case *ir.ForStmt:
		for _, ss := range ir.BlockStmts(n.Body) {
			walkForEnclosingFunc(ss, cursor, out)
		}

	case *ir.WhileStmt:
		for _, ss := range ir.BlockStmts(n.Body) {
			walkForEnclosingFunc(ss, cursor, out)
		}
	}
}

func collectLocalDecls(c *compiler.CompilerContext, body *ir.BlockStmt, cursor position, out *[]protocol.CompletionItem) {
	if body == nil {
		return
	}

	for _, st := range ir.BlockStmts(body) {
		if ir.IsNilStmt(st) {
			continue
		}

		span := st.Span()
		if span.EndLn != 0 && cursorBefore(cursor, span) {
			break
		}

		switch n := st.(type) {
		case *ir.VarDeclStmt:
			if !cursorAfter(cursor, span) {
				continue
			}

			for _, ci := range varCompletions(c, n) {
				ci.Detail = "local " + strings.TrimSpace(strings.TrimPrefix(ci.Detail, "let "))
				if n.IsConst {
					ci.Detail = "local const " + n.Targets[0].Name.Name
				}

				*out = append(*out, ci)
			}

		case *ir.FuncDeclStmt:
			if !cursorAfter(cursor, span) {
				continue
			}

			ci := funcCompletion(c, n)
			ci.Detail = "local " + ci.Detail
			*out = append(*out, ci)
		case *ir.BlockStmt:
			if cursor.inSpan(n.Span()) {
				collectLocalDecls(c, n, cursor, out)
			}

		case *ir.IfStmt:
			if n.Then != nil && cursor.inSpan(n.Then.Span()) {
				collectLocalDecls(c, n.Then, cursor, out)
			}

			for _, eb := range n.ElseIfs {
				if eb.Then != nil && cursor.inSpan(eb.Then.Span()) {
					collectLocalDecls(c, eb.Then, cursor, out)
				}
			}

			if n.Else != nil && cursor.inSpan(n.Else.Span()) {
				collectLocalDecls(c, n.Else, cursor, out)
			}

		case *ir.ForStmt:
			if n.Body != nil && cursor.inSpan(n.Body.Span()) {
				if n.CondType == ir.ForCondInt && n.CondInt != nil && n.CondInt.Init != nil {
					for _, tgt := range n.CondInt.Init.Targets {
						if tgt.Name == nil {
							continue
						}

						*out = append(*out, protocol.CompletionItem{
							Label:  tgt.Name.Name,
							Kind:   protocol.CompletionItemKindVariable,
							Detail: "loop var " + tgt.Name.Name,
						})
					}
				}

				if n.CondType == ir.ForCondIn && n.CondIn != nil {
					if n.CondIn.InFirstVar.Name != "" {
						*out = append(*out, protocol.CompletionItem{
							Label:  n.CondIn.InFirstVar.Name,
							Kind:   protocol.CompletionItemKindVariable,
							Detail: "loop var " + n.CondIn.InFirstVar.Name,
						})
					}

					if n.CondIn.InSecondVar != nil && n.CondIn.InSecondVar.Name != "" {
						*out = append(*out, protocol.CompletionItem{
							Label:  n.CondIn.InSecondVar.Name,
							Kind:   protocol.CompletionItemKindVariable,
							Detail: "loop var " + n.CondIn.InSecondVar.Name,
						})
					}
				}

				if n.CondType == ir.ForCondRange && n.CondRange != nil && n.CondRange.RangeVar.Name != "" {
					*out = append(*out, protocol.CompletionItem{
						Label:  n.CondRange.RangeVar.Name,
						Kind:   protocol.CompletionItemKindVariable,
						Detail: "loop var " + n.CondRange.RangeVar.Name,
					})
				}

				collectLocalDecls(c, n.Body, cursor, out)
			}

		case *ir.WhileStmt:
			if n.Body != nil && cursor.inSpan(n.Body.Span()) {
				collectLocalDecls(c, n.Body, cursor, out)
			}
		}
	}
}

func cursorAfter(cursor position, span diag.TextSpan) bool {
	if span.EndLn == 0 {
		return false
	}

	if cursor.line > span.EndLn {
		return true
	}

	if cursor.line == span.EndLn && cursor.col > span.EndCol {
		return true
	}

	return false
}

func cursorBefore(cursor position, span diag.TextSpan) bool {
	if span.StartLn == 0 {
		return false
	}

	if cursor.line < span.StartLn {
		return true
	}

	if cursor.line == span.StartLn && cursor.col < span.StartCol {
		return true
	}

	return false
}

func identifierCompletions(c *compiler.CompilerContext, pkg *ir.PackageContext, file *ir.File) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	for _, kw := range sovaKeywords {
		items = append(items, protocol.CompletionItem{
			Label: kw,
			Kind:  protocol.CompletionItemKindKeyword,
		})
	}

	if file != nil {
		consumerSide := currentFileSide(file)
		for _, st := range file.Statements {
			switch n := st.(type) {
			case *ir.FuncDeclStmt:
				if !funcVisibleFromSide(n, file, consumerSide) {
					continue
				}

				items = append(items, funcCompletion(c, n))
			case *ir.VarDeclStmt:
				items = append(items, varCompletions(c, n)...)
			case *ir.TypeDeclStmt:
				items = append(items, protocol.CompletionItem{
					Label:  n.Name.Name,
					Kind:   protocol.CompletionItemKindClass,
					Detail: "type " + n.Name.Name,
				})
			case *ir.EnumDeclStmt:
				items = append(items, protocol.CompletionItem{
					Label:  n.Name.Name,
					Kind:   protocol.CompletionItemKindEnum,
					Detail: "enum " + n.Name.Name,
				})
			case *ir.InterfaceDeclStmt:
				items = append(items, protocol.CompletionItem{
					Label:  n.Name.Name,
					Kind:   protocol.CompletionItemKindInterface,
					Detail: "interface " + n.Name.Name,
				})
			case *ir.MixinDeclStmt:
				items = append(items, protocol.CompletionItem{
					Label:  n.Name.Name,
					Kind:   protocol.CompletionItemKindClass,
					Detail: "mixin " + n.Name.Name,
				})
			case *ir.ImportStmt:
				items = append(items, protocol.CompletionItem{
					Label:  n.Alias,
					Kind:   protocol.CompletionItemKindModule,
					Detail: "import " + n.Path.String(),
				})
			}
		}
	}

	for _, b := range builtinIdentifiers(c, pkg) {
		items = append(items, b)
	}

	sort.Slice(items, func(i, j int) bool { return items[i].Label < items[j].Label })
	return dedupeCompletionItems(items)
}

func memberCompletions(c *compiler.CompilerContext, file *ir.File, pos protocol.Position, recvText string) []protocol.CompletionItem {
	if recvText == "" {
		return nil
	}

	consumerSide := currentFileSide(file)
	if recvText == "@" {
		if sessionTyp, ok := c.Cache[compiler.SessionsSessionTypeCacheKey].(ir.TypID); ok && sessionTyp != 0 {
			return typeMemberCompletions(c, sessionTyp, consumerSide)
		}

		return nil
	}

	if recvText == "this" {
		cursor := position{line: int(pos.Line) + 1, col: int(pos.Character)}

		if typ := findEnclosingThisType(c, file, cursor); typ != 0 {
			return typeMemberCompletions(c, typ, consumerSide)
		}

		return nil
	}

	if recvText == "()" {
		dotLine := int(pos.Line) + 1
		dotCol := int(pos.Character)
		if typ := findExprTypeEndingAt(file, dotLine, dotCol); typ != 0 {
			return typeMemberCompletions(c, typ, consumerSide)
		}

		return nil
	}

	if file != nil {
		for _, st := range file.Statements {
			imp, ok := st.(*ir.ImportStmt)
			if !ok {
				continue
			}

			if imp.Alias != recvText {
				continue
			}

			target := lookupPackageByImportPath(c, imp.Path.String())
			if target == nil {
				return nil
			}

			return packageMembers(c, target, currentFileSide(file))
		}
	}

	cursor := position{line: int(pos.Line) + 1, col: int(pos.Character)}

	t := &cursorTarget{file: file}

	for _, st := range file.Statements {
		if walkStmt(t, st, cursor) {
			break
		}
	}

	if t.typ == 0 {

		if sym := findLocalSymByName(file, recvText); sym != 0 {
			s, _ := lookupSymbol(c, sym)
			if s != nil {
				t.typ = s.Typ
			}
		}
	}

	if t.typ == 0 {
		return nil
	}

	return typeMemberCompletions(c, t.typ, consumerSide)
}

func findExprTypeEndingAt(file *ir.File, line, col int) ir.TypID {
	if file == nil {
		return 0
	}

	var best ir.Expr
	visit := func(e ir.Expr) {
		if ir.IsNilExpr(e) {
			return
		}

		sp := e.Span()
		if sp.EndLn == line && sp.EndCol == col {
			best = e
		}
	}

	var walkE func(e ir.Expr)
	var walkS func(s ir.Stmt)
	walkE = func(e ir.Expr) {
		if ir.IsNilExpr(e) {
			return
		}

		visit(e)
		switch n := e.(type) {
		case *ir.FuncCallExpr:
			walkE(n.Callee)
			for _, a := range n.Args {
				walkE(a.Expr)
			}

		case *ir.FieldAccessExpr:
			walkE(n.Expr)
		case *ir.IndexExpr:
			walkE(n.Expr)
			walkE(n.Index)
		case *ir.BinaryExpr:
			walkE(n.Left)
			walkE(n.Right)
		case *ir.UnaryExpr:
			walkE(n.Expr)
		case *ir.PrefixUnaryExpr:
			walkE(n.Expr)
		case *ir.PostfixUnaryExpr:
			walkE(n.Expr)
		case *ir.GroupedExpr:
			walkE(n.Expr)
		case *ir.TenaryExpr:
			walkE(n.Cond)
			walkE(n.Then)
			walkE(n.Else)
		case *ir.AssignmentExpr:
			walkE(n.Right)
		case *ir.NewExpr:
			for _, a := range n.Args {
				walkE(a.Expr)
			}

		case *ir.CoalesceExpr:
			walkE(n.Left)
			walkE(n.Default)
		case *ir.AsExpr:
			walkE(n.Expr)
		}
	}

	walkS = func(s ir.Stmt) {
		if ir.IsNilStmt(s) {
			return
		}

		switch n := s.(type) {
		case *ir.BlockStmt:
			for _, ss := range n.Stmts {
				walkS(ss)
			}

		case *ir.VarDeclStmt:
			walkE(n.Init)
		case *ir.ExprStmt:
			walkE(n.Expr)
		case *ir.FieldAssignmentStmt:
			walkE(n.Value)
		case *ir.MultiAssignmentStmt:
			walkE(n.Value)
		case *ir.ReturnStmt:
			for _, r := range n.Results {
				walkE(r)
			}

		case *ir.IfStmt:
			walkE(n.Cond)
			for _, ss := range ir.BlockStmts(n.Then) {
				walkS(ss)
			}

			for _, eb := range n.ElseIfs {
				walkE(eb.Cond)
				for _, ss := range ir.BlockStmts(eb.Then) {
					walkS(ss)
				}
			}

			for _, ss := range ir.BlockStmts(n.Else) {
				walkS(ss)
			}

		case *ir.ForStmt:
			for _, ss := range ir.BlockStmts(n.Body) {
				walkS(ss)
			}

		case *ir.WhileStmt:
			walkE(n.Cond)
			for _, ss := range ir.BlockStmts(n.Body) {
				walkS(ss)
			}

		case *ir.FuncDeclStmt:
			for _, ss := range ir.BlockStmts(n.Body) {
				walkS(ss)
			}

		case *ir.TypeDeclStmt:
			for _, ctor := range n.Ctors {
				for _, ss := range ir.BlockStmts(ctor.Body) {
					walkS(ss)
				}
			}

			for _, m := range n.Methods {
				walkS(m.Func)
			}
		}
	}

	for _, st := range file.Statements {
		walkS(st)
	}

	if best == nil {
		return 0
	}

	return best.GetType()
}

func findLocalSymByName(file *ir.File, name string) ir.SymID {
	for _, st := range file.Statements {
		switch n := st.(type) {
		case *ir.VarDeclStmt:
			for _, tgt := range n.Targets {
				if tgt.Name != nil && tgt.Name.Name == name {
					return tgt.Name.Sym
				}
			}

		case *ir.FuncDeclStmt:
			if n.Name.Name == name {
				return n.Name.Sym
			}

			if hit := findLocalSymInBlock(n.Body, name); hit != 0 {
				return hit
			}
		}
	}

	return 0
}

func findLocalSymInBlock(b *ir.BlockStmt, name string) ir.SymID {
	if b == nil {
		return 0
	}

	for _, st := range b.Stmts {
		switch n := st.(type) {
		case *ir.VarDeclStmt:
			for _, tgt := range n.Targets {
				if tgt.Name != nil && tgt.Name.Name == name {
					return tgt.Name.Sym
				}
			}

		case *ir.IfStmt:
			if hit := findLocalSymInBlock(n.Then, name); hit != 0 {
				return hit
			}

			for _, eb := range n.ElseIfs {
				if hit := findLocalSymInBlock(eb.Then, name); hit != 0 {
					return hit
				}
			}

			if hit := findLocalSymInBlock(n.Else, name); hit != 0 {
				return hit
			}

		case *ir.ForStmt:
			if hit := findLocalSymInBlock(n.Body, name); hit != 0 {
				return hit
			}

		case *ir.WhileStmt:
			if hit := findLocalSymInBlock(n.Body, name); hit != 0 {
				return hit
			}
		}
	}

	return 0
}

func findEnclosingThisType(c *compiler.CompilerContext, file *ir.File, cursor position) ir.TypID {
	if file == nil {
		return 0
	}

	for _, st := range file.Statements {
		td, ok := st.(*ir.TypeDeclStmt)
		if !ok {
			continue
		}

		for _, m := range td.Methods {
			if m == nil || m.Func == nil || m.Func.Body == nil {
				continue
			}

			if cursor.inSpan(m.Func.Body.Span()) {
				return typeIDOfDecl(c, td)
			}
		}

		for _, ctor := range td.Ctors {
			if ctor == nil || ctor.Body == nil {
				continue
			}

			if cursor.inSpan(ctor.Body.Span()) {
				return typeIDOfDecl(c, td)
			}
		}
	}

	return 0
}

func typeIDOfDecl(c *compiler.CompilerContext, td *ir.TypeDeclStmt) ir.TypID {
	if c == nil || td == nil || td.Name.Sym == 0 {
		return 0
	}

	sym, _ := lookupSymbol(c, td.Name.Sym)
	if sym == nil {
		return 0
	}

	return sym.Typ
}

func structTypeDeclSide(c *compiler.CompilerContext, ty *ir.Type) ir.SideKind {
	if c == nil || ty == nil || ty.Kind != ir.TK_Struct {
		return ir.SideShared
	}

	for _, pkg := range c.Packages {
		if pkg == nil || pkg.Path.String() != ty.PackagePath {
			continue
		}

		for _, f := range pkg.Files {
			if f == nil || f.Hir == nil {
				continue
			}

			for _, st := range f.Hir.Statements {
				td, ok := st.(*ir.TypeDeclStmt)
				if !ok || td.IsExtern || td.Name.Sym == 0 {
					continue
				}

				sym, ok := pkg.Syms.GetByID(td.Name.Sym)
				if !ok {
					continue
				}

				if structType, ok := c.TypeUniverse.GetByID(sym.Typ); ok && structType == ty {
					return f.Hir.Side.Kind
				}
			}
		}
	}

	return ir.SideShared
}

func typeMemberCompletions(c *compiler.CompilerContext, typ ir.TypID, consumerSide ir.SideKind) []protocol.CompletionItem {
	ty, ok := c.TypeUniverse.GetByID(typ)
	if !ok {
		return nil
	}

	declSide := structTypeDeclSide(c, ty)
	filterShared := consumerSide != ir.SideShared && declSide != ir.SideShared && consumerSide != declSide
	var out []protocol.CompletionItem
	switch ty.Kind {
	case ir.TK_Struct:
		for _, f := range ty.StructFields {
			if filterShared && !f.IsShared {
				continue
			}

			out = append(out, protocol.CompletionItem{
				Label:  f.Name,
				Kind:   protocol.CompletionItemKindField,
				Detail: f.Name + ": " + formatType(c.TypeUniverse, f.Type),
			})
		}

		for _, m := range ty.StructMethods {
			if filterShared && !m.IsShared {
				continue
			}

			label := m.Name
			detail := label
			if fnTy, ok := c.TypeUniverse.GetByID(m.FuncTyp); ok {
				detail = "func " + label + "(" + funcTypeParamList(c.TypeUniverse, fnTy) + ")"
				if fnTy.ReturnType != 0 {
					detail += ": " + formatType(c.TypeUniverse, fnTy.ReturnType)
				}
			}

			item := protocol.CompletionItem{
				Label:  label,
				Kind:   protocol.CompletionItemKindMethod,
				Detail: detail,
			}

			if m.Sym != 0 {
				if sym, _ := lookupSymbol(c, m.Sym); sym != nil {
					attachDocToCompletionItem(&item, sym.Doc)
				}
			}

			out = append(out, item)
		}

	case ir.TK_Enum:
		for _, ec := range ty.EnumCases {
			out = append(out, protocol.CompletionItem{
				Label:  ec.Name,
				Kind:   protocol.CompletionItemKindEnumMember,
				Detail: ty.EnumName + "." + ec.Name,
			})
		}

		for _, m := range ty.EnumMethods {
			out = append(out, protocol.CompletionItem{
				Label:  m.Name,
				Kind:   protocol.CompletionItemKindMethod,
				Detail: "func " + m.Name,
			})
		}

	case ir.TK_Interface:
		for _, m := range ty.InterfaceMethods {
			detail := "func " + m.Name
			if fnTy, ok := c.TypeUniverse.GetByID(m.FuncTyp); ok {
				detail += "(" + funcTypeParamList(c.TypeUniverse, fnTy) + ")"
				if fnTy.ReturnType != 0 {
					detail += ": " + formatType(c.TypeUniverse, fnTy.ReturnType)
				}
			}

			out = append(out, protocol.CompletionItem{
				Label:  m.Name,
				Kind:   protocol.CompletionItemKindMethod,
				Detail: detail,
			})
		}

	case ir.TK_Chan:
		out = append(out, protocol.CompletionItem{Label: "send", Kind: protocol.CompletionItemKindMethod, Detail: "func send(v: " + formatType(c.TypeUniverse, ty.ElemType) + ")"})
		out = append(out, protocol.CompletionItem{Label: "recv", Kind: protocol.CompletionItemKindMethod, Detail: "func recv(): (" + formatType(c.TypeUniverse, ty.ElemType) + ", bool)"})
		out = append(out, protocol.CompletionItem{Label: "close", Kind: protocol.CompletionItemKindMethod, Detail: "func close()"})
	}

	return out
}

func packageMembers(c *compiler.CompilerContext, pkg *ir.PackageContext, consumerSide ir.SideKind) []protocol.CompletionItem {
	var out []protocol.CompletionItem
	hasFiles := false
	for _, f := range pkg.Files {
		if f.Hir == nil {
			continue
		}

		hasFiles = true
		for _, st := range f.Hir.Statements {
			switch n := st.(type) {
			case *ir.FuncDeclStmt:
				if isPackagePrivateName(n.Name.Name) {
					continue
				}

				if !funcVisibleFromSide(n, f.Hir, consumerSide) {
					continue
				}

				out = append(out, funcCompletion(c, n))
			case *ir.VarDeclStmt:
				out = append(out, filterPrivateVarCompletions(varCompletions(c, n))...)
			case *ir.TypeDeclStmt:
				if isPackagePrivateName(n.Name.Name) {
					continue
				}

				out = append(out, protocol.CompletionItem{
					Label:  n.Name.Name,
					Kind:   protocol.CompletionItemKindClass,
					Detail: "type " + n.Name.Name,
				})
			case *ir.EnumDeclStmt:
				if isPackagePrivateName(n.Name.Name) {
					continue
				}

				out = append(out, protocol.CompletionItem{
					Label:  n.Name.Name,
					Kind:   protocol.CompletionItemKindEnum,
					Detail: "enum " + n.Name.Name,
				})
			case *ir.InterfaceDeclStmt:
				if isPackagePrivateName(n.Name.Name) {
					continue
				}

				out = append(out, protocol.CompletionItem{
					Label:  n.Name.Name,
					Kind:   protocol.CompletionItemKindInterface,
					Detail: "interface " + n.Name.Name,
				})
			case *ir.ExternDeclStmt:
				out = append(out, externDeclCompletions(c, n)...)
			}
		}
	}

	if !hasFiles {
		out = append(out, syntheticPackageMembers(c, pkg)...)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return dedupeCompletionItems(out)
}

func isPackagePrivateName(name string) bool {
	return name != "" && name[0] == '_'
}

func effectiveFuncSide(fn *ir.FuncDeclStmt, declFile *ir.File) ir.SideKind {
	if fn != nil && fn.Side != nil {
		return fn.Side.Kind
	}

	if declFile != nil {
		return declFile.Side.Kind
	}

	return ir.SideShared
}

func funcVisibleFromSide(fn *ir.FuncDeclStmt, declFile *ir.File, currentSide ir.SideKind) bool {
	if fn == nil {
		return true
	}

	if fn.IsWired {
		return true
	}

	fs := effectiveFuncSide(fn, declFile)
	if fs == ir.SideShared {
		return true
	}

	if currentSide == ir.SideShared {
		return fs == ir.SideShared
	}

	return fs == currentSide
}

func currentFileSide(file *ir.File) ir.SideKind {
	if file == nil {
		return ir.SideShared
	}

	return file.Side.Kind
}

func filterPrivateVarCompletions(items []protocol.CompletionItem) []protocol.CompletionItem {
	out := items[:0]
	for _, it := range items {
		if isPackagePrivateName(it.Label) {
			continue
		}

		out = append(out, it)
	}

	return out
}

func externDeclCompletions(c *compiler.CompilerContext, ext *ir.ExternDeclStmt) []protocol.CompletionItem {
	var out []protocol.CompletionItem
	for _, fn := range ext.Funcs {
		if isPackagePrivateName(fn.Name.Name) {
			continue
		}

		parts := make([]string, len(fn.Params))
		for i, p := range fn.Params {
			label := p.Name.Name
			if p.Type != nil {
				label += ": " + formatTypeFromRef(p.Type)
			}

			parts[i] = label
		}

		detail := "func " + fn.Name.Name + "(" + strings.Join(parts, ", ") + ")"
		if fn.ReturnType != nil {
			detail += ": " + formatTypeFromRef(fn.ReturnType)
		}

		item := protocol.CompletionItem{
			Label:  fn.Name.Name,
			Kind:   protocol.CompletionItemKindFunction,
			Detail: detail,
		}

		attachDocToCompletionItem(&item, fn.GetDoc())
		out = append(out, item)
	}

	for _, v := range ext.Vars {
		if isPackagePrivateName(v.Name.Name) {
			continue
		}

		kind := protocol.CompletionItemKindVariable
		keyword := "let"
		if v.IsConst {
			kind = protocol.CompletionItemKindConstant
			keyword = "const"
		}

		detail := keyword + " " + v.Name.Name
		if v.Type != nil {
			detail += ": " + formatTypeFromRef(v.Type)
		}

		item := protocol.CompletionItem{
			Label:  v.Name.Name,
			Kind:   kind,
			Detail: detail,
		}

		attachDocToCompletionItem(&item, v.GetDoc())
		out = append(out, item)
	}

	return out
}

func syntheticPackageMembers(c *compiler.CompilerContext, pkg *ir.PackageContext) []protocol.CompletionItem {
	var out []protocol.CompletionItem
	if pkg == nil || pkg.Syms == nil {
		return out
	}

	for _, sym := range pkg.Syms.ByID() {
		if sym == nil || sym.Owner != pkg.Root {
			continue
		}

		item := protocol.CompletionItem{Label: sym.Name}

		if sym.Typ != 0 {
			if ty, ok := c.TypeUniverse.GetByID(sym.Typ); ok {
				switch ty.Kind {
				case ir.TK_Function:
					item.Kind = protocol.CompletionItemKindFunction
					item.Detail = formatType(c.TypeUniverse, sym.Typ)
				case ir.TK_Struct, ir.TK_Enum, ir.TK_Interface:
					item.Kind = protocol.CompletionItemKindClass
					item.Detail = formatType(c.TypeUniverse, sym.Typ)
				default:
					item.Kind = protocol.CompletionItemKindVariable
					item.Detail = formatType(c.TypeUniverse, sym.Typ)
				}
			}
		}

		if item.Kind == 0 {
			item.Kind = protocol.CompletionItemKindVariable
		}

		attachDocToCompletionItem(&item, sym.Doc)
		out = append(out, item)
	}

	return out
}

func builtinIdentifiers(c *compiler.CompilerContext, pkg *ir.PackageContext) []protocol.CompletionItem {
	var out []protocol.CompletionItem
	if pkg == nil {
		return out
	}

	raw, ok := c.Cache["builtin_intrinsics"]
	if !ok {
		return out
	}

	intrinsics, ok := raw.(map[ir.SymID]string)
	if !ok {
		return out
	}

	for symID, name := range intrinsics {
		sym, ok := pkg.Syms.GetByID(symID)
		if !ok {
			continue
		}

		detail := name
		if fnTy, ok := c.TypeUniverse.GetByID(sym.Typ); ok && fnTy.Kind == ir.TK_Function {
			detail = "func " + name + "(" + funcTypeParamList(c.TypeUniverse, fnTy) + ")"
			if fnTy.ReturnType != 0 {
				detail += ": " + formatType(c.TypeUniverse, fnTy.ReturnType)
			}
		}

		out = append(out, protocol.CompletionItem{
			Label:  name,
			Kind:   protocol.CompletionItemKindFunction,
			Detail: detail,
		})
	}

	return out
}

func funcCompletion(c *compiler.CompilerContext, fn *ir.FuncDeclStmt) protocol.CompletionItem {
	parts := make([]string, len(fn.Params))
	for i, p := range fn.Params {
		label := p.Name.Name
		if p.Type != nil {
			label += ": " + formatTypeFromRef(p.Type)
		}

		parts[i] = label
	}

	detail := "func " + fn.Name.Name + "(" + strings.Join(parts, ", ") + ")"
	if fn.ReturnType != nil {
		detail += ": " + formatTypeFromRef(fn.ReturnType)
	}

	_ = c
	item := protocol.CompletionItem{
		Label:  fn.Name.Name,
		Kind:   protocol.CompletionItemKindFunction,
		Detail: detail,
	}

	attachDocToCompletionItem(&item, fn.GetDoc())
	return item
}

func varCompletions(c *compiler.CompilerContext, vd *ir.VarDeclStmt) []protocol.CompletionItem {
	var out []protocol.CompletionItem
	for _, tgt := range vd.Targets {
		if tgt.Name == nil {
			continue
		}

		kind := protocol.CompletionItemKindVariable
		keyword := "let"
		if vd.IsConst {
			kind = protocol.CompletionItemKindConstant
			keyword = "const"
		}

		detail := keyword + " " + tgt.Name.Name
		if tgt.TypeAnn != nil && tgt.TypeAnn.Typ != 0 {
			detail += ": " + formatType(c.TypeUniverse, tgt.TypeAnn.Typ)
		}

		item := protocol.CompletionItem{
			Label:  tgt.Name.Name,
			Kind:   kind,
			Detail: detail,
		}

		attachDocToCompletionItem(&item, vd.GetDoc())
		out = append(out, item)
	}

	return out
}

func attachDocToCompletionItem(item *protocol.CompletionItem, doc string) {
	doc = strings.TrimSpace(doc)
	if doc == "" {
		return
	}

	item.Documentation = protocol.MarkupContent{
		Kind:  protocol.Markdown,
		Value: renderDocComment(doc),
	}
}

func formatTypeFromRef(tr *ir.TypeRef) string {
	if tr == nil {
		return ""
	}

	if tr.Typ != 0 {

		_ = tr.Typ
	}

	switch tr.Kind {
	case ir.TK_PrimitiveAny:
		return "any"
	case ir.TK_PrimitiveNone:
		return "none"
	case ir.TK_PrimitiveInt:
		return "int"
	case ir.TK_PrimitiveFloat:
		return "float"
	case ir.TK_PrimitiveBool:
		return "bool"
	case ir.TK_PrimitiveString:
		return "string"
	case ir.TK_PrimitiveChar:
		return "char"
	}

	if tr.CustomName != "" {
		if tr.CustomQualifier != "" {
			return tr.CustomQualifier + "." + tr.CustomName
		}

		return tr.CustomName
	}

	return ""
}

func funcTypeParamList(tt *ir.TypeTable, fn *ir.Type) string {
	parts := make([]string, len(fn.ParamTypes))
	for i, p := range fn.ParamTypes {
		label := ""
		if p.Name.Name != "" {
			label = p.Name.Name + ": "
		}

		if p.Type != nil && p.Type.Typ != 0 {
			label += formatType(tt, p.Type.Typ)
		}

		parts[i] = label
	}

	return strings.Join(parts, ", ")
}

func lookupPackageByImportPath(c *compiler.CompilerContext, path string) *ir.PackageContext {
	if pkg, ok := c.Packages[path]; ok {
		return pkg
	}

	return nil
}

func dedupeCompletionItems(items []protocol.CompletionItem) []protocol.CompletionItem {
	type key struct {
		label string
		kind  protocol.CompletionItemKind
	}

	seen := map[key]bool{}

	out := make([]protocol.CompletionItem, 0, len(items))
	for _, it := range items {
		k := key{label: it.Label, kind: it.Kind}

		if seen[k] {
			continue
		}

		seen[k] = true
		out = append(out, it)
	}

	return out
}

var sovaKeywords = []string{
	"let", "const", "func", "type", "enum", "interface", "mixin", "extern",
	"if", "else", "for", "while", "return", "guard", "break", "continue",
	"new", "this", "when", "in", "step",
	"true", "false", "none",
	"async", "go", "defer", "select", "case", "default",
	"import", "package", "on", "shared", "frontend", "backend", "test",
	"wire", "ruleset", "assert", "implements", "with", "private",
}
