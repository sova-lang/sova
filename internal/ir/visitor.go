package ir

import (
	"sova/internal/diag"
	"sova/internal/parser"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/antlr4-go/antlr/v4"
)

// HirVisitor is used to convert the ANTLR tokens into the high-level intermediate representation (HIR) for further semantic analysis.
type HirVisitor struct {
	parser.BaseSovaVisitor
	filename  string // filename is the name of the file being visited.
	diag      *diag.DiagnosticsBag
	nodeAlloc *IdAlloc
	tokens    *antlr.CommonTokenStream // tokens is the input token stream, retained so we can read doc-comment tokens that the lexer routed to the hidden channel.
}

// NewVisitor creates a new HirVisitor instance.
func NewVisitor(filename string, nodeAlloc *IdAlloc, diag *diag.DiagnosticsBag) *HirVisitor {
	return &HirVisitor{
		filename:  filename,
		nodeAlloc: nodeAlloc,
		diag:      diag,
	}
}

// SetTokenStream wires the buffered token stream so doc-comment extraction
// can look at hidden-channel tokens preceding each decl. Called by the
// compiler just after constructing the parser, before Visit() is invoked.
func (v *HirVisitor) SetTokenStream(ts *antlr.CommonTokenStream) {
	v.tokens = ts
}

