package lsp

import (
	"sort"
	"strings"

	"go.lsp.dev/protocol"

	"sova/internal/diag"
	"sova/internal/ir"
	"sova/internal/passes"
	"sova/internal/services/compiler"
)

var sovaKeywords = []string{
	"let", "const", "func", "type", "enum", "interface", "mixin", "extern",
	"if", "else", "for", "while", "return", "guard", "break", "continue",
	"new", "this", "when", "in", "step",
	"true", "false", "none",
	"async", "go", "defer", "select", "case", "default",
	"import", "package", "on", "shared", "frontend", "backend", "test",
	"wire", "ruleset", "assert", "implements", "with", "private",
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
			doc:     "Overrides the auto-derived HTTP method for backend wires. The default is derived from the function name (`get*` -> GET, otherwise POST).",
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
			if fnTy.Func.ReturnType != 0 {
				detail += ": " + formatType(c.TypeUniverse, fnTy.Func.ReturnType)
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
