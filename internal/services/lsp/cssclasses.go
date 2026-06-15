package lsp

import (
	"os"
	"path/filepath"
	"sort"
	"sova/internal/cssclasses"
	"sova/internal/ir"
	"sova/internal/passes"
	"sova/internal/services/compiler"
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// projectCSSClasses returns the union of every class name found across every
// CSS / SCSS file the current build's `@embed` / `@StyleFile` resolved.
// Sources are deduplicated by their content hash so re-extracting the same
// file (the very common case where a project's CSS is referenced from many
// places) costs one parse, not N. The result is sorted alphabetically so
// the completion list is stable regardless of file-walk order.
//
// Implementation note: we read the resolver records out of the cache
// (`EmbedAssetsCacheKey`) rather than re-walking the IR. The resolver
// already absolute-resolved every path, classified text-vs-binary, and
// dropped invalid entries with diagnostics — we trust that exit point.
func projectCSSClasses(c *compiler.CompilerContext) []string {
	index := projectCSSClassIndex(c)
	if len(index) == 0 {
		return nil
	}
	out := make([]string, 0, len(index))
	for name := range index {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// classRef bundles one positional occurrence of a class name with the source file it lives in and the file's raw contents (so the hover handler can extract the matching rule on demand without re-reading from disk). Kept as a value type so the per-name slice in the index can be range-iterated without pointer indirection.
type classRef struct {
	cssclasses.ClassEntry
	File   string
	Source string
}

// projectCSSClassIndex returns a name → slice-of-occurrences map across every CSS / SCSS file the embed resolver discovered in the current build, plus every SCSS partial those files transitively `@use` or `@import`. Multiple records pointing at the same content hash share a single parsed entry list; multiple files importing the same partial parse the partial once. Hover and go-to-definition consume this index to render rule bodies and jump locations.
func projectCSSClassIndex(c *compiler.CompilerContext) map[string][]classRef {
	if c == nil {
		return nil
	}
	raw, ok := c.Cache[passes.EmbedAssetsCacheKey]
	if !ok {
		return nil
	}
	records, ok := raw.([]*passes.EmbedRecord)
	if !ok || len(records) == 0 {
		return nil
	}

	type fileEntry struct {
		path    string
		source  string
		entries []cssclasses.ClassEntry
	}
	byPath := map[string]fileEntry{}
	visit := func(path string) {}
	visit = func(path string) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return
		}
		if _, done := byPath[abs]; done {
			return
		}
		content, err := readEmbeddedSource(abs)
		if err != nil {
			byPath[abs] = fileEntry{}
			return
		}
		byPath[abs] = fileEntry{
			path:    abs,
			source:  content,
			entries: cssclasses.Extract(content),
		}
		baseDir := filepath.Dir(abs)
		for _, importRef := range cssclasses.Imports(content) {
			if partial := resolveSassPartial(baseDir, importRef); partial != "" {
				visit(partial)
			}
		}
	}

	for _, rec := range records {
		if rec == nil || rec.Info == nil {
			continue
		}
		if !isStylesheetPath(rec.Info.SourcePath) {
			continue
		}
		visit(rec.Info.SourcePath)
	}

	index := map[string][]classRef{}
	for _, fe := range byPath {
		if fe.path == "" {
			continue
		}
		for _, e := range fe.entries {
			index[e.Name] = append(index[e.Name], classRef{
				ClassEntry: e,
				File:       fe.path,
				Source:     fe.source,
			})
		}
	}
	return index
}

// resolveSassPartial probes the four standard Sass spellings (`name.scss`, `_name.scss`, `name.sass`, `_name.sass`) relative to the importing file's directory and returns the first one that exists on disk. Returns "" when none match — the LSP treats that as "the import doesn't resolve" and silently skips it rather than surfacing an error, because partial-resolution failures are noisier than they're worth (the user's build system, not the LSP, is authoritative on what a `@use` ultimately resolves to).
func resolveSassPartial(baseDir, importRef string) string {
	if filepath.IsAbs(importRef) {
		return ""
	}
	clean := strings.TrimSuffix(strings.TrimSuffix(importRef, ".scss"), ".sass")
	dir := filepath.Join(baseDir, filepath.FromSlash(filepath.Dir(clean)))
	base := filepath.Base(clean)
	candidates := []string{
		filepath.Join(dir, base+".scss"),
		filepath.Join(dir, "_"+base+".scss"),
		filepath.Join(dir, base+".sass"),
		filepath.Join(dir, "_"+base+".sass"),
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

// cssClassCompletions turns the project-wide class set into completion
// items. The label is the bare class name (`primary`, `btn-large`), the
// detail explains where it came from at a glance, and the insert text is
// the class name with no extra punctuation so the user can keep typing
// (multiple classes inside the same string literal stay one cursor click
// away). Kind is `Color` so it stands out visually next to identifiers
// and keywords in VS Code's completion popup.
//
// When `slot` is non-nil the cursor sits in a `@cssClass`-annotated
// parameter position (Phase 2's precise detection): items are tagged with
// the call's callee + param index in the detail string so users see why a
// suggestion is being offered, and the `SortText` is prefixed with `0`
// so the class names rank above any identifier-fallback noise the editor
// might mix in. When `slot` is nil the items get the unqualified "CSS
// class" detail (V1's broad fallback).
func cssClassCompletions(c *compiler.CompilerContext, slot *callContext) []protocol.CompletionItem {
	classes := projectCSSClasses(c)
	if len(classes) == 0 {
		return nil
	}
	detail := "CSS class"
	sortPrefix := ""
	if slot != nil {
		detail = "CSS class · " + slot.callee + " (arg #" + itoa(slot.argIndex+1) + ")"
		sortPrefix = "0"
	}
	out := make([]protocol.CompletionItem, 0, len(classes))
	for _, name := range classes {
		item := protocol.CompletionItem{
			Label:  name,
			Detail: detail,
			Kind:   protocol.CompletionItemKindColor,
		}
		if sortPrefix != "" {
			item.SortText = sortPrefix + name
		}
		out = append(out, item)
	}
	return out
}

// classNameAtCursor returns the class-like token the cursor sits inside, plus its byte range in the source, when both (a) the cursor is inside a single-line string literal and (b) the token immediately around the cursor is a valid CSS class identifier (the same identifier shape `cssclasses.Extract` accepts). When either condition fails, ok=false. Used by hover and go-to-definition to identify which class the user is pointing at — the existence check against the project index happens at the caller, so this helper is purely textual.
func classNameAtCursor(src string, offset int) (string, int, int, bool) {
	if _, ok := openingQuoteOfEnclosingString(src, offset); !ok {
		return "", 0, 0, false
	}
	start := offset
	for start > 0 && isClassIdentByte(src[start-1]) {
		start--
	}
	end := offset
	for end < len(src) && isClassIdentByte(src[end]) {
		end++
	}
	if start == end {
		return "", 0, 0, false
	}
	name := src[start:end]
	if name == "" || !isClassNameValid(name) {
		return "", 0, 0, false
	}
	return name, start, end, true
}

func isClassIdentByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_' || b == '-'
}

// isClassNameValid mirrors `readClassName` in the extractor — accepts an optional leading `-` followed by an identifier head and identifier tail characters. Filters out leading digits (`5rem`) and empty strings so hover-vs-not is consistent with what the extractor would have captured as a class name.
func isClassNameValid(name string) bool {
	if name == "" {
		return false
	}
	i := 0
	if name[0] == '-' {
		i = 1
	}
	if i >= len(name) {
		return false
	}
	first := name[i]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || first == '_') {
		return false
	}
	return true
}

// cssClassHover returns a markdown hover popup describing the class name the cursor sits on, when (a) the cursor is inside a single-line string literal and (b) the token under the cursor matches an entry in the project's CSS / SCSS class index. Returns nil when either condition fails — the regular hover path then takes over and the user sees the usual symbol-based hover. When multiple files define the same class (a common pattern with global utility CSS plus per-component overrides) only the first occurrence is shown in the popup; the doc body mentions the total count so the user knows there's more.
func cssClassHover(c *compiler.CompilerContext, src string, pos protocol.Position) *protocol.Hover {
	offset := lspPositionToOffset(src, pos)
	name, start, end, ok := classNameAtCursor(src, offset)
	if !ok {
		return nil
	}
	index := projectCSSClassIndex(c)
	refs, ok := index[name]
	if !ok || len(refs) == 0 {
		return nil
	}
	ref := refs[0]
	rule := cssclasses.RuleAt(ref.Source, ref.Offset)
	body := "**." + name + "**"
	if rel := filepath.Base(ref.File); rel != "" {
		body += " · `" + rel + "`"
	}
	if len(refs) > 1 {
		body += " · " + itoa(len(refs)) + " occurrences"
	}
	if rule != "" {
		body += "\n\n```css\n" + rule + "\n```"
	}
	startPos := offsetToLSPPosition(src, start)
	endPos := offsetToLSPPosition(src, end)
	rng := protocol.Range{Start: startPos, End: endPos}
	return &protocol.Hover{
		Contents: protocol.MarkupContent{Kind: protocol.Markdown, Value: body},
		Range:    &rng,
	}
}

// cssClassDefinition returns the source location of the first occurrence of the class name the cursor sits on, when the cursor is inside a string literal and the token matches a known class. Used by go-to-definition. When multiple occurrences exist, V1 of P3 returns just the first — supporting "list all occurrences" properly is a Phase 4 (or "find all references")-style follow-up.
func cssClassDefinition(c *compiler.CompilerContext, src string, pos protocol.Position) []protocol.Location {
	offset := lspPositionToOffset(src, pos)
	name, _, _, ok := classNameAtCursor(src, offset)
	if !ok {
		return nil
	}
	index := projectCSSClassIndex(c)
	refs, ok := index[name]
	if !ok || len(refs) == 0 {
		return nil
	}
	out := make([]protocol.Location, 0, len(refs))
	for _, ref := range refs {
		// Compute end-of-selector range: same start, plus the length of `.<name>`.
		startCol := uint32(ref.Char)
		endCol := startCol + uint32(len(ref.Name)) + 1 // +1 for the leading `.`
		out = append(out, protocol.Location{
			URI: uri.URI("file://" + filepath.ToSlash(ref.File)),
			Range: protocol.Range{
				Start: protocol.Position{Line: uint32(ref.Line), Character: startCol},
				End:   protocol.Position{Line: uint32(ref.Line), Character: endCol},
			},
		})
	}
	return out
}

// cssClassDiagnostics returns a slice of warnings for string-literal arguments passed at `@cssClass`-marked parameter positions whose value does not match any class in the project's index. Only fires when the file has a non-empty class index (so projects without any CSS embeds never see spurious warnings), and only when the argument is a pure string literal — computed strings (`prefix + cls`, template literals with interpolation) are silently skipped because the LSP can't statically verify them.
//
// Multi-class strings like `"primary large"` are split on whitespace and each token is checked individually; the warning span tightens to just the unknown token within the literal so the editor highlights exactly what's wrong.
func cssClassDiagnostics(c *compiler.CompilerContext, f *ir.File) []protocol.Diagnostic {
	if f == nil {
		return nil
	}
	index := projectCSSClassIndex(c)
	if len(index) == 0 {
		return nil
	}
	known := map[string]struct{}{}
	for name := range index {
		known[name] = struct{}{}
	}
	var out []protocol.Diagnostic
	for _, st := range f.Statements {
		walkStmtForClassChecks(c, st, known, &out)
	}
	return out
}

// walkStmtForClassChecks descends into a statement looking for function calls that pass string literals to `@cssClass`-marked params. Coverage is the subset of statement kinds that can house an expression in V1 — top-level expression statements, var decls, function/method bodies. New stmt kinds get added here as needed; the walker is deliberately conservative because a missed call site just means a missed diagnostic, never a wrong one.
func walkStmtForClassChecks(c *compiler.CompilerContext, s ir.Stmt, known map[string]struct{}, out *[]protocol.Diagnostic) {
	switch n := s.(type) {
	case *ir.BlockStmt:
		for _, ss := range n.Stmts {
			walkStmtForClassChecks(c, ss, known, out)
		}
	case *ir.ExprStmt:
		walkExprForClassChecks(c, n.Expr, known, out)
	case *ir.VarDeclStmt:
		walkExprForClassChecks(c, n.Init, known, out)
	case *ir.IfStmt:
		walkExprForClassChecks(c, n.Cond, known, out)
		if n.Then != nil {
			walkStmtForClassChecks(c, n.Then, known, out)
		}
		for _, eb := range n.ElseIfs {
			walkExprForClassChecks(c, eb.Cond, known, out)
			if eb.Then != nil {
				walkStmtForClassChecks(c, eb.Then, known, out)
			}
		}
		if n.Else != nil {
			walkStmtForClassChecks(c, n.Else, known, out)
		}
	case *ir.ReturnStmt:
		for _, e := range n.Results {
			walkExprForClassChecks(c, e, known, out)
		}
	case *ir.FuncDeclStmt:
		if n.Body != nil {
			walkStmtForClassChecks(c, n.Body, known, out)
		}
	case *ir.TypeDeclStmt:
		for _, m := range n.Methods {
			if m != nil && m.Func != nil && m.Func.Body != nil {
				walkStmtForClassChecks(c, m.Func.Body, known, out)
			}
		}
		for _, ct := range n.Ctors {
			if ct != nil && ct.Body != nil {
				walkStmtForClassChecks(c, ct.Body, known, out)
			}
		}
	}
}

func walkExprForClassChecks(c *compiler.CompilerContext, e ir.Expr, known map[string]struct{}, out *[]protocol.Diagnostic) {
	if e == nil {
		return
	}
	switch n := e.(type) {
	case *ir.FuncCallExpr:
		checkCallForClassArgs(c, n, known, out)
		walkExprForClassChecks(c, n.Callee, known, out)
		for _, arg := range n.Args {
			walkExprForClassChecks(c, arg.Expr, known, out)
		}
	case *ir.BinaryExpr:
		walkExprForClassChecks(c, n.Left, known, out)
		walkExprForClassChecks(c, n.Right, known, out)
	case *ir.GroupedExpr:
		walkExprForClassChecks(c, n.Expr, known, out)
	}
}

// checkCallForClassArgs identifies whether `call`'s callee resolves to a known callable — either a top-level `FuncDeclStmt` whose param has `@cssClass`, or a `TypeDeclStmt` whose field (including a mixin-inlined field like Strix's `HtmlElement.class`) has `@cssClass`. For each matching slot it pulls the actual argument, splits a string literal on whitespace (so `"primary large"` checks two tokens), and emits a warning per unknown token.
func checkCallForClassArgs(c *compiler.CompilerContext, call *ir.FuncCallExpr, known map[string]struct{}, out *[]protocol.Diagnostic) {
	calleeName := calleeNameForUnknownClass(call.Callee)
	if calleeName == "" {
		return
	}
	if fn := findFuncByName(c, calleeName); fn != nil {
		for i, arg := range call.Args {
			param := resolveFuncArgParam(fn, arg, i)
			if param == nil || !paramHasCSSClass(param) {
				continue
			}
			reportUnknownClassesInArg(arg, known, out)
		}
		return
	}
	if td := findTypeByName(c, calleeName); td != nil {
		for i, arg := range call.Args {
			field := resolveTypeArgField(td, arg, i)
			if field == nil || !fieldHasCSSClass(field) {
				continue
			}
			reportUnknownClassesInArg(arg, known, out)
		}
	}
}

func resolveFuncArgParam(fn *ir.FuncDeclStmt, arg ir.FuncCallArg, i int) *ir.FuncParam {
	if arg.Name != "" {
		for _, p := range fn.Params {
			if p != nil && p.Name.Name == arg.Name {
				return p
			}
		}
		return nil
	}
	if i >= len(fn.Params) {
		return nil
	}
	return fn.Params[i]
}

func resolveTypeArgField(td *ir.TypeDeclStmt, arg ir.FuncCallArg, i int) *ir.TypeField {
	if arg.Name != "" {
		for _, f := range td.Fields {
			if f != nil && f.Name.Name == arg.Name {
				return f
			}
		}
		return nil
	}
	if i >= len(td.Fields) {
		return nil
	}
	return td.Fields[i]
}

func reportUnknownClassesInArg(arg ir.FuncCallArg, known map[string]struct{}, out *[]protocol.Diagnostic) {
	lit, ok := arg.Expr.(*ir.LitString)
	if !ok || lit.Value == "" {
		return
	}
	litSpan := lit.Span()
	for _, token := range strings.Fields(lit.Value) {
		if _, ok := known[token]; ok {
			continue
		}
		*out = append(*out, protocol.Diagnostic{
			Severity: protocol.DiagnosticSeverityWarning,
			Source:   "sova-lsp",
			Message:  "unknown CSS class `" + token + "` — does not appear in any project stylesheet",
			Range:    spanToLSPRange(litSpan),
		})
	}
}

// calleeNameForUnknownClass pulls the bare callee identifier out of a `*ir.FuncCallExpr.Callee` for the diagnostic lookup. Supports the same shapes Phase 2 supports (VarRef + dotted FieldAccessExpr); anything else returns "" and the diagnostic walker silently skips.
func calleeNameForUnknownClass(e ir.Expr) string {
	switch n := e.(type) {
	case *ir.VarRef:
		return n.Ref.Name
	case *ir.FieldAccessExpr:
		if len(n.Fields) > 0 {
			return n.Fields[len(n.Fields)-1].Name
		}
	}
	return ""
}

// itoa is a no-imports tiny int-to-string for the single int we need to format in the detail line. Avoids pulling `strconv` into the file just for this.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// isStylesheetPath reports whether the resolved on-disk path looks like a
// stylesheet the extractor can read. We accept `.css`, `.scss`, and
// `.sass` — anything else (JSON, binary, plaintext) is ignored at this
// layer regardless of how it was embedded.
func isStylesheetPath(path string) bool {
	switch filepath.Ext(path) {
	case ".css", ".scss", ".sass":
		return true
	}
	return false
}

// readEmbeddedSource reads the on-disk source for a stylesheet referenced by an `@embed`. For `.scss`/`.sass` files we read the **source text**, not the compiled CSS, so the extractor surfaces classes the user actually wrote (including the nested ones and the SCSS partials' classes if the LSP is later extended to follow them). For `.css` files this is identical to what the embed resolver would store. Returns the empty string + nil error on read failure so the caller treats it as "no classes from this file" rather than propagating the I/O error up to the user — completion is best-effort.
func readEmbeddedSource(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// callContext is what the text-back-walker recovers about the function call the cursor is inside: the bare callee identifier (`Element`, `Div`, …), the zero-based argument index the cursor sits in (0 for the first arg, 1 for the second after a `,`, …), and — when the current arg is a named argument (`name: value` syntax) — the explicit `argName`. The "callee" is the longest dotted-identifier chain immediately before the opening `(` (`pkg.Element`, `H1.Button`) so cross-package calls are recognised; the dispatch in cssClassSlotAt strips the package qualifier when resolving against the compiler's symbol table. `argName` is non-empty when Strix-style ctor calls like `Div(class: "primary")` are being typed — the resolver then matches the field by name rather than by index.
type callContext struct {
	callee   string
	argIndex int
	argName  string
}

// findEnclosingCallTextual walks backwards from `offset` through `src` until it finds the opening paren of the innermost enclosing call. Skips strings (matched quotes), comments (line and block), and matched nesting (`()`, `[]`, `{}`) so a `Element("button", foo("nested"))` cursor inside `"nested"` correctly recovers `foo`/`0` rather than `Element`/`1`. Returns ok=false when the walk falls off the start of the buffer without finding an unmatched open paren — that's the "not inside a call" case (top-level expression, assignment, etc.).
//
// When the cursor starts inside a string literal (the most common case for class-name completion), the walker can't tell from a single `"` whether it opens or closes a string — backward scanning is ambiguous in that one case. The entry point handles this by checking string state at the cursor on the current line: if we're inside a string, jump back to just BEFORE the opening quote and start the walk from there. Comments inside the same line have an analogous treatment but in practice rarely matter for class-name positions.
func findEnclosingCallTextual(src string, offset int) (callContext, bool) {
	depthParen, depthBracket, depthBrace := 0, 0, 0
	commas := 0
	argStart := -1
	i := offset - 1
	if openQuote, ok := openingQuoteOfEnclosingString(src, offset); ok {
		i = openQuote - 1
	}
	for i >= 0 {
		c := src[i]
		switch c {
		case '"', '\'':
			i = skipStringBackward(src, i)
			continue
		case '/':
			if i > 0 && src[i-1] == '*' {
				i = skipBlockCommentBackward(src, i)
				continue
			}
		case ')':
			depthParen++
		case ']':
			depthBracket++
		case '}':
			depthBrace++
		case '(':
			if depthParen > 0 {
				depthParen--
				break
			}
			callee, ok := readCalleeBefore(src, i)
			if !ok {
				return callContext{}, false
			}
			if argStart < 0 {
				argStart = i + 1
			}
			argName := readNamedArgPrefix(src, argStart, offset)
			return callContext{callee: callee, argIndex: commas, argName: argName}, true
		case '[':
			if depthBracket > 0 {
				depthBracket--
			}
		case '{':
			if depthBrace > 0 {
				depthBrace--
			}
		case ',':
			if depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
				if argStart < 0 {
					argStart = i + 1
				}
				commas++
			}
		case ';', '\n':
			// Statement boundary at depth 0 means we left the call we were tracking; give up.
			if depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
				return callContext{}, false
			}
		}
		i--
	}
	return callContext{}, false
}