// docCommentBefore returns the joined / stripped doc-comment text attached to
// the token immediately starting `ctx`, or "" when no doc comments precede
// it. Walks the hidden token stream backwards from the rule's start,
// gathering DOC_COMMENT (`///`) lines and DOC_BLOCK_COMMENT (`/** */`)
// blocks. Enforces the "no blank line between comment and decl" rule
// (and the "no blank line between consecutive doc comments" rule) by
// comparing line numbers — the lexer drops whitespace via `-> skip`,
// so there are no WS tokens we could inspect for newline counts.
//
// Gap detection: each iteration computes the END line of the current
// comment (`startLine + count('\n', text)` so block comments measure
// correctly) and compares it against the previous anchor's start line.
// A gap > 1 means there's at least one blank line in between → the
// current comment is NOT part of this doc-comment chain and we stop.
func (v *HirVisitor) docCommentBefore(ctx antlr.ParserRuleContext) string {
	if v.tokens == nil {
		return ""
	}
	start := ctx.GetStart()
	if start == nil {
		return ""
	}
	startIdx := start.GetTokenIndex()
	if startIdx <= 0 {
		return ""
	}
	allTokens := v.tokens.GetAllTokens()
	if startIdx > len(allTokens) {
		return ""
	}
	type tok struct {
		text string
		line int
		kind int
	}
	var collected []tok
	prevStartLine := start.GetLine()
	for i := startIdx - 1; i >= 0; i-- {
		t := allTokens[i]
		ttype := t.GetTokenType()
		text := t.GetText()
		isDocLine := ttype == parser.SovaLexerDOC_COMMENT
		isDocBlock := ttype == parser.SovaLexerDOC_BLOCK_COMMENT
		if !isDocLine && !isDocBlock {
			break
		}
		tStart := t.GetLine()
		tEnd := tStart + strings.Count(text, "\n")
		if prevStartLine-tEnd > 1 {
			break
		}
		collected = append(collected, tok{text: text, line: tStart, kind: ttype})
		prevStartLine = tStart
	}
	if len(collected) == 0 {
		return ""
	}
	// Reverse so we have them in source order, then strip + join.
	for i, j := 0, len(collected)-1; i < j; i, j = i+1, j-1 {
		collected[i], collected[j] = collected[j], collected[i]
	}
	var parts []string
	for _, c := range collected {
		if c.kind == parser.SovaLexerDOC_COMMENT {
			parts = append(parts, stripDocLine(c.text))
			continue
		}
		parts = append(parts, stripDocBlock(c.text))
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

// stripDocLine removes the leading `///` (and one optional space) from a
// single doc-comment line. The trailing newline is already absent because the
// lexer matches up to but not including the line terminator.
func stripDocLine(s string) string {
	s = strings.TrimPrefix(s, "///")
	if strings.HasPrefix(s, " ") {
		s = s[1:]
	}
	return strings.TrimRight(s, "\r")
}

// stripDocBlock strips the `/** ... */` wrapper plus the conventional
// leading ` * ` on each interior line. Mirrors JSDoc/TSDoc behaviour: the
// returned text is the inner narrative, ready to feed into a markdown
// renderer.
func stripDocBlock(s string) string {
	s = strings.TrimPrefix(s, "/**")
	s = strings.TrimSuffix(s, "*/")
	lines := strings.Split(s, "\n")
	var out []string
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimLeft(line, " \t")
		switch {
		case trimmed == "*":
			out = append(out, "")
		case strings.HasPrefix(trimmed, "* "):
			out = append(out, trimmed[2:])
		case strings.HasPrefix(trimmed, "*"):
			out = append(out, trimmed[1:])
		default:
			out = append(out, line)
		}
	}
	// Drop leading / trailing blank lines.
	for len(out) > 0 && strings.TrimSpace(out[0]) == "" {
		out = out[1:]
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

// --- Internal Helper ---

func (v *HirVisitor) nid() NodeID { return NodeID(v.nodeAlloc.Next()) }

func (v *HirVisitor) spanFromCtx(ctx antlr.ParserRuleContext) diag.TextSpan {
	s := ctx.GetStart()
	e := ctx.GetStop()

	// ANTLR column is 0-based so we need to adjust to 1-based columns.
	startCol := s.GetColumn() + 1
	endCol := e.GetColumn() + 1 + len(e.GetText())
	return diag.TextSpan{
		File:     v.filename,
		StartLn:  s.GetLine(),
		StartCol: startCol,
		EndLn:    e.GetLine(),
		EndCol:   endCol,
	}
}

func (v *HirVisitor) spanFromTok(t antlr.Token) diag.TextSpan {
	return diag.TextSpan{
		File:     v.filename,
		StartLn:  t.GetLine(),
		StartCol: t.GetColumn() + 1,
		EndLn:    t.GetLine(),
		EndCol:   t.GetColumn() + 1 + len(t.GetText()),
	}
}

func (v *HirVisitor) mkNode(ctx antlr.ParserRuleContext) node {
	return node{id: v.nid(), span: v.spanFromCtx(ctx)}
}

// softIdNameAndSpan extracts the identifier text and its source span from a `softId` parser context. softId admits both a real `ID` token and the synth-soft-reserved keyword tokens (`where`, `to`, `append`) wherever an identifier is grammatically expected — see grammar comment + BUGS.md #20. Callers that previously held a `TerminalNode` from `ctx.ID()` should switch to this helper since the underlying token may now be any of the alternatives.
func (v *HirVisitor) softIdNameAndSpan(ctx parser.ISoftIdContext) (string, diag.TextSpan) {
	if ctx == nil {
		return "", diag.TextSpan{File: v.filename}
	}
	return ctx.GetText(), v.spanFromCtx(ctx)
}

func (v *HirVisitor) mkNodeFromTok(t antlr.Token) node {
	return node{id: v.nid(), span: v.spanFromTok(t)}
}

func unquoteString(raw string) string {
	// raw: "...." with optional `\"`, `\\`, `\n`, `\t`, `\r` escapes inside.
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return raw
	}
	inner := raw[1 : len(raw)-1]
	var b strings.Builder
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		if c == '\\' && i+1 < len(inner) {
			next := inner[i+1]
			switch next {
			case '"', '\\':
				b.WriteByte(next)
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			default:
				b.WriteByte('\\')
				b.WriteByte(next)
			}
			i++
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func unquoteChar(raw string) rune {
	// raw: 'x'
	if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
		r, _ := utf8.DecodeRuneInString(raw[1 : len(raw)-1])
		return r
	}
	return 0
}

// splitExternModuleSpec splits an extern module reference of the form `path[@version]` into its module path and the optional version pin. Sova accepts versioned references on Go-backend externs so the package manager can synthesise a `require <path> <version>` line into the generated `go.mod` and pull the dependency through `go mod tidy`. The split uses the last `@` so that legitimate Go module paths (which never contain `@`) parse cleanly while still allowing future ref selectors that could contain extra punctuation. Returns the raw input unchanged and an empty version when no `@` is present.
func splitExternModuleSpec(raw string) (string, string) {
	at := strings.LastIndexByte(raw, '@')
	if at < 0 {
		return raw, ""
	}
	path := strings.TrimSpace(raw[:at])
	version := strings.TrimSpace(raw[at+1:])
	if path == "" {
		return raw, ""
	}
	return path, version
}

func parseIntLiteral(raw string) (int64, error) {
	if strings.HasPrefix(raw, "0x") || strings.HasPrefix(raw, "0X") {
		u, err := strconv.ParseUint(raw[2:], 16, 64)
		return int64(u), err
	}
	if strings.HasPrefix(raw, "0b") || strings.HasPrefix(raw, "0B") {
		u, err := strconv.ParseUint(raw[2:], 2, 64)
		return int64(u), err
	}
	i, err := strconv.ParseInt(raw, 10, 64)
	return i, err
}

func parseFloatLiteral(raw string) (float64, error) {
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, err
	}
	return f, nil
}

// --- File & Header ---

func (v *HirVisitor) visitRule(node antlr.RuleNode) any {
	return node.Accept(v)
}

func (v *HirVisitor) Visit(tree antlr.ParseTree) any {
	if tree == nil {
		return nil
	}
	switch ctx := tree.(type) {
	case *parser.FileContext:
		return v.VisitFile(ctx)
	case antlr.RuleNode:
		return v.visitRule(ctx)
	}
	return nil
}

// visitExpr converts the result of v.Visit(tree) into an ir.Expr, returning nil
// when the parse tree was missing (e.g., partial source being typed in the
// editor) or the visitor produced no expression. Use this in place of a raw
// `v.Visit(ctx.Expr()).(Expr)` assertion to avoid panics on incomplete input.
func (v *HirVisitor) visitExpr(tree antlr.ParseTree) Expr {
	if tree == nil {
		return nil
	}
	out := v.Visit(tree)
	if out == nil {
		return nil
	}
	if e, ok := out.(Expr); ok {
		return e
	}
	return nil
}

// visitBlock is the *BlockStmt-shaped counterpart to visitExpr.
func (v *HirVisitor) visitBlock(tree antlr.ParseTree) *BlockStmt {
	if tree == nil {
		return nil
	}
	out := v.Visit(tree)
	if out == nil {
		return nil
	}
	if b, ok := out.(*BlockStmt); ok {
		return b
	}
	return nil
}

// visitStmt is the ir.Stmt-shaped counterpart to visitExpr.
func (v *HirVisitor) visitStmt(tree antlr.ParseTree) Stmt {
	if tree == nil {
		return nil
	}
	out := v.Visit(tree)
	if out == nil {
		return nil
	}
	if s, ok := out.(Stmt); ok {
		return s
	}
	return nil
}

func (v *HirVisitor) VisitFile(ctx *parser.FileContext) any {
	f := &File{
		node:    v.mkNode(ctx),
		Path:    v.filename,
		Package: PackagePath{"main"},        // Default package path, will be set later if a package declaration is present.
		Side:    SideSpec{Kind: SideShared}, // Default side is always shared.
	}

	if fh := ctx.FileHeader(); fh != nil {
		if hi, ok := v.Visit(fh).(*fileHeaderOut); ok && hi != nil {
			f.Package = hi.pkg
			f.Side = hi.side
		}
	}

	// Statements
	for _, s := range ctx.AllStmt() {
		st := v.Visit(s)
		if st == nil {
			continue
		}
		f.Statements = append(f.Statements, st.(Stmt))
	}
	f.Statements = flattenWireGroups(f.Statements)
	return f
}

type fileHeaderOut struct {
	pkg  PackagePath
	side SideSpec
}

func (v *HirVisitor) VisitFileHeader(ctx *parser.FileHeaderContext) any {
	out := &fileHeaderOut{}
	if pd := ctx.PackageDecl(); pd != nil {
		if p, ok := v.Visit(pd).(PackagePath); ok {
			out.pkg = p
		}
	}
	if sd := ctx.SideDecl(); sd != nil {
		if s, ok := v.Visit(sd).(SideSpec); ok {
			out.side = s
		}
	}
	return out
}

func (v *HirVisitor) VisitPackageDecl(ctx *parser.PackageDeclContext) any {
	pp := v.Visit(ctx.PackagePath()).(PackagePath)
	return pp
}

func (v *HirVisitor) VisitPackagePath(ctx *parser.PackagePathContext) any {
	idents := ctx.AllPkgIdent()
	pp := make(PackagePath, 0, len(idents))
	for _, ident := range idents {
		pp = append(pp, ident.GetText())
	}
	return pp
}

func (v *HirVisitor) VisitSideDecl(ctx *parser.SideDeclContext) any {
	return v.Visit(ctx.Side())
}

func (v *HirVisitor) VisitSide(ctx *parser.SideContext) any {
	var s SideSpec
	switch {
	case ctx.SIDE_FRONTEND() != nil:
		s.Kind = SideFrontend
	case ctx.SIDE_SHARED() != nil:
		s.Kind = SideShared
	case ctx.SIDE_TEST() != nil:
		s.Kind = SideTest
	case ctx.SIDE_SYNTH() != nil:
		s.Kind = SideSynth
	case ctx.SIDE_BACKEND() != nil:
		s.Kind = SideBackend
		if id := ctx.ID(); id != nil {
			s.Target = id.GetText()
		}
	default:
		s.Kind = SideUnknown
	}
	return s
}

// --- Statements ---

func (v *HirVisitor) VisitStmt(ctx *parser.StmtContext) any {
	if bl := ctx.Block(); bl != nil {
		return v.Visit(bl)
	}
	if vd := ctx.VarDeclStmt(); vd != nil {
		return v.Visit(vd)
	}
	if fd := ctx.FuncDeclStmt(); fd != nil {
		return v.Visit(fd)
	}
	if ed := ctx.ExternDecl(); ed != nil {
		return v.Visit(ed)
	}
	if ex := ctx.ExprStmt(); ex != nil {
		return v.Visit(ex)
	}
	if is := ctx.IfStmt(); is != nil {
		return v.Visit(is)
	}
	if ss := ctx.SwitchStmt(); ss != nil {
		return v.Visit(ss)
	}
	if bs := ctx.BreakStmt(); bs != nil {
		return v.Visit(bs)
	}
	if cs := ctx.ContinueStmt(); cs != nil {
		return v.Visit(cs)
	}
	if rs := ctx.ReturnStmt(); rs != nil {
		return v.Visit(rs)
	}
	if gs := ctx.GuardStmt(); gs != nil {
		return v.Visit(gs)
	}
	if fs := ctx.ForStmt(); fs != nil {
		return v.Visit(fs)
	}
	if ws := ctx.WhileStmt(); ws != nil {
		return v.Visit(ws)
	}
	if en := ctx.EnumDeclStmt(); en != nil {
		return v.Visit(en)
	}
	if td := ctx.TypeDeclStmt(); td != nil {
		return v.Visit(td)
	}
	if id := ctx.InterfaceDeclStmt(); id != nil {
		return v.Visit(id)
	}
	if md := ctx.MixinDeclStmt(); md != nil {
		return v.Visit(md)
	}
	if ta := ctx.TypeAliasStmt(); ta != nil {
		return v.Visit(ta)
	}
	if im := ctx.ImportStmt(); im != nil {
		return v.Visit(im)
	}
	if wg := ctx.WireGroupStmt(); wg != nil {
		return v.Visit(wg)
	}
	if wr := ctx.WireRulesetStmt(); wr != nil {
		return v.Visit(wr)
	}
	if td := ctx.TestDeclStmt(); td != nil {
		return v.Visit(td)
	}
	if gd := ctx.GroupDeclStmt(); gd != nil {
		return v.Visit(gd)
	}
	if ss := ctx.SetupStmt(); ss != nil {
		return v.Visit(ss)
	}
	if ts := ctx.TeardownStmt(); ts != nil {
		return v.Visit(ts)
	}
	if as := ctx.AssertStmt(); as != nil {
		return v.Visit(as)
	}
	if asn := ctx.AsSessionStmt(); asn != nil {
		return v.Visit(asn)
	}
	if gs := ctx.GoStmt(); gs != nil {
		return v.Visit(gs)
	}
	if ds := ctx.DeferStmt(); ds != nil {
		return v.Visit(ds)
	}
	if ss := ctx.SelectStmt(); ss != nil {
		return v.Visit(ss)
	}
	if sd := ctx.SynthDeclStmt(); sd != nil {
		return v.Visit(sd)
	}
	return nil
}

func (v *HirVisitor) VisitGoStmt(ctx *parser.GoStmtContext) any {
	st := &GoStmt{node: v.mkNode(ctx)}
	if es := ctx.ExprStmt(); es != nil {
		if call, ok := v.Visit(es).(*ExprStmt); ok && call != nil {
			st.Call = call.Expr
		}
	} else if bl := ctx.Block(); bl != nil {
		if body, ok := v.Visit(bl).(*BlockStmt); ok {
			st.Body = body
		}
	}
	return st
}

func (v *HirVisitor) VisitDeferStmt(ctx *parser.DeferStmtContext) any {
	st := &DeferStmt{node: v.mkNode(ctx)}
	if es := ctx.ExprStmt(); es != nil {
		if call, ok := v.Visit(es).(*ExprStmt); ok && call != nil {
			st.Call = call.Expr
		}
	} else if bl := ctx.Block(); bl != nil {
		if body, ok := v.Visit(bl).(*BlockStmt); ok {
			st.Body = body
		}
	}
	return st
}

func (v *HirVisitor) VisitSelectStmt(ctx *parser.SelectStmtContext) any {
	st := &SelectStmt{node: v.mkNode(ctx)}
	for _, cc := range ctx.AllSelectCase() {
		if sc, ok := v.Visit(cc).(*SelectCase); ok && sc != nil {
			st.Cases = append(st.Cases, sc)
		}
	}
	if dc := ctx.SelectDefaultCase(); dc != nil {
		if body, ok := v.Visit(dc).(*BlockStmt); ok {
			st.Default = body
		}
	}
	return st
}

func (v *HirVisitor) VisitSelectCase(ctx *parser.SelectCaseContext) any {
	sc := &SelectCase{Span: v.spanFromCtx(ctx)}
	if guard := ctx.SelectCaseGuard(); guard != nil {
		v.fillSelectCaseGuard(sc, guard.(*parser.SelectCaseGuardContext))
	}
	if bl := ctx.Block(); bl != nil {
		if body, ok := v.Visit(bl).(*BlockStmt); ok {
			sc.Body = body
		}
	} else if stmt := ctx.Stmt(); stmt != nil {
		body := &BlockStmt{node: v.mkNode(stmt)}
		if s, ok := v.Visit(stmt).(Stmt); ok && s != nil {
			body.Stmts = append(body.Stmts, s)
		}
		sc.Body = body
	}
	return sc
}

func (v *HirVisitor) VisitSelectDefaultCase(ctx *parser.SelectDefaultCaseContext) any {
	if bl := ctx.Block(); bl != nil {
		if body, ok := v.Visit(bl).(*BlockStmt); ok {
			return body
		}
	}
	if stmt := ctx.Stmt(); stmt != nil {
		body := &BlockStmt{node: v.mkNode(stmt)}
		if s, ok := v.Visit(stmt).(Stmt); ok && s != nil {
			body.Stmts = append(body.Stmts, s)
		}
		return body
	}
	return nil
}

func (v *HirVisitor) fillSelectCaseGuard(sc *SelectCase, ctx *parser.SelectCaseGuardContext) {
	if rb := ctx.SelectRecvBinding(); rb != nil {
		rbc := rb.(*parser.SelectRecvBindingContext)
		for _, tgt := range rbc.AllVarDeclTarget() {
			if vt, ok := v.Visit(tgt).(VarDeclTarget); ok {
				sc.Targets = append(sc.Targets, vt)
			}
		}
		if ex := rbc.Expr(); ex != nil {
			if e, ok := v.Visit(ex).(Expr); ok {
				sc.Kind = SelectCaseRecvBind
				if call, isCall := e.(*FuncCallExpr); isCall {
					if fa, isField := call.Callee.(*FieldAccessExpr); isField && len(fa.Fields) == 1 && fa.Fields[0].Name == "recv" {
						sc.ChanExpr = fa.Expr
					} else {
						sc.ChanExpr = e
					}
				} else {
					sc.ChanExpr = e
				}
			}
		}
		return
	}
	if ex := ctx.Expr(); ex != nil {
		if e, ok := v.Visit(ex).(Expr); ok {
			if call, isCall := e.(*FuncCallExpr); isCall {
				if fa, isField := call.Callee.(*FieldAccessExpr); isField && len(fa.Fields) == 1 {
					switch fa.Fields[0].Name {
					case "send":
						sc.Kind = SelectCaseSend
						sc.ChanExpr = fa.Expr
						if len(call.Args) >= 1 {
							sc.SendValue = call.Args[0].Expr
						}
						return
					case "recv":
						sc.Kind = SelectCaseRecvDiscard
						sc.ChanExpr = fa.Expr
						return
					}
				}
			}
			sc.Kind = SelectCaseRecvDiscard
			sc.ChanExpr = e
		}
	}
}

func (v *HirVisitor) VisitTestDeclStmt(ctx *parser.TestDeclStmtContext) any {
	st := &TestDeclStmt{node: v.mkNode(ctx)}
	if lit := ctx.STRING_LITERAL(); lit != nil {
		st.Name = unquoteString(lit.GetText())
	}
	if bl := ctx.Block(); bl != nil {
		if body, ok := v.Visit(bl).(*BlockStmt); ok {
			st.Body = body
		}
	}
	if ctx.PARALLEL() != nil {
		st.Parallel = true
	}
	if tl := ctx.TestTagList(); tl != nil {
		st.Tags = collectTagList(tl)
	}
	return st
}

func (v *HirVisitor) VisitGroupDeclStmt(ctx *parser.GroupDeclStmtContext) any {
	st := &GroupDeclStmt{node: v.mkNode(ctx)}
	if lit := ctx.STRING_LITERAL(); lit != nil {
		st.Name = unquoteString(lit.GetText())
	}
	for _, item := range ctx.AllGroupItem() {
		if s := v.Visit(item); s != nil {
			if stmt, ok := s.(Stmt); ok {
				st.Body = append(st.Body, stmt)
			}
		}
	}
	if ctx.PARALLEL() != nil {
		st.Parallel = true
	}
	if tl := ctx.TestTagList(); tl != nil {
		st.Tags = collectTagList(tl)
	}
	return st
}

func collectTagList(ctx parser.ITestTagListContext) []string {
	var tags []string
	for _, lit := range ctx.AllSTRING_LITERAL() {
		tags = append(tags, unquoteString(lit.GetText()))
	}
	return tags
}

func (v *HirVisitor) VisitGroupItem(ctx *parser.GroupItemContext) any {
	if td := ctx.TestDeclStmt(); td != nil {
		return v.Visit(td)
	}
	if gd := ctx.GroupDeclStmt(); gd != nil {
		return v.Visit(gd)
	}
	if ss := ctx.SetupStmt(); ss != nil {
		return v.Visit(ss)
	}
	if ts := ctx.TeardownStmt(); ts != nil {
		return v.Visit(ts)
	}
	return nil
}

func (v *HirVisitor) VisitSetupStmt(ctx *parser.SetupStmtContext) any {
	st := &SetupStmt{node: v.mkNode(ctx), IsAll: strings.Contains(ctx.GetText(), "setupAll")}
	if bl := ctx.Block(); bl != nil {
		if body, ok := v.Visit(bl).(*BlockStmt); ok {
			st.Body = body
		}
	}
	return st
}

func (v *HirVisitor) VisitTeardownStmt(ctx *parser.TeardownStmtContext) any {
	st := &TeardownStmt{node: v.mkNode(ctx), IsAll: strings.Contains(ctx.GetText(), "teardownAll")}
	if bl := ctx.Block(); bl != nil {
		if body, ok := v.Visit(bl).(*BlockStmt); ok {
			st.Body = body
		}
	}
	return st
}

func (v *HirVisitor) VisitAssertStmt(ctx *parser.AssertStmtContext) any {
	st := &AssertStmt{node: v.mkNode(ctx)}
	if ex := ctx.Expr(); ex != nil {
		if e, ok := v.Visit(ex).(Expr); ok {
			st.Expr = e
		}
	}
	return st
}

func (v *HirVisitor) VisitAsSessionStmt(ctx *parser.AsSessionStmtContext) any {
	st := &AsSessionStmt{node: v.mkNode(ctx)}
	if lit := ctx.STRING_LITERAL(); lit != nil {
		st.Name = unquoteString(lit.GetText())
	}
	if bl := ctx.Block(); bl != nil {
		if body, ok := v.Visit(bl).(*BlockStmt); ok {
			st.Body = body
		}
	}
	return st
}

func (v *HirVisitor) VisitWireGroupStmt(ctx *parser.WireGroupStmtContext) any {
	wg := &WireGroupStmt{
		node: v.mkNode(ctx),
		Wire: &WireSpec{Options: map[string]WireOptValue{}},
	}
	if ws := ctx.WireSpec(); ws != nil {
		wg.Wire = v.buildWireSpec(ws)
	}
	for _, s := range ctx.AllStmt() {
		inner := v.VisitStmt(s.(*parser.StmtContext))
		if inner == nil {
			continue
		}
		if st, ok := inner.(Stmt); ok {
			wg.Stmts = append(wg.Stmts, st)
		}
	}
	return wg
}

func (v *HirVisitor) VisitWireRulesetStmt(ctx *parser.WireRulesetStmtContext) any {
	rs := &WireRulesetStmt{
		node:    v.mkNode(ctx),
		Options: map[string]WireOptValue{},
	}
	if id := ctx.ID(); id != nil {
		rs.Name = id.GetSymbol().GetText()
	}
	if opts := ctx.WireOptions(); opts != nil {
		for _, o := range opts.AllWireOption() {
			key := o.ID().GetSymbol().GetText()
			rs.Options[key] = v.parseWireOptionValue(o.Expr())
		}
	}
	return rs
}

func (v *HirVisitor) buildWireSpec(ws parser.IWireSpecContext) *WireSpec {
	spec := &WireSpec{Options: map[string]WireOptValue{}}
	if id := ws.ID(); id != nil {
		spec.Ruleset = id.GetSymbol().GetText()
	}
	if opts := ws.WireOptions(); opts != nil {
		for _, o := range opts.AllWireOption() {
			key := o.ID().GetSymbol().GetText()
			spec.Options[key] = v.parseWireOptionValue(o.Expr())
		}
	}
	return spec
}

// flattenWireGroups expands any WireGroupStmt nodes inline, propagating the group's wire spec to each inner decl statement that supports it.
func flattenWireGroups(stmts []Stmt) []Stmt {
	out := make([]Stmt, 0, len(stmts))
	for _, st := range stmts {
		switch g := st.(type) {
		case *WireGroupStmt:
			expanded := flattenWireGroups(g.Stmts)
			for _, child := range expanded {
				applyGroupWire(child, g.Wire)
				out = append(out, child)
			}
		default:
			out = append(out, st)
		}
	}
	return out
}

func applyGroupWire(st Stmt, group *WireSpec) {
	if group == nil {
		return
	}
	switch d := st.(type) {
	case *VarDeclStmt:
		d.IsWired = true
		d.Wire = mergeWireSpec(d.Wire, group)
	case *FuncDeclStmt:
		d.IsWired = true
		d.Wire = mergeWireSpec(d.Wire, group)
	}
}

func mergeWireSpec(child, group *WireSpec) *WireSpec {
	if group == nil {
		return child
	}
	if child == nil {
		out := &WireSpec{Options: map[string]WireOptValue{}}
		for k, v := range group.Options {
			out.Options[k] = v
		}
		return out
	}
	if child.Options == nil {
		child.Options = map[string]WireOptValue{}
	}
	for k, v := range group.Options {
		if _, has := child.Options[k]; !has {
			child.Options[k] = v
		}
	}
	return child
}

func (v *HirVisitor) VisitImportStmt(ctx *parser.ImportStmtContext) any {
	tok := ctx.STRING_LITERAL().GetSymbol()
	raw := unquoteString(tok.GetText())
	parts := strings.Split(raw, "/")
	alias := ""
	if len(parts) > 0 {
		alias = parts[len(parts)-1]
	}
	stmt := &ImportStmt{
		node:  v.mkNode(ctx),
		Path:  PackagePath(parts),
		Alias: alias,
	}
	if u := ctx.UsingClause(); u != nil {
		if u.MULT() != nil {
			stmt.UsingAll = true
		}
		for _, id := range u.AllID() {
			stmt.UsingList = append(stmt.UsingList, id.GetText())
		}
	}
	return stmt
}

func (v *HirVisitor) VisitBlock(ctx *parser.BlockContext) any {
	b := &BlockStmt{
		node:  v.mkNode(ctx),
		Stmts: []Stmt{},
	}

	for _, s := range ctx.AllStmt() {
		st := v.VisitStmt(s.(*parser.StmtContext))
		if st == nil {
			continue
		}
		b.Stmts = append(b.Stmts, st.(Stmt))
	}
	b.Stmts = flattenWireGroups(b.Stmts)
	return b
}

func (v *HirVisitor) VisitVarDeclStmt(ctx *parser.VarDeclStmtContext) any {
	st := &VarDeclStmt{
		node:    v.mkNode(ctx),
		docBase: docBase{doc: v.docCommentBefore(ctx)},
		IsConst: ctx.CONST() != nil,
	}

	st.Annotations = v.collectAnnotations(ctx.AllAnnotation())

	if wireCtx := ctx.WireSpec(); wireCtx != nil {
		st.IsWired = true
		st.Wire = v.buildWireSpec(wireCtx)
	}

	// Targets (can be multiple for tuple destructuring)
	for _, targetCtx := range ctx.AllVarDeclTarget() {
		target := v.Visit(targetCtx)
		if t, ok := target.(VarDeclTarget); ok {
			st.Targets = append(st.Targets, t)
		}
	}

	if ctx.Expr() != nil {
		initExpr := v.Visit(ctx.Expr())
		if expr, ok := initExpr.(Expr); ok {
			st.Init = expr
		} else {
			return nil
		}
	}

	return st
}

func (v *HirVisitor) VisitVarDeclTarget(ctx *parser.VarDeclTargetContext) any {
	target := VarDeclTarget{}

	// Check if this is a discard '_'
	if ctx.GetText() == "_" {
		target.Name = nil
	} else if soft := ctx.SoftId(); soft != nil {
		name, span := v.softIdNameAndSpan(soft)
		target.Name = &NameRef{Name: name, Sym: 0, Span: span}

		if ta := ctx.TypeAnnot(); ta != nil {
			if tr, ok := v.Visit(ta).(*TypeRef); ok {
				target.TypeAnn = tr
			}
		}
	}

	return target
}

func (v *HirVisitor) parseWireOptionValue(exprCtx parser.IExprContext) WireOptValue {
	if exprCtx == nil {
		return WireOptValue{}
	}
	res := v.Visit(exprCtx)
	switch lit := res.(type) {
	case *LitString:
		return WireOptValue{Kind: WireOptString, Str: lit.Value}
	case *LitInt:
		return WireOptValue{Kind: WireOptInt, Int: lit.Value}
	case *LitBool:
		return WireOptValue{Kind: WireOptBool, Bool: lit.Value}
	case *ArrayLiteral:
		strs := make([]string, 0, len(lit.Elems))
		for _, el := range lit.Elems {
			if s, ok := el.(*LitString); ok {
				strs = append(strs, s.Value)
			}
		}
		return WireOptValue{Kind: WireOptStringArray, Strs: strs}
	}
	return WireOptValue{}
}

func (v *HirVisitor) VisitFuncDeclStmt(ctx *parser.FuncDeclStmtContext) any {
	st := &FuncDeclStmt{
		node:    v.mkNode(ctx),
		docBase: docBase{doc: v.docCommentBefore(ctx)},
	}
	st.Annotations = v.collectAnnotations(ctx.AllAnnotation())
	if wireCtx := ctx.WireSpec(); wireCtx != nil {
		st.IsWired = true
		st.Wire = v.buildWireSpec(wireCtx)
	}

	// Side
	if ctx.SideDecl() != nil {
		sideSpec := v.Visit(ctx.SideDecl().Side()).(SideSpec)
		st.Side = &sideSpec
	}

	// Name
	name, span := v.softIdNameAndSpan(ctx.SoftId())
	st.Name = NameRef{Name: name, Sym: 0, Span: span}

	// Generic type parameters
	if gp := ctx.GenericParams(); gp != nil {
		for _, p := range gp.AllGenericParam() {
			st.TypeParams = append(st.TypeParams, v.buildGenericParamDecl(p))
		}
	}

	// Parameters
	if ctx.FuncParamList() != nil {
		for _, paramCtx := range ctx.FuncParamList().AllFuncParam() {
			param := v.Visit(paramCtx).(FuncParam)
			st.Params = append(st.Params, &param)
		}
	}

	// Return type
	if ctx.TypeAnnot() != nil {
		if tr, ok := v.Visit(ctx.TypeAnnot()).(*TypeRef); ok {
			st.ReturnType = tr
		}
	}

	// Body
	if ctx.Block() != nil {
		body := v.Visit(ctx.Block()).(*BlockStmt)
		st.Body = body
	}

	return st
}

func (v *HirVisitor) VisitFuncParam(ctx *parser.FuncParamContext) any {
	param := FuncParam{
		node:       v.mkNode(ctx),
		IsVariadic: ctx.VARARG() != nil,
	}

	// Name
	pname, pspan := v.softIdNameAndSpan(ctx.SoftId())
	param.Name = NameRef{Span: pspan, Name: pname, Sym: 0}

	// Type
	if ctx.TypeAnnot() != nil {
		if tr, ok := v.Visit(ctx.TypeAnnot()).(*TypeRef); ok {
			param.Type = tr
		}
	}

	if ctx.Expr() != nil {
		initExpr := v.Visit(ctx.Expr())
		if expr, ok := initExpr.(Expr); ok {
			param.Default = expr
		}
	}

	param.Annotations = v.collectAnnotations(ctx.AllAnnotation())

	return param
}

func (v *HirVisitor) VisitExternDecl(ctx *parser.ExternDeclContext) any {
	st := &ExternDeclStmt{
		node:    v.mkNode(ctx),
		docBase: docBase{doc: v.docCommentBefore(ctx)},
	}

	if ctx.STRING_LITERAL() != nil {
		raw := unquoteString(ctx.STRING_LITERAL().GetText())
		modulePath, version := splitExternModuleSpec(raw)
		st.Module = &modulePath
		st.Version = version
	}
	st.IsDefaultImport = hasLiteralChild(ctx, "default")

	moduleStr := ""
	if st.Module != nil {
		moduleStr = *st.Module
	}
	for _, itemCtx := range ctx.AllExternItem() {
		item := v.Visit(itemCtx)
		switch v := item.(type) {
		case *ExternFunc:
			st.Funcs = append(st.Funcs, v)
		case *ExternVar:
			st.Vars = append(st.Vars, v)
		case *TypeDeclStmt:
			v.IsExtern = true
			v.ExternModule = moduleStr
			filtered := v.Ctors[:0]
			for _, c := range v.Ctors {
				if !c.IsSynthetic {
					filtered = append(filtered, c)
				}
			}
			v.Ctors = filtered
			st.Types = append(st.Types, v)
		case *InterfaceDeclStmt:
			v.IsExtern = true
			v.ExternModule = moduleStr
			st.Interfaces = append(st.Interfaces, v)
		}
	}

	return st
}

func (v *HirVisitor) VisitExternItem(ctx *parser.ExternItemContext) any {
	if ctx.ExternFunc() != nil {
		return v.Visit(ctx.ExternFunc())
	}
	if ctx.ExternVar() != nil {
		return v.Visit(ctx.ExternVar())
	}
	if ctx.TypeDeclStmt() != nil {
		return v.Visit(ctx.TypeDeclStmt())
	}
	if ctx.InterfaceDeclStmt() != nil {
		return v.Visit(ctx.InterfaceDeclStmt())
	}
	return nil
}

func (v *HirVisitor) VisitExternFunc(ctx *parser.ExternFuncContext) any {
	fn := &ExternFunc{
		node:    v.mkNode(ctx),
		docBase: docBase{doc: v.docCommentBefore(ctx)},
		IsAsync: strings.HasPrefix(ctx.GetText(), "async"),
	}

	if ctx.SoftId() == nil {
		return fn
	}
	fname, fspan := v.softIdNameAndSpan(ctx.SoftId())
	fn.Name = NameRef{Name: fname, Sym: 0, Span: fspan}

	if gp := ctx.GenericParams(); gp != nil {
		for _, p := range gp.AllGenericParam() {
			fn.TypeParams = append(fn.TypeParams, v.buildGenericParamDecl(p))
		}
	}

	if ctx.FuncParamList() != nil {
		for _, paramCtx := range ctx.FuncParamList().AllFuncParam() {
			param := v.Visit(paramCtx).(FuncParam)
			fn.Params = append(fn.Params, &param)
		}
	}

	if ctx.TypeAnnot() != nil {
		if tr, ok := v.Visit(ctx.TypeAnnot()).(*TypeRef); ok {
			fn.ReturnType = tr
		}
	}

	if ctx.ExternMapping() != nil {
		if mapping, ok := v.Visit(ctx.ExternMapping()).(*ExternMapping); ok {
			fn.Mapping = mapping
		}
	}

	return fn
}

func (v *HirVisitor) VisitExternVar(ctx *parser.ExternVarContext) any {
	ev := &ExternVar{
		node:    v.mkNode(ctx),
		docBase: docBase{doc: v.docCommentBefore(ctx)},
		IsConst: ctx.CONST() != nil,
	}

	evname, evspan := v.softIdNameAndSpan(ctx.SoftId())
	ev.Name = NameRef{Name: evname, Sym: 0, Span: evspan}

	if ctx.TypeAnnot() != nil {
		if tr, ok := v.Visit(ctx.TypeAnnot()).(*TypeRef); ok {
			ev.Type = tr
		}
	}

	if ctx.ExternMapping() != nil {
		if mapping, ok := v.Visit(ctx.ExternMapping()).(*ExternMapping); ok {
			ev.Mapping = mapping
		}
	}

	return ev
}

func (v *HirVisitor) VisitSimpleExternMapping(ctx *parser.SimpleExternMappingContext) any {
	simple := unquoteString(ctx.STRING_LITERAL().GetText())
	return &ExternMapping{
		Simple: &simple,
	}
}

func (v *HirVisitor) VisitSharedExternMapping(ctx *parser.SharedExternMappingContext) any {
	mapping := &ExternMapping{
		Shared: make(map[SideKind]*SideMapping),
	}

	for _, sideMappingCtx := range ctx.AllExternSideMapping() {
		sideMapping := v.Visit(sideMappingCtx).(*struct {
			side    SideKind
			mapping *SideMapping
		})
		mapping.Shared[sideMapping.side] = sideMapping.mapping
	}

	return mapping
}

func (v *HirVisitor) VisitExternSideMapping(ctx *parser.ExternSideMappingContext) any {
	side := v.Visit(ctx.ExternSide()).(SideKind)

	literals := ctx.AllSTRING_LITERAL()
	var nativeFunc string
	var module *string
	var version string

	switch len(literals) {
	case 2:
		raw := unquoteString(literals[0].GetText())
		modulePath, ver := splitExternModuleSpec(raw)
		module = &modulePath
		version = ver
		nativeFunc = unquoteString(literals[1].GetText())
	case 1:
		nativeFunc = unquoteString(literals[0].GetText())
	default:
		nativeFunc = ""
	}

	sideMapping := &SideMapping{
		NativeFunc: nativeFunc,
		Module:     module,
		Version:    version,
	}

	return &struct {
		side    SideKind
		mapping *SideMapping
	}{
		side:    side,
		mapping: sideMapping,
	}
}

func (v *HirVisitor) VisitExternSide(ctx *parser.ExternSideContext) any {
	if ctx.SIDE_FRONTEND() != nil {
		return SideFrontend
	}
	if ctx.SIDE_BACKEND() != nil {
		return SideBackend
	}
	return SideUnknown
}

func (v *HirVisitor) VisitEnumDeclStmt(ctx *parser.EnumDeclStmtContext) any {
	st := &EnumDeclStmt{
		node:    v.mkNode(ctx),
		docBase: docBase{doc: v.docCommentBefore(ctx)},
	}

	nameTok := ctx.ID().GetSymbol()
	st.Name = NameRef{
		Name: nameTok.GetText(),
		Sym:  0,
		Span: v.spanFromTok(nameTok),
	}

	// Parse payload fields if present
	if payloadDef := ctx.EnumPayloadDef(); payloadDef != nil {
		for _, fieldCtx := range payloadDef.AllEnumFieldDef() {
			field := v.Visit(fieldCtx).(*EnumFieldDef)
			st.Fields = append(st.Fields, field)
		}
	}

	// Parse cases and methods from body
	if bodyCtx := ctx.EnumBody(); bodyCtx != nil {
		for _, caseCtx := range bodyCtx.AllEnumCase() {
			enumCase := v.Visit(caseCtx).(*EnumCase)
			st.Cases = append(st.Cases, enumCase)
		}

		for _, methodCtx := range bodyCtx.AllEnumMethod() {
			method := v.Visit(methodCtx).(*FuncDeclStmt)
			st.Methods = append(st.Methods, method)
		}
	}

	return st
}

func (v *HirVisitor) VisitEnumPayloadDef(ctx *parser.EnumPayloadDefContext) any {
	// This is handled in VisitEnumDeclStmt
	return nil
}

func (v *HirVisitor) VisitEnumFieldDef(ctx *parser.EnumFieldDefContext) any {
	field := &EnumFieldDef{
		node: v.mkNode(ctx),
	}

	nameTok := ctx.ID().GetSymbol()
	field.Name = NameRef{
		Name: nameTok.GetText(),
		Sym:  0,
		Span: v.spanFromTok(nameTok),
	}

	if ctx.TypeAnnot() != nil {
		if tr, ok := v.Visit(ctx.TypeAnnot()).(*TypeRef); ok {
			field.Type = tr
		}
	}

	if ctx.Expr() != nil {
		field.Default = v.Visit(ctx.Expr()).(Expr)
	}

	return field
}

func (v *HirVisitor) VisitEnumBody(ctx *parser.EnumBodyContext) any {
	// This is handled in VisitEnumDeclStmt
	return nil
}

func (v *HirVisitor) VisitEnumCase(ctx *parser.EnumCaseContext) any {
	c := &EnumCase{
		node: v.mkNode(ctx),
	}

	nameTok := ctx.ID().GetSymbol()
	c.Name = NameRef{
		Name: nameTok.GetText(),
		Sym:  0,
		Span: v.spanFromTok(nameTok),
	}

	// Parse case arguments if present
	if argsCtx := ctx.EnumCaseArgs(); argsCtx != nil {
		for _, exprCtx := range argsCtx.AllExpr() {
			arg := v.Visit(exprCtx).(Expr)
			c.Args = append(c.Args, arg)
		}
	}

	// Parse explicit value if present
	if ctx.INT_LITERAL() != nil {
		val, _ := parseIntLiteral(ctx.INT_LITERAL().GetText())
		c.Value = &val
	}

	return c
}

func (v *HirVisitor) VisitEnumCaseArgs(ctx *parser.EnumCaseArgsContext) any {
	// This is handled in VisitEnumCase
	return nil
}

func (v *HirVisitor) VisitEnumMethod(ctx *parser.EnumMethodContext) any {
	st := &FuncDeclStmt{
		node: v.mkNode(ctx),
	}

	mname, mspan := v.softIdNameAndSpan(ctx.SoftId())
	st.Name = NameRef{Name: mname, Sym: 0, Span: mspan}

	if ctx.FuncParamList() != nil {
		for _, paramCtx := range ctx.FuncParamList().AllFuncParam() {
			param := v.Visit(paramCtx).(FuncParam)
			st.Params = append(st.Params, &param)
		}
	}

	if ctx.TypeAnnot() != nil {
		if tr, ok := v.Visit(ctx.TypeAnnot()).(*TypeRef); ok {
			st.ReturnType = tr
		}
	}

	st.Body = v.Visit(ctx.Block()).(*BlockStmt)

	return st
}

func (v *HirVisitor) VisitIfStmt(ctx *parser.IfStmtContext) any {
	cond := v.visitExpr(ctx.Expr())
	thenBlock := v.visitBlock(ctx.Block())

	var elseIfBranches []ElseIfBranch
	var elseBlock *BlockStmt

	for _, eibCtx := range ctx.AllElseIfBranch() {
		elseIfBranches = append(elseIfBranches, ElseIfBranch{
			Cond: v.visitExpr(eibCtx.Expr()),
			Then: v.visitBlock(eibCtx.Block()),
		})
	}

	if ebCtx := ctx.ElseBranch(); ebCtx != nil {
		elseBlock = v.visitBlock(ebCtx.Block())
	}

	return &IfStmt{
		node:    v.mkNode(ctx),
		Cond:    cond,
		Then:    thenBlock,
		ElseIfs: elseIfBranches,
		Else:    elseBlock,
	}
}

func (v *HirVisitor) VisitSwitchStmt(ctx *parser.SwitchStmtContext) any {
	cond := v.Visit(ctx.Expr()).(Expr)

	var cases []SwitchCase
	for _, caseCtx := range ctx.AllSwitchCase() {
		var caseExprs []Expr
		for _, exprCtx := range caseCtx.AllExpr() {
			caseExprs = append(caseExprs, v.Visit(exprCtx).(Expr))
		}

		var caseStmts []Stmt
		for _, stmtCtx := range caseCtx.AllStmt() {
			st := v.Visit(stmtCtx)
			if st != nil {
				caseStmts = append(caseStmts, st.(Stmt))
			}
		}

		cases = append(cases, SwitchCase{
			Values: caseExprs,
			Stmts:  caseStmts,
		})
	}

	var defaultStmts []Stmt
	if defCtx := ctx.DefaultCase(); defCtx != nil {
		for _, stmtCtx := range defCtx.AllStmt() {
			st := v.Visit(stmtCtx)
			if st != nil {
				defaultStmts = append(defaultStmts, st.(Stmt))
			}
		}
	}

	return &SwitchStmt{
		node:    v.mkNode(ctx),
		Expr:    cond,
		Cases:   cases,
		Default: defaultStmts,
	}
}

func (v *HirVisitor) VisitBreakStmt(ctx *parser.BreakStmtContext) any {
	depth := 1
	if ctx.INT_LITERAL() != nil {
		val, err := parseIntLiteral(ctx.INT_LITERAL().GetText())
		if err != nil || val < 1 {
			v.diag.Report(diag.ErrInvalidControlFlowDepth, v.spanFromTok(ctx.INT_LITERAL().GetSymbol()), ctx.INT_LITERAL().GetText())
			return nil
		}
		depth = int(val)
	}

	return &BreakStmt{
		node:  v.mkNode(ctx),
		Depth: depth,
	}
}

func (v *HirVisitor) VisitContinueStmt(ctx *parser.ContinueStmtContext) any {
	depth := 1
	if ctx.INT_LITERAL() != nil {
		val, err := parseIntLiteral(ctx.INT_LITERAL().GetText())
		if err != nil || val < 1 {
			v.diag.Report(diag.ErrInvalidControlFlowDepth, v.spanFromTok(ctx.INT_LITERAL().GetSymbol()), ctx.INT_LITERAL().GetText())
			return nil
		}
		depth = int(val)
	}

	return &ContinueStmt{
		node:  v.mkNode(ctx),
		Depth: depth,
	}
}

func (v *HirVisitor) VisitReturnStmt(ctx *parser.ReturnStmtContext) any {
	st := &ReturnStmt{
		node: v.mkNode(ctx),
	}

	// Handle multiple return expressions
	for _, exprCtx := range ctx.AllExpr() {
		expr := v.Visit(exprCtx).(Expr)
		st.Results = append(st.Results, expr)
	}

	return st
}

func (v *HirVisitor) VisitGuardStmt(ctx *parser.GuardStmtContext) any {
	cond := v.Visit(ctx.Expr()).(Expr)

	st := &GuardStmt{
		node: v.mkNode(ctx),
		Cond: cond,
	}

	// Handle multiple return expressions
	if ctx.GuardReturn() != nil {
		for _, exprCtx := range ctx.GuardReturn().AllExpr() {
			expr := v.Visit(exprCtx).(Expr)
			st.Returns = append(st.Returns, expr)
		}
	}

	return st
}

func (v *HirVisitor) VisitForStmt(ctx *parser.ForStmtContext) any {
	forSt := &ForStmt{
		node:     v.mkNode(ctx),
		CondType: ForCondInfinite,
	}

	if fc := ctx.ForCondition(); fc != nil {
		if fintc := fc.ForIntCondition(); fintc != nil {
			forSt.CondType = ForCondInt
			forSt.CondInt = v.Visit(fintc).(*ForCondIntDecl)
		} else if finc := fc.ForInCondition(); finc != nil {
			forSt.CondType = ForCondIn
			forSt.CondIn = v.Visit(finc).(*ForCondInDecl)
		} else if frc := fc.ForRangeCondition(); frc != nil {
			forSt.CondType = ForCondRange
			forSt.CondRange = v.Visit(frc).(*ForCondRangeDecl)
		}
	}

	forSt.Body = v.Visit(ctx.Block()).(*BlockStmt)

	return forSt
}

func (v *HirVisitor) VisitForIntCondition(ctx *parser.ForIntConditionContext) any {
	var init *VarDeclStmt
	var cond Expr
	var post Expr

	if ctx.ForIntConditionInit() != nil {
		init = &VarDeclStmt{
			node: v.mkNode(ctx),
		}

		nameTok := ctx.ForIntConditionInit().ID().GetSymbol()
		target := VarDeclTarget{
			Name: &NameRef{
				Name: nameTok.GetText(),
				Sym:  0,
				Span: v.spanFromTok(nameTok),
			},
		}

		if ta := ctx.ForIntConditionInit().TypeAnnot(); ta != nil {
			if tr, ok := v.Visit(ta).(*TypeRef); ok {
				target.TypeAnn = tr
			}
		}

		init.Targets = []VarDeclTarget{target}

		if ctx.ForIntConditionInit().Expr() != nil {
			initExpr := v.Visit(ctx.ForIntConditionInit().Expr())
			if expr, ok := initExpr.(Expr); ok {
				init.Init = expr
			}
		}
	}

	if firstExprCtx := ctx.Expr(0); firstExprCtx != nil {
		cond = v.Visit(firstExprCtx).(Expr)
	}

	if secondExprCtx := ctx.Expr(1); secondExprCtx != nil {
		post = v.Visit(secondExprCtx).(Expr)
	}

	return &ForCondIntDecl{
		Init: init,
		Cond: cond,
		Post: post,
	}
}

func (v *HirVisitor) VisitForInCondition(ctx *parser.ForInConditionContext) any {
	targets := ctx.AllForInTarget()

	targetName := func(t parser.IForInTargetContext) NameRef {
		var tok antlr.Token
		if t.ID() != nil {
			tok = t.ID().GetSymbol()
		} else {
			tok = t.GetStart()
		}
		return NameRef{
			Name: tok.GetText(),
			Sym:  0,
			Span: v.spanFromTok(tok),
		}
	}

	firstVarName := targetName(targets[0])

	var secondVarName *NameRef
	if len(targets) > 1 {
		n := targetName(targets[1])
		secondVarName = &n
	}

	var thirdVarName *NameRef
	if len(targets) > 2 {
		n := targetName(targets[2])
		thirdVarName = &n
	}

	iterableExpr := v.Visit(ctx.Expr()).(Expr)

	return &ForCondInDecl{
		InFirstVar:  firstVarName,
		InSecondVar: secondVarName,
		InThirdVar:  thirdVarName,
		IterExpr:    iterableExpr,
	}
}

func (v *HirVisitor) VisitForRangeCondition(ctx *parser.ForRangeConditionContext) any {
	firstVarTok := ctx.ID().GetSymbol()
	varName := NameRef{
		Name: firstVarTok.GetText(),
		Sym:  0,
		Span: v.spanFromTok(firstVarTok),
	}

	rangeStart := v.Visit(ctx.Expr(0)).(Expr)
	rangeEnd := v.Visit(ctx.Expr(1)).(Expr)

	return &ForCondRangeDecl{
		RangeVar:   varName,
		RangeStart: rangeStart,
		RangeEnd:   rangeEnd,
	}
}

func (v *HirVisitor) VisitWhileStmt(ctx *parser.WhileStmtContext) any {
	cond := v.Visit(ctx.Expr()).(Expr)
	body := v.Visit(ctx.Block()).(*BlockStmt)

	return &WhileStmt{
		node: v.mkNode(ctx),
		Cond: cond,
		Body: body,
	}
}

func (v *HirVisitor) VisitPrefixUnaryExprStmt(ctx *parser.PrefixUnaryExprStmtContext) any {
	op := OpUnknown
	if ctx.INC() != nil {
		op = OpInc
	} else if ctx.DEC() != nil {
		op = OpDec
	} else {
		v.diag.Report(diag.ErrInvalidOperator, v.spanFromCtx(ctx), ctx.GetText())
		return nil
	}

	name, span := v.softIdNameAndSpan(ctx.SoftId())
	expr := &PrefixUnaryExpr{
		node:     v.mkNode(ctx),
		exprBase: exprBase{},
		Op:       op,
		Expr: &VarRef{
			node:     v.mkNode(ctx),
			exprBase: exprBase{},
			Ref:      NameRef{Name: name, Sym: 0, Span: span},
		},
	}

	return &ExprStmt{
		node: v.mkNode(ctx),
		Expr: expr,
	}
}

func (v *HirVisitor) VisitPostfixUnaryExprStmt(ctx *parser.PostfixUnaryExprStmtContext) any {
	op := OpUnknown
	if ctx.INC() != nil {
		op = OpInc
	} else if ctx.DEC() != nil {
		op = OpDec
	} else {
		v.diag.Report(diag.ErrInvalidOperator, v.spanFromCtx(ctx), ctx.GetText())
		return nil
	}

	name, span := v.softIdNameAndSpan(ctx.SoftId())
	expr := &PostfixUnaryExpr{
		node:     v.mkNode(ctx),
		exprBase: exprBase{},
		Op:       op,
		Expr: &VarRef{
			node:     v.mkNode(ctx),
			exprBase: exprBase{},
			Ref:      NameRef{Name: name, Sym: 0, Span: span},
		},
	}

	return &ExprStmt{
		node: v.mkNode(ctx),
		Expr: expr,
	}
}

func (v *HirVisitor) VisitFieldAssignmentExprStmt(ctx *parser.FieldAssignmentExprStmtContext) any {
	opStr := ctx.AssignmentOp().GetText()
	op, ok := v.parseOp(opStr)
	if !ok {
		v.diag.Report(diag.ErrInvalidOperator, v.spanFromCtx(ctx.AssignmentOp()), opStr)
		return nil
	}

	ids := ctx.AllSoftId()
	if len(ids) < 2 {
		v.diag.Report(diag.ErrUnexpectedToken, v.spanFromCtx(ctx), ctx.GetText())
		return nil
	}

	recvName, recvSpan := v.softIdNameAndSpan(ids[0])
	stmt := &FieldAssignmentStmt{
		node:     v.mkNode(ctx),
		Receiver: NameRef{Name: recvName, Span: recvSpan},
		Op:       op,
		Value:    v.Visit(ctx.Expr()).(Expr),
	}
	for _, idNode := range ids[1:] {
		fname, fspan := v.softIdNameAndSpan(idNode)
		stmt.Fields = append(stmt.Fields, FieldName{Name: fname, Span: fspan})
	}
	return stmt
}

func (v *HirVisitor) VisitAssignmentExprStmt(ctx *parser.AssignmentExprStmtContext) any {
	opStr := ctx.AssignmentOp().GetText()
	op, ok := v.parseOp(opStr)
	if !ok {
		v.diag.Report(diag.ErrInvalidOperator, v.spanFromCtx(ctx.AssignmentOp()), opStr)
		return nil
	}

	name, span := v.softIdNameAndSpan(ctx.SoftId())
	expr := &AssignmentExpr{
		node:     v.mkNode(ctx),
		exprBase: exprBase{},
		Left:     NameRef{Name: name, Sym: 0, Span: span},
		Op:       op,
		Right:    v.Visit(ctx.Expr()).(Expr),
	}

	return &ExprStmt{
		node: v.mkNode(ctx),
		Expr: expr,
	}
}

// VisitIndexAssignmentExprStmt lowers `recv[idx] = value` (and the compound `recv[idx] += value` forms) to an IndexAssignmentStmt. The three sub-expressions (receiver, index, value) are pulled off the rule's `Expr(N)` accessors in declaration order — receiver first, then index inside the brackets, then the right-hand side after the assignment operator.
func (v *HirVisitor) VisitIndexAssignmentExprStmt(ctx *parser.IndexAssignmentExprStmtContext) any {
	opStr := ctx.AssignmentOp().GetText()
	op, ok := v.parseOp(opStr)
	if !ok {
		v.diag.Report(diag.ErrInvalidOperator, v.spanFromCtx(ctx.AssignmentOp()), opStr)
		return nil
	}

	exprs := ctx.AllExpr()
	if len(exprs) != 3 {
		v.diag.Report(diag.ErrUnexpectedToken, v.spanFromCtx(ctx), ctx.GetText())
		return nil
	}

	recv, _ := v.Visit(exprs[0]).(Expr)
	idx, _ := v.Visit(exprs[1]).(Expr)
	val, _ := v.Visit(exprs[2]).(Expr)

	return &IndexAssignmentStmt{
		node:     v.mkNode(ctx),
		Receiver: recv,
		Index:    idx,
		Op:       op,
		Value:    val,
	}
}

func (v *HirVisitor) VisitMultiAssignmentExprStmt(ctx *parser.MultiAssignmentExprStmtContext) any {
	st := &MultiAssignmentStmt{
		node: v.mkNode(ctx),
	}

	for _, targetCtx := range ctx.AllAssignmentTarget() {
		target := v.Visit(targetCtx)
		if t, ok := target.(AssignmentTarget); ok {
			st.Targets = append(st.Targets, t)
		}
	}

	st.Value = v.Visit(ctx.Expr()).(Expr)

	return st
}

func (v *HirVisitor) VisitAssignmentTarget(ctx *parser.AssignmentTargetContext) any {
	target := AssignmentTarget{}

	if ctx.GetText() == "_" {
		target.Name = nil
	} else if soft := ctx.SoftId(); soft != nil {
		name, span := v.softIdNameAndSpan(soft)
		target.Name = &NameRef{Name: name, Sym: 0, Span: span}
	}

	return target
}

func (v *HirVisitor) VisitFuncCallExprStmt(ctx *parser.FuncCallExprStmtContext) any {
	if ctx.Expr() == nil {
		return &FuncCallExpr{node: v.mkNode(ctx)}
	}
	visited := v.Visit(ctx.Expr())
	callee, _ := visited.(Expr)
	fc := &FuncCallExpr{
		node:     v.mkNode(ctx),
		exprBase: exprBase{},
		Callee:   callee,
	}

	if ctx.FuncArgList() != nil {
		for _, argCtx := range ctx.FuncArgList().AllFuncArg() {
			var argName string
			if argCtx.SoftId() != nil {
				argName = argCtx.SoftId().GetText()
			}
			exprCtx := argCtx.Expr()
			if exprCtx == nil {
				continue
			}
			argExpr, ok := v.Visit(exprCtx).(Expr)
			if !ok || argExpr == nil {
				continue
			}
			fc.Args = append(fc.Args, FuncCallArg{
				Name: argName,
				Expr: argExpr,
			})
		}
	}

	return &ExprStmt{
		node: v.mkNode(ctx),
		Expr: fc,
	}
}

// --- Expressions ---

func (v *HirVisitor) VisitWhenExpr(ctx *parser.WhenExprContext) any {
	expr := v.Visit(ctx.Expr()).(Expr)

	var cases []WhenCase
	for _, caseCtx := range ctx.AllWhenCase() {
		var caseExprs []Expr
		var caseResult Expr
		exprCount := len(caseCtx.AllExpr())
		for idx, exprCtx := range caseCtx.AllExpr() {
			if idx+1 == exprCount { // last expression is the result
				caseResult = v.Visit(exprCtx).(Expr)
			} else {
				caseExprs = append(caseExprs, v.Visit(exprCtx).(Expr))
			}
		}

		cases = append(cases, WhenCase{
			Values: caseExprs,
			Then:   caseResult,
		})
	}

	var elseResult Expr
	if ebCtx := ctx.DefaultWhenCase(); ebCtx != nil {
		elseResult = v.Visit(ebCtx.Expr()).(Expr)
	}

	return &WhenExpr{
		node:    v.mkNode(ctx),
		Expr:    expr,
		Cases:   cases,
		Default: elseResult,
	}
}

func (v *HirVisitor) VisitUnaryExpr(ctx *parser.UnaryExprContext) any {
	opStr := ctx.UnaryOp().GetText()
	op, ok := v.parseOp(opStr)
	if !ok {
		v.diag.Report(diag.ErrInvalidOperator, v.spanFromCtx(ctx.UnaryOp()), opStr)
		return nil
	}

	return &UnaryExpr{
		node:     v.mkNode(ctx),
		exprBase: exprBase{},
		Op:       op,
		Expr:     v.Visit(ctx.Expr()).(Expr),
	}
}

func (v *HirVisitor) VisitPrefixUnaryExpr(ctx *parser.PrefixUnaryExprContext) any {
	op := OpUnknown
	if ctx.INC() != nil {
		op = OpInc
	} else if ctx.DEC() != nil {
		op = OpDec
	} else {
		v.diag.Report(diag.ErrInvalidOperator, v.spanFromCtx(ctx), ctx.GetText())
		return nil
	}

	return &PrefixUnaryExpr{
		node:     v.mkNode(ctx),
		exprBase: exprBase{},
		Op:       op,
		Expr:     v.Visit(ctx.Expr()).(Expr),
	}
}

func (v *HirVisitor) VisitPostfixUnaryExpr(ctx *parser.PostfixUnaryExprContext) any {
	op := OpUnknown
	if ctx.INC() != nil {
		op = OpInc
	} else if ctx.DEC() != nil {
		op = OpDec
	} else {
		v.diag.Report(diag.ErrInvalidOperator, v.spanFromCtx(ctx), ctx.GetText())
		return nil
	}

	return &PostfixUnaryExpr{
		node:     v.mkNode(ctx),
		exprBase: exprBase{},
		Op:       op,
		Expr:     v.Visit(ctx.Expr()).(Expr),
	}
}

func (v *HirVisitor) buildBinaryExpr(ctx antlr.ParserRuleContext, leftCtx, rightCtx antlr.ParserRuleContext) any {
	opChild := ctx.GetChild(1)
	tn, ok := opChild.(antlr.TerminalNode)
	if !ok {
		v.diag.Report(diag.ErrInvalidOperator, v.spanFromCtx(ctx), ctx.GetText())
		return nil
	}
	opTok := tn.GetSymbol()
	op, ok := v.parseOp(opTok.GetText())
	if !ok {
		v.diag.Report(diag.ErrInvalidOperator, v.spanFromTok(opTok), opTok.GetText())
		return nil
	}
	return &BinaryExpr{
		node:     v.mkNode(ctx),
		exprBase: exprBase{},
		Left:     v.Visit(leftCtx).(Expr),
		Op:       op,
		Right:    v.Visit(rightCtx).(Expr),
	}
}

func (v *HirVisitor) VisitMulBinaryExpr(ctx *parser.MulBinaryExprContext) any {
	return v.buildBinaryExpr(ctx, ctx.Expr(0), ctx.Expr(1))
}

func (v *HirVisitor) VisitAddBinaryExpr(ctx *parser.AddBinaryExprContext) any {
	return v.buildBinaryExpr(ctx, ctx.Expr(0), ctx.Expr(1))
}

func (v *HirVisitor) VisitShiftBinaryExpr(ctx *parser.ShiftBinaryExprContext) any {
	return v.buildBinaryExpr(ctx, ctx.Expr(0), ctx.Expr(1))
}

func (v *HirVisitor) VisitCmpBinaryExpr(ctx *parser.CmpBinaryExprContext) any {
	return v.buildBinaryExpr(ctx, ctx.Expr(0), ctx.Expr(1))
}

func (v *HirVisitor) VisitEqBinaryExpr(ctx *parser.EqBinaryExprContext) any {
	return v.buildBinaryExpr(ctx, ctx.Expr(0), ctx.Expr(1))
}

func (v *HirVisitor) VisitBitAndBinaryExpr(ctx *parser.BitAndBinaryExprContext) any {
	return v.buildBinaryExpr(ctx, ctx.Expr(0), ctx.Expr(1))
}

func (v *HirVisitor) VisitBitXorBinaryExpr(ctx *parser.BitXorBinaryExprContext) any {
	return v.buildBinaryExpr(ctx, ctx.Expr(0), ctx.Expr(1))
}

func (v *HirVisitor) VisitBitOrBinaryExpr(ctx *parser.BitOrBinaryExprContext) any {
	return v.buildBinaryExpr(ctx, ctx.Expr(0), ctx.Expr(1))
}

func (v *HirVisitor) VisitLAndBinaryExpr(ctx *parser.LAndBinaryExprContext) any {
	return v.buildBinaryExpr(ctx, ctx.Expr(0), ctx.Expr(1))
}

func (v *HirVisitor) VisitLOrBinaryExpr(ctx *parser.LOrBinaryExprContext) any {
	return v.buildBinaryExpr(ctx, ctx.Expr(0), ctx.Expr(1))
}

func (v *HirVisitor) VisitCoalesceExpr(ctx *parser.CoalesceExprContext) any {
	leftExpr := v.Visit(ctx.Expr(0))
	defaultExpr := v.Visit(ctx.Expr(1))

	if leftExpr == nil || defaultExpr == nil {
		// Debug: one of the expressions failed to parse
		return &CoalesceExpr{
			node:     v.mkNode(ctx),
			exprBase: exprBase{},
			Left:     nil,
			Default:  nil,
		}
	}

	return &CoalesceExpr{
		node:     v.mkNode(ctx),
		exprBase: exprBase{},
		Left:     leftExpr.(Expr),
		Default:  defaultExpr.(Expr),
	}
}

func (v *HirVisitor) VisitTernaryExpr(ctx *parser.TernaryExprContext) any {
	return &TenaryExpr{
		node:     v.mkNode(ctx),
		exprBase: exprBase{},
		Cond:     v.Visit(ctx.Expr(0)).(Expr),
		Then:     v.Visit(ctx.Expr(1)).(Expr),
		Else:     v.Visit(ctx.Expr(2)).(Expr),
	}
}

func (v *HirVisitor) VisitGroupedExpr(ctx *parser.GroupedExprContext) any {
	return &GroupedExpr{
		node:     v.mkNode(ctx),
		exprBase: exprBase{},
		Expr:     v.Visit(ctx.Expr()).(Expr),
	}
}

func (v *HirVisitor) VisitAsExpr(ctx *parser.AsExprContext) any {
	expr := v.visitExpr(ctx.Expr())
	target, _ := v.Visit(ctx.TypeAnnot()).(*TypeRef)
	safe := false
	for _, child := range ctx.GetChildren() {
		if tn, ok := child.(antlr.TerminalNode); ok && tn.GetText() == "?" {
			safe = true
			break
		}
	}
	return &AsExpr{
		node:     v.mkNode(ctx),
		exprBase: exprBase{},
		Expr:     expr,
		Target:   target,
		Safe:     safe,
	}
}

func (v *HirVisitor) VisitOptionUnwrapExpr(ctx *parser.OptionUnwrapExprContext) any {
	return &OptionUnwrapExpr{
		node:     v.mkNode(ctx),
		exprBase: exprBase{},
		Expr:     v.visitExpr(ctx.Expr()),
	}
}

func (v *HirVisitor) VisitIndexExpr(ctx *parser.IndexExprContext) any {
	baseExpr := v.Visit(ctx.Expr(0)).(Expr)
	indexExpr := v.Visit(ctx.Expr(1)).(Expr)
	return &IndexExpr{
		node:     v.mkNode(ctx),
		exprBase: exprBase{},
		Expr:     baseExpr,
		Index:    indexExpr,
	}
}

// VisitSliceRangeExpr lowers `expr[low?:high?]`. Either bound may be absent (`s[:5]`, `s[5:]`, `s[:]`); the corresponding IR field stays `nil` and codegen substitutes 0 / `len(expr)` on the target side.
func (v *HirVisitor) VisitSliceRangeExpr(ctx *parser.SliceRangeExprContext) any {
	exprs := ctx.AllExpr()
	base, _ := v.Visit(exprs[0]).(Expr)
	out := &SliceRangeExpr{node: v.mkNode(ctx), exprBase: exprBase{}, Expr: base}

	sawLow := false
	sawHigh := false
	pastColon := false
	for _, child := range ctx.GetChildren() {
		switch n := child.(type) {
		case antlr.TerminalNode:
			tokText := n.GetText()
			if tokText == ":" {
				pastColon = true
			}
		case parser.IExprContext:
			if n == exprs[0] {
				continue
			}
			val, _ := v.Visit(n).(Expr)
			if pastColon {
				out.High = val
				sawHigh = true
			} else {
				out.Low = val
				sawLow = true
			}
		}
	}
	_ = sawLow
	_ = sawHigh
	return out
}

func (v *HirVisitor) VisitFieldAccessExpr(ctx *parser.FieldAccessExprContext) any {
	base := v.Visit(ctx.Expr()).(Expr)
	softs := ctx.AllSoftId()
	fields := make([]FieldName, 0, len(softs))
	for _, soft := range softs {
		fname, fspan := v.softIdNameAndSpan(soft)
		fields = append(fields, FieldName{Name: fname, Span: fspan})
	}
	return &FieldAccessExpr{node: v.mkNode(ctx), Expr: base, Fields: fields}
}

func (v *HirVisitor) VisitIdExpr(ctx *parser.IdExprContext) any {
	pi := ctx.PkgIdent()
	return &VarRef{
		node:     v.mkNode(ctx),
		exprBase: exprBase{},
		Ref: NameRef{
			Name: pi.GetText(),
			Sym:  0,
			Span: v.spanFromCtx(pi),
		},
	}
}

func (v *HirVisitor) VisitRangeExpr(ctx *parser.RangeExprContext) any {
	startExpr := v.Visit(ctx.Expr(0)).(Expr)
	endExpr := v.Visit(ctx.Expr(1)).(Expr)

	var incExpr Expr
	if incRawExpr := ctx.Expr(2); incRawExpr != nil {
		incExpr = v.Visit(incRawExpr).(Expr)
	}

	return &RangeExpr{
		node:     v.mkNode(ctx),
		exprBase: exprBase{},
		Start:    startExpr,
		End:      endExpr,
		Inc:      incExpr,
	}
}

func (v *HirVisitor) VisitFuncCallExpr(ctx *parser.FuncCallExprContext) any {
	callee := v.Visit(ctx.Expr()).(Expr)
	fc := &FuncCallExpr{
		node:     v.mkNode(ctx),
		exprBase: exprBase{},
		Callee:   callee,
	}

	if ctx.FuncArgList() != nil {
		for _, argCtx := range ctx.FuncArgList().AllFuncArg() {
			var argName string
			if argCtx.SoftId() != nil {
				argName = argCtx.SoftId().GetText()
			}
			argExpr := v.Visit(argCtx.Expr()).(Expr)
			fc.Args = append(fc.Args, FuncCallArg{
				Name: argName,
				Expr: argExpr,
			})
		}
	}

	return fc
}

func (v *HirVisitor) VisitGenericFuncCallExpr(ctx *parser.GenericFuncCallExprContext) any {
	name, span := v.softIdNameAndSpan(ctx.SoftId())
	callee := &VarRef{
		node: v.mkNode(ctx),
		Ref:  NameRef{Name: name, Span: span},
	}
	fc := &FuncCallExpr{
		node:     v.mkNode(ctx),
		exprBase: exprBase{},
		Callee:   callee,
	}
	if ga := ctx.GenericArgs(); ga != nil {
		for _, t := range ga.AllType_() {
			if argRef, ok := v.Visit(t).(*TypeRef); ok {
				fc.TypeArgs = append(fc.TypeArgs, argRef)
			}
		}
	}
	if ctx.FuncArgList() != nil {
		for _, argCtx := range ctx.FuncArgList().AllFuncArg() {
			var argName string
			if argCtx.SoftId() != nil {
				argName = argCtx.SoftId().GetText()
			}
			argExpr := v.Visit(argCtx.Expr()).(Expr)
			fc.Args = append(fc.Args, FuncCallArg{
				Name: argName,
				Expr: argExpr,
			})
		}
	}
	return fc
}

func (v *HirVisitor) VisitGenericFuncCallExprStmt(ctx *parser.GenericFuncCallExprStmtContext) any {
	name, span := v.softIdNameAndSpan(ctx.SoftId())
	callee := &VarRef{
		node: v.mkNode(ctx),
		Ref:  NameRef{Name: name, Span: span},
	}
	fc := &FuncCallExpr{
		node:     v.mkNode(ctx),
		exprBase: exprBase{},
		Callee:   callee,
	}
	if ga := ctx.GenericArgs(); ga != nil {
		for _, t := range ga.AllType_() {
			if argRef, ok := v.Visit(t).(*TypeRef); ok {
				fc.TypeArgs = append(fc.TypeArgs, argRef)
			}
		}
	}
	if ctx.FuncArgList() != nil {
		for _, argCtx := range ctx.FuncArgList().AllFuncArg() {
			var argName string
			if argCtx.SoftId() != nil {
				argName = argCtx.SoftId().GetText()
			}
			argExpr := v.Visit(argCtx.Expr()).(Expr)
			fc.Args = append(fc.Args, FuncCallArg{
				Name: argName,
				Expr: argExpr,
			})
		}
	}
	return &ExprStmt{
		node: v.mkNode(ctx),
		Expr: fc,
	}
}

func (v *HirVisitor) VisitWildcardType(ctx *parser.WildcardTypeContext) any {
	tr := &TypeRef{node: v.mkNode(ctx), Kind: TK_TypeParam, CustomName: "?"}
	return tr
}

func (v *HirVisitor) VisitChanType(ctx *parser.ChanTypeContext) any {
	tr := &TypeRef{node: v.mkNode(ctx), Kind: TK_Chan}
	if t := ctx.Type_(); t != nil {
		if elem, ok := v.Visit(t).(*TypeRef); ok {
			tr.Elem = elem
		}
	}
	return tr
}

func (v *HirVisitor) VisitChanInitExpr(ctx *parser.ChanInitExprContext) any {
	expr := &ChanInitExpr{node: v.mkNode(ctx)}
	if t := ctx.Type_(); t != nil {
		if elem, ok := v.Visit(t).(*TypeRef); ok {
			expr.ElemType = elem
		}
	}
	if ex := ctx.Expr(); ex != nil {
		if e, ok := v.Visit(ex).(Expr); ok {
			expr.Capacity = e
		}
	}
	return expr
}

// buildGenericParamDecl extracts a single generic parameter's name plus its optional `: InterfaceA + InterfaceB` and `with MixinA + MixinB` constraint lists from the parsed generic-param node.
func (v *HirVisitor) buildGenericParamDecl(ctx parser.IGenericParamContext) TypeParamDecl {
	decl := TypeParamDecl{Name: ctx.ID().GetText()}
	hadColon := false
	for i := 0; i < ctx.GetChildCount(); i++ {
		child := ctx.GetChild(i)
		switch tok := child.(type) {
		case antlr.TerminalNode:
			text := tok.GetText()
			if text == ":" {
				hadColon = true
			}
			if text == "with" {
				hadColon = false
			}
		case parser.IQualifiedRefContext:
			ids := tok.AllSoftId()
			ref := NameRef{}
			if len(ids) >= 2 {
				ref.Qualifier = ids[0].GetText()
				ref.Name = ids[1].GetText()
			} else if len(ids) == 1 {
				ref.Name = ids[0].GetText()
			}
			if hadColon {
				decl.ImplementsConstraints = append(decl.ImplementsConstraints, ref)
			} else {
				decl.WithConstraints = append(decl.WithConstraints, ref)
			}
		}
	}
	return decl
}

func (v *HirVisitor) VisitComposableCallExpr(ctx *parser.ComposableCallExprContext) any {
	callee := v.Visit(ctx.Expr()).(Expr)
	cc := &ComposableCallExpr{
		node:     v.mkNode(ctx),
		exprBase: exprBase{},
		Callee:   callee,
	}
	if ctx.FuncArgList() != nil {
		for _, argCtx := range ctx.FuncArgList().AllFuncArg() {
			var argName string
			if argCtx.SoftId() != nil {
				argName = argCtx.SoftId().GetText()
			}
			argExpr := v.Visit(argCtx.Expr()).(Expr)
			cc.Args = append(cc.Args, FuncCallArg{Name: argName, Expr: argExpr})
		}
	}
	for _, childCtx := range ctx.AllComposableChild() {
		child, ok := v.buildComposableChild(childCtx)
		if !ok {
			continue
		}
		cc.Children = append(cc.Children, child)
	}
	return cc
}

func (v *HirVisitor) buildComposableChild(ctx parser.IComposableChildContext) (ComposableChild, bool) {
	switch c := ctx.(type) {
	case *parser.ComposableBareChildContext:
		ids := c.AllSoftId()
		if len(ids) == 0 {
			return ComposableChild{}, false
		}
		var qualifier, calleeName string
		var calleeSpan diag.TextSpan
		if len(ids) >= 2 {
			qualifier = ids[0].GetText()
			calleeName, calleeSpan = v.softIdNameAndSpan(ids[1])
		} else {
			calleeName, calleeSpan = v.softIdNameAndSpan(ids[0])
		}
		callee := buildBareComposableCallee(v, c, qualifier, calleeName, calleeSpan)
		cc := &ComposableCallExpr{
			node:     v.mkNode(c),
			exprBase: exprBase{},
			Callee:   callee,
		}
		for _, childCtx := range c.AllComposableChild() {
			child, ok := v.buildComposableChild(childCtx)
			if !ok {
				continue
			}
			cc.Children = append(cc.Children, child)
		}
		return ComposableChild{Kind: ComposableChildExpr, Expr: cc}, true
	case *parser.ComposableExprChildContext:
		if e := c.Expr(); e != nil {
			if expr, ok := v.Visit(e).(Expr); ok {
				return ComposableChild{Kind: ComposableChildExpr, Expr: expr}, true
			}
		}
		return ComposableChild{}, false
	case *parser.ComposableIfChildContext:
		if s := c.IfStmt(); s != nil {
			if st, ok := v.Visit(s).(Stmt); ok {
				return ComposableChild{Kind: ComposableChildIf, Stmt: st}, true
			}
		}
	case *parser.ComposableForChildContext:
		if s := c.ForStmt(); s != nil {
			if st, ok := v.Visit(s).(Stmt); ok {
				return ComposableChild{Kind: ComposableChildFor, Stmt: st}, true
			}
		}
	case *parser.ComposableWhileChildContext:
		if s := c.WhileStmt(); s != nil {
			if st, ok := v.Visit(s).(Stmt); ok {
				return ComposableChild{Kind: ComposableChildWhile, Stmt: st}, true
			}
		}
	case *parser.ComposableSwitchChildContext:
		if s := c.SwitchStmt(); s != nil {
			if st, ok := v.Visit(s).(Stmt); ok {
				return ComposableChild{Kind: ComposableChildSwitch, Stmt: st}, true
			}
		}
	}
	return ComposableChild{}, false
}

// buildBareComposableCallee constructs the callee expression for the bare composable form `Type { ... }` (no parenthesised argument list). The callee is rendered as either a plain IdExpr (`H1`) or a FieldAccessExpr (`dom.H1`) so that downstream name resolution and synthesisableComposableCallType treat it identically to the parenthesised form. Takes pre-extracted name+span (rather than an `antlr.Token`) because the softId grammar rule may match a keyword-like token whose `TerminalNode` accessor returns nil.
func buildBareComposableCallee(v *HirVisitor, ctx antlr.ParserRuleContext, qualifier, name string, span diag.TextSpan) Expr {
	nameRef := NameRef{Name: name, Span: span}
	if qualifier == "" {
		return &VarRef{
			node:     node{id: v.nid(), span: span},
			exprBase: exprBase{},
			Ref:      nameRef,
		}
	}
	return &VarRef{
		node:     node{id: v.nid(), span: span},
		exprBase: exprBase{},
		Ref:      NameRef{Name: name, Qualifier: qualifier, Span: span},
	}
}

func (v *HirVisitor) VisitSessionExpr(ctx *parser.SessionExprContext) any {
	return &SessionExpr{node: v.mkNode(ctx)}
}

func (v *HirVisitor) VisitNewInstanceExpr(ctx *parser.NewInstanceExprContext) any {
	expr := &NewExpr{node: v.mkNode(ctx), exprBase: exprBase{}}

	head := ctx.PkgIdent()
	if tail := ctx.SoftId(); tail != nil {
		expr.Qualifier = head.GetText()
		tname, tspan := v.softIdNameAndSpan(tail)
		expr.TypeName = NameRef{Name: tname, Span: tspan}
	} else {
		expr.TypeName = NameRef{Name: head.GetText(), Span: v.spanFromCtx(head)}
	}

	if ga := ctx.GenericArgs(); ga != nil {
		for _, t := range ga.AllType_() {
			if tr, ok := v.Visit(t).(*TypeRef); ok {
				expr.TypeArgs = append(expr.TypeArgs, tr)
			}
		}
	}

	if ctx.FuncArgList() != nil {
		for _, argCtx := range ctx.FuncArgList().AllFuncArg() {
			var argName string
			if argCtx.SoftId() != nil {
				argName = argCtx.SoftId().GetText()
			}
			argExpr := v.Visit(argCtx.Expr()).(Expr)
			expr.Args = append(expr.Args, FuncCallArg{Name: argName, Expr: argExpr})
		}
	}
	return expr
}

func (v *HirVisitor) VisitFuncLiteralExpr(ctx *parser.FuncLiteralExprContext) any {
	fl := &FuncLitExpr{
		node:     v.mkNode(ctx),
		exprBase: exprBase{},
	}

	// Parameters
	if pl := ctx.FuncParamList(); pl != nil {
		for _, paramCtx := range pl.AllFuncParam() {
			param := v.Visit(paramCtx).(FuncParam)
			fl.Params = append(fl.Params, &param)
		}
	}

	// Return type
	if ctx.TypeAnnot() != nil {
		if tr, ok := v.Visit(ctx.TypeAnnot()).(*TypeRef); ok {
			fl.ReturnType = tr
		}
	}

	// Body
	if ctx.Block() != nil {
		body := v.Visit(ctx.Block()).(*BlockStmt)
		fl.Body = body
	}

	return fl
}

func (v *HirVisitor) VisitLitExpr(ctx *parser.LitExprContext) any {
	return v.Visit(ctx.Literal()).(Expr)
}

func (v *HirVisitor) VisitLiteral(ctx *parser.LiteralContext) any {
	switch {
	case ctx.INT_LITERAL() != nil:
		tok := ctx.INT_LITERAL().GetSymbol()
		txt := tok.GetText()
		val, err := parseIntLiteral(txt)
		if err != nil {
			v.diag.Report(diag.ErrInvalidLiteral, v.spanFromTok(tok), txt, "int")
			return &LitInt{node: v.mkNode(ctx), exprBase: exprBase{}, Value: 0}
		}
		return &LitInt{node: v.mkNode(ctx), exprBase: exprBase{}, Value: val}
	case ctx.FLOAT_LITERAL() != nil:
		tok := ctx.FLOAT_LITERAL().GetSymbol()
		txt := tok.GetText()
		val, err := parseFloatLiteral(txt)
		if err != nil {
			v.diag.Report(diag.ErrInvalidLiteral, v.spanFromTok(tok), txt, "float")
			return &LitFloat{node: v.mkNode(ctx), exprBase: exprBase{}, Value: 0.0}
		}
		return &LitFloat{node: v.mkNode(ctx), exprBase: exprBase{}, Value: val}
	case ctx.STRING_LITERAL() != nil:
		tok := ctx.STRING_LITERAL().GetSymbol()
		return &LitString{node: v.mkNode(ctx), exprBase: exprBase{}, Value: unquoteString(tok.GetText())}
	case ctx.TEMPLATE_STRING() != nil:
		return v.buildTemplateString(ctx)
	case ctx.CHAR_LITERAL() != nil:
		tok := ctx.CHAR_LITERAL().GetSymbol()
		return &LitChar{node: v.mkNode(ctx), exprBase: exprBase{}, Value: unquoteChar(tok.GetText())}
	case ctx.TRUE() != nil:
		return &LitBool{node: v.mkNode(ctx), exprBase: exprBase{}, Value: true}
	case ctx.FALSE() != nil:
		return &LitBool{node: v.mkNode(ctx), exprBase: exprBase{}, Value: false}
	case ctx.NONE() != nil:
		return &LitNone{node: v.mkNode(ctx), exprBase: exprBase{}}
	case ctx.Array_literal() != nil:
		return v.Visit(ctx.Array_literal()).(Expr)
	case ctx.Map_literal() != nil:
		return v.Visit(ctx.Map_literal()).(Expr)
	case ctx.Tuple_literal() != nil:
		return v.Visit(ctx.Tuple_literal()).(Expr)
	default:
		return nil
	}
}

func (v *HirVisitor) VisitArray_literal(ctx *parser.Array_literalContext) any {
	al := &ArrayLiteral{node: v.mkNode(ctx)}
	for _, ex := range ctx.AllExpr() {
		al.Elems = append(al.Elems, v.Visit(ex).(Expr))
	}
	return al
}

func (v *HirVisitor) VisitMap_literal(ctx *parser.Map_literalContext) any {
	ml := &MapLiteral{node: v.mkNode(ctx)}
	exprs := ctx.AllExpr()
	for i := 0; i+1 < len(exprs); i += 2 {
		k, _ := v.Visit(exprs[i]).(Expr)
		val, _ := v.Visit(exprs[i+1]).(Expr)
		if k == nil || val == nil {
			continue
		}
		ml.Entries = append(ml.Entries, MapEntry{Key: k, Value: val})
	}
	return ml
}

func (v *HirVisitor) VisitTuple_literal(ctx *parser.Tuple_literalContext) any {
	tl := &TupleLiteral{node: v.mkNode(ctx)}
	for _, ex := range ctx.AllExpr() {
		tl.Elems = append(tl.Elems, v.Visit(ex).(Expr))
	}
	return tl
}

// --- Types ---

func (v *HirVisitor) VisitTypeAnnot(ctx *parser.TypeAnnotContext) any {
	// ':'? type
	r := v.Visit(ctx.Type_())
	if r == nil {
		println("DEBUG nil TypeAnnot at", ctx.GetStart().GetLine(), ":", ctx.GetStart().GetColumn(), "text=", ctx.GetText())
		return &TypeRef{node: v.mkNode(ctx), Kind: TK_PrimitiveAny}
	}
	return r.(*TypeRef)
}

func (v *HirVisitor) VisitType(ctx *parser.TypeContext) any {
	switch {
	case ctx.PrimitiveType() != nil:
		return v.Visit(ctx.PrimitiveType()).(*TypeRef)
	case ctx.OptionType() != nil:
		return v.Visit(ctx.OptionType()).(*TypeRef)
	case ctx.SliceType() != nil:
		return v.Visit(ctx.SliceType()).(*TypeRef)
	case ctx.ArrayType() != nil:
		return v.Visit(ctx.ArrayType()).(*TypeRef)
	case ctx.MapType() != nil:
		return v.Visit(ctx.MapType()).(*TypeRef)
	case ctx.TupleType() != nil:
		return v.Visit(ctx.TupleType()).(*TypeRef)
	case ctx.FuncType() != nil:
		return v.Visit(ctx.FuncType()).(*TypeRef)
	case ctx.CustomType() != nil:
		return v.Visit(ctx.CustomType()).(*TypeRef)
	default:
		return &TypeRef{node: v.mkNode(ctx), Kind: TK_Tuple} // fallback, sollte nicht passieren
	}
}

func (v *HirVisitor) VisitFuncType(ctx *parser.FuncTypeContext) any {
	tr := &TypeRef{
		node: v.mkNode(ctx),
		Kind: TK_Function,
	}
	if list := ctx.FuncTypeParamList(); list != nil {
		if items, ok := v.Visit(list).([]FuncTypeParamRef); ok {
			tr.FuncParams = items
		}
	}
	if ret := ctx.TypeAnnot(); ret != nil {
		if rtRef, ok := v.Visit(ret).(*TypeRef); ok {
			tr.FuncReturn = rtRef
		}
	}
	return tr
}

func (v *HirVisitor) VisitFuncTypeParamList(ctx *parser.FuncTypeParamListContext) any {
	var out []FuncTypeParamRef
	for _, item := range ctx.AllFuncTypeParam() {
		if param, ok := v.Visit(item).(FuncTypeParamRef); ok {
			out = append(out, param)
		}
	}
	return out
}

func (v *HirVisitor) VisitFuncTypeParam(ctx *parser.FuncTypeParamContext) any {
	param := FuncTypeParamRef{}
	if id := ctx.ID(); id != nil {
		param.Name = id.GetText()
	}
	if t := ctx.Type_(); t != nil {
		if tr, ok := v.Visit(t).(*TypeRef); ok {
			param.Type = tr
		}
	}
	return param
}

func (v *HirVisitor) VisitCustomType(ctx *parser.CustomTypeContext) any {
	tr := &TypeRef{
		node: v.mkNode(ctx),
		Kind: TK_Enum, // Will be verified during type resolution
	}
	head := ctx.PkgIdent().GetText()
	if tail := ctx.ID(); tail != nil {
		tr.CustomQualifier = head
		tr.CustomName = tail.GetText()
	} else {
		tr.CustomName = head
	}
	if ga := ctx.GenericArgs(); ga != nil {
		for _, t := range ga.AllType_() {
			if argRef, ok := v.Visit(t).(*TypeRef); ok {
				tr.TypeArgs = append(tr.TypeArgs, argRef)
			}
		}
	}
	return tr
}

func (v *HirVisitor) VisitPrimitiveType(ctx *parser.PrimitiveTypeContext) any {
	tr := &TypeRef{node: v.mkNode(ctx)}
	switch {
	case ctx.INT() != nil:
		tr.Kind = TK_PrimitiveInt
	case ctx.FLOAT() != nil:
		tr.Kind = TK_PrimitiveFloat
	case ctx.STRING() != nil:
		tr.Kind = TK_PrimitiveString
	case ctx.CHAR() != nil:
		tr.Kind = TK_PrimitiveChar
	case ctx.BOOL() != nil:
		tr.Kind = TK_PrimitiveBool
	case ctx.ANY() != nil:
		tr.Kind = TK_PrimitiveAny
	case ctx.BYTE() != nil:
		tr.Kind = TK_PrimitiveByte
	}
	return tr
}

func (v *HirVisitor) VisitOptionType(ctx *parser.OptionTypeContext) any {
	inner := v.Visit(ctx.Type_()).(*TypeRef)
	return &TypeRef{
		node: v.mkNode(ctx),
		Kind: TK_Option,
		Elem: inner,
	}
}

func (v *HirVisitor) VisitSliceType(ctx *parser.SliceTypeContext) any {
	pairs := len(ctx.AllLBRACK())
	if pairs < 1 {
		pairs = 1
	}
	tr := v.Visit(ctx.Type_()).(*TypeRef)
	for i := 0; i < pairs; i++ {
		tr = &TypeRef{
			node: v.mkNode(ctx),
			Kind: TK_Slice,
			Elem: tr,
		}
	}
	return tr
}

func (v *HirVisitor) VisitArrayType(ctx *parser.ArrayTypeContext) any {
	inner := v.Visit(ctx.Type_()).(*TypeRef)
	dim, _ := parseIntLiteral(ctx.INT_LITERAL().GetText())
	return &TypeRef{
		node: v.mkNode(ctx),
		Kind: TK_Array,
		Elem: inner,
		Dim:  dim,
	}
}

func (v *HirVisitor) VisitMapType(ctx *parser.MapTypeContext) any {
	key := v.Visit(ctx.Type_(0)).(*TypeRef)
	val := v.Visit(ctx.Type_(1)).(*TypeRef)
	return &TypeRef{
		node:  v.mkNode(ctx),
		Kind:  TK_Map,
		Key:   key,
		Value: val,
	}
}

func (v *HirVisitor) VisitTupleType(ctx *parser.TupleTypeContext) any {
	tr := &TypeRef{node: v.mkNode(ctx), Kind: TK_Tuple}
	for _, tf := range ctx.AllTupleField() {
		fr := v.Visit(tf).(TupleFieldRef)
		tr.Tuple = append(tr.Tuple, fr)
	}
	return tr
}

func (v *HirVisitor) VisitTupleField(ctx *parser.TupleFieldContext) any {
	var name string
	if id := ctx.ID(); id != nil {
		name = id.GetText()
	}
	ty := v.Visit(ctx.Type_()).(*TypeRef)
	return TupleFieldRef{Name: name, Type: ty}
}

func (v *HirVisitor) parseOp(opStr string) (Op, bool) {
	switch opStr {
	case "+":
		return OpAdd, true
	case "-":
		return OpSub, true
	case "*":
		return OpMul, true
	case "/":
		return OpDiv, true
	case "%":
		return OpMod, true
	case "++":
		return OpInc, true
	case "--":
		return OpDec, true
	case "&":
		return OpAnd, true
	case "|":
		return OpOr, true
	case "^":
		return OpXor, true
	case "<<":
		return OpShl, true
	case ">>":
		return OpShr, true
	case "~":
		return OpNot, true
	case "&&":
		return OpLAnd, true
	case "||":
		return OpLOr, true
	case "!":
		return OpLNot, true
	case "==":
		return OpEq, true
	case "!=":
		return OpNeq, true
	case "<":
		return OpLt, true
	case "<=":
		return OpLte, true
	case ">":
		return OpGt, true
	case ">=":
		return OpGte, true
	case "=":
		return OpAssign, true
	case "+=":
		return OpAddEq, true
	case "-=":
		return OpSubEq, true
	case "*=":
		return OpMulEq, true
	case "/=":
		return OpDivEq, true
	case "%=":
		return OpModEq, true
	case "&=":
		return OpAndEq, true
	case "|=":
		return OpOrEq, true
	case "^=":
		return OpXorEq, true
	case "<<=":
		return OpShlEq, true
	case ">>=":
		return OpShrEq, true
	}
	return OpUnknown, false
}

func (v *HirVisitor) VisitTypeDeclStmt(ctx *parser.TypeDeclStmtContext) any {
	st := &TypeDeclStmt{node: v.mkNode(ctx), docBase: docBase{doc: v.docCommentBefore(ctx)}}
	st.Annotations = v.collectAnnotations(ctx.AllAnnotation())

	nameTok := ctx.ID().GetSymbol()
	st.Name = NameRef{Name: nameTok.GetText(), Span: v.spanFromTok(nameTok)}

	if gp := ctx.GenericParams(); gp != nil {
		for _, p := range gp.AllGenericParam() {
			st.TypeParams = append(st.TypeParams, v.buildGenericParamDecl(p))
		}
	}

	for _, clauseCtx := range ctx.AllTypeClause() {
		if impl := clauseCtx.ImplementsClause(); impl != nil {
			for _, idNode := range impl.AllID() {
				tok := idNode.GetSymbol()
				st.Implements = append(st.Implements, NameRef{Name: tok.GetText(), Span: v.spanFromTok(tok)})
			}
		}
		if with := clauseCtx.WithClause(); with != nil {
			for _, qr := range with.AllQualifiedRef() {
				ids := qr.AllSoftId()
				if len(ids) == 0 {
					continue
				}
				ref := NameRef{Span: v.spanFromCtx(qr.(antlr.ParserRuleContext))}
				if len(ids) >= 2 {
					ref.Qualifier = ids[0].GetText()
					ref.Name = ids[1].GetText()
				} else {
					ref.Name = ids[0].GetText()
				}
				st.MixedIn = append(st.MixedIn, ref)
			}
		}
	}

	for _, memberCtx := range ctx.AllTypeMember() {
		if fieldCtx := memberCtx.FieldDecl(); fieldCtx != nil {
			if field, ok := v.Visit(fieldCtx).(*TypeField); ok {
				st.Fields = append(st.Fields, field)
			}
		}
		if ctorCtx := memberCtx.CtorDecl(); ctorCtx != nil {
			if ctor, ok := v.Visit(ctorCtx).(*CtorDecl); ok {
				st.Ctors = append(st.Ctors, ctor)
			}
		}
		if methodCtx := memberCtx.MethodDecl(); methodCtx != nil {
			if method, ok := v.Visit(methodCtx).(*TypeMethodDecl); ok {
				st.Methods = append(st.Methods, method)
			}
		}
		if castCtx := memberCtx.CastDecl(); castCtx != nil {
			if cast, ok := v.Visit(castCtx).(*CastDecl); ok {
				st.Casts = append(st.Casts, cast)
			}
		}
	}

	if len(st.Ctors) == 0 && len(st.Fields) > 0 && !st.IsExtern {
		st.Ctors = append(st.Ctors, v.synthFieldCtor(ctx, st.Fields))
	}
	return st
}

// synthFieldCtor builds an implicit field-init constructor for a `type X { ... }` declaration that has no explicit `new(...) {...}` block. Parameters mirror the fields in declaration order (typed, with field defaults propagated), and the body assigns each parameter to the matching `this.<field>`.
func (v *HirVisitor) synthFieldCtor(ctx antlr.ParserRuleContext, fields []*TypeField) *CtorDecl {
	ctor := &CtorDecl{node: v.mkNode(ctx), IsSynthetic: true}
	body := &BlockStmt{node: v.mkNode(ctx)}
	for _, fld := range fields {
		param := &FuncParam{
			node: node{id: v.nid(), span: fld.Name.Span},
			Name: NameRef{Name: fld.Name.Name, Span: fld.Name.Span},
			Type: cloneTypeRef(fld.Type, v.nodeAlloc),
		}
		if fld.Default != nil {
			param.Default = CloneExpr(fld.Default, v.nodeAlloc)
		}
		ctor.Params = append(ctor.Params, param)

		rhs := &VarRef{
			node: node{id: v.nid(), span: fld.Name.Span},
			Ref:  NameRef{Name: fld.Name.Name, Span: fld.Name.Span},
		}
		assign := &FieldAssignmentStmt{
			node:     node{id: v.nid(), span: fld.Name.Span},
			Receiver: NameRef{Name: "this", Span: fld.Name.Span},
			Fields:   []FieldName{{Name: fld.Name.Name, Span: fld.Name.Span}},
			Op:       OpAssign,
			Value:    rhs,
		}
		body.Stmts = append(body.Stmts, assign)
	}
	ctor.Body = body
	return ctor
}

func (v *HirVisitor) VisitMixinDeclStmt(ctx *parser.MixinDeclStmtContext) any {
	st := &MixinDeclStmt{node: v.mkNode(ctx), docBase: docBase{doc: v.docCommentBefore(ctx)}}
	nameTok := ctx.ID().GetSymbol()
	st.Name = NameRef{Name: nameTok.GetText(), Span: v.spanFromTok(nameTok)}

	for _, memberCtx := range ctx.AllMixinMember() {
		if fieldCtx := memberCtx.FieldDecl(); fieldCtx != nil {
			if field, ok := v.Visit(fieldCtx).(*TypeField); ok {
				st.Fields = append(st.Fields, field)
			}
		}
		if methodCtx := memberCtx.MethodDecl(); methodCtx != nil {
			if method, ok := v.Visit(methodCtx).(*TypeMethodDecl); ok {
				st.Methods = append(st.Methods, method)
			}
		}
	}
	return st
}

func (v *HirVisitor) VisitTypeAliasStmt(ctx *parser.TypeAliasStmtContext) any {
	st := &TypeAliasStmt{node: v.mkNode(ctx), docBase: docBase{doc: v.docCommentBefore(ctx)}}
	nameTok := ctx.ID().GetSymbol()
	st.Name = NameRef{Name: nameTok.GetText(), Span: v.spanFromTok(nameTok)}
	if t, ok := v.Visit(ctx.Type_()).(*TypeRef); ok {
		st.Target = t
	}
	return st
}

func (v *HirVisitor) VisitInterfaceDeclStmt(ctx *parser.InterfaceDeclStmtContext) any {
	st := &InterfaceDeclStmt{node: v.mkNode(ctx), docBase: docBase{doc: v.docCommentBefore(ctx)}}
	nameTok := ctx.ID().GetSymbol()
	st.Name = NameRef{Name: nameTok.GetText(), Span: v.spanFromTok(nameTok)}

	if gp := ctx.GenericParams(); gp != nil {
		for _, p := range gp.AllGenericParam() {
			st.TypeParams = append(st.TypeParams, v.buildGenericParamDecl(p))
		}
	}

	for _, sigCtx := range ctx.AllMethodSignature() {
		if sig, ok := v.Visit(sigCtx).(*InterfaceMethodSig); ok {
			st.Methods = append(st.Methods, sig)
		}
	}
	return st
}

func (v *HirVisitor) VisitMethodSignature(ctx *parser.MethodSignatureContext) any {
	sig := &InterfaceMethodSig{node: v.mkNode(ctx)}
	sig.IsShared = hasModifierChild(ctx, "shared")
	sname, sspan := v.softIdNameAndSpan(ctx.SoftId())
	sig.Name = NameRef{Name: sname, Span: sspan}

	if paramListCtx := ctx.FuncParamList(); paramListCtx != nil {
		for _, paramCtx := range paramListCtx.AllFuncParam() {
			param := v.Visit(paramCtx).(FuncParam)
			sig.Params = append(sig.Params, &param)
		}
	}
	if ctx.TypeAnnot() != nil {
		if tr, ok := v.Visit(ctx.TypeAnnot()).(*TypeRef); ok {
			sig.ReturnType = tr
		}
	}
	return sig
}

func (v *HirVisitor) VisitMethodDecl(ctx *parser.MethodDeclContext) any {
	method := &TypeMethodDecl{node: v.mkNode(ctx)}
	method.Private = hasModifierChild(ctx, "private")
	method.IsShared = hasModifierChild(ctx, "shared")
	method.Annotations = v.collectAnnotations(ctx.AllAnnotation())

	fn := &FuncDeclStmt{node: v.mkNode(ctx)}
	nameCtx := ctx.MethodName()
	var nameText string
	var nameSpan diag.TextSpan
	if soft := nameCtx.SoftId(); soft != nil {
		nameText, nameSpan = v.softIdNameAndSpan(soft)
	} else if opSym := nameCtx.OpSymbol(); opSym != nil {
		nameText = "op" + opSym.GetText()
		nameSpan = v.spanFromCtx(nameCtx)
	}
	fn.Name = NameRef{Name: nameText, Span: nameSpan}

	if gp := ctx.GenericParams(); gp != nil {
		for _, p := range gp.AllGenericParam() {
			fn.TypeParams = append(fn.TypeParams, v.buildGenericParamDecl(p))
		}
	}
	if paramListCtx := ctx.FuncParamList(); paramListCtx != nil {
		for _, paramCtx := range paramListCtx.AllFuncParam() {
			param := v.Visit(paramCtx).(FuncParam)
			fn.Params = append(fn.Params, &param)
		}
	}
	if ctx.TypeAnnot() != nil {
		if tr, ok := v.Visit(ctx.TypeAnnot()).(*TypeRef); ok {
			fn.ReturnType = tr
		}
	}
	if blockCtx := ctx.Block(); blockCtx != nil {
		if block, ok := v.Visit(blockCtx).(*BlockStmt); ok {
			fn.Body = block
		}
	}
	method.Func = fn
	return method
}

func (v *HirVisitor) VisitCtorDecl(ctx *parser.CtorDeclContext) any {
	ctor := &CtorDecl{node: v.mkNode(ctx)}
	ctor.IsShared = hasModifierChild(ctx, "shared")
	ctor.Annotations = v.collectAnnotations(ctx.AllAnnotation())

	if paramListCtx := ctx.FuncParamList(); paramListCtx != nil {
		for _, paramCtx := range paramListCtx.AllFuncParam() {
			param := v.Visit(paramCtx).(FuncParam)
			ctor.Params = append(ctor.Params, &param)
		}
	}
	if blockCtx := ctx.Block(); blockCtx != nil {
		if block, ok := v.Visit(blockCtx).(*BlockStmt); ok {
			ctor.Body = block
		}
	}
	return ctor
}

func (v *HirVisitor) VisitCastDecl(ctx *parser.CastDeclContext) any {
	decl := &CastDecl{node: v.mkNode(ctx)}
	decl.IsShared = hasModifierChild(ctx, "shared")
	decl.Annotations = v.collectAnnotations(ctx.AllAnnotation())

	idNode := ctx.ID()
	annotList := ctx.AllTypeAnnot()
	if idNode != nil && len(annotList) >= 1 {
		paramType, _ := v.Visit(annotList[0]).(*TypeRef)
		paramTok := idNode.GetSymbol()
		decl.Param = &FuncParam{
			node: node{id: v.nid(), span: v.spanFromTok(paramTok)},
			Name: NameRef{Name: paramTok.GetText(), Span: v.spanFromTok(paramTok)},
			Type: paramType,
		}
	}
	if len(annotList) >= 2 {
		if rt, ok := v.Visit(annotList[1]).(*TypeRef); ok {
			decl.ReturnType = rt
		}
	}
	if blockCtx := ctx.Block(); blockCtx != nil {
		if block, ok := v.Visit(blockCtx).(*BlockStmt); ok {
			decl.Body = block
		}
	}
	return decl
}

// collectAnnotations walks a slice of annotation parse contexts, builds IR Annotation values, and returns them in source order. Arg expressions are visited but not folded; pass_fold_annotations does the const folding later.
func (v *HirVisitor) collectAnnotations(ctxs []parser.IAnnotationContext) []Annotation {
	if len(ctxs) == 0 {
		return nil
	}
	out := make([]Annotation, 0, len(ctxs))
	for _, ac := range ctxs {
		anno := Annotation{}
		if id := ac.ID(); id != nil {
			tok := id.GetSymbol()
			anno.Name = NameRef{Name: tok.GetText(), Span: v.spanFromTok(tok)}
		}
		for _, argCtx := range ac.AllExpr() {
			if e, ok := v.Visit(argCtx).(Expr); ok {
				anno.Args = append(anno.Args, e)
			}
		}
		out = append(out, anno)
	}
	return out
}

// hasLiteralChild returns true when one of ctx's direct children is a terminal node whose token text equals literal. This is the modifier-presence check (`private`, etc.) used by decls that may be preceded by annotation rule nodes.
func hasLiteralChild(ctx antlr.ParserRuleContext, literal string) bool {
	for _, child := range ctx.GetChildren() {
		if tn, ok := child.(antlr.TerminalNode); ok {
			if tn.GetSymbol().GetText() == literal {
				return true
			}
		}
	}
	return false
}

// hasModifierChild returns true when any of ctx's direct children is a `memberModifier` rule whose terminal text matches `literal`. Mirrors `hasLiteralChild` for the per-member-modifier (`private`, `shared`) grammar shape used inside type members. The grammar wraps each modifier in its own production (`memberModifier : 'private' | 'shared'`) so the terminal-text scan walks one level deeper than `hasLiteralChild`'s direct-child match.
func hasModifierChild(ctx antlr.ParserRuleContext, literal string) bool {
	for _, child := range ctx.GetChildren() {
		ruleCtx, ok := child.(antlr.RuleNode)
		if !ok {
			if tn, ok := child.(antlr.TerminalNode); ok && tn.GetSymbol().GetText() == literal {
				return true
			}
			continue
		}
		for _, sub := range ruleCtx.GetChildren() {
			if tn, ok := sub.(antlr.TerminalNode); ok && tn.GetSymbol().GetText() == literal {
				return true
			}
		}
	}
	return false
}

func (v *HirVisitor) VisitFieldDecl(ctx *parser.FieldDeclContext) any {
	field := &TypeField{node: v.mkNode(ctx)}
	field.Annotations = v.collectAnnotations(ctx.AllAnnotation())

	soft := ctx.SoftId()
	if soft == nil {
		v.diag.Report(diag.ErrUnexpectedToken, v.spanFromCtx(ctx), ctx.GetText())
		return field
	}
	fname, fspan := v.softIdNameAndSpan(soft)
	field.Name = NameRef{Name: fname, Span: fspan}

	if ctx.TypeAnnot() != nil {
		if tr, ok := v.Visit(ctx.TypeAnnot()).(*TypeRef); ok {
			field.Type = tr
		}
	}
	if ctx.Expr() != nil {
		if e, ok := v.Visit(ctx.Expr()).(Expr); ok {
			field.Default = e
		}
	}
	field.Private = hasModifierChild(ctx, "private")
	field.IsShared = hasModifierChild(ctx, "shared")
	return field
}

func (v *HirVisitor) buildTemplateString(ctx *parser.LiteralContext) Expr {
	tok := ctx.TEMPLATE_STRING().GetSymbol()
	raw := tok.GetText()
	if len(raw) < 2 || raw[0] != '`' || raw[len(raw)-1] != '`' {
		v.diag.Report(diag.ErrInvalidLiteral, v.spanFromTok(tok), raw, "template string")
		return &LitString{node: v.mkNode(ctx), Value: ""}
	}

	body := raw[1 : len(raw)-1]
	parts := splitTemplateBody(body)

	out := &StringTemplateExpr{node: v.mkNode(ctx)}
	for _, p := range parts {
		if p.isExpr {
			expr := v.subParseExpr(p.text)
			out.Parts = append(out.Parts, StringTemplatePart{Expr: expr})
		} else {
			out.Parts = append(out.Parts, StringTemplatePart{Lit: unescapeTemplateLiteral(p.text)})
		}
	}
	return out
}

type templatePiece struct {
	isExpr bool
	text   string
}

func splitTemplateBody(body string) []templatePiece {
	var pieces []templatePiece
	var lit strings.Builder
	i := 0
	for i < len(body) {
		c := body[i]
		if c == '\\' && i+1 < len(body) {
			lit.WriteByte(c)
			lit.WriteByte(body[i+1])
			i += 2
			continue
		}
		if c == '$' && i+1 < len(body) && body[i+1] == '{' {
			if lit.Len() > 0 {
				pieces = append(pieces, templatePiece{text: lit.String()})
				lit.Reset()
			}
			depth := 1
			j := i + 2
			for j < len(body) && depth > 0 {
				switch body[j] {
				case '{':
					depth++
				case '}':
					depth--
				case '\\':
					if j+1 < len(body) {
						j++
					}
				}
				if depth == 0 {
					break
				}
				j++
			}
			if j >= len(body) {
				pieces = append(pieces, templatePiece{isExpr: true, text: body[i+2:]})
				return pieces
			}
			pieces = append(pieces, templatePiece{isExpr: true, text: body[i+2 : j]})
			i = j + 1
			continue
		}
		lit.WriteByte(c)
		i++
	}
	if lit.Len() > 0 {
		pieces = append(pieces, templatePiece{text: lit.String()})
	}
	return pieces
}

func unescapeTemplateLiteral(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case '`', '$', '\\':
				b.WriteByte(s[i+1])
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			default:
				b.WriteByte(s[i])
				b.WriteByte(s[i+1])
			}
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func (v *HirVisitor) subParseExpr(text string) Expr {
	is := antlr.NewInputStream(text)
	lexer := parser.NewSovaLexer(is)
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(diag.NewAntlrErrorListener(v.filename, v.diag))
	cts := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	p := parser.NewSovaParser(cts)
	p.RemoveErrorListeners()
	p.AddErrorListener(diag.NewAntlrErrorListener(v.filename, v.diag))
	exprCtx := p.Expr()
	if exprCtx == nil {
		return &LitString{Value: ""}
	}
	if e, ok := v.Visit(exprCtx).(Expr); ok {
		return e
	}
	return &LitString{Value: ""}
}

// VisitSynthDeclStmt builds an IR `SynthDeclStmt` from `synth Name(params) on <kind> Bind { body }`. The target-kind token comes from the grammar's `synthTargetKind` production which accepts the keyword tokens (`type`, `func`, `let`) plus a bare ID fallback for `field` and `param` (neither is a Sova-wide reserved word). Unknown kinds produce a diagnostic at parse time so the synth never reaches the expander with an unrecognised target.
func (v *HirVisitor) VisitSynthDeclStmt(ctx *parser.SynthDeclStmtContext) any {
	st := &SynthDeclStmt{
		node:    v.mkNode(ctx),
		docBase: docBase{doc: v.docCommentBefore(ctx)},
	}
	if idTok := ctx.ID(); idTok != nil {
		t := idTok.GetSymbol()
		st.Name = NameRef{Name: t.GetText(), Span: v.spanFromTok(t)}
	}
	if pl := ctx.SynthParams(); pl != nil {
		if fpl := pl.FuncParamList(); fpl != nil {
			for _, paramCtx := range fpl.AllFuncParam() {
				p := v.Visit(paramCtx).(FuncParam)
				st.Params = append(st.Params, &p)
			}
		}
	}
	if rs := ctx.SynthRequiredSide(); rs != nil {
		switch {
		case rs.SIDE_FRONTEND() != nil:
			st.RequiredSide = SideFrontend
		case rs.SIDE_BACKEND() != nil:
			st.RequiredSide = SideBackend
		case rs.SIDE_SHARED() != nil:
			st.RequiredSide = SideShared
		}
	}
	if tgt := ctx.SynthTarget(); tgt != nil {
		st.Target = v.parseSynthTarget(tgt)
	}
	st.Body = v.collectSynthBody(ctx.AllSynthBodyItem())
	return st
}

func (v *HirVisitor) collectSynthBody(items []parser.ISynthBodyItemContext) []SynthBodyItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]SynthBodyItem, 0, len(items))
	for _, bodyCtx := range items {
		if emit := bodyCtx.SynthEmitOn(); emit != nil {
			if e, ok := v.Visit(emit).(*SynthEmitOn); ok {
				out = append(out, e)
			}
			continue
		}
		if app := bodyCtx.SynthEmitAppend(); app != nil {
			if e, ok := v.Visit(app).(*SynthEmitAppend); ok {
				out = append(out, e)
			}
			continue
		}
		if fld := bodyCtx.SynthEmitField(); fld != nil {
			if e, ok := v.Visit(fld).(*SynthEmitField); ok {
				out = append(out, e)
			}
			continue
		}
		if mth := bodyCtx.SynthEmitMethod(); mth != nil {
			if e, ok := v.Visit(mth).(*SynthEmitMethod); ok {
				out = append(out, e)
			}
			continue
		}
		if cth := bodyCtx.SynthEmitCtor(); cth != nil {
			if e, ok := v.Visit(cth).(*SynthEmitCtor); ok {
				out = append(out, e)
			}
			continue
		}
		if loop := bodyCtx.SynthForStmt(); loop != nil {
			if e, ok := v.Visit(loop).(*SynthForStmt); ok {
				out = append(out, e)
			}
			continue
		}
	}
	return out
}

