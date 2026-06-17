package javascript

import (
	"fmt"
	"sova/internal/codegen"
	"sova/internal/codegen/javascript/jsgen"
	"sova/internal/diag"
	"sova/internal/ir"
	"strings"
)

// reactiveWireVarOriginalNameJS mirrors the Go-side helper of the same name: returns the original (pre-mangle) name of a `@reactive wire let` declaration identified by `sym`, or "" when the symbol does not refer to one. The JS emitter consults this whenever it walks a `VarRef` / `AssignmentExpr` / `MultiAssignmentStmt` target so that reads of a reactive wire-let symbol go through the cell's `value` accessor (which calls `__sovaReactiveRead`) and writes go through the cell's setter (which fires observers and propagates into Strix's reactive system). The cache slot is populated by `analyze_wire` once per build under `ReactiveWireVarsCacheKey`.
func reactiveWireVarOriginalNameJS(ctx *codegen.EmitContext, sym ir.SymID) string {
	if ctx == nil || ctx.Cache == nil || sym == 0 {
		return ""
	}
	raw, ok := ctx.Cache["reactive_wire_vars"]
	if !ok {
		return ""
	}
	vars, ok := raw.([]*ir.VarDeclStmt)
	if !ok {
		return ""
	}
	for _, vd := range vars {
		if len(vd.Targets) == 0 || vd.Targets[0].Name == nil {
			continue
		}
		if vd.Targets[0].Name.Sym == sym {
			return vd.Targets[0].Name.Name
		}
	}
	return ""
}

// buildTypeDescriptorJSLiteral returns a JS object-literal source string describing `typID` for the wire-reification runtime (`__sovaReify`). The descriptor is compact and recursive:
//
//   primitive / any / none → {kind:"primitive"}
//   option<T>              → {kind:"option", elem: <desc T>}
//   []T  / [N]T            → {kind:"slice", elem: <desc T>}
//   map<K,V>               → {kind:"map", value: <desc V>}
//   (A, B, ...)            → {kind:"tuple", elems:[<desc A>, <desc B>, ...]}
//   struct X               → {kind:"struct", name:"<mangled-X>"}      // resolved via registry at runtime
//   enum / func / chan     → {kind:"primitive"}                       // passthrough; not reified
//
// Struct references are stored by name only so circular topologies (User has friends:[]User) terminate naturally; the runtime registry maps names to constructors. When a struct type's emitted class lives on the OTHER side (e.g. a backend-only type referenced from a frontend wire arg), the registry lookup returns nothing and the value passes through as a plain object — preserving the property bag without crashing.
func (e *CodeEmitter) buildTypeDescriptorJSLiteral(ctx *codegen.EmitContext, typID ir.TypID) string {
	if typID == 0 {
		return `{kind:"any"}`
	}
	ty, ok := ctx.Types.GetByID(typID)
	if !ok {
		return `{kind:"any"}`
	}
	switch ty.Kind {
	case ir.TK_PrimitiveAny, ir.TK_PrimitiveNone:
		return `{kind:"any"}`
	case ir.TK_PrimitiveInt, ir.TK_PrimitiveFloat, ir.TK_PrimitiveBool, ir.TK_PrimitiveString, ir.TK_PrimitiveChar, ir.TK_PrimitiveByte:
		return `{kind:"primitive"}`
	case ir.TK_Option:
		return fmt.Sprintf(`{kind:"option",elem:%s}`, e.buildTypeDescriptorJSLiteral(ctx, ty.ElemType))
	case ir.TK_Slice, ir.TK_Array:
		return fmt.Sprintf(`{kind:"slice",elem:%s}`, e.buildTypeDescriptorJSLiteral(ctx, ty.ElemType))
	case ir.TK_Map:
		return fmt.Sprintf(`{kind:"map",value:%s}`, e.buildTypeDescriptorJSLiteral(ctx, ty.ElemType))
	case ir.TK_Tuple:
		parts := make([]string, len(ty.Fields))
		for i, te := range ty.Fields {
			parts[i] = e.buildTypeDescriptorJSLiteral(ctx, te.Type)
		}
		return fmt.Sprintf(`{kind:"tuple",elems:[%s]}`, strings.Join(parts, ","))
	case ir.TK_Struct:
		if ty.IsExtern {
			return `{kind:"any"}`
		}
		structName := lookupMangledNameForType(ctx, typID)
		if structName == "" {
			return `{kind:"any"}`
		}
		return fmt.Sprintf(`{kind:"struct",name:%q}`, structName)
	default:
		return `{kind:"primitive"}`
	}
}