// openingQuoteOfEnclosingString reports whether `offset` sits inside a string literal on the current line and, when so, returns the index of the opening quote. Implements the same single-line forward scan that `classifyCompletion`'s `isInsideGenericString` uses but exposes the opening-quote position so the back-walker can resume from outside the string rather than misinterpreting the opening `"` as a closer. Returns ok=false when the cursor is not inside a single-line string on the current line.
func openingQuoteOfEnclosingString(src string, offset int) (int, bool) {
	lineStart := offset
	for lineStart > 0 && src[lineStart-1] != '\n' {
		lineStart--
	}
	openIdx := -1
	openQuote := byte(0)
	for i := lineStart; i < offset; i++ {
		c := src[i]
		if c == '\\' && i+1 < offset {
			i++
			continue
		}
		if openIdx < 0 {
			if c == '"' || c == '\'' {
				openIdx = i
				openQuote = c
			}
		} else if c == openQuote {
			openIdx = -1
			openQuote = 0
		}
	}
	if openIdx < 0 {
		return 0, false
	}
	return openIdx, true
}

// skipStringBackward consumes a backwards string literal starting at `end` (the closing quote) and returns the index of the next byte to examine (one before the opening quote). Escaped quotes (`\"`) inside the body are honoured by looking at the preceding byte before stopping on a quote of the same kind.
func skipStringBackward(src string, end int) int {
	quote := src[end]
	i := end - 1
	for i >= 0 {
		if src[i] == quote {
			if i == 0 || src[i-1] != '\\' {
				return i - 1
			}
		}
		i--
	}
	return i
}

