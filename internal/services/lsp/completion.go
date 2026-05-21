package lsp

import (
	"context"
	"sort"
	"strings"

	"go.lsp.dev/protocol"

	"sova/internal/diag"
	"sova/internal/ir"
	"sova/internal/services/compiler"
)

// Completion returns the symbols/keywords that fit at the cursor. Two contexts matter for v1:
//
//   - **After `.`** - the cursor is on a member access (or about to be). We resolve the receiver's type and list its members: struct fields/methods, enum cases/methods, interface methods, chan methods (send/recv/close), or, when the receiver is a package alias, the package's exported decls.
//   - **Identifier prefix** - anywhere else. We list every in-scope name (current file's top-level decls + builtins + imported package aliases) plus a small set of Sova keywords. Editors filter the returned list by the prefix the user has typed.
//
// We return `IsIncomplete: true` only when we can't enumerate everything - today that never happens since we always have the full HIR; the field stays false so editors cache results.
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
	default:
		items = identifierCompletions(c, pkg, file)
		items = append(items, localScopeCompletions(c, file, params.Position)...)
	}
	applyWordReplaceRange(items, src, params.Position, ctxKind)
	return &protocol.CompletionList{Items: items, IsIncomplete: false}, nil
}

// applyWordReplaceRange attaches a TextEdit to every completion item whose
// range covers the identifier token the cursor sits inside. Without this the
// editor's default behaviour is to *insert* at the cursor - so triggering
// completion in the middle of a word (e.g. cursor between `pri` and `nt` of
// `print`) and selecting `println` would splice it in to produce
// `priprintlnnt`. By replacing the full word the editor consistently produces
// `println` regardless of where in the word the cursor was. The range is
// derived per LSP position (UTF-16 columns), and we deliberately skip it for
// import-path completions where the surrounding token is a string literal
// rather than an identifier.
func applyWordReplaceRange(items []protocol.CompletionItem, src string, pos protocol.Position, kind completionContextKind) {
	if len(items) == 0 {
		return
	}
	if kind == completionImportPath || kind == completionWireOption {
		return
	}
	offset := lspPositionToOffset(src, pos)
	wordStart := offset
	for wordStart > 0 && isIdentChar(src[wordStart-1]) {
		wordStart--
	}
	wordEnd := offset
	for wordEnd < len(src) && isIdentChar(src[wordEnd]) {
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

// offsetToLSPPosition is the inverse of lspPositionToOffset: converts a byte
// offset back into a 0-based (line, character) pair. Matches the editor's
// UTF-16-code-unit column counting for ASCII Sova source.
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
)

// classifyCompletion peeks at the raw source just before the cursor and decides whether the user is partway through a member access (`recv.<TAB>`), typing a standalone identifier, or inside an `import "..."` string literal. Returns the receiver text when in dot context.
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
	end := offset
	for end > 0 && isIdentChar(src[end-1]) {
		end--
	}
	if end == 0 || src[end-1] != '.' {
		return completionIdentifier, ""
	}
	dotEnd := end - 1
	recvEnd := dotEnd
	if recvEnd > 0 && src[recvEnd-1] == '@' {
		return completionAfterDot, "@"
	}
	// Chain case: the receiver ends in a `)` (e.g. `foo().bar()` or
	// `pkg.create()`). We can't represent the whole receiver as a single
	// identifier, so we return a sentinel marker; memberCompletions falls
	// back to a HIR scan to find the call expression that ends at this dot
	// and uses its return type to drive completion.
	if recvEnd > 0 && src[recvEnd-1] == ')' {
		return completionAfterDot, "()"
	}
	recvStart := recvEnd
	for recvStart > 0 && isIdentChar(src[recvStart-1]) {
		recvStart--
	}
	return completionAfterDot, src[recvStart:recvEnd]
}

// wireOptionCompletions returns the known option keys accepted by Sova's
// `wire(...)` clause, each annotated with a snippet template (`key: $0`) and
// markdown documentation. The list mirrors the keys analyze_wire understands;
// keep it in sync whenever a new option is added.
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

// isInsideWireOptions reports whether `offset` sits inside the option list
// of a `wire(...)` clause - i.e., between the opening `(` of `wire(` and its
// closing `)`. Walks backward from the cursor over balanced parens/brackets,
// stops at the first unmatched `(`, and checks the identifier immediately
// preceding it is `wire`. When true, completion offers the known option keys
// (`authn`, `authz`, `transport`, etc.) instead of generic identifiers.
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
				// Identifier immediately before this `(` decides the context.
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

// isInsideImportString reports whether `offset` sits between the two quotes of
// an `import "..."` literal on the same line. Walks backwards from the cursor:
// finds an unmatched `"`, then checks that the preceding non-whitespace tokens
// on that line start with `import`. Keeps the check single-line so we never
// confuse multi-line constructs with a string body.
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

func isIdentChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// lspPositionToOffset converts a 0-based LSP `(line, character)` to a byte offset into the source. Mirrors the editor's idea of column counts as Unicode-code-unit increments; for ASCII Sova source the byte offset matches.
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

// identifierCompletions gathers in-scope names for "type-ahead" suggestions. Order: keywords first, then locally-declared symbols, then imported package aliases. Each item carries a Detail (rendered type) so editors render rich previews.
// localScopeCompletions returns completions for symbols introduced inside the
// function / method / ctor whose body contains the cursor: the function's
// parameters plus every `let`/`const` declared in a block on the cursor's
// lexical path that lexically precedes the cursor. Sibling-block declarations
// (e.g. variables inside an `if` branch the cursor is not in) are skipped so
// shadowing rules stay intuitive. Falls through silently when the cursor
// isn't inside any function body - identifierCompletions already covers the
// top-level + builtins for that case.
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

// findEnclosingFunc walks `file` looking for the innermost FuncDeclStmt /
// method / ctor whose body's span contains `cursor`. Returned value is the
// function shape we can read params + body from. Returns nil when the cursor
// is at file scope.
func findEnclosingFunc(file *ir.File, cursor position) *funcShape {
	var found *funcShape
	for _, st := range file.Statements {
		walkForEnclosingFunc(st, cursor, &found)
	}
	return found
}

// funcShape unifies the relevant subset of FuncDeclStmt / TypeMethod / Ctor
// so localScopeCompletions can handle the three function flavours uniformly.
// Only params + body are needed for completion purposes.
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

// collectLocalDecls walks the body block and appends a completion item for
// every `let`/`const` whose declaration lexically precedes the cursor. The
// walker recurses only into nested blocks that themselves contain the cursor
// (so shadowing is approximated: sibling blocks don't bleed their decls
// in), and tracks scopes implicitly via the recursion shape.
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

// cursorAfter reports whether the cursor lies strictly past the end of
// `span` - used to skip declarations that haven't yet been written when the
// completion popup fires.
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

// cursorBefore reports whether the cursor lies strictly before the start of
// `span` - used to terminate the lexical-order walk once we've passed every
// declaration that could be in scope.
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