// lookupMangledNameForType resolves the mangled JS class name for a struct `typID` by walking every package's TypeDeclStmts and returning the mangled name of the one whose declaration-sym carries `typID`. Going via the declaration (rather than walking all symbols) is necessary because per-field symbols and other type-typed values often share the same Typ — picking the first sym-by-Typ would land on a variable's mangling instead of the class's mangling. Falls back to the empty string when the type does not correspond to a user-declared type-decl in any in-build package (e.g. extern struct types).
func lookupMangledNameForType(ctx *codegen.EmitContext, typID ir.TypID) string {
	for _, pkg := range ctx.Pkgs {
		if name := lookupMangledNameInPkg(ctx, pkg, typID); name != "" {
			return name
		}
	}
	for _, pkg := range ctx.TransPkgs {
		if name := lookupMangledNameInPkg(ctx, pkg, typID); name != "" {
			return name
		}
	}
	return ""
}

func lookupMangledNameInPkg(ctx *codegen.EmitContext, pkg *ir.PackageContext, typID ir.TypID) string {
	if pkg == nil {
		return ""
	}
	for _, f := range pkg.Files {
		if f.Hir == nil {
			continue
		}
		for _, st := range f.Hir.Statements {
			td, ok := st.(*ir.TypeDeclStmt)
			if !ok || td.IsExtern || td.Name.Sym == 0 {
				continue
			}
			sym, ok := pkg.Syms.GetByID(td.Name.Sym)
			if !ok || sym.Typ != typID {
				continue
			}
			if name, ok := ctx.Names.GetMangledName(td.Name.Sym); ok {
				return name
			}
		}
	}
	return ""
}

// emitTypeRegistration appends a `__sovaRegisterType("<mangled>", ClassRef, {field1:<desc1>, ...})` call after a class declaration so the wire-reification runtime can later construct instances with the correct prototype. Gated at runtime by a `typeof` check: if the reification helper isn't present (e.g. legacy builds without wires), the registration is a no-op.
func (e *CodeEmitter) emitTypeRegistration(ctx *codegen.EmitContext, typeName string, fields []*ir.TypeField) {
	parts := make([]string, 0, len(fields))
	for _, fld := range fields {
		if fld == nil || fld.Name.Name == "" {
			continue
		}
		desc := e.buildTypeDescriptorJSLiteral(ctx, fld.Type.Typ)
		parts = append(parts, fmt.Sprintf("%q:%s", fld.Name.Name, desc))
	}
	literal := fmt.Sprintf("{%s}", strings.Join(parts, ","))
	e.jf.Add(jsgen.Raw(fmt.Sprintf("if (typeof __sovaRegisterType === 'function') { __sovaRegisterType(%q, %s, %s); }", typeName, typeName, literal)))
}