// skipBlockCommentBackward is the rough inverse of the forward `/* ... */` scan: starts at the `/` of the closing `*/` and walks back until the matching `/*`. Returns the position one before the opening.
func skipBlockCommentBackward(src string, end int) int {
	i := end - 2
	for i >= 1 {
		if src[i-1] == '/' && src[i] == '*' {
			return i - 2
		}
		i--
	}
	return i
}

// cssClassSlotAt is the P2 precision check: combines the text-back-walker (to recover the enclosing call's callee name, the cursor's argument index, and, when present, the named-arg key) with a HIR walk (to look up the callee and inspect the parameter or field at the matched position for an `@cssClass` annotation). The callee can be a top-level function (Phase 2 case) or a type — type-ctor calls like `Div(class: "primary")` resolve the field by named-arg key, leveraging the `inline_mixins` pass having already pulled mixin fields into the type's Fields slice. Returns ok=true when the slot matches; falls back to V1's broad string heuristic otherwise.
func cssClassSlotAt(c *compiler.CompilerContext, src string, offset int) (callContext, bool) {
	ctx, ok := findEnclosingCallTextual(src, offset)
	if !ok {
		return callContext{}, false
	}
	calleeName := unqualifiedCallee(ctx.callee)
	if calleeName == "" {
		return callContext{}, false
	}
	if fn := findFuncByName(c, calleeName); fn != nil {
		if param, ok := lookupFuncParam(fn, ctx); ok && paramHasCSSClass(param) {
			return ctx, true
		}
	}
	if td := findTypeByName(c, calleeName); td != nil {
		if field, ok := lookupTypeField(td, ctx); ok && fieldHasCSSClass(field) {
			return ctx, true
		}
	}
	return callContext{}, false
}