// memberCompletions resolves the receiver of a dot-access in the source (`receiver.<TAB>`) and returns the appropriate member list - struct fields/methods, enum cases, interface methods, chan methods, package members, or the synthetic session methods exposed by the `@` shorthand inside wired backend functions.
func memberCompletions(c *compiler.CompilerContext, file *ir.File, pos protocol.Position, recvText string) []protocol.CompletionItem {
	if recvText == "" {
		return nil
	}
	if recvText == "@" {
		if sessionTyp, ok := c.Cache[compiler.SessionsSessionTypeCacheKey].(ir.TypID); ok && sessionTyp != 0 {
			return typeMemberCompletions(c, sessionTyp)
		}
		return nil
	}
	if recvText == "()" {
		dotLine := int(pos.Line) + 1
		dotCol := int(pos.Character)
		if typ := findExprTypeEndingAt(file, dotLine, dotCol); typ != 0 {
			return typeMemberCompletions(c, typ)
		}
		return nil
	}
	// First: treat the receiver as a package alias. If we find an import with this alias, list its package's exported decls.
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
	// Otherwise resolve via cursor target (cursor sits just past the `.`).
	cursor := position{line: int(pos.Line) + 1, col: int(pos.Character)}
	t := &cursorTarget{file: file}
	for _, st := range file.Statements {
		if walkStmt(t, st, cursor) {
			break
		}
	}
	if t.typ == 0 {
		// Fall back to walking the HIR for a VarRef matching `recvText` - handles "fresh char typed after dot" cases.
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
	return typeMemberCompletions(c, t.typ)
}

// findExprTypeEndingAt walks the file's HIR looking for the deepest expression
// whose end-position lands immediately before the dot at (line, col). Used by
// method-chain completion: source like `browser.doc().<cursor>` has the call
// `browser.doc()` parsed even if the trailing dot itself errored, and the
// expression's recorded span lets us recover the call's return type and drive
// the member list off of it.
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

// findLocalSymByName scans the file for any declaration whose Name.Name matches `name` and returns its SymID. Cheap fall-back used when the cursor lookup misses (e.g. user typed `recv.` so the dot isn't yet attached to any HIR node).
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

// typeMemberCompletions emits the appropriate member list for a value of `typ`. Struct → fields + methods; enum → cases + methods; interface → methods; chan → send/recv/close.
func typeMemberCompletions(c *compiler.CompilerContext, typ ir.TypID) []protocol.CompletionItem {
	ty, ok := c.TypeUniverse.GetByID(typ)
	if !ok {
		return nil
	}
	var out []protocol.CompletionItem
	switch ty.Kind {
	case ir.TK_Struct:
		for _, f := range ty.StructFields {
			out = append(out, protocol.CompletionItem{
				Label:  f.Name,
				Kind:   protocol.CompletionItemKindField,
				Detail: f.Name + ": " + formatType(c.TypeUniverse, f.Type),
			})
		}
		for _, m := range ty.StructMethods {
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

// packageMembers returns one CompletionItem per exported (top-level) declaration in `pkg` callable from a file on `consumerSide`. Synthetic packages (e.g. the built-in `sessions` package) have no Files; we fall through to the package's root scope and emit a completion item per declared symbol so `sessions.<TAB>` lists `Session`, `Broadcast`, `all`, `broadcast`, etc. The list is sorted alphabetically by label. Functions whose effective side disagrees with `consumerSide` (and aren't wired or shared) are dropped so a frontend caller never sees backend-only routines.
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

// isPackagePrivateName reports whether a symbol should be hidden from
// cross-package completion. Sova follows the convention that identifiers
// starting with `_` are internal to the declaring package and aren't part of
// its surface - `std/browser._h`, `std/sync._newMutex`, etc. The check is
// applied at package-member completion time so `pkg.<TAB>` doesn't expose
// the package's private wiring.
func isPackagePrivateName(name string) bool {
	return name != "" && name[0] == '_'
}

// effectiveFuncSide returns the host-side a function declaration is bound to: its explicit `: backend|frontend|shared` annotation when present, otherwise the declaring file's `on backend|frontend|shared` header.
func effectiveFuncSide(fn *ir.FuncDeclStmt, declFile *ir.File) ir.SideKind {
	if fn != nil && fn.Side != nil {
		return fn.Side.Kind
	}
	if declFile != nil {
		return declFile.Side.Kind
	}
	return ir.SideShared
}

// funcVisibleFromSide reports whether a function should appear in completion / surface lists when the user is editing a file on `currentSide`. Wire-flagged functions are always visible (they intentionally cross the boundary). Otherwise a function whose effective side matches `currentSide` or is `shared` shows up; anything else (frontend-only seen from backend, etc.) is hidden so the suggestion list reflects what is actually callable from this side.
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

// currentFileSide returns the side of `file` (the file the user is editing). Defaults to `SideShared` when `file` is nil so callers can use the result directly as the "consumer side" without an extra nil check.
func currentFileSide(file *ir.File) ir.SideKind {
	if file == nil {
		return ir.SideShared
	}
	return file.Side.Kind
}

// filterPrivateVarCompletions drops completion items whose Label starts with
// `_` from a slice of var completions. Used to apply the underscore-private
// convention to top-level `let`/`const` declarations and extern vars.
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

// externDeclCompletions surfaces every `extern` function and variable as a
// completion item on the enclosing package's surface. Treats `_`-prefixed
// names as package-private (matching the std-lib convention where helper
// shims like `_newMutex` are internal). Attaches doc comments via the
// per-extern-item `docBase` field so hover/completion stays in sync with
// the declared Sova surface rather than the native target.
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

// syntheticPackageMembers emits completion items for built-in packages that
// live entirely in the symbol arena (no source files). Iterates the package's
// symbol table and surfaces every entry whose Owner is the package's root
// scope, rendering it as a function / type / variable completion based on the
// resolved type. Drives `sessions.<TAB>` and similar built-in package lookups.
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

// builtinIdentifiers returns one CompletionItem per compiler-injected builtin (`print`, `len`, `error`, etc.). Reads from the same `builtin_intrinsics` cache key that codegen consults - single source of truth.
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

// attachDocToCompletionItem wraps the doc-comment text as markdown content on
// the CompletionItem so editors show it in the side panel as the user scrolls
// the list. We render via the same `renderDocComment` formatter the hover
// uses so the two surfaces stay visually consistent.
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
		// We may not have a TypeTable handy here - caller can pass one in if it does; otherwise approximate via the ref.
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

// sovaKeywords is the static set of reserved words we offer as completion items at identifier positions. Tracks the lexer's keyword tokens; updating one updates the other.
var sovaKeywords = []string{
	"let", "const", "func", "type", "enum", "interface", "mixin", "extern",
	"if", "else", "for", "while", "return", "guard", "break", "continue",
	"new", "this", "when", "in", "step",
	"true", "false", "none",
	"async", "go", "defer", "select", "case", "default",
	"import", "package", "on", "shared", "frontend", "backend", "test",
	"wire", "ruleset", "assert", "implements", "with", "private",
}