// sharedSubsetTypeDecl returns a shallow-copy `*ir.TypeDeclStmt` whose Fields/Methods/Ctors/Casts contain only the members marked `shared` (or, when the enclosing file is `on shared`, the full set). When the type has no shared members and isn't declared `on shared`, returns nil — signalling "no cross-side emission needed". The returned copy points at the same field/method nodes as the original; downstream codegen treats it like any other type-decl. This is the surgical hook the Stage 3 design uses to produce a parallel JS class that exposes only the shared subset of a backend-declared type (and symmetrically a parallel Go struct from a frontend-declared type).
//
// Looking up the summary in the `shared_type_members` cache (populated by `pass_analyze_shared_members`) keeps codegen decoupled from the validator's specifics: codegen consumes the precomputed subset and does not re-derive it from `IsShared` markers.
func sharedSubsetTypeDecl(ctx *codegen.EmitContext, pkg *ir.PackageContext, td *ir.TypeDeclStmt) *ir.TypeDeclStmt {
	if ctx == nil || ctx.Cache == nil || td == nil || td.Name.Sym == 0 {
		return nil
	}
	raw, ok := ctx.Cache["shared_type_members"]
	if !ok {
		return nil
	}
	store, ok := raw.(map[ir.TypID]*ir.SharedTypeMembers)
	if !ok {
		return nil
	}
	sym, ok := pkg.Syms.GetByID(td.Name.Sym)
	if !ok || sym.Typ == 0 {
		return nil
	}
	summary, ok := store[sym.Typ]
	if !ok || summary == nil {
		return nil
	}
	tdCopy := *td
	tdCopy.Fields = summary.Fields
	tdCopy.Methods = summary.Methods
	tdCopy.Ctors = summary.Ctors
	tdCopy.Casts = summary.Casts
	return &tdCopy
}

// fieldHasReactiveAnnotationJS mirrors the Go-side helper: true when the annotation list carries an `@reactive` marker.
func fieldHasReactiveAnnotationJS(annos []ir.Annotation) bool {
	for _, a := range annos {
		if a.Name.Name == "reactive" {
			return true
		}
	}
	return false
}

// jsHasBuiltinAnnotation reports whether the declaration carries an `@builtin` marker. Same purpose as the Go-side counterpart: skip emitting host code for the placeholder declarations in `std/__globals__.sova`.
func jsHasBuiltinAnnotation(annos []ir.Annotation) bool {
	for _, a := range annos {
		if a.Name.Name == "builtin" {
			return true
		}
	}
	return false
}

// isReactiveFieldOfJS reports whether receiverSym refers to a value whose type has a field named fieldName carrying `@reactive`. Used to rewrite direct field writes into setter calls so observers fire.
func isReactiveFieldOfJS(ctx *codegen.EmitContext, pkg *ir.PackageContext, receiverSym ir.SymID, fieldName string) bool {
	if receiverSym == 0 {
		return false
	}
	sym, ok := pkg.Syms.GetByID(receiverSym)
	if !ok || sym.Typ == 0 {
		return false
	}
	ty, ok := ctx.Types.GetByID(sym.Typ)
	if !ok || ty.Kind != ir.TK_Struct {
		return false
	}
	for _, f := range ty.StructFields {
		if f.Name == fieldName {
			return f.IsReactive
		}
	}
	return false
}

// upperFirstJS capitalises the first rune of s. Used to derive method/storage-field suffixes from a field name.
func upperFirstJS(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	if r[0] >= 'a' && r[0] <= 'z' {
		r[0] = r[0] - 'a' + 'A'
	}
	return string(r)
}

// composableCalleeSymJS pulls the resolved type symbol from a composable call's callee for the JS emitter, mirroring the equivalent helper in the Go emitter.
func composableCalleeSymJS(callee ir.Expr) ir.SymID {
	switch c := callee.(type) {
	case *ir.VarRef:
		return c.Ref.Sym
	case *ir.FieldAccessExpr:
		if c.ResolvedSym != 0 {
			return c.ResolvedSym
		}
	}
	return 0
}

func lookupImportedPackage(ctx *codegen.EmitContext, currentPkg *ir.PackageContext, alias string) *ir.PackageContext {
	for _, pkg := range ctx.Pkgs {
		if pkg == currentPkg || len(pkg.Path) == 0 {
			continue
		}
		if pkg.Path[len(pkg.Path)-1] == alias {
			return pkg
		}
	}
	for _, pkg := range ctx.TransPkgs {
		if len(pkg.Path) == 0 {
			continue
		}
		if pkg.Path[len(pkg.Path)-1] == alias {
			return pkg
		}
	}
	return nil
}

