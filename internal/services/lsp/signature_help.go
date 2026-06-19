package lsp

import (
	"context"
	"strings"

	"go.lsp.dev/protocol"

	"sova/internal/ir"
	"sova/internal/services/compiler"
)

func (s *Server) SignatureHelp(ctx context.Context, params *protocol.SignatureHelpParams) (*protocol.SignatureHelp, error) {
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

	cursor := position{line: int(params.Position.Line) + 1, col: int(params.Position.Character) + 1}

	var sym *ir.Symbol
	var activeArg uint32
	if enclosing := findEnclosingCall(file, cursor); enclosing != nil {
		if calleeSym := calleeSymbolFor(enclosing.callee); calleeSym != 0 {
			sym, _ = lookupSymbol(c, calleeSym)
			activeArg = uint32(enclosing.activeArg)
		}
	}

	if sym == nil {
		src, ok := snap.ReadFile(params.TextDocument.URI)
		if ok {
			if s, ai := signatureHelpFromSource(c, file, src, params.Position); s != nil {
				sym = s
				activeArg = ai
			}
		}
	}

	if sym == nil {
		return nil, nil
	}

	fnTy, ok := c.TypeUniverse.GetByID(sym.Typ)
	if !ok || fnTy.Kind != ir.TK_Function {
		return nil, nil
	}

	label, paramLabels := renderSignature(c, sym, fnTy)
	sigParams := make([]protocol.ParameterInformation, len(paramLabels))
	for i, pl := range paramLabels {
		sigParams[i] = protocol.ParameterInformation{Label: pl}
	}

	active := activeArg
	if active >= uint32(len(sigParams)) {
		if len(sigParams) == 0 {
			active = 0
		} else {
			active = uint32(len(sigParams) - 1)
		}
	}

	sigInfo := protocol.SignatureInformation{
		Label:           label,
		Parameters:      sigParams,
		ActiveParameter: active,
	}

	if doc := strings.TrimSpace(sym.Doc); doc != "" {
		sigInfo.Documentation = protocol.MarkupContent{
			Kind:  protocol.Markdown,
			Value: renderDocComment(doc),
		}
	}

	return &protocol.SignatureHelp{
		Signatures:      []protocol.SignatureInformation{sigInfo},
		ActiveSignature: 0,
		ActiveParameter: active,
	}, nil
}

type enclosingCallInfo struct {
	callee    ir.Expr
	activeArg int
}

func findEnclosingCall(f *ir.File, cursor position) *enclosingCallInfo {
	var found *enclosingCallInfo
	for _, st := range f.Statements {
		stmtFindCall(st, cursor, &found)
	}

	return found
}

