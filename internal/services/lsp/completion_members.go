package lsp

import (
	"sort"
	"strings"

	"go.lsp.dev/protocol"

	"sova/internal/ir"
	"sova/internal/services/compiler"
)

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
		for _, f := range ty.Struct.Fields {
			if filterShared && !f.IsShared {
				continue
			}

			out = append(out, protocol.CompletionItem{
				Label:  f.Name,
				Kind:   protocol.CompletionItemKindField,
				Detail: f.Name + ": " + formatType(c.TypeUniverse, f.Type),
			})
		}

		for _, m := range ty.Struct.Methods {
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
		for _, ec := range ty.Enum.Cases {
			out = append(out, protocol.CompletionItem{
				Label:  ec.Name,
				Kind:   protocol.CompletionItemKindEnumMember,
				Detail: ty.Enum.Name + "." + ec.Name,
			})
		}

		for _, m := range ty.Enum.Methods {
			out = append(out, protocol.CompletionItem{
				Label:  m.Name,
				Kind:   protocol.CompletionItemKindMethod,
				Detail: "func " + m.Name,
			})
		}

	case ir.TK_Interface:
		for _, m := range ty.Interface.Methods {
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