func symName(ctx *codegen.EmitContext, sym ir.SymID) string {
	if name, ok := ctx.Names.GetMangledName(sym); ok {
		return name
	}
	panic(fmt.Sprintf("unresolved symbol: %d", sym))
}

func symOrigName(ctx *codegen.EmitContext, sym ir.SymID) string {
	if name, ok := ctx.Names.GetOriginalName(sym); ok {
		return name
	}
	return ""
}

func symNameWithUnused(ctx *codegen.EmitContext, pkg *ir.PackageContext, sym ir.SymID) string {
	if sym == 0 {
		return "_"
	}
	if symbol, ok := pkg.Syms.GetByID(sym); ok {
		if symbol.Flags&ir.SF_Unused != 0 {
			return "_"
		}
	}
	return symName(ctx, sym)
}

// nextDiscardName generates a unique discard name (_discard_0, _discard_1, etc.)
func (e *CodeEmitter) nextDiscardName() string {
	name := fmt.Sprintf("_discard_%d", e.discardCounter)
	e.discardCounter++
	return name
}

// bindForModule registers a host-language module and returns the local JS identifier the emitter will use to refer to it. Subsequent calls for the same module return the same bind. The first registration's IsDefault wins.
func (e *CodeEmitter) bindForModule(module string, isDefault bool) string {
	if e.moduleBinds == nil {
		e.moduleBinds = map[string]moduleBind{}
	}
	if existing, ok := e.moduleBinds[module]; ok {
		return existing.Name
	}
	name := sanitizeModuleName(module, len(e.moduleBinds))
	e.moduleBinds[module] = moduleBind{Name: name, IsDefault: isDefault}
	e.moduleOrder = append(e.moduleOrder, module)
	return name
}

