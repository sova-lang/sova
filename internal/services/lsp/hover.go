package lsp

import (
	"context"
	"fmt"
	"strings"

	"go.lsp.dev/protocol"

	"sova/internal/ir"
	"sova/internal/services/compiler"
)

// Hover returns the type + declaration summary for the symbol under the cursor. Renders as a markdown code block (sova) - gives editors a colorized preview without us shipping a separate highlight grammar. Returns nil (not an error) when the cursor isn't on a known symbol, which tells the editor to fall back to its own help.
func (s *Server) Hover(ctx context.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	snap := s.session.Snapshot()
	if snap == nil {
		return nil, nil
	}
	c, _, err := snap.Compile(s.compileSnapshot)
	if err != nil || c == nil {
		return nil, nil
	}
	target := findCursorTarget(c, params.TextDocument.URI, params.Position.Line, params.Position.Character)
	if target == nil {
		return nil, nil
	}
	consumerSide := ir.SideShared
	if _, file, _ := lookupFileByURI(c, params.TextDocument.URI); file != nil {
		consumerSide = file.Side.Kind
	}
	contents := renderHover(c, target, consumerSide)
	if contents == "" {
		return nil, nil
	}
	rng := spanToLSPRange(target.span)
	return &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.Markdown,
			Value: contents,
		},
		Range: &rng,
	}, nil
}

// renderHover assembles the markdown body shown in the hover popup. Builds a `sova`-fenced signature line (`let x: int`, `func foo(a: int): bool`, etc.); returns "" when the target has no symbol we can describe so the LSP returns nil and the editor falls back to its own help. `consumerSide` is the side of the file the cursor sits in, used by member-resolution to filter out non-shared members of types declared on the opposite side (so a frontend hover on a backend-only field returns nothing rather than misleading IntelliSense).
func renderHover(c *compiler.CompilerContext, target *cursorTarget, consumerSide ir.SideKind) string {
	if target == nil {
		return ""
	}
	if target.kind == cursorKindSymbol && target.sym == 0 && target.fieldName == "@" {
		typeStr := "sessions.Session"
		if target.typ != 0 {
			typeStr = formatType(c.TypeUniverse, target.typ)
		}
		body := "```sova\n@: " + typeStr + "\n```"
		if doc := strings.TrimSpace(lookupSessionTypeDoc(c)); doc != "" {
			body += "\n\n" + renderDocComment(doc)
		} else {
			body += "\n\nthe current request's session handle"
		}
		return body
	}
	if target.kind == cursorKindTypeRef {
		return renderTypeRefHover(c, target)
	}
	if target.kind == cursorKindImportPath {
		return "```sova\nimport \"" + target.importPath + "\"\n```"
	}
	if target.kind == cursorKindMember && target.sym == 0 {
		if memberSym := findMemberSym(c, target.memberOf, target.fieldName, consumerSide); memberSym != nil {
			sig := formatSymbolSignature(c.TypeUniverse, memberSym)
			body := "```sova\n" + sig + "\n```"
			if doc := strings.TrimSpace(memberSym.Doc); doc != "" {
				body += "\n\n" + renderDocComment(doc)
			}
			return body
		}
		var sig strings.Builder
		sig.WriteString(target.fieldName)
		if target.typ != 0 {
			sig.WriteString(": ")
			sig.WriteString(formatType(c.TypeUniverse, target.typ))
		}
		return "```sova\n" + sig.String() + "\n```"
	}
	if target.sym == 0 {
		return ""
	}
	sym, pkg := lookupSymbol(c, target.sym)
	if sym == nil {
		return ""
	}
	sig := formatSymbolSignature(c.TypeUniverse, sym)
	body := "```sova\n" + sig + "\n```"
	if doc := strings.TrimSpace(sym.Doc); doc != "" {
		body += "\n\n" + renderDocComment(doc)
	}
	if pkg != nil && pkg.Path != nil {
		body += "\n\nfrom *" + pkg.Path.String() + "*"
	}
	return body
}

// renderTypeRefHover builds the hover popup for a `TypeRef` cursor target - the user is hovering over a type name in a `let x: T`, `func(p: T): T`, `option<T>`, etc. context. Resolves the name (and optional qualifier) through the file's imports to the declaring `type` / `interface` / `enum` symbol, then shares the same signature + doc rendering used for regular symbols so the popup shows the same content the user would see hovering over the declaration site itself.
func renderTypeRefHover(c *compiler.CompilerContext, target *cursorTarget) string {
	if target.typeRefName == "" {
		return ""
	}
	sym, declPkg := findTypeDeclSym(c, target.pkg, target.typeRefName, target.typeRefQualifier)
	if sym == nil {
		body := "```sova\ntype " + target.typeRefName + "\n```"
		return body
	}
	sig := formatSymbolSignature(c.TypeUniverse, sym)
	body := "```sova\n" + sig + "\n```"
	if doc := strings.TrimSpace(sym.Doc); doc != "" {
		body += "\n\n" + renderDocComment(doc)
	}
	if declPkg != nil && declPkg.Path != nil {
		body += "\n\nfrom *" + declPkg.Path.String() + "*"
	}
	return body
}