func (v *HirVisitor) parseSynthTarget(ctx parser.ISynthTargetContext) SynthTarget {
	out := SynthTarget{Kind: SynthTargetUnknown}
	kindCtx := ctx.SynthTargetKind()
	if kindCtx != nil {
		kindText := strings.TrimSpace(kindCtx.GetText())
		switch kindText {
		case "type":
			out.Kind = SynthTargetType
		case "field":
			out.Kind = SynthTargetField
		case "func":
			out.Kind = SynthTargetFunc
		case "param":
			out.Kind = SynthTargetParam
		case "let":
			out.Kind = SynthTargetLet
		case "method":
			out.Kind = SynthTargetMethod
		case "ctor":
			out.Kind = SynthTargetCtor
		default:
			v.diag.Report(diag.ErrUnexpectedToken, v.spanFromCtx(kindCtx), kindText)
		}
	}
	if id := ctx.ID(); id != nil {
		out.BindName = id.GetText()
	}
	return out
}

// VisitSynthEmitOn builds an `emit on <scope> { @annotation* }` block. The scope is a bare identifier that the interpreter resolves at expansion time against the current bind environment (the synth's outer target, or a for-loop iteration variable).
func (v *HirVisitor) VisitSynthEmitOn(ctx *parser.SynthEmitOnContext) any {
	out := &SynthEmitOn{node: v.mkNode(ctx)}
	if idTok := ctx.ID(); idTok != nil {
		out.Scope = idTok.GetText()
	}
	out.AnnotationEmits = v.collectAnnotations(ctx.AllAnnotation())
	return out
}