func stmtFindCall(s ir.Stmt, cursor position, found **enclosingCallInfo) {
	if s == nil {
		return
	}

	switch n := s.(type) {
	case *ir.BlockStmt:
		for _, ss := range n.Stmts {
			stmtFindCall(ss, cursor, found)
		}

	case *ir.VarDeclStmt:
		exprFindCall(n.Init, cursor, found)
	case *ir.ExprStmt:
		exprFindCall(n.Expr, cursor, found)
	case *ir.FieldAssignmentStmt:
		exprFindCall(n.Value, cursor, found)
	case *ir.MultiAssignmentStmt:
		exprFindCall(n.Value, cursor, found)
	case *ir.IfStmt:
		exprFindCall(n.Cond, cursor, found)
		if n.Then != nil {
			for _, ss := range ir.BlockStmts(n.Then) {
				stmtFindCall(ss, cursor, found)
			}
		}

		for _, eb := range n.ElseIfs {
			exprFindCall(eb.Cond, cursor, found)
			if eb.Then != nil {
				for _, ss := range ir.BlockStmts(eb.Then) {
					stmtFindCall(ss, cursor, found)
				}
			}
		}

		if n.Else != nil {
			for _, ss := range ir.BlockStmts(n.Else) {
				stmtFindCall(ss, cursor, found)
			}
		}

	case *ir.ReturnStmt:
		for _, r := range n.Results {
			exprFindCall(r, cursor, found)
		}

	case *ir.ForStmt:
		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				stmtFindCall(ss, cursor, found)
			}
		}

	case *ir.WhileStmt:
		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				stmtFindCall(ss, cursor, found)
			}
		}

	case *ir.FuncDeclStmt:
		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				stmtFindCall(ss, cursor, found)
			}
		}

	case *ir.TypeDeclStmt:
		for _, ctor := range n.Ctors {
			if ctor.Body != nil {
				for _, ss := range ir.BlockStmts(ctor.Body) {
					stmtFindCall(ss, cursor, found)
				}
			}
		}

		for _, m := range n.Methods {
			stmtFindCall(m.Func, cursor, found)
		}

	case *ir.GoStmt:
		exprFindCall(n.Call, cursor, found)
		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				stmtFindCall(ss, cursor, found)
			}
		}

	case *ir.DeferStmt:
		exprFindCall(n.Call, cursor, found)
		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				stmtFindCall(ss, cursor, found)
			}
		}

	case *ir.SelectStmt:
		for _, cc := range n.Cases {
			exprFindCall(cc.ChanExpr, cursor, found)
			exprFindCall(cc.SendValue, cursor, found)
			if cc.Body != nil {
				for _, ss := range ir.BlockStmts(cc.Body) {
					stmtFindCall(ss, cursor, found)
				}
			}
		}

		if n.Default != nil {
			for _, ss := range n.Default.Stmts {
				stmtFindCall(ss, cursor, found)
			}
		}

	case *ir.AssertStmt:
		exprFindCall(n.Expr, cursor, found)
	case *ir.TestDeclStmt:
		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				stmtFindCall(ss, cursor, found)
			}
		}
	}
}

func exprFindCall(e ir.Expr, cursor position, found **enclosingCallInfo) {
	if e == nil {
		return
	}

	switch n := e.(type) {
	case *ir.FuncCallExpr:

		exprFindCall(n.Callee, cursor, found)
		for _, arg := range n.Args {
			exprFindCall(arg.Expr, cursor, found)
		}

		if cursorInArgArea(n, cursor) {
			active := activeArgIndex(n, cursor)
			*found = &enclosingCallInfo{callee: n.Callee, activeArg: active}
		}

	case *ir.NewExpr:
		for _, arg := range n.Args {
			exprFindCall(arg.Expr, cursor, found)
		}

	case *ir.BinaryExpr:
		exprFindCall(n.Left, cursor, found)
		exprFindCall(n.Right, cursor, found)
	case *ir.GroupedExpr:
		exprFindCall(n.Expr, cursor, found)
	case *ir.UnaryExpr:
		exprFindCall(n.Expr, cursor, found)
	case *ir.PrefixUnaryExpr:
		exprFindCall(n.Expr, cursor, found)
	case *ir.PostfixUnaryExpr:
		exprFindCall(n.Expr, cursor, found)
	case *ir.FieldAccessExpr:
		exprFindCall(n.Expr, cursor, found)
	case *ir.IndexExpr:
		exprFindCall(n.Expr, cursor, found)
		exprFindCall(n.Index, cursor, found)
	case *ir.AssignmentExpr:
		exprFindCall(n.Right, cursor, found)
	case *ir.TenaryExpr:
		exprFindCall(n.Cond, cursor, found)
		exprFindCall(n.Then, cursor, found)
		exprFindCall(n.Else, cursor, found)
	case *ir.CoalesceExpr:
		exprFindCall(n.Left, cursor, found)
		exprFindCall(n.Default, cursor, found)
	case *ir.FuncLitExpr:
		if n.Body != nil {
			for _, ss := range ir.BlockStmts(n.Body) {
				stmtFindCall(ss, cursor, found)
			}
		}

	case *ir.ArrayLiteral:
		for _, el := range n.Elems {
			exprFindCall(el, cursor, found)
		}

	case *ir.MapLiteral:
		for _, kv := range n.Entries {
			exprFindCall(kv.Key, cursor, found)
			exprFindCall(kv.Value, cursor, found)
		}

	case *ir.TupleLiteral:
		for _, el := range n.Elems {
			exprFindCall(el, cursor, found)
		}
	}
}