// lookupFuncParam resolves the call's argument to a `*ir.FuncParam` either by name (when the caller wrote `name: value`) or by index. Named-arg lookup wins when both are possible because a user that explicitly named an arg is being more specific than the positional fallback.
func lookupFuncParam(fn *ir.FuncDeclStmt, ctx callContext) (*ir.FuncParam, bool) {
	if ctx.argName != "" {
		for _, p := range fn.Params {
			if p != nil && p.Name.Name == ctx.argName {
				return p, true
			}
		}
		return nil, false
	}
	if ctx.argIndex < 0 || ctx.argIndex >= len(fn.Params) {
		return nil, false
	}
	return fn.Params[ctx.argIndex], true
}

// lookupTypeField does the same as `lookupFuncParam` but for type-ctor calls: the type's Fields slice (which `pass_inline_mixins` has already populated with mixin fields like `class`/`id`/`style` from `HtmlElement`) is the source of truth for "what does this ctor accept". Named-arg lookup again wins over index.
func lookupTypeField(td *ir.TypeDeclStmt, ctx callContext) (*ir.TypeField, bool) {
	if ctx.argName != "" {
		for _, f := range td.Fields {
			if f != nil && f.Name.Name == ctx.argName {
				return f, true
			}
		}
		return nil, false
	}
	if ctx.argIndex < 0 || ctx.argIndex >= len(td.Fields) {
		return nil, false
	}
	return td.Fields[ctx.argIndex], true
}