// VisitSynthEmitAppend builds an `emit append to <registry> { <expr> }` clause. The expression is visited like any other Sova expression so it participates in synth-param substitution before being appended to the registry slice in the compiler cache.
func (v *HirVisitor) VisitSynthEmitAppend(ctx *parser.SynthEmitAppendContext) any {
	out := &SynthEmitAppend{node: v.mkNode(ctx)}
	if idTok := ctx.ID(); idTok != nil {
		out.Registry = idTok.GetText()
	}
	if exCtx := ctx.Expr(); exCtx != nil {
		if e, ok := v.Visit(exCtx).(Expr); ok {
			out.Fragment = e
		}
	}
	return out
}

// VisitSynthForStmt builds a `for <loopVar> in <bind>.<member> [where <pred>] { <body> }` clause. The iterable is parsed as a `<bind>.<member>` pair — interpreted at expansion time, not at parse time — so adding new iterable members later is a one-line interpreter change. The optional `where` predicate is captured for filter-time evaluation.
func (v *HirVisitor) VisitSynthForStmt(ctx *parser.SynthForStmtContext) any {
	out := &SynthForStmt{node: v.mkNode(ctx)}
	if idTok := ctx.ID(); idTok != nil {
		out.LoopVar = idTok.GetText()
	}
	if it := ctx.SynthIterable(); it != nil {
		itIds := it.AllID()
		if len(itIds) >= 1 {
			out.BindName = itIds[0].GetText()
		}
		if len(itIds) >= 2 {
			out.Member = itIds[1].GetText()
		}
	}
	if w := ctx.SynthWhere(); w != nil {
		if be := w.SynthBoolExpr(); be != nil {
			out.Where = v.parseSynthBoolExpr(be)
		}
	}
	out.Body = v.collectSynthBody(ctx.AllSynthBodyItem())
	return out
}