// findTypeDeclSym mirrors `typeRefDeclSpan` but returns the declaring `*ir.Symbol` (and its owning package) so hover can render the same `func toString(): string` / doc-comment view it uses for any other symbol. Resolves qualifier through the current file's imports; an empty qualifier falls back to the current package then any other loaded package that exposes the name.
func findTypeDeclSym(c *compiler.CompilerContext, currentPkg *ir.PackageContext, name, qualifier string) (*ir.Symbol, *ir.PackageContext) {
	if name == "" {
		return nil, nil
	}
	var target *ir.PackageContext
	if qualifier == "" {
		target = currentPkg
	} else if currentPkg != nil {
		for _, f := range currentPkg.Files {
			if f.Hir == nil {
				continue
			}
			for _, st := range f.Hir.Statements {
				imp, ok := st.(*ir.ImportStmt)
				if !ok {
					continue
				}
				alias := imp.Alias
				if alias == "" && len(imp.Path) > 0 {
					alias = imp.Path[len(imp.Path)-1]
				}
				if alias == qualifier {
					if pkg, found := c.Packages[imp.Path.String()]; found {
						target = pkg
					}
					break
				}
			}
			if target != nil {
				break
			}
		}
	}
	if target != nil {
		if sym := lookupTypeDeclSymInPkg(target, name); sym != nil {
			return sym, target
		}
	}
	if qualifier == "" {
		for _, pkg := range c.Packages {
			if pkg == currentPkg {
				continue
			}
			if sym := lookupTypeDeclSymInPkg(pkg, name); sym != nil {
				return sym, pkg
			}
		}
	}
	return nil, nil
}

// lookupTypeDeclSymInPkg walks `pkg`'s top-level statements for a `type` / `interface` / `enum` named `name` and returns its declaring `*ir.Symbol`, or nil when not found.
func lookupTypeDeclSymInPkg(pkg *ir.PackageContext, name string) *ir.Symbol {
	if pkg == nil {
		return nil
	}
	for _, f := range pkg.Files {
		if f.Hir == nil {
			continue
		}
		for _, st := range f.Hir.Statements {
			var sym ir.SymID
			switch n := st.(type) {
			case *ir.TypeDeclStmt:
				if n.Name.Name == name {
					sym = n.Name.Sym
				}
			case *ir.InterfaceDeclStmt:
				if n.Name.Name == name {
					sym = n.Name.Sym
				}
			case *ir.EnumDeclStmt:
				if n.Name.Name == name {
					sym = n.Name.Sym
				}
			}
			if sym != 0 {
				if s, ok := pkg.Syms.GetByID(sym); ok {
					return s
				}
			}
		}
	}
	return nil
}

// findMemberSym resolves a `receiver.name` member access to the declaring `*ir.Symbol` when the receiver's type is a user-defined struct / enum / interface. Methods (struct methods and interface signatures) carry the doc-comment from their declaration site so hover can show it; plain struct fields don't currently store per-field docs in the IR and return nil. `consumerSide` filters out non-shared members when the cursor's file lives on a different side from the type's declaration — symmetric with `typeMemberCompletions`, so the IntelliSense surface (completion + hover) stays consistent: anything not callable on the consumer's side is invisible.
func findMemberSym(c *compiler.CompilerContext, receiverTyp ir.TypID, name string, consumerSide ir.SideKind) *ir.Symbol {
	if receiverTyp == 0 || name == "" {
		return nil
	}
	ty, ok := c.TypeUniverse.GetByID(receiverTyp)
	if !ok || ty == nil {
		return nil
	}
	declSide := structTypeDeclSide(c, ty)
	filterShared := consumerSide != ir.SideShared && declSide != ir.SideShared && consumerSide != declSide
	switch ty.Kind {
	case ir.TK_Struct:
		for _, m := range ty.StructMethods {
			if filterShared && !m.IsShared {
				continue
			}
			if m.Name == name && m.Sym != 0 {
				if sym, ok := lookupSymbolGlobally(c, m.Sym); ok {
					return sym
				}
			}
		}
	case ir.TK_Interface:
		for _, m := range ty.InterfaceMethods {
			if m.Name != name {
				continue
			}
			if sym := findInterfaceMethodSym(c, ty, name); sym != nil {
				return sym
			}
		}
	case ir.TK_Enum:
		for _, m := range ty.EnumMethods {
			if m.Name != name {
				continue
			}
			if sym := findEnumMethodSym(c, ty, name); sym != nil {
				return sym
			}
		}
	}
	return nil
}