// fieldHasCSSClass mirrors `paramHasCSSClass` for type fields. The annotation is purely compile-time metadata; the LSP reads it to know that a string literal flowing into this field at a ctor call site should be treated as a CSS class name.
func fieldHasCSSClass(f *ir.TypeField) bool {
	for _, a := range f.Annotations {
		if a.Name.Name == "cssClass" {
			return true
		}
	}
	return false
}

// findTypeByName walks every package's top-level statements and returns the first TypeDeclStmt whose name matches. Mirrors `findFuncByName` for the ctor-call path.
func findTypeByName(c *compiler.CompilerContext, name string) *ir.TypeDeclStmt {
	if c == nil || name == "" {
		return nil
	}
	for _, pkg := range c.Packages {
		if pkg == nil {
			continue
		}
		for _, f := range pkg.Files {
			if f == nil || f.Hir == nil {
				continue
			}
			for _, st := range f.Hir.Statements {
				td, ok := st.(*ir.TypeDeclStmt)
				if !ok {
					continue
				}
				if td.Name.Name == name {
					return td
				}
			}
		}
	}
	return nil
}

// unqualifiedCallee strips any leading package alias from a dotted-identifier callee so the symbol lookup matches the declared name in any package. `pkg.Element` → `Element`; `H1.Button` → `Button` (which is intentional — V2's lookup is by-name and unscoped because Strix's component helpers ship as bare top-level declarations).
func unqualifiedCallee(callee string) string {
	if idx := strings.LastIndex(callee, "."); idx >= 0 {
		return callee[idx+1:]
	}
	return callee
}