// VisitSynthEmitField wraps a regular `fieldDecl` parse-tree in a synth body item. We delegate to the existing field visitor so the produced TypeField is structurally identical to a hand-written one — the synth interpreter just clones-and-appends it onto the target type's `Fields` slice.
func (v *HirVisitor) VisitSynthEmitField(ctx *parser.SynthEmitFieldContext) any {
	out := &SynthEmitField{node: v.mkNode(ctx)}
	if fd := ctx.FieldDecl(); fd != nil {
		if f, ok := v.Visit(fd).(*TypeField); ok {
			out.Field = f
		}
	}
	return out
}

// VisitSynthEmitMethod wraps a regular `methodDecl` parse-tree (so the synth author writes `emit func compute(): int { ... }` — same shape as a hand-written method on a type body).
func (v *HirVisitor) VisitSynthEmitMethod(ctx *parser.SynthEmitMethodContext) any {
	out := &SynthEmitMethod{node: v.mkNode(ctx)}
	if md := ctx.MethodDecl(); md != nil {
		if m, ok := v.Visit(md).(*TypeMethodDecl); ok {
			out.Method = m
		}
	}
	return out
}

// VisitSynthEmitCtor wraps a regular `ctorDecl` parse-tree (so the synth author writes `emit new(x: int) { ... }` — same shape as a hand-written ctor on a type body).
func (v *HirVisitor) VisitSynthEmitCtor(ctx *parser.SynthEmitCtorContext) any {
	out := &SynthEmitCtor{node: v.mkNode(ctx)}
	if cd := ctx.CtorDecl(); cd != nil {
		if c, ok := v.Visit(cd).(*CtorDecl); ok {
			out.Ctor = c
		}
	}
	return out
}

func (v *HirVisitor) parseSynthBoolExpr(ctx parser.ISynthBoolExprContext) *SynthBoolExpr {
	out := &SynthBoolExpr{node: v.mkNode(ctx)}
	out.Negate = strings.HasPrefix(strings.TrimSpace(ctx.GetText()), "!")
	ids := ctx.AllID()
	if len(ids) >= 1 {
		out.BindName = ids[0].GetText()
	}
	if len(ids) >= 2 {
		out.Property = ids[1].GetText()
	}
	return out
}
