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

type classRef struct {
	cssclasses.ClassEntry
	File   string
	Source string
}

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

func cssClassHover(c *compiler.CompilerContext, src string, pos protocol.Position) *protocol.Hover {
	offset := positionToOffset(src, pos)
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

	startPos := offsetToPosition(src, start)
	endPos := offsetToPosition(src, end)
	rng := protocol.Range{Start: startPos, End: endPos}

	return &protocol.Hover{
		Contents: protocol.MarkupContent{Kind: protocol.Markdown, Value: body},
		Range:    &rng,
	}
}

func cssClassDefinition(c *compiler.CompilerContext, src string, pos protocol.Position) []protocol.Location {
	offset := positionToOffset(src, pos)
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

		startCol := uint32(ref.Char)
		endCol := startCol + uint32(len(ref.Name)) + 1
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
			Message:  "unknown CSS class `" + token + "` - does not appear in any project stylesheet",
			Range:    spanToRange(litSpan),
		})
	}
}

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

func isStylesheetPath(path string) bool {
	switch filepath.Ext(path) {
	case ".css", ".scss", ".sass":
		return true
	}

	return false
}

func readEmbeddedSource(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

type callContext struct {
	callee   string
	argIndex int
	argName  string
}

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

			if depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
				return callContext{}, false
			}
		}

		i--
	}

	return callContext{}, false
}

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

func fieldHasCSSClass(f *ir.TypeField) bool {
	for _, a := range f.Annotations {
		if a.Name.Name == "cssClass" {
			return true
		}
	}

	return false
}

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

func unqualifiedCallee(callee string) string {
	if idx := strings.LastIndex(callee, "."); idx >= 0 {
		return callee[idx+1:]
	}

	return callee
}

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

func paramHasCSSClass(p *ir.FuncParam) bool {
	for _, a := range p.Annotations {
		if a.Name.Name == "cssClass" {
			return true
		}
	}

	return false
}

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