// sanitizeModuleName produces a stable, JS-identifier-safe local name for a module path. The fallback counter disambiguates colliding short names from different scoped packages.
func sanitizeModuleName(module string, counter int) string {
	var b strings.Builder
	b.WriteString("__sova_mod_")
	lastSlash := strings.LastIndex(module, "/")
	core := module
	if lastSlash >= 0 {
		core = module[lastSlash+1:]
	}
	core = strings.TrimPrefix(core, "@")
	if core == "" {
		core = "anon"
	}
	for _, r := range core {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	if counter > 0 {
		b.WriteString(fmt.Sprintf("_%d", counter))
	}
	return b.String()
}

func (e *CodeEmitter) getNativeMapping(mapping *ir.ExternMapping, side ir.SideKind, module *string, isDefault bool) string {
	if mapping == nil {
		return ""
	}

	if mapping.Simple != nil {
		nativeCall := *mapping.Simple

		if module != nil && strings.Contains(nativeCall, "@mod") {
			bind := e.bindForModule(*module, isDefault)
			nativeCall = strings.ReplaceAll(nativeCall, "@mod", bind)
		}

		return nativeCall
	}

	if mapping.Shared != nil {
		if sideMapping, ok := mapping.Shared[side]; ok {
			nativeCall := sideMapping.NativeFunc

			if sideMapping.Module != nil {
				// Module is already included in NativeFunc for shared mappings
				return nativeCall
			}

			return nativeCall
		}
	}

	return ""
}

func (e *CodeEmitter) buildNativeRef(nativeCall string) *jsgen.Statement {
	// If the mapping is anything other than a dotted identifier path (e.g. an arrow function like `(s, n) => s.includes(n)` or a method-call expression), emit it verbatim wrapped in parens so the subsequent `.Call(args...)` produces a syntactically correct `(expr)(args)` form. This is what lets stdlib extern mappings carry inline JS without any compiler-side runtime helper.
	if !isDottedIdent(nativeCall) {
		return jsgen.Raw("(" + nativeCall + ")")
	}
	parts := strings.Split(nativeCall, ".")
	if len(parts) == 0 {
		return jsgen.Id(nativeCall)
	}

	result := jsgen.Id(parts[0])
	for i := 1; i < len(parts); i++ {
		result = result.Dot(parts[i])
	}

	return result
}

// isDottedIdent reports whether s is a chain of identifiers separated by dots (e.g. `console.log`, `strings.Contains`). Anything else - arrow functions, method-call expressions, parenthesised forms - is treated as a raw JS expression by the extern emitter.
func isDottedIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '.':
			if i == 0 {
				return false
			}
		case r == '_' || r == '$':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

func (e *CodeEmitter) buildNativeCallWithArgs(nativeCall string, args []*jsgen.Statement) *jsgen.Statement {
	ref := e.buildNativeRef(nativeCall)
	return ref.Call(args...)
}

func typeOfSym(pkg *ir.PackageContext, sym ir.SymID) ir.TypID {
	if s, ok := pkg.Syms.GetByID(sym); ok {
		return s.Typ
	}
	return 0
}

// getEnumSymbol looks up the symbol ID for an enum type by its name. Walks the package's
// symbol arena map directly so that sparse SymIDs (the arena is shared across packages, each
// pkg keeps only the IDs it allocated) don't fool the lookup into bailing early. We also
// extend the search across every loaded package so an enum defined in browserx and used from
// a consumer package still resolves; without that, cross-package enum case access in a struct
// field default emits the dreaded `unresolved symbol: 0` panic during JS codegen.
func getEnumSymbol(ctx *codegen.EmitContext, pkg *ir.PackageContext, enumName string) ir.SymID {
	if sym := findEnumInPackage(ctx, pkg, enumName); sym != 0 {
		return sym
	}
	for _, group := range [][]*ir.PackageContext{ctx.Pkgs, ctx.TransPkgs} {
		for _, p := range group {
			if p == nil || p == pkg {
				continue
			}
			if sym := findEnumInPackage(ctx, p, enumName); sym != 0 {
				return sym
			}
		}
	}
	return 0
}

func findEnumInPackage(ctx *codegen.EmitContext, pkg *ir.PackageContext, enumName string) ir.SymID {
	if pkg == nil {
		return 0
	}
	for sym, s := range pkg.Syms.ByID() {
		if s == nil || s.Kind != ir.SK_Function {
			continue
		}
		if orig, ok := ctx.Names.GetOriginalName(sym); ok && orig == enumName {
			return sym
		}
	}
	return 0
}

// getMethodSymbol looks up the symbol ID for an enum method by enum name and method name.
// Same gap-aware iteration as getEnumSymbol: walk the arena map directly and fall through to
// other loaded packages when the receiver isn't in the current one.
func getMethodSymbol(ctx *codegen.EmitContext, pkg *ir.PackageContext, enumName string, methodName string) ir.SymID {
	if sym := findMethodInPackage(ctx, pkg, methodName); sym != 0 {
		return sym
	}
	for _, group := range [][]*ir.PackageContext{ctx.Pkgs, ctx.TransPkgs} {
		for _, p := range group {
			if p == nil || p == pkg {
				continue
			}
			if sym := findMethodInPackage(ctx, p, methodName); sym != 0 {
				return sym
			}
		}
	}
	return 0
}

func findMethodInPackage(ctx *codegen.EmitContext, pkg *ir.PackageContext, methodName string) ir.SymID {
	if pkg == nil {
		return 0
	}
	for sym, s := range pkg.Syms.ByID() {
		if s == nil || s.Kind != ir.SK_Function {
			continue
		}
		if orig, ok := ctx.Names.GetOriginalName(sym); ok && orig == methodName {
			return sym
		}
	}
	return 0
}

// ============================================================================
// Source Position Helpers (for source maps)
// ============================================================================

// addPos adds source position to a statement from a TextSpan
func addPos(stmt *jsgen.Statement, span diag.TextSpan) *jsgen.Statement {
	if span.File == "" {
		return stmt
	}
	// TextSpan uses 1-based columns, jsgen.Pos expects 0-based columns
	return stmt.Pos(span.StartLn, span.StartCol-1, span.File)
}

// withPos is a helper that creates a statement and adds position info
func withPos(stmt *jsgen.Statement, node ir.Node) *jsgen.Statement {
	return addPos(stmt, node.Span())
}

// withPosFromStmt adds position from an IR statement
func withPosFromStmt(stmt *jsgen.Statement, irStmt ir.Stmt) *jsgen.Statement {
	return addPos(stmt, irStmt.Span())
}

// withPosFromExpr adds position from an IR expression
func withPosFromExpr(stmt *jsgen.Statement, irExpr ir.Expr) *jsgen.Statement {
	return addPos(stmt, irExpr.Span())
}

// pushLoop adds a new loop label to the stack and returns the label name.
func (e *CodeEmitter) pushLoop() string {
	label := fmt.Sprintf("loop_%d", len(e.loopLabels))
	e.loopLabels = append(e.loopLabels, label)
	return label
}

// popLoop removes the most recent loop label from the stack.
func (e *CodeEmitter) popLoop() {
	if len(e.loopLabels) > 0 {
		e.loopLabels = e.loopLabels[:len(e.loopLabels)-1]
	}
}

// getLoopLabel returns the label for a loop at the specified depth (1 = innermost).
func (e *CodeEmitter) getLoopLabel(depth int) string {
	if depth < 1 || depth > len(e.loopLabels) {
		return "" // Invalid depth
	}
	idx := len(e.loopLabels) - depth
	return e.loopLabels[idx]
}

// loopNeedsLabel checks if a loop at loopLevel needs a label.
// A loop needs a label if any break/continue from a nested position targets it with depth > 1.
func (e *CodeEmitter) loopNeedsLabel(stmts []ir.Stmt, loopLevel int) bool {
	return e.scanForTargetedBreaks(stmts, loopLevel, loopLevel)
}

// scanForTargetedBreaks scans statements for break/continue that target loopLevel.
// currentLevel is the nesting level of the statements being scanned.
func (e *CodeEmitter) scanForTargetedBreaks(stmts []ir.Stmt, loopLevel int, currentLevel int) bool {
	for _, st := range stmts {
		switch s := st.(type) {
		case *ir.BreakStmt:
			targetLevel := currentLevel - s.Depth + 1
			if targetLevel == loopLevel && s.Depth > 1 {
				return true
			}
		case *ir.ContinueStmt:
			targetLevel := currentLevel - s.Depth + 1
			if targetLevel == loopLevel && s.Depth > 1 {
				return true
			}
		case *ir.BlockStmt:
			if e.scanForTargetedBreaks(s.Stmts, loopLevel, currentLevel) {
				return true
			}
		case *ir.IfStmt:
			if e.scanForTargetedBreaks(s.Then.Stmts, loopLevel, currentLevel) {
				return true
			}
			for _, elif := range s.ElseIfs {
				if e.scanForTargetedBreaks(elif.Then.Stmts, loopLevel, currentLevel) {
					return true
				}
			}
			if s.Else != nil && e.scanForTargetedBreaks(s.Else.Stmts, loopLevel, currentLevel) {
				return true
			}
		case *ir.SwitchStmt:
			for _, c := range s.Cases {
				if e.scanForTargetedBreaks(c.Stmts, loopLevel, currentLevel) {
					return true
				}
			}
			if len(s.Default) > 0 && e.scanForTargetedBreaks(s.Default, loopLevel, currentLevel) {
				return true
			}
		case *ir.ForStmt:
			if e.scanForTargetedBreaks(s.Body.Stmts, loopLevel, currentLevel+1) {
				return true
			}
		case *ir.WhileStmt:
			if e.scanForTargetedBreaks(s.Body.Stmts, loopLevel, currentLevel+1) {
				return true
			}
		}
	}
	return false
}
