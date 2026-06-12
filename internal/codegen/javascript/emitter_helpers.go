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

// getEnumSymbol looks up the symbol ID for an enum type by its name.
func getEnumSymbol(ctx *codegen.EmitContext, pkg *ir.PackageContext, enumName string) ir.SymID {
	// Search through all symbols to find the enum by name
	for sym := ir.SymID(1); ; sym++ {
		s, ok := pkg.Syms.GetByID(sym)
		if !ok {
			break
		}
		if s.Kind == ir.SK_Function { // Enums are declared as SK_Function
			if orig, ok := ctx.Names.GetOriginalName(sym); ok && orig == enumName {
				return sym
			}
		}
	}
	return 0
}

// getMethodSymbol looks up the symbol ID for an enum method by enum name and method name.
func getMethodSymbol(ctx *codegen.EmitContext, pkg *ir.PackageContext, enumName string, methodName string) ir.SymID {
	// Search through all symbols to find the method
	for sym := ir.SymID(1); ; sym++ {
		s, ok := pkg.Syms.GetByID(sym)
		if !ok {
			break
		}
		if s.Kind == ir.SK_Function {
			if orig, ok := ctx.Names.GetOriginalName(sym); ok && orig == methodName {
				// Check if this method belongs to the enum
				// The method's scope should be the enum's scope
				return sym
			}
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