// findFuncByName walks every package's top-level statements and returns the first FuncDeclStmt whose name matches. Sufficient for P2's scope — Strix's `Element`, `Div`, `Button` style component helpers all live at top level. Methods on types are out of scope; once a real callable-resolution path lands (or P3 hover/jump), we can extend this to methods via `recv.method()` recovery.
func findFuncByName(c *compiler.CompilerContext, name string) *ir.FuncDeclStmt {
	if c == nil || name == "" {
		return nil
	}
	for _, pkg := range c.Packages {
		if pkg == nil {
			continue
		}
		for _, f := range pkg.Files {
			if f == nil || f.Hir == nil {
				continue
			}
			for _, st := range f.Hir.Statements {
				fn, ok := st.(*ir.FuncDeclStmt)
				if !ok {
					continue
				}
				if fn.Name.Name == name {
					return fn
				}
			}
		}
	}
	return nil
}

// paramHasCSSClass returns true when one of the parameter's annotations is `@cssClass` (the LSP-recognised marker for class-name string slots). The annotation is compile-time metadata — `pass_fold_annotations` accepts it like any other annotation, no codegen path mutates anything.
func paramHasCSSClass(p *ir.FuncParam) bool {
	for _, a := range p.Annotations {
		if a.Name.Name == "cssClass" {
			return true
		}
	}
	return false
}