// lookupSymbolGlobally is the non-panicking package-walking variant of `pkg.Syms.GetByID`: scans every loaded package for a symbol with the given SymID. Returns (nil, false) when no package owns it.
func lookupSymbolGlobally(c *compiler.CompilerContext, id ir.SymID) (*ir.Symbol, bool) {
	for _, pkg := range c.Packages {
		if sym, ok := pkg.Syms.GetByID(id); ok {
			return sym, true
		}
	}
	return nil, false
}

// findInterfaceMethodSym walks the package that owns the interface type and returns the `*ir.Symbol` for the method named `name` on the interface so its doc-comment is available to hover.
func findInterfaceMethodSym(c *compiler.CompilerContext, ty *ir.Type, name string) *ir.Symbol {
	for _, pkg := range c.Packages {
		if pkg.Path == nil || pkg.Path.String() != ty.PackagePath {
			continue
		}
		for _, f := range pkg.Files {
			if f.Hir == nil {
				continue
			}
			for _, st := range f.Hir.Statements {
				iface, ok := st.(*ir.InterfaceDeclStmt)
				if !ok || iface.Name.Name != ty.InterfaceName {
					continue
				}
				for _, sig := range iface.Methods {
					if sig.Name.Name == name && sig.Name.Sym != 0 {
						if sym, ok := pkg.Syms.GetByID(sig.Name.Sym); ok {
							return sym
						}
					}
				}
			}
		}
	}
	return nil
}

// findEnumMethodSym walks the package that owns the enum type and returns the `*ir.Symbol` for the method named `name` on the enum.
func findEnumMethodSym(c *compiler.CompilerContext, ty *ir.Type, name string) *ir.Symbol {
	for _, pkg := range c.Packages {
		if pkg.Path == nil || pkg.Path.String() != ty.PackagePath {
			continue
		}
		for _, f := range pkg.Files {
			if f.Hir == nil {
				continue
			}
			for _, st := range f.Hir.Statements {
				e, ok := st.(*ir.EnumDeclStmt)
				if !ok || e.Name.Name != ty.EnumName {
					continue
				}
				for _, m := range e.Methods {
					if m.Name.Name == name && m.Name.Sym != 0 {
						if sym, ok := pkg.Syms.GetByID(m.Name.Sym); ok {
							return sym
						}
					}
				}
			}
		}
	}
	return nil
}

// lookupSessionTypeDoc returns the markdown doc-comment attached to the
// synthetic `sessions.Session` symbol in the built-in `sessions` package, or
// "" when the package hasn't been registered (e.g. during very early LSP
// startup). Used by the `@` hover so the popup carries the same description
// users would see hovering over a `sessions.Session`-typed value.
func lookupSessionTypeDoc(c *compiler.CompilerContext) string {
	pkg, ok := c.Packages[compiler.SessionsPackagePath]
	if !ok || pkg == nil {
		return ""
	}
	symID, ok := pkg.Scopes.LookupOnlyCurrent(pkg.Root, "Session")
	if !ok {
		return ""
	}
	sym, ok := pkg.Syms.GetByID(symID)
	if !ok || sym == nil {
		return ""
	}
	return sym.Doc
}