func cursorInArgArea(call *ir.FuncCallExpr, cursor position) bool {
	span := call.Span()
	if span.StartLn == 0 {
		return false
	}

	if cursor.line < span.StartLn || cursor.line > span.EndLn {
		return false
	}

	if cursor.line == span.EndLn && cursor.col > span.EndCol {
		return false
	}

	calleeSpan := call.Callee.Span()
	if calleeSpan.EndLn != 0 {
		if cursor.line < calleeSpan.EndLn {
			return false
		}

		if cursor.line == calleeSpan.EndLn && cursor.col <= calleeSpan.EndCol {
			return false
		}
	}

	return true
}

func activeArgIndex(call *ir.FuncCallExpr, cursor position) int {
	for i, arg := range call.Args {
		if arg.Expr == nil {
			continue
		}

		span := arg.Expr.Span()
		if span.EndLn == 0 {
			continue
		}

		if cursor.line < span.EndLn || (cursor.line == span.EndLn && cursor.col <= span.EndCol) {
			return i
		}
	}

	return len(call.Args)
}

func calleeSymbolFor(e ir.Expr) ir.SymID {
	switch n := e.(type) {
	case *ir.VarRef:
		return n.Ref.Sym
	case *ir.FieldAccessExpr:
		return n.ResolvedSym
	}

	return 0
}

func renderSignature(c *compiler.CompilerContext, sym *ir.Symbol, fnTy *ir.Type) (string, []string) {
	paramLabels := make([]string, len(fnTy.ParamTypes))
	parts := make([]string, len(fnTy.ParamTypes))
	for i, p := range fnTy.ParamTypes {
		label := ""
		if p.Name.Name != "" {
			label = p.Name.Name + ": "
		}

		typeStr := ""
		if p.Type != nil && p.Type.Typ != 0 {
			typeStr = formatType(c.TypeUniverse, p.Type.Typ)
		}

		parts[i] = label + typeStr
		paramLabels[i] = parts[i]
	}

	head := "func"
	if fnTy.IsAsync {
		head = "async func"
	}

	label := head + " " + sym.Name + "(" + strings.Join(parts, ", ") + ")"
	if fnTy.ReturnType != 0 {
		label += ": " + formatType(c.TypeUniverse, fnTy.ReturnType)
	}

	return label, paramLabels
}