// readNamedArgPrefix scans forward from `argStart` (the first byte of the current argument, i.e. just after the `(` or the previous `,`) up to `cursor`, looking for a `<ident>:` named-argument prefix. Returns the identifier when found; "" when the arg is positional. Skips leading whitespace so `Div(  class: "primary")` works the same as `Div(class: "primary")`. Stops on string/comment/operator boundaries so a value-position colon (`{key: val}` inside the arg) does not get misread as a named-arg marker.
func readNamedArgPrefix(src string, argStart, cursor int) string {
	i := argStart
	for i < cursor && (src[i] == ' ' || src[i] == '\t' || src[i] == '\n' || src[i] == '\r') {
		i++
	}
	nameStart := i
	for i < cursor && isIdentChar(src[i]) {
		i++
	}
	if nameStart == i {
		return ""
	}
	j := i
	for j < cursor && (src[j] == ' ' || src[j] == '\t') {
		j++
	}
	if j >= cursor || src[j] != ':' {
		return ""
	}
	return src[nameStart:i]
}

// readCalleeBefore grabs the bare identifier (or dotted-identifier chain) immediately before the `(` at position parenIdx. Returns ok=false when there's no identifier — that's the case for `let x = (a + b)` style groupings, `func() { ... }()` IIFEs, and similar non-call parens.
func readCalleeBefore(src string, parenIdx int) (string, bool) {
	i := parenIdx - 1
	for i >= 0 && (src[i] == ' ' || src[i] == '\t') {
		i--
	}
	end := i + 1
	for i >= 0 && (isIdentChar(src[i]) || src[i] == '.') {
		i--
	}
	start := i + 1
	if start >= end {
		return "", false
	}
	name := src[start:end]
	for _, c := range name {
		if c >= '0' && c <= '9' {
			continue
		}
		if c == '.' {
			continue
		}
		break
	}
	return name, true
}