// renderDocComment shapes the raw doc-comment text for hover display. We pass
// markdown through untouched and just reformat the @-tags into a small bullet
// list: `@param x foo` → `**Parameters**\n- \`x\` - foo`. Unrecognised tags
// remain literal so users can invent their own without us swallowing them.
func renderDocComment(doc string) string {
	if doc == "" {
		return ""
	}
	lines := strings.Split(doc, "\n")
	var body []string
	var params []string
	var returns []string
	var examples []string
	var deprecated []string
	var sees []string
	var sinces []string
	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		switch {
		case strings.HasPrefix(trimmed, "@param "):
			rest := strings.TrimSpace(trimmed[len("@param "):])
			fields := strings.SplitN(rest, " ", 2)
			name := fields[0]
			desc := ""
			if len(fields) == 2 {
				desc = fields[1]
			}
			if desc != "" {
				params = append(params, "- `"+name+"` - "+desc)
			} else {
				params = append(params, "- `"+name+"`")
			}
		case strings.HasPrefix(trimmed, "@returns "):
			returns = append(returns, strings.TrimSpace(trimmed[len("@returns "):]))
		case strings.HasPrefix(trimmed, "@return "):
			returns = append(returns, strings.TrimSpace(trimmed[len("@return "):]))
		case trimmed == "@example" || strings.HasPrefix(trimmed, "@example "):
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "@example"))
			if rest != "" {
				examples = append(examples, rest)
			} else {
				examples = append(examples, "")
			}
		case strings.HasPrefix(trimmed, "@deprecated"):
			rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "@deprecated"))
			if rest != "" {
				deprecated = append(deprecated, rest)
			}
		case strings.HasPrefix(trimmed, "@since "):
			sinces = append(sinces, strings.TrimSpace(trimmed[len("@since "):]))
		case strings.HasPrefix(trimmed, "@see "):
			sees = append(sees, strings.TrimSpace(trimmed[len("@see "):]))
		default:
			body = append(body, ln)
		}
	}
	// Trim leading/trailing blank body lines.
	for len(body) > 0 && strings.TrimSpace(body[0]) == "" {
		body = body[1:]
	}
	for len(body) > 0 && strings.TrimSpace(body[len(body)-1]) == "" {
		body = body[:len(body)-1]
	}
	var out []string
	if len(deprecated) > 0 {
		out = append(out, "> ⚠ **Deprecated** - "+strings.Join(deprecated, " "))
	}
	if len(body) > 0 {
		out = append(out, strings.Join(body, "\n"))
	}
	if len(params) > 0 {
		out = append(out, "**Parameters**\n"+strings.Join(params, "\n"))
	}
	if len(returns) > 0 {
		out = append(out, "**Returns** - "+strings.Join(returns, " "))
	}
	if len(examples) > 0 {
		out = append(out, "**Example**\n"+strings.Join(examples, "\n"))
	}
	if len(sees) > 0 {
		out = append(out, "**See** - "+strings.Join(sees, ", "))
	}
	if len(sinces) > 0 {
		out = append(out, "_Since "+strings.Join(sinces, ", ")+"_")
	}
	return strings.Join(out, "\n\n")
}

// lookupSymbol resolves a SymID to a Symbol by scanning every loaded package's symbol arena. Returns the symbol + the package that owns it. We scan rather than maintain a global index because the LSP package mustn't reach into compiler internals.
func lookupSymbol(c *compiler.CompilerContext, id ir.SymID) (*ir.Symbol, *ir.PackageContext) {
	for _, pkg := range c.Packages {
		if sym, ok := pkg.Syms.GetByID(id); ok {
			return sym, pkg
		}
	}
	return nil, nil
}

// formatSymbolSignature builds the prefixed declaration line we show for a symbol: `let name: T`, `func name(params): T`, `type Name`, etc.
func formatSymbolSignature(tt *ir.TypeTable, sym *ir.Symbol) string {
	prefix := keywordFor(sym)
	typeStr := ""
	if sym.Typ != 0 {
		typeStr = formatType(tt, sym.Typ)
	}
	switch sym.Kind {
	case ir.SK_Function:
		if typeStr != "" {
			if ty, ok := tt.GetByID(sym.Typ); ok && ty.Kind == ir.TK_Function && ty.IsAsync {
				prefix = "async func"
			}
			return prefix + " " + sym.Name + stripFuncPrefix(typeStr)
		}
		return prefix + " " + sym.Name + "()"
	case ir.SK_Package:
		return "package " + sym.Name
	}
	if typeStr != "" {
		return fmt.Sprintf("%s %s: %s", prefix, sym.Name, typeStr)
	}
	return prefix + " " + sym.Name
}

func keywordFor(sym *ir.Symbol) string {
	switch sym.Kind {
	case ir.SK_Variable:
		if sym.IsConst() {
			return "const"
		}
		return "let"
	case ir.SK_Function:
		return "func"
	case ir.SK_Package:
		return "package"
	}
	return ""
}

// stripFuncPrefix removes the leading `func` (or `async func`) from a rendered
// function-type string so we can re-emit `<keyword> name(...)`. Our type printer
// always prefixes `func` (or `async func`); for declarations we want
// `func name(...)` rather than `func func(...)`.
func stripFuncPrefix(s string) string {
	if rest, ok := strings.CutPrefix(s, "async func"); ok {
		return rest
	}
	if rest, ok := strings.CutPrefix(s, "func"); ok {
		return rest
	}
	return s
}