func signatureHelpFromSource(c *compiler.CompilerContext, file *ir.File, src string, pos protocol.Position) (*ir.Symbol, uint32) {
	offset := positionToOffset(src, pos)
	if offset > len(src) {
		offset = len(src)
	}

	depthParen, depthBracket, depthBrace := 0, 0, 0
	inString := byte(0)
	parenPos := -1
	commaCount := 0
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
				parenPos = i
			} else if depthParen > 0 {
				depthParen--
			}

		case ',':
			if depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
				commaCount++
			}
		}

		if parenPos >= 0 {
			break
		}
	}

	if parenPos < 0 {
		return nil, 0
	}

	calleeEnd := parenPos
	for calleeEnd > 0 && (src[calleeEnd-1] == ' ' || src[calleeEnd-1] == '\t') {
		calleeEnd--
	}

	nameEnd := calleeEnd
	nameStart := nameEnd
	for nameStart > 0 && isIdentChar(src[nameStart-1]) {
		nameStart--
	}

	name := src[nameStart:nameEnd]
	if name == "" {
		return nil, 0
	}

	if nameStart > 0 && src[nameStart-1] == '.' {
		dotPos := nameStart - 1
		dotLine, dotCol := offsetToLineCol(src, dotPos)
		if recvTyp := findExprTypeEndingAt(file, dotLine, dotCol); recvTyp != 0 {
			if sym := lookupMethodOnType(c, recvTyp, name); sym != nil {
				return sym, uint32(commaCount)
			}
		}

		recvText := extractReceiverText(src, dotPos)
		if recvText != "" {
			if sym := resolveQualifiedCallee(c, file, recvText, name); sym != nil {
				return sym, uint32(commaCount)
			}
		}

		recvStart := dotPos
		for recvStart > 0 && (src[recvStart-1] == ' ' || src[recvStart-1] == '\t') {
			recvStart--
		}

		if recvStart > 0 && src[recvStart-1] == '@' {
			if sessionTyp, ok := c.Cache[compiler.SessionsSessionTypeCacheKey].(ir.TypID); ok && sessionTyp != 0 {
				if sym := lookupMethodOnType(c, sessionTyp, name); sym != nil {
					return sym, uint32(commaCount)
				}
			}
		}

		return nil, 0
	}

	if sym := findLocalSymByName(file, name); sym != 0 {
		s, _ := lookupSymbol(c, sym)
		if s != nil {
			return s, uint32(commaCount)
		}
	}

	return nil, 0
}

func resolveQualifiedCallee(c *compiler.CompilerContext, file *ir.File, recvText, methodName string) *ir.Symbol {
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

			if id, ok := target.Scopes.LookupOnlyCurrent(target.Root, methodName); ok {
				s, _ := target.Syms.GetByID(id)
				return s
			}

			return nil
		}
	}

	recvSym := findLocalSymByName(file, recvText)
	if recvSym == 0 {
		return nil
	}

	rs, _ := lookupSymbol(c, recvSym)
	if rs == nil || rs.Typ == 0 {
		return nil
	}

	ty, ok := c.TypeUniverse.GetByID(rs.Typ)
	if !ok {
		return nil
	}

	for _, m := range ty.StructMethods {
		if m.Name == methodName {
			if m.Sym != 0 {
				ms, _ := lookupSymbol(c, m.Sym)
				if ms != nil {
					return ms
				}
			}

			return &ir.Symbol{Name: m.Name, Typ: m.FuncTyp}
		}
	}

	return nil
}

func offsetToLineCol(src string, offset int) (int, int) {
	if offset < 0 {
		offset = 0
	}

	if offset > len(src) {
		offset = len(src)
	}

	line, col := 1, 1
	for i := 0; i < offset; i++ {
		if src[i] == '\n' {
			line++
			col = 1
			continue
		}

		col++
	}

	return line, col
}

func lookupMethodOnType(c *compiler.CompilerContext, typID ir.TypID, methodName string) *ir.Symbol {
	if typID == 0 {
		return nil
	}

	ty, ok := c.TypeUniverse.GetByID(typID)
	if !ok {
		return nil
	}

	for _, m := range ty.StructMethods {
		if m.Name != methodName {
			continue
		}

		if m.Sym != 0 {
			if ms, _ := lookupSymbol(c, m.Sym); ms != nil {
				return ms
			}
		}

		return &ir.Symbol{Name: m.Name, Typ: m.FuncTyp}
	}

	return nil
}

func extractReceiverText(src string, dotPos int) string {
	end := dotPos
	for end > 0 && (src[end-1] == ' ' || src[end-1] == '\t') {
		end--
	}

	start := end
	for start > 0 {
		ch := src[start-1]
		if isIdentChar(ch) || ch == '.' {
			start--
			continue
		}

		if ch == ')' || ch == ']' {
			open := byte('(')
			if ch == ']' {
				open = '['
			}

			depth := 1
			start--
			for start > 0 && depth > 0 {
				start--
				if src[start] == ch {
					depth++
				} else if src[start] == open {
					depth--
				}
			}

			continue
		}

		break
	}

	return src[start:end]
}
