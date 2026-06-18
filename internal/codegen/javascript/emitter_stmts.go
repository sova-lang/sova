package javascript

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"sova/internal/codegen"
	"sova/internal/codegen/javascript/jsgen"
	"sova/internal/ir"
)

// emitStmt emits a statement to the output
func (e *CodeEmitter) emitStmt(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, st ir.Stmt, topLevel bool) {
	switch s := st.(type) {
	case *ir.WireRulesetStmt:
		_ = s
	case *ir.TestDeclStmt, *ir.GroupDeclStmt, *ir.SetupStmt, *ir.TeardownStmt:
		// Handled by the JS-side test driver (see emitJSTestImplFuncs); the regular statement emitter ignores them so `on test` files don't leak stray top-level functions outside the test registry.
		return
	case *ir.BlockStmt:
		if topLevel {
			for _, stmt := range s.Stmts {
				e.emitStmt(ctx, pkg, f, stmt, true)
			}
		}

	case *ir.VarDeclStmt:
		e.emitVarDecl(ctx, pkg, f, s, topLevel)

	case *ir.FuncDeclStmt:
		if jsHasBuiltinAnnotation(s.Annotations) {
			break
		}
		if !s.IsWired {
			side := ir.SideShared
			if s.Side != nil {
				side = s.Side.Kind
			} else if f != nil {
				side = f.Side.Kind
			}
			if side == ir.SideBackend {
				break
			}
		}
		e.emitFuncDecl(ctx, pkg, f, s, topLevel)

	case *ir.ExternDeclStmt:
		e.emitExternDecl(ctx, pkg, f, s, topLevel)

	case *ir.EnumDeclStmt:
		e.emitEnumDecl(ctx, pkg, f, s, topLevel)

	case *ir.TypeDeclStmt:
		if s.IsExtern || jsHasBuiltinAnnotation(s.Annotations) {
			break
		}
		e.emitTypeDecl(ctx, pkg, f, s, topLevel)

	case *ir.MixinDeclStmt:
		_ = s

	case *ir.ImportStmt:
		_ = s

	case *ir.InterfaceDeclStmt:

	case *ir.ExprStmt:
		if !topLevel {
			e.jf.Add(e.buildExpr(ctx, pkg, f, s.Expr))
		}

	case *ir.MultiAssignmentStmt:
		e.emitMultiAssignment(ctx, pkg, f, s, topLevel)

	case *ir.FieldAssignmentStmt:
		if !topLevel {
			var recvName string
			if s.Receiver.Name == "this" && !e.suppressThisKeyword {
				recvName = "this"
			} else {
				recvName = symName(ctx, s.Receiver.Sym)
			}
			if s.Op == ir.OpAssign && len(s.Fields) == 1 && !e.inSyntheticCtor {
				fld := s.Fields[0]
				if isReactiveFieldOfJS(ctx, pkg, s.Receiver.Sym, fld.Name) {
					setterName := "set" + upperFirstJS(fld.Name)
					e.jf.Add(jsgen.Id(recvName).Dot(setterName).Call(e.buildExpr(ctx, pkg, f, s.Value)))
					break
				}
			}
			target := jsgen.Id(recvName)
			for _, fld := range s.Fields {
				target = target.Dot(fld.Name)
			}
			e.jf.Add(target.Op(string(s.Op)).Add(e.buildExpr(ctx, pkg, f, s.Value)))
		}

	case *ir.IndexAssignmentStmt:
		if !topLevel {
			recv := e.buildExpr(ctx, pkg, f, s.Receiver)
			idx := e.buildExpr(ctx, pkg, f, s.Index)
			rhs := e.buildExpr(ctx, pkg, f, s.Value)
			e.jf.Add(recv.Index(idx).Op(string(s.Op)).Add(rhs))
		}

	case *ir.IfStmt:
		e.emitIfStmt(ctx, pkg, f, s, topLevel)

	case *ir.SwitchStmt:
		e.emitSwitchStmt(ctx, pkg, f, s, topLevel)

	case *ir.ReturnStmt:
		e.emitReturnStmt(ctx, pkg, f, s, topLevel)

	case *ir.GuardStmt:
		e.emitGuardStmt(ctx, pkg, f, s, topLevel)

	case *ir.BreakStmt:
		if !topLevel {
			if s.Depth > 1 {
				label := e.getLoopLabel(s.Depth)
				if label != "" {
					e.jf.Add(jsgen.Break(label))
				} else {
					e.jf.Add(jsgen.Break())
				}
			} else {
				e.jf.Add(jsgen.Break())
			}
		}

	case *ir.ContinueStmt:
		if !topLevel {
			if s.Depth > 1 {
				label := e.getLoopLabel(s.Depth)
				if label != "" {
					e.jf.Add(jsgen.Continue(label))
				} else {
					e.jf.Add(jsgen.Continue())
				}
			} else {
				e.jf.Add(jsgen.Continue())
			}
		}

	case *ir.ForStmt:
		e.emitForStmt(ctx, pkg, f, s, topLevel)

	case *ir.WhileStmt:
		e.emitWhileStmt(ctx, pkg, f, s, topLevel)

	case *ir.TypeAliasStmt:
		// Aliases are erased at the type-resolution stage; no JS declaration needed.

	default:
		panic(fmt.Sprintf("javascript codegen: unhandled top-level statement type %T", st))
	}
}

func (e *CodeEmitter) emitVarDecl(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, s *ir.VarDeclStmt, topLevel bool) {
	if s.Embed != nil && topLevel {
		e.emitEmbeddedVar(ctx, pkg, s)
		return
	}
	if s.IsWired && topLevel {
		e.emitWiredVarStub(ctx, pkg, f, s)
		return
	}
	if len(s.Targets) == 1 {
		target := &s.Targets[0]

		if topLevel {
			if _, isFuncLit := s.Init.(*ir.FuncLitExpr); isFuncLit && target.Name != nil {
				var name string
				if symbol, ok := pkg.Syms.GetByID(target.Name.Sym); ok {
					if symbol.Flags&ir.SF_Unused != 0 {
						name = e.nextDiscardName()
					} else {
						name = symName(ctx, target.Name.Sym)
					}
				} else {
					name = symName(ctx, target.Name.Sym)
				}

				stmt := withPosFromStmt(jsgen.Let(name), s)
				orig := symOrigName(ctx, target.Name.Sym)
				if orig != "" {
					e.jf.Add(jsgen.Comment(fmt.Sprintf("Original name: %s", orig)))
				}
				e.jf.Add(stmt)

				rhs := e.buildExpr(ctx, pkg, f, s.Init)
				e.deferredInits = append(e.deferredInits, jsgen.Id(name).Op("=").Add(rhs))
				return
			}
		}

		if target.Name == nil {
			e.jf.Add(e.buildExpr(ctx, pkg, f, s.Init))
		} else {
			var name string
			if symbol, ok := pkg.Syms.GetByID(target.Name.Sym); ok {
				if symbol.Flags&ir.SF_Unused != 0 {
					name = e.nextDiscardName()
				} else {
					name = symName(ctx, target.Name.Sym)
				}
			} else {
				name = symName(ctx, target.Name.Sym)
			}

			var stmt *jsgen.Statement
			if s.IsConst {
				stmt = jsgen.Const(name)
			} else {
				stmt = jsgen.Let(name)
			}
			stmt = withPosFromStmt(stmt.Op("=").Add(e.buildExpr(ctx, pkg, f, s.Init)), s)

			orig := symOrigName(ctx, target.Name.Sym)
			if orig != "" {
				e.jf.Add(jsgen.Comment(fmt.Sprintf("Original name: %s", orig)))
			}
			e.jf.Add(stmt)
		}
	} else {
		hasNonDiscard := false
		var names []string
		for _, target := range s.Targets {
			if target.Name == nil {
				names = append(names, e.nextDiscardName())
			} else {
				if symbol, ok := pkg.Syms.GetByID(target.Name.Sym); ok {
					if symbol.Flags&ir.SF_Unused != 0 {
						names = append(names, e.nextDiscardName())
					} else {
						hasNonDiscard = true
						names = append(names, symName(ctx, target.Name.Sym))
					}
				} else {
					hasNonDiscard = true
					names = append(names, symName(ctx, target.Name.Sym))
				}
			}
		}

		if hasNonDiscard {
			kind := "let"
			if s.IsConst {
				kind = "const"
			}
			e.jf.Add(withPosFromStmt(jsgen.DestructArray(kind, names, e.buildExpr(ctx, pkg, f, s.Init)), s))
		}
	}
}

func (e *CodeEmitter) emitFuncDecl(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, s *ir.FuncDeclStmt, topLevel bool) {
	if s.IsWired {
		effSide := f.Side.Kind
		if s.Side != nil {
			effSide = s.Side.Kind
		}
		if effSide == ir.SideFrontend {
			e.emitFrontendWireImpl(ctx, pkg, f, s)
			return
		}
		e.emitWiredStub(ctx, pkg, f, s)
		return
	}
	funcName := symName(ctx, s.Name.Sym)
	orig := symOrigName(ctx, s.Name.Sym)

	if (orig == "main" || orig == "boot") && len(s.Params) == 0 && e.mangledMainName == "" {
		e.mangledMainName = funcName
		e.mainIsAsync = s.IsAsync
	}

	params := make([]string, len(s.Params))
	for i, param := range s.Params {
		paramName := symNameWithUnused(ctx, pkg, param.Name.Sym)
		params[i] = paramName
	}

	prevFunc := e.currentFunc
	e.currentFunc = s

	var body []jsgen.Code
	for _, stmt := range s.Body.Stmts {
		body = append(body, e.buildStmtAsCode(ctx, pkg, f, stmt))
	}

	e.currentFunc = prevFunc

	if funcBodyContainsDefer(s.Body) {
		body = wrapWithDeferDrain(body)
	}

	if orig != "" {
		e.jf.Add(jsgen.Comment(fmt.Sprintf("Original name: %s", orig)))
	}
	fb := jsgen.Func(funcName).Params(params...)
	if s.IsAsync {
		fb = fb.Async()
	}
	e.jf.Add(withPosFromStmt(fb.Block(body...), s))
}

// funcBodyContainsDefer reports whether the block contains a top-level DeferStmt. For the v1 JS-side defer support we only handle defers declared directly inside the function body (not inside nested control-flow). Nested defers would require a more invasive transform; flagged as a follow-up.
func funcBodyContainsDefer(blk *ir.BlockStmt) bool {
	if blk == nil {
		return false
	}
	for _, st := range blk.Stmts {
		if _, ok := st.(*ir.DeferStmt); ok {
			return true
		}
	}
	return false
}

// wrapWithDeferDrain wraps the function body with a local `__sovaDeferStack` slot and a try/finally that drains it in LIFO order at function exit. Defers that throw inside the drain are swallowed so a later defer cannot prevent earlier ones from firing (mirrors Go's behaviour where a panicking defer still allows recover to run other defers). Built as a Raw try-statement plus a Raw finally-tail since jsgen has no native try/finally builder; the function-body emitter joins the resulting code slice with `;\n` so the try/finally pairs syntactically.
func wrapWithDeferDrain(body []jsgen.Code) []jsgen.Code {
	var bodyStr strings.Builder
	for _, c := range body {
		switch v := c.(type) {
		case *jsgen.Statement:
			bodyStr.WriteString(v.String())
		default:
			bodyStr.WriteString(fmt.Sprintf("%v", v))
		}
		bodyStr.WriteString(";\n  ")
	}
	wrapped := fmt.Sprintf(
		"const __sovaDeferStack = []; try { %s} finally { for (let __i = __sovaDeferStack.length - 1; __i >= 0; __i--) { try { __sovaDeferStack[__i](); } catch (__e) {} } }",
		bodyStr.String(),
	)
	return []jsgen.Code{jsgen.Raw(wrapped)}
}

// emitFrontendWireImpl emits the actual JavaScript body of a frontend `wire func` (rather than the backend-side HTTP fetch stub) and registers it with the runtime WS dispatcher so backend-pushed RPC calls can reach it. Same body-emission path as a regular func; the only addition is the registration line.
func (e *CodeEmitter) emitFrontendWireImpl(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, s *ir.FuncDeclStmt) {
	funcName := symName(ctx, s.Name.Sym)
	orig := symOrigName(ctx, s.Name.Sym)

	params := make([]string, len(s.Params))
	for i, param := range s.Params {
		params[i] = symNameWithUnused(ctx, pkg, param.Name.Sym)
	}

	prevFunc := e.currentFunc
	e.currentFunc = s

	var body []jsgen.Code
	for _, stmt := range s.Body.Stmts {
		body = append(body, e.buildStmtAsCode(ctx, pkg, f, stmt))
	}

	e.currentFunc = prevFunc

	if orig != "" {
		e.jf.Add(jsgen.Comment(fmt.Sprintf("frontend wire: %s", orig)))
	}
	fb := jsgen.Func(funcName).Params(params...)
	if s.IsAsync {
		fb = fb.Async()
	}
	e.jf.Add(withPosFromStmt(fb.Block(body...), s))

	wireName := orig
	if wireName == "" {
		wireName = s.Name.Name
	}
	paramDescs := make([]string, len(s.Params))
	for i, p := range s.Params {
		if p != nil && p.Type != nil && p.Type.Typ != 0 {
			paramDescs[i] = e.buildTypeDescriptorJSLiteral(ctx, p.Type.Typ)
		} else {
			paramDescs[i] = `{kind:"any"}`
		}
	}
	descLiteral := "[" + strings.Join(paramDescs, ",") + "]"
	e.jf.Add(jsgen.Raw(fmt.Sprintf("__sovaRegisterWire(%q, %s, %s);", wireName, funcName, descLiteral)))
}

func (e *CodeEmitter) emitExternDecl(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, s *ir.ExternDeclStmt, topLevel bool) {
	targetSide := ir.SideFrontend
	if f.Side.Kind == ir.SideBackend {
		return // Skip backend externs in frontend
	}

	testMode := jsTestModeFromCache(ctx)
	skipMock := pkg != nil && pkg.Path.String() == "std/testing"

	for _, fn := range s.Funcs {
		if fn.Name.Sym != 0 {
			if sym, ok := pkg.Syms.GetByID(fn.Name.Sym); ok && sym.Flags&ir.SF_Reachable == 0 {
				continue
			}
		}
		funcName := symName(ctx, fn.Name.Sym)
		orig := symOrigName(ctx, fn.Name.Sym)

		params := make([]string, len(fn.Params))
		for i, param := range fn.Params {
			params[i] = fmt.Sprintf("__p%d", i)
			_ = param
		}

		nativeCall := e.getNativeMapping(fn.Mapping, targetSide, s.Module, s.IsDefaultImport)

		var argExprs []*jsgen.Statement
		for _, paramName := range params {
			argExprs = append(argExprs, jsgen.Id(paramName))
		}

		mockableName := orig
		if mockableName == "" {
			mockableName = fn.Name.Name
		}

		var body []jsgen.Code
		if testMode && !skipMock && mockableName != "" {
			argList := strings.Join(params, ", ")
			body = append(body, jsgen.Raw(fmt.Sprintf("var __mockHit = (typeof __sovaJSMockHook === 'function') ? __sovaJSMockHook(%q, [%s]) : null; if (__mockHit) return __mockHit.value", mockableName, argList)))
		}
		body = append(body, jsgen.Return(e.buildNativeCallWithArgs(nativeCall, argExprs)))

		if orig != "" {
			e.jf.Add(jsgen.Comment(fmt.Sprintf("Original name: %s", orig)))
		}
		e.jf.Add(withPosFromStmt(jsgen.Func(funcName).Params(params...).Block(body...), s))
	}

	for _, v := range s.Vars {
		varName := symName(ctx, v.Name.Sym)
		orig := symOrigName(ctx, v.Name.Sym)

		nativeCall := e.getNativeMapping(v.Mapping, targetSide, s.Module, s.IsDefaultImport)

		var stmt *jsgen.Statement
		if v.IsConst {
			stmt = jsgen.Const(varName)
		} else {
			stmt = jsgen.Let(varName)
		}
		stmt = stmt.Op("=").Add(e.buildNativeRef(nativeCall))

		if orig != "" {
			e.jf.Add(jsgen.Comment(fmt.Sprintf("Original name: %s", orig)))
		}
		e.jf.Add(withPosFromStmt(stmt, s))
	}
}

func (e *CodeEmitter) emitMultiAssignment(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, s *ir.MultiAssignmentStmt, topLevel bool) {
	if topLevel {
		return
	}

	hasNonDiscard := false
	var names []string
	var targetExprs []*jsgen.Statement
	for _, target := range s.Targets {
		if target.Name == nil {
			discardName := e.nextDiscardName()
			names = append(names, discardName)
			targetExprs = append(targetExprs, jsgen.Id(discardName))
			continue
		}
		if symbol, ok := pkg.Syms.GetByID(target.Name.Sym); ok && symbol.Flags&ir.SF_Unused != 0 {
			discardName := e.nextDiscardName()
			names = append(names, discardName)
			targetExprs = append(targetExprs, jsgen.Id(discardName))
			continue
		}
		hasNonDiscard = true
		if memberName, isMethod, ok := e.classMemberLookup(ctx, target.Name.Sym); ok && !isMethod {
			names = append(names, "this."+memberName)
			targetExprs = append(targetExprs, jsgen.Raw("this.").Add(jsgen.Id(memberName)))
		} else if reactiveWireVarOriginalNameJS(ctx, target.Name.Sym) != "" {
			cellName := symName(ctx, target.Name.Sym)
			names = append(names, cellName+".value")
			targetExprs = append(targetExprs, jsgen.Id(cellName).Dot("value"))
		} else {
			plain := symName(ctx, target.Name.Sym)
			names = append(names, plain)
			targetExprs = append(targetExprs, jsgen.Id(plain))
		}
	}

	if !hasNonDiscard {
		return
	}
	if len(targetExprs) == 1 {
		e.jf.Add(withPosFromStmt(targetExprs[0].Op("=").Add(e.buildExpr(ctx, pkg, f, s.Value)), s))
		return
	}
	e.jf.Add(withPosFromStmt(jsgen.DestructArray("let", names, e.buildExpr(ctx, pkg, f, s.Value)), s))
}

func (e *CodeEmitter) emitIfStmt(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, s *ir.IfStmt, topLevel bool) {
	if topLevel {
		return
	}
	e.jf.Add(e.buildIfStmt(ctx, pkg, f, s))
}

// buildIfStmt assembles the jsgen statement for an `if/else if/else` chain
// without emitting it. Used by both emitIfStmt (top-level if) and
// buildStmtAsCode (nested if inside another statement, where the caller needs
// the Statement returned rather than appended to the file).
func (e *CodeEmitter) buildIfStmt(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, s *ir.IfStmt) *jsgen.Statement {
	ifStmt := jsgen.If(e.buildExpr(ctx, pkg, f, s.Cond))

	var thenBody []jsgen.Code
	for _, stmt := range s.Then.Stmts {
		thenBody = append(thenBody, e.buildStmtAsCode(ctx, pkg, f, stmt))
	}
	ifStmt.Block(thenBody...)

	if len(s.ElseIfs) > 0 || s.Else != nil {
		var elseBody []jsgen.Code
		for _, elif := range s.ElseIfs {
			elifStmt := jsgen.If(e.buildExpr(ctx, pkg, f, elif.Cond))
			var elifBody []jsgen.Code
			for _, stmt := range elif.Then.Stmts {
				elifBody = append(elifBody, e.buildStmtAsCode(ctx, pkg, f, stmt))
			}
			elseBody = append(elseBody, elifStmt.Block(elifBody...).ToStatement())
		}
		if s.Else != nil {
			for _, stmt := range s.Else.Stmts {
				elseBody = append(elseBody, e.buildStmtAsCode(ctx, pkg, f, stmt))
			}
		}
		return withPosFromStmt(ifStmt.Else(elseBody...), s)
	}
	return withPosFromStmt(ifStmt.ToStatement(), s)
}

func (e *CodeEmitter) emitSwitchStmt(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, s *ir.SwitchStmt, topLevel bool) {
	if topLevel {
		return
	}
	e.jf.Add(e.buildSwitchStmt(ctx, pkg, f, s))
}

// buildSwitchStmt is the Statement-returning counterpart to emitSwitchStmt, so
// `buildStmtAsCode` can use it when a switch shows up inside a nested context.
func (e *CodeEmitter) buildSwitchStmt(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, s *ir.SwitchStmt) *jsgen.Statement {
	switchBuilder := jsgen.Switch(e.buildExpr(ctx, pkg, f, s.Expr))
	for _, c := range s.Cases {
		var caseBody []jsgen.Code
		for _, stmt := range c.Stmts {
			caseBody = append(caseBody, e.buildStmtAsCode(ctx, pkg, f, stmt))
		}
		if len(c.Values) > 0 {
			switchBuilder.Case(e.buildExpr(ctx, pkg, f, c.Values[0]), caseBody...)
		}
	}
	if len(s.Default) > 0 {
		var defaultBody []jsgen.Code
		for _, stmt := range s.Default {
			defaultBody = append(defaultBody, e.buildStmtAsCode(ctx, pkg, f, stmt))
		}
		return switchBuilder.Default(defaultBody...)
	}
	return switchBuilder.ToStatement()
}

func (e *CodeEmitter) emitReturnStmt(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, s *ir.ReturnStmt, topLevel bool) {
	if topLevel {
		return
	}

	if len(s.Results) == 0 {
		e.jf.Add(withPosFromStmt(jsgen.Return(), s))
	} else if len(s.Results) == 1 {
		e.jf.Add(withPosFromStmt(jsgen.Return(e.buildExpr(ctx, pkg, f, s.Results[0])), s))
	} else {
		var returnType *ir.Type
		if e.currentFunc != nil && e.currentFunc.ReturnType != nil {
			returnType, _ = ctx.Types.GetByID(e.currentFunc.ReturnType.Typ)
		}

		var exprs []*jsgen.Statement
		for _, result := range s.Results {
			exprs = append(exprs, e.buildExpr(ctx, pkg, f, result))
		}

		if returnType != nil && returnType.Kind == ir.TK_Tuple {
			e.jf.Add(withPosFromStmt(jsgen.Return(jsgen.Array(exprs...)), s))
		} else {
			e.jf.Add(withPosFromStmt(jsgen.Return(exprs...), s))
		}
	}
}

func (e *CodeEmitter) emitGuardStmt(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, s *ir.GuardStmt, topLevel bool) {
	if topLevel {
		return
	}

	notCond := jsgen.Unary("!", e.buildExpr(ctx, pkg, f, s.Cond))

	var guardBody []jsgen.Code
	if len(s.Returns) == 0 {
		guardBody = append(guardBody, jsgen.Return())
	} else if len(s.Returns) == 1 {
		guardBody = append(guardBody, jsgen.Return(e.buildExpr(ctx, pkg, f, s.Returns[0])))
	} else {
		var exprs []*jsgen.Statement
		for _, ret := range s.Returns {
			exprs = append(exprs, e.buildExpr(ctx, pkg, f, ret))
		}
		guardBody = append(guardBody, jsgen.Return(jsgen.Array(exprs...)))
	}

	e.jf.Add(jsgen.If(notCond).Block(guardBody...).ToStatement())
}

func (e *CodeEmitter) emitForStmt(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, s *ir.ForStmt, topLevel bool) {
	if topLevel {
		return
	}

	e.loopDepth++
	defer func() { e.loopDepth-- }()

	loopLevel := len(e.loopLabels) + 1
	needsLabel := e.loopNeedsLabel(s.Body.Stmts, loopLevel)

	label := e.pushLoop()
	defer e.popLoop()

	var loopStmt *jsgen.Statement

	if s.CondType == ir.ForCondInfinite {
		var body []jsgen.Code
		for _, stmt := range s.Body.Stmts {
			body = append(body, e.buildStmtAsCode(ctx, pkg, f, stmt))
		}
		loopStmt = jsgen.While(jsgen.Lit(true)).Block(body...)

	} else if s.CondType == ir.ForCondInt {
		initVarName := symNameWithUnused(ctx, pkg, s.CondInt.Init.Targets[0].Name.Sym)

		initStmt := jsgen.Let(initVarName).Op("=").Add(e.buildExpr(ctx, pkg, f, s.CondInt.Init.Init))
		condExpr := e.buildExpr(ctx, pkg, f, s.CondInt.Cond)
		postExpr := e.buildExpr(ctx, pkg, f, s.CondInt.Post)

		var body []jsgen.Code
		for _, stmt := range s.Body.Stmts {
			body = append(body, e.buildStmtAsCode(ctx, pkg, f, stmt))
		}

		loopStmt = jsgen.For(initStmt, condExpr, postExpr).Block(body...)

	} else if s.CondType == ir.ForCondRange {
		rangeVar := symNameWithUnused(ctx, pkg, s.CondRange.RangeVar.Sym)

		rangeExpr := e.buildRangeExpr(ctx, pkg, f, s.CondRange.RangeStart, s.CondRange.RangeEnd, nil)

		var body []jsgen.Code
		for _, stmt := range s.Body.Stmts {
			body = append(body, e.buildStmtAsCode(ctx, pkg, f, stmt))
		}

		loopStmt = jsgen.For(
			jsgen.Const(rangeVar).Op("of").Add(rangeExpr),
			nil,
			nil,
		).Block(body...)

	} else if s.CondType == ir.ForCondIn {
		inFirstVar := symNameWithUnused(ctx, pkg, s.CondIn.InFirstVar.Sym)

		var inSecondVar string
		if s.CondIn.InSecondVar != nil {
			inSecondVar = symNameWithUnused(ctx, pkg, s.CondIn.InSecondVar.Sym)
		}

		iterExpr := e.buildExpr(ctx, pkg, f, s.CondIn.IterExpr)

		isMap := ctx.Types.IsTypeOfKind(s.CondIn.IterExpr.GetType(), ir.TK_Map)
		isIterable := s.CondIn.IterNextSym != 0

		var body []jsgen.Code
		for _, stmt := range s.Body.Stmts {
			body = append(body, e.buildStmtAsCode(ctx, pkg, f, stmt))
		}

		if isIterable {
			loopStmt = e.buildIterableLoop(ctx, s, iterExpr, inFirstVar, inSecondVar, body)
		} else if isMap {
			if inSecondVar != "" {
				destructVars := "[" + inFirstVar + "," + inSecondVar + "]"
				loopStmt = jsgen.For(
					jsgen.Const(destructVars).Op("of").Add(jsgen.Id("Object").Dot("entries").Call(iterExpr)),
					nil,
					nil,
				).Block(body...)
			} else {
				loopStmt = jsgen.For(
					jsgen.Const(inFirstVar).Op("of").Add(jsgen.Id("Object").Dot("keys").Call(iterExpr)),
					nil,
					nil,
				).Block(body...)
			}
		} else {
			if inSecondVar != "" {
				destructVars := "[" + inSecondVar + "," + inFirstVar + "]"
				loopStmt = jsgen.For(
					jsgen.Const(destructVars).Op("of").Add(iterExpr.Dot("entries").Call()),
					nil,
					nil,
				).Block(body...)
			} else {
				loopStmt = jsgen.For(
					jsgen.Const(inFirstVar).Op("of").Add(iterExpr),
					nil,
					nil,
				).Block(body...)
			}
		}
	} else {
		panic("unsupported for loop condition type")
	}

	if needsLabel {
		loopStmt = jsgen.Id(label).Op(":").Add(loopStmt)
	}

	e.jf.Add(loopStmt)
}

// buildIterableLoop lowers `for x in iter { body }` (and the optional `for x, idx in iter` form) into a JS while-loop that calls `iter.<next>()` once per iteration, breaks when the option is null, and exposes the unwrapped value as `x`. Used by both emitForStmt and buildForStmt so nested iterables stay self-contained.
func (e *CodeEmitter) buildIterableLoop(ctx *codegen.EmitContext, s *ir.ForStmt, iterExpr *jsgen.Statement, inFirstVar, inSecondVar string, body []jsgen.Code) *jsgen.Statement {
	nextName := symName(ctx, s.CondIn.IterNextSym)
	optName := "__sovaIterNext_" + inFirstVar
	inner := []jsgen.Code{
		jsgen.Let(optName).Op("=").Add(iterExpr.Dot(nextName).Call()),
		jsgen.Raw(fmt.Sprintf("if (%s == null) break;", optName)),
		jsgen.Let(inFirstVar).Op("=").Id(optName),
	}
	if inSecondVar != "" {
		idxName := "__sovaIterIdx_" + inFirstVar
		inner = append(inner, jsgen.Raw(idxName+"++"), jsgen.Let(inSecondVar).Op("=").Id(idxName))
		loop := jsgen.While(jsgen.Lit(true)).Block(append(inner, body...)...)
		return jsgen.Raw(fmt.Sprintf("{ let %s = -1; ", idxName)).Add(loop).Add(jsgen.Raw(" }"))
	}
	return jsgen.While(jsgen.Lit(true)).Block(append(inner, body...)...)
}

func (e *CodeEmitter) emitWhileStmt(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, s *ir.WhileStmt, topLevel bool) {
	if topLevel {
		return
	}

	e.loopDepth++
	defer func() { e.loopDepth-- }()

	loopLevel := len(e.loopLabels) + 1
	needsLabel := e.loopNeedsLabel(s.Body.Stmts, loopLevel)

	label := e.pushLoop()
	defer e.popLoop()

	var body []jsgen.Code
	for _, stmt := range s.Body.Stmts {
		body = append(body, e.buildStmtAsCode(ctx, pkg, f, stmt))
	}

	loopStmt := jsgen.While(e.buildExpr(ctx, pkg, f, s.Cond)).Block(body...)

	if needsLabel {
		loopStmt = jsgen.Id(label).Op(":").Add(loopStmt)
	}

	e.jf.Add(loopStmt)
}

// buildForStmt mirrors emitForStmt but returns the resulting statement instead
// of appending it. Loop-label and loop-depth bookkeeping happens here so a
// nested `break N` referencing this loop still resolves correctly when it is
// emitted via buildStmtAsCode.
func (e *CodeEmitter) buildForStmt(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, s *ir.ForStmt) *jsgen.Statement {
	e.loopDepth++
	defer func() { e.loopDepth-- }()

	loopLevel := len(e.loopLabels) + 1
	needsLabel := e.loopNeedsLabel(s.Body.Stmts, loopLevel)

	label := e.pushLoop()
	defer e.popLoop()

	var loopStmt *jsgen.Statement

	if s.CondType == ir.ForCondInfinite {
		var body []jsgen.Code
		for _, stmt := range s.Body.Stmts {
			body = append(body, e.buildStmtAsCode(ctx, pkg, f, stmt))
		}
		loopStmt = jsgen.While(jsgen.Lit(true)).Block(body...)
	} else if s.CondType == ir.ForCondInt {
		initVarName := symNameWithUnused(ctx, pkg, s.CondInt.Init.Targets[0].Name.Sym)
		initStmt := jsgen.Let(initVarName).Op("=").Add(e.buildExpr(ctx, pkg, f, s.CondInt.Init.Init))
		condExpr := e.buildExpr(ctx, pkg, f, s.CondInt.Cond)
		postExpr := e.buildExpr(ctx, pkg, f, s.CondInt.Post)
		var body []jsgen.Code
		for _, stmt := range s.Body.Stmts {
			body = append(body, e.buildStmtAsCode(ctx, pkg, f, stmt))
		}
		loopStmt = jsgen.For(initStmt, condExpr, postExpr).Block(body...)
	} else if s.CondType == ir.ForCondRange {
		rangeVar := symNameWithUnused(ctx, pkg, s.CondRange.RangeVar.Sym)
		rangeExpr := e.buildRangeExpr(ctx, pkg, f, s.CondRange.RangeStart, s.CondRange.RangeEnd, nil)
		var body []jsgen.Code
		for _, stmt := range s.Body.Stmts {
			body = append(body, e.buildStmtAsCode(ctx, pkg, f, stmt))
		}
		loopStmt = jsgen.For(jsgen.Const(rangeVar).Op("of").Add(rangeExpr), nil, nil).Block(body...)
	} else if s.CondType == ir.ForCondIn {
		inFirstVar := symNameWithUnused(ctx, pkg, s.CondIn.InFirstVar.Sym)
		var inSecondVar string
		if s.CondIn.InSecondVar != nil {
			inSecondVar = symNameWithUnused(ctx, pkg, s.CondIn.InSecondVar.Sym)
		}
		iterExpr := e.buildExpr(ctx, pkg, f, s.CondIn.IterExpr)
		isMap := ctx.Types.IsTypeOfKind(s.CondIn.IterExpr.GetType(), ir.TK_Map)
		isIterable := s.CondIn.IterNextSym != 0
		var body []jsgen.Code
		for _, stmt := range s.Body.Stmts {
			body = append(body, e.buildStmtAsCode(ctx, pkg, f, stmt))
		}
		if isIterable {
			loopStmt = e.buildIterableLoop(ctx, s, iterExpr, inFirstVar, inSecondVar, body)
		} else if isMap {
			if inSecondVar != "" {
				destructVars := "[" + inFirstVar + "," + inSecondVar + "]"
				loopStmt = jsgen.For(jsgen.Const(destructVars).Op("of").Add(jsgen.Id("Object").Dot("entries").Call(iterExpr)), nil, nil).Block(body...)
			} else {
				loopStmt = jsgen.For(jsgen.Const(inFirstVar).Op("of").Add(jsgen.Id("Object").Dot("keys").Call(iterExpr)), nil, nil).Block(body...)
			}
		} else {
			if inSecondVar != "" {
				destructVars := "[" + inSecondVar + "," + inFirstVar + "]"
				loopStmt = jsgen.For(jsgen.Const(destructVars).Op("of").Add(iterExpr.Dot("entries").Call()), nil, nil).Block(body...)
			} else {
				loopStmt = jsgen.For(jsgen.Const(inFirstVar).Op("of").Add(iterExpr), nil, nil).Block(body...)
			}
		}
	} else {
		panic("unsupported for loop condition type")
	}

	if needsLabel {
		loopStmt = jsgen.Id(label).Op(":").Add(loopStmt)
	}
	return loopStmt
}

// buildWhileStmt is the Statement-returning counterpart to emitWhileStmt.
func (e *CodeEmitter) buildWhileStmt(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, s *ir.WhileStmt) *jsgen.Statement {
	e.loopDepth++
	defer func() { e.loopDepth-- }()

	loopLevel := len(e.loopLabels) + 1
	needsLabel := e.loopNeedsLabel(s.Body.Stmts, loopLevel)

	label := e.pushLoop()
	defer e.popLoop()

	var body []jsgen.Code
	for _, stmt := range s.Body.Stmts {
		body = append(body, e.buildStmtAsCode(ctx, pkg, f, stmt))
	}
	loopStmt := jsgen.While(e.buildExpr(ctx, pkg, f, s.Cond)).Block(body...)
	if needsLabel {
		loopStmt = jsgen.Id(label).Op(":").Add(loopStmt)
	}
	return loopStmt
}

func (e *CodeEmitter) emitTypeDecl(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, s *ir.TypeDeclStmt, topLevel bool) {
	prevTypeDecl := e.currentTypeDecl
	e.currentTypeDecl = s
	defer func() { e.currentTypeDecl = prevTypeDecl }()
	typeName := symName(ctx, s.Name.Sym)

	parentClass := ""
	for _, ref := range s.MixedIn {
		if ref.Sym == 0 {
			continue
		}
		symPkg := pkg
		if ref.Qualifier != "" {
			if found := lookupImportedPackage(ctx, pkg, ref.Qualifier); found != nil {
				symPkg = found
			}
		}
		embedSym, ok := symPkg.Syms.GetByID(ref.Sym)
		if !ok || embedSym.Typ == 0 {
			continue
		}
		embedTy, ok := ctx.Types.GetByID(embedSym.Typ)
		if !ok || embedTy.Kind != ir.TK_Struct {
			continue
		}
		if parentClass != "" {
			break
		}
		if embedTy.IsExtern {
			parentClass = embedTy.StructName
		} else {
			parentClass = symName(ctx, ref.Sym)
		}
	}

	var sb strings.Builder
	if parentClass != "" {
		sb.WriteString(fmt.Sprintf("class %s extends %s {\n", typeName, parentClass))
	} else {
		sb.WriteString(fmt.Sprintf("class %s {\n", typeName))
	}

	var primaryCtor *ir.CtorDecl
	if len(s.Ctors) > 0 {
		primaryCtor = s.Ctors[0]
	}

	if primaryCtor != nil && primaryCtor.Body != nil {
		paramNames := make([]string, 0, len(primaryCtor.Params))
		for _, p := range primaryCtor.Params {
			name := symNameWithUnused(ctx, pkg, p.Name.Sym)
			if p.Default != nil {
				defStr := e.buildExpr(ctx, pkg, f, p.Default).String()
				paramNames = append(paramNames, fmt.Sprintf("%s = %s", name, defStr))
			} else {
				paramNames = append(paramNames, name)
			}
		}
		sb.WriteString(fmt.Sprintf("  constructor(%s) {\n", strings.Join(paramNames, ", ")))
		if parentClass != "" {
			sb.WriteString("    super();\n")
		}
		for _, field := range s.Fields {
			reactive := fieldHasReactiveAnnotationJS(field.Annotations)
			if reactive {
				sb.WriteString(fmt.Sprintf("    this.__obs%s = [];\n", upperFirstJS(field.Name.Name)))
			}
			if field.Default == nil {
				continue
			}
			valStr := e.buildExpr(ctx, pkg, f, field.Default).String()
			if reactive {
				sb.WriteString(fmt.Sprintf("    this._%s = %s;\n", field.Name.Name, valStr))
			} else {
				sb.WriteString(fmt.Sprintf("    this.%s = %s;\n", field.Name.Name, valStr))
			}
		}
		prevSynth := e.inSyntheticCtor
		if primaryCtor.IsSynthetic {
			e.inSyntheticCtor = true
		}
		for _, stmt := range primaryCtor.Body.Stmts {
			code := e.buildStmtAsCode(ctx, pkg, f, stmt)
			if stmtCode, ok := code.(*jsgen.Statement); ok {
				sb.WriteString("    " + stmtCode.String() + ";\n")
			}
		}
		e.inSyntheticCtor = prevSynth
		sb.WriteString("  }\n")
	} else {
		sb.WriteString("  constructor() {\n")
		if parentClass != "" {
			sb.WriteString("    super();\n")
		}
		for _, field := range s.Fields {
			reactive := fieldHasReactiveAnnotationJS(field.Annotations)
			if reactive {
				sb.WriteString(fmt.Sprintf("    this.__obs%s = [];\n", upperFirstJS(field.Name.Name)))
			}
			var valStr string
			if field.Default != nil {
				valStr = e.buildExpr(ctx, pkg, f, field.Default).String()
			} else {
				valStr = jsZeroFor(ctx, field.Type)
			}
			if reactive {
				sb.WriteString(fmt.Sprintf("    this._%s = %s;\n", field.Name.Name, valStr))
			} else {
				sb.WriteString(fmt.Sprintf("    this.%s = %s;\n", field.Name.Name, valStr))
			}
		}
		sb.WriteString("  }\n")
	}

	for _, field := range s.Fields {
		if !fieldHasReactiveAnnotationJS(field.Annotations) {
			continue
		}
		fieldName := field.Name.Name
		exported := upperFirstJS(fieldName)
		obsName := "__obs" + exported
		storeName := "_" + fieldName
		sb.WriteString(fmt.Sprintf("  get %s() {\n", fieldName))
		sb.WriteString("    const __t = globalThis.__sovaReactiveRead;\n")
		sb.WriteString(fmt.Sprintf("    if (__t) { __t(this, %q); }\n", fieldName))
		sb.WriteString(fmt.Sprintf("    return this.%s;\n", storeName))
		sb.WriteString("  }\n")
		sb.WriteString(fmt.Sprintf("  set %s(v) { this.set%s(v); }\n", fieldName, exported))
		sb.WriteString(fmt.Sprintf("  set%s(v) {\n", exported))
		sb.WriteString(fmt.Sprintf("    const __old = this.%s;\n", storeName))
		sb.WriteString(fmt.Sprintf("    this.%s = v;\n", storeName))
		sb.WriteString(fmt.Sprintf("    for (const __o of this.%s) { __o(__old, v); }\n", obsName))
		sb.WriteString("  }\n")
		sb.WriteString(fmt.Sprintf("  observe%s(__fn) {\n", exported))
		sb.WriteString(fmt.Sprintf("    const __idx = this.%s.length;\n", obsName))
		sb.WriteString(fmt.Sprintf("    this.%s.push(__fn);\n", obsName))
		sb.WriteString("    return () => {\n")
		sb.WriteString(fmt.Sprintf("      if (__idx >= this.%s.length) return null;\n", obsName))
		sb.WriteString(fmt.Sprintf("      this.%s.splice(__idx, 1);\n", obsName))
		sb.WriteString("      return null;\n")
		sb.WriteString("    };\n")
		sb.WriteString("  }\n")
	}

	hasUserToString := false
	hasUserHashCode := false
	for _, method := range s.Methods {
		fn := method.Func
		methodName := symName(ctx, fn.Name.Sym)
		if fn.Name.Name == "toString" {
			hasUserToString = true
		}
		if fn.Name.Name == "hashCode" {
			hasUserHashCode = true
		}
		params := ""
		for i, param := range fn.Params {
			if i > 0 {
				params += ", "
			}
			params += symNameWithUnused(ctx, pkg, param.Name.Sym)
		}
		prefix := ""
		if fn.IsAsync {
			prefix = "async "
		}
		sb.WriteString(fmt.Sprintf("  %s%s(%s) {\n", prefix, methodName, params))
		for _, stmt := range fn.Body.Stmts {
			code := e.buildStmtAsCode(ctx, pkg, f, stmt)
			if stmtCode, ok := code.(*jsgen.Statement); ok {
				sb.WriteString("    " + stmtCode.String() + ";\n")
			}
		}
		sb.WriteString("  }\n")
	}

	if !hasUserToString {
		var inner strings.Builder
		inner.WriteString(s.Name.Name)
		inner.WriteString("{")
		for i, field := range s.Fields {
			if i > 0 {
				inner.WriteString(", ")
			}
			inner.WriteString(fmt.Sprintf("%s: ${this.%s}", field.Name.Name, field.Name.Name))
		}
		inner.WriteString("}")
		sb.WriteString(fmt.Sprintf("  toString() { return `%s`; }\n", inner.String()))
	}
	if !hasUserHashCode {
		var repr strings.Builder
		repr.WriteString(s.Name.Name)
		for _, field := range s.Fields {
			repr.WriteString(fmt.Sprintf("|${this.%s}", field.Name.Name))
		}
		sb.WriteString(fmt.Sprintf("  hashCode() { let h = 5381; const r = `%s`; for (let i = 0; i < r.length; i++) { h = ((h << 5) + h) + r.charCodeAt(i); } return h | 0; }\n", repr.String()))
	}

	sb.WriteString("}")
	e.jf.Add(jsgen.Raw(sb.String()))
	e.emitTypeRegistration(ctx, typeName, s.Fields)

	prevSuppress := e.suppressThisKeyword
	e.suppressThisKeyword = true
	for i, ctor := range s.Ctors {
		ctorName := symName(ctx, ctor.Sym)
		params := make([]string, len(ctor.Params))
		argRefs := make([]*jsgen.Statement, len(ctor.Params))
		for j, param := range ctor.Params {
			params[j] = symNameWithUnused(ctx, pkg, param.Name.Sym)
			argRefs[j] = jsgen.Id(params[j])
		}
		if i == 0 {
			body := []jsgen.Code{
				jsgen.Return(jsgen.Raw(fmt.Sprintf("new %s", typeName)).Call(argRefs...)),
			}
			e.jf.Add(jsgen.Func(ctorName).Params(params...).Block(body...))
			continue
		}
		thisName := symName(ctx, ctor.ThisSym)
		var body []jsgen.Code
		body = append(body, jsgen.Let(thisName).Op("=").Raw(fmt.Sprintf("new %s()", typeName)))
		for _, stmt := range ctor.Body.Stmts {
			body = append(body, e.buildStmtAsCode(ctx, pkg, f, stmt))
		}
		body = append(body, jsgen.Return(jsgen.Id(thisName)))
		e.jf.Add(jsgen.Func(ctorName).Params(params...).Block(body...))
	}
	for _, cast := range s.Casts {
		castName := symName(ctx, cast.Sym)
		paramName := symNameWithUnused(ctx, pkg, cast.Param.Name.Sym)
		var body []jsgen.Code
		if cast.Body != nil {
			for _, stmt := range cast.Body.Stmts {
				body = append(body, e.buildStmtAsCode(ctx, pkg, f, stmt))
			}
		}
		e.jf.Add(jsgen.Func(castName).Params(paramName).Block(body...))
	}
	e.suppressThisKeyword = prevSuppress
}

func jsZeroFor(ctx *codegen.EmitContext, tr *ir.TypeRef) string {
	if tr == nil {
		return "null"
	}
	if ty, ok := ctx.Types.GetByID(tr.Typ); ok {
		switch ty.Kind {
		case ir.TK_PrimitiveInt, ir.TK_PrimitiveFloat:
			return "0"
		case ir.TK_PrimitiveString, ir.TK_PrimitiveChar:
			return "\"\""
		case ir.TK_PrimitiveBool:
			return "false"
		case ir.TK_Slice, ir.TK_Array, ir.TK_Tuple:
			return "[]"
		case ir.TK_Map:
			return "{}"
		}
	}
	return "null"
}

func (e *CodeEmitter) emitEnumDecl(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, s *ir.EnumDeclStmt, topLevel bool) {
	enumName := symName(ctx, s.Name.Sym)
	enumTyp, _ := ctx.Types.GetByID(typeOfSym(pkg, s.Name.Sym))

	if enumTyp != nil && enumTyp.IsNumeric {
		// Numeric enum: Object.freeze with numeric values as a single statement
		var sb strings.Builder
		sb.WriteString("Object.freeze({\n")
		for i, c := range enumTyp.EnumCases {
			sb.WriteString(fmt.Sprintf("  %s: %d", c.Name, c.Value))
			if i < len(enumTyp.EnumCases)-1 {
				sb.WriteString(",")
			}
			sb.WriteString("\n")
		}
		sb.WriteString("})")
		e.jf.Add(jsgen.Const(enumName).Op("=").Raw(sb.String()))
	} else if enumTyp != nil {
		// Payload enum: Class with __ordinal, __name, fields, and methods
		// Build class as a single string
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("class %s {\n", enumName))

		// Constructor
		constructorParams := "__ordinal, __name"
		for _, fld := range enumTyp.EnumFields {
			constructorParams += ", " + fld.Name
		}
		sb.WriteString(fmt.Sprintf("  constructor(%s) {\n", constructorParams))
		sb.WriteString("    this.__ordinal = __ordinal;\n")
		sb.WriteString("    this.__name = __name;\n")
		for _, fld := range enumTyp.EnumFields {
			sb.WriteString(fmt.Sprintf("    this.%s = %s;\n", fld.Name, fld.Name))
		}
		sb.WriteString("  }\n")

		// toString method
		sb.WriteString("  toString() { return this.__name; }\n")

		// hashCode method
		sb.WriteString("  hashCode() { return this.__ordinal; }\n")

		// User-defined methods
		for _, method := range s.Methods {
			methodName := symName(ctx, method.Name.Sym)

			params := ""
			for i, param := range method.Params {
				if i > 0 {
					params += ", "
				}
				params += symNameWithUnused(ctx, pkg, param.Name.Sym)
			}

			sb.WriteString(fmt.Sprintf("  %s(%s) {\n", methodName, params))
			for _, stmt := range method.Body.Stmts {
				code := e.buildStmtAsCode(ctx, pkg, f, stmt)
				if stmtCode, ok := code.(*jsgen.Statement); ok {
					sb.WriteString("    " + stmtCode.String() + ";\n")
				}
			}
			sb.WriteString("  }\n")
		}

		sb.WriteString("}")
		e.jf.Add(jsgen.Raw(sb.String()))

		// Case instances
		for i, c := range s.Cases {
			args := fmt.Sprintf("%d, \"%s\"", i, c.Name.Name)
			for _, arg := range c.Args {
				argStr := e.buildExpr(ctx, pkg, f, arg).String()
				args += ", " + argStr
			}
			// Fill defaults for missing args
			for j := len(c.Args); j < len(s.Fields); j++ {
				if s.Fields[j].Default != nil {
					defStr := e.buildExpr(ctx, pkg, f, s.Fields[j].Default).String()
					args += ", " + defStr
				}
			}

			e.jf.Add(jsgen.Const(enumName + c.Name.Name).Op("=").Raw(fmt.Sprintf("new %s(%s)", enumName, args)))
		}

		// Values array for iteration
		var caseNames []string
		for _, c := range s.Cases {
			caseNames = append(caseNames, enumName+c.Name.Name)
		}
		caseNamesStr := ""
		for i, name := range caseNames {
			if i > 0 {
				caseNamesStr += ", "
			}
			caseNamesStr += name
		}
		e.jf.Add(jsgen.Const(enumName + "Values").Op("=").Raw(fmt.Sprintf("Object.freeze([%s])", caseNamesStr)))
	}
}

// buildStmtAsCode builds a statement as a Code interface for use in blocks
func (e *CodeEmitter) buildStmtAsCode(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, st ir.Stmt) jsgen.Code {
	switch s := st.(type) {
	case *ir.AssertStmt:
		return e.emitJSAssertStmt(ctx, pkg, f, s)
	case *ir.AsSessionStmt:
		var inner []jsgen.Code
		if s.Body != nil {
			for _, st := range s.Body.Stmts {
				inner = append(inner, e.buildStmtAsCode(ctx, pkg, f, st))
			}
		}
		return jsgen.Raw("(").Add(jsgen.Arrow().Block(inner...)).Add(jsgen.Raw(")()"))
	case *ir.GoStmt:
		var body []jsgen.Code
		if s.Call != nil {
			body = append(body, e.buildExpr(ctx, pkg, f, s.Call))
		} else if s.Body != nil {
			for _, st := range s.Body.Stmts {
				body = append(body, e.buildStmtAsCode(ctx, pkg, f, st))
			}
		}
		return jsgen.Raw("Promise.resolve().then(async ").Add(jsgen.Arrow().Block(body...)).Add(jsgen.Raw(")"))
	case *ir.DeferStmt:
		var body []jsgen.Code
		if s.Call != nil {
			body = append(body, e.buildExpr(ctx, pkg, f, s.Call))
		} else if s.Body != nil {
			for _, st := range s.Body.Stmts {
				body = append(body, e.buildStmtAsCode(ctx, pkg, f, st))
			}
		}
		return jsgen.Raw("__sovaDeferStack.push(").Add(jsgen.Arrow().Block(body...)).Add(jsgen.Raw(")"))
	case *ir.SelectStmt:
		return e.buildSelectStmt(ctx, pkg, f, s)
	case *ir.ReturnStmt:
		if len(s.Results) == 0 {
			return jsgen.Return()
		} else if len(s.Results) == 1 {
			return jsgen.Return(e.buildExpr(ctx, pkg, f, s.Results[0]))
		} else {
			var exprs []*jsgen.Statement
			for _, result := range s.Results {
				exprs = append(exprs, e.buildExpr(ctx, pkg, f, result))
			}
			return jsgen.Return(jsgen.Array(exprs...))
		}

	case *ir.ExprStmt:
		if e.composableDepth > 0 {
			return jsgen.Id("__c").Dot("children").Dot("push").Call(e.buildExpr(ctx, pkg, f, s.Expr))
		}
		return e.buildExpr(ctx, pkg, f, s.Expr)

	case *ir.VarDeclStmt:
		if len(s.Targets) == 1 {
			if s.Targets[0].Name == nil {
				return e.buildExpr(ctx, pkg, f, s.Init)
			}
			var name string
			if symbol, ok := pkg.Syms.GetByID(s.Targets[0].Name.Sym); ok {
				if symbol.Flags&ir.SF_Unused != 0 {
					name = e.nextDiscardName()
				} else {
					name = symName(ctx, s.Targets[0].Name.Sym)
				}
			} else {
				name = symName(ctx, s.Targets[0].Name.Sym)
			}

			var stmt *jsgen.Statement
			if s.IsConst {
				stmt = jsgen.Const(name)
			} else {
				stmt = jsgen.Let(name)
			}
			return stmt.Op("=").Add(e.buildExpr(ctx, pkg, f, s.Init))
		}

		var names []string
		for _, target := range s.Targets {
			if target.Name == nil {
				names = append(names, e.nextDiscardName())
			} else {
				if symbol, ok := pkg.Syms.GetByID(target.Name.Sym); ok {
					if symbol.Flags&ir.SF_Unused != 0 {
						names = append(names, e.nextDiscardName())
					} else {
						names = append(names, symName(ctx, target.Name.Sym))
					}
				} else {
					names = append(names, symName(ctx, target.Name.Sym))
				}
			}
		}
		kind := "let"
		if s.IsConst {
			kind = "const"
		}
		return jsgen.DestructArray(kind, names, e.buildExpr(ctx, pkg, f, s.Init))

	case *ir.BreakStmt:
		return jsgen.Break()

	case *ir.ContinueStmt:
		return jsgen.Continue()
	case *ir.GuardStmt:
		notCond := jsgen.Unary("!", e.buildExpr(ctx, pkg, f, s.Cond))

		var guardBody []jsgen.Code
		if len(s.Returns) == 0 {
			guardBody = append(guardBody, jsgen.Return())
		} else if len(s.Returns) == 1 {
			guardBody = append(guardBody, jsgen.Return(e.buildExpr(ctx, pkg, f, s.Returns[0])))
		} else {
			var exprs []*jsgen.Statement
			for _, ret := range s.Returns {
				exprs = append(exprs, e.buildExpr(ctx, pkg, f, ret))
			}
			guardBody = append(guardBody, jsgen.Return(jsgen.Array(exprs...)))
		}

		return jsgen.If(notCond).Block(guardBody...).ToStatement()

	case *ir.MultiAssignmentStmt:
		var names []string
		var targetExprs []*jsgen.Statement
		for _, target := range s.Targets {
			if target.Name == nil {
				names = append(names, "")
				targetExprs = append(targetExprs, nil)
				continue
			}
			if memberName, isMethod, ok := e.classMemberLookup(ctx, target.Name.Sym); ok && !isMethod {
				names = append(names, "this."+memberName)
				targetExprs = append(targetExprs, jsgen.Raw("this.").Add(jsgen.Id(memberName)))
				continue
			}
			if reactiveWireVarOriginalNameJS(ctx, target.Name.Sym) != "" {
				cellName := symName(ctx, target.Name.Sym)
				names = append(names, cellName+".value")
				targetExprs = append(targetExprs, jsgen.Id(cellName).Dot("value"))
				continue
			}
			plain := symName(ctx, target.Name.Sym)
			names = append(names, plain)
			targetExprs = append(targetExprs, jsgen.Id(plain))
		}
		if len(targetExprs) == 1 && targetExprs[0] != nil {
			return targetExprs[0].Op("=").Add(e.buildExpr(ctx, pkg, f, s.Value))
		}
		return jsgen.DestructAssign(names, e.buildExpr(ctx, pkg, f, s.Value))

	case *ir.FieldAssignmentStmt:
		var recvName string
		if s.Receiver.Name == "this" && !e.suppressThisKeyword {
			recvName = "this"
		} else {
			recvName = symName(ctx, s.Receiver.Sym)
		}
		if s.Op == ir.OpAssign && len(s.Fields) == 1 && !e.inSyntheticCtor {
			fld := s.Fields[0]
			if isReactiveFieldOfJS(ctx, pkg, s.Receiver.Sym, fld.Name) {
				setterName := "set" + upperFirstJS(fld.Name)
				return jsgen.Id(recvName).Dot(setterName).Call(e.buildExpr(ctx, pkg, f, s.Value))
			}
		}
		target := jsgen.Id(recvName)
		for _, fld := range s.Fields {
			target = target.Dot(fld.Name)
		}
		return target.Op(string(s.Op)).Add(e.buildExpr(ctx, pkg, f, s.Value))

	case *ir.IndexAssignmentStmt:
		recv := e.buildExpr(ctx, pkg, f, s.Receiver)
		idx := e.buildExpr(ctx, pkg, f, s.Index)
		rhs := e.buildExpr(ctx, pkg, f, s.Value)
		return recv.Index(idx).Op(string(s.Op)).Add(rhs)

	case *ir.IfStmt:
		return e.buildIfStmt(ctx, pkg, f, s)
	case *ir.SwitchStmt:
		return e.buildSwitchStmt(ctx, pkg, f, s)
	case *ir.ForStmt:
		return e.buildForStmt(ctx, pkg, f, s)
	case *ir.WhileStmt:
		return e.buildWhileStmt(ctx, pkg, f, s)
	default:
		panic(fmt.Sprintf("javascript codegen: unhandled statement type %T in function body", st))
	}
}

// emitWiredStub emits the frontend fetch-based stub for a wired function. The stub builds the URL from the WireSpec path (substituting :name placeholders), serializes non-path arguments either as a query string (GET/DELETE) or a JSON body (POST/PUT/PATCH), awaits the response, and returns the [value, state] tuple.
//
// `wire(transport: "raw")` handlers are skipped here: they live exclusively on the backend (analyze_wire rejects raw on frontend), they have no JSON contract to generate a stub against, and they are reached either via a browser navigation (OAuth redirects, form posts) or via a hand-written `fetch` from the frontend. Emitting a stub would produce a function nothing can sensibly call.
func (e *CodeEmitter) emitWiredStub(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, s *ir.FuncDeclStmt) {
	if s.Wire != nil && s.Wire.Transport == "raw" {
		return
	}
	funcName := symName(ctx, s.Name.Sym)
	orig := symOrigName(ctx, s.Name.Sym)

	pathArgSet := map[string]bool{}
	for _, a := range s.Wire.PathArgs {
		pathArgSet[a] = true
	}

	type paramBinding struct {
		mangled string
		orig    string
		binding string
		bindKey string
	}
	var bindings []paramBinding
	for _, param := range s.Params {
		key := param.Name.Name
		if param.WireBindAs != "" {
			key = param.WireBindAs
		}
		b := param.WireBinding
		if b == "" && pathArgSet[param.Name.Name] {
			b = "path"
		}
		bindings = append(bindings, paramBinding{
			mangled: symNameWithUnused(ctx, pkg, param.Name.Sym),
			orig:    param.Name.Name,
			binding: b,
			bindKey: key,
		})
	}

	returnDesc := `{kind:"any"}`
	if s.ReturnType != nil && s.ReturnType.Typ != 0 {
		returnDesc = e.buildTypeDescriptorJSLiteral(ctx, s.ReturnType.Typ)
	}

	if s.Wire.Transport == "ws" {
		var sb strings.Builder
		if orig != "" {
			sb.WriteString(fmt.Sprintf("// wire(transport: \"ws\") %s\n", orig))
		}
		sb.WriteString(fmt.Sprintf("async function %s(", funcName))
		paramNames := make([]string, len(bindings))
		for i, b := range bindings {
			paramNames[i] = b.mangled
		}
		sb.WriteString(strings.Join(paramNames, ", "))
		sb.WriteString(") {\n")
		sb.WriteString(fmt.Sprintf("  const __r = await __sovaWSCall(%q, [%s]);\n", orig, strings.Join(paramNames, ", ")))
		sb.WriteString(fmt.Sprintf("  return [__sovaReify(__r.value, %s), __r.state];\n", returnDesc))
		sb.WriteString("}")
		e.jf.Add(jsgen.Raw(sb.String()))
		return
	}

	method := s.Wire.Method
	rawPath := s.Wire.Path
	for _, a := range s.Wire.PathArgs {
		var binding paramBinding
		for _, b := range bindings {
			if b.binding == "path" && b.bindKey == a {
				binding = b
				break
			}
			if b.orig == a {
				binding = b
				break
			}
		}
		placeholder := ":" + a
		repl := "${encodeURIComponent(String(" + binding.mangled + "))}"
		rawPath = strings.ReplaceAll(rawPath, placeholder, repl)
	}

	var sb strings.Builder
	if orig != "" {
		sb.WriteString(fmt.Sprintf("// Wired %s %s -> %s\n", method, s.Wire.Path, orig))
	}
	sb.WriteString(fmt.Sprintf("async function %s(", funcName))
	for i, b := range bindings {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(b.mangled)
	}
	sb.WriteString(") {\n")
	sb.WriteString("  const __base = (typeof process !== 'undefined' && process.env && process.env.WIRE_BACKEND) || window.WIRE_BACKEND || window.location.origin || '';\n")
	sb.WriteString(fmt.Sprintf("  let __url = __base + `%s`;\n", rawPath))

	hasBodyMethod := method != "GET" && method != "DELETE"

	var queryBound []paramBinding
	var headerBound []paramBinding
	var bodyBound []paramBinding
	for _, b := range bindings {
		switch b.binding {
		case "path", "cookie":
			continue
		case "query":
			queryBound = append(queryBound, b)
		case "header":
			headerBound = append(headerBound, b)
		case "body":
			bodyBound = append(bodyBound, b)
		case "":
			if hasBodyMethod {
				bodyBound = append(bodyBound, b)
			} else {
				queryBound = append(queryBound, b)
			}
		}
	}

	if len(queryBound) > 0 {
		sb.WriteString("  const __qs = new URLSearchParams();\n")
		for _, b := range queryBound {
			sb.WriteString(fmt.Sprintf("  __qs.set(%q, String(%s));\n", b.bindKey, b.mangled))
		}
		sb.WriteString("  const __qsStr = __qs.toString();\n")
		sb.WriteString("  if (__qsStr) __url += (__url.includes('?') ? '&' : '?') + __qsStr;\n")
	}

	sb.WriteString("  const __headers = {};\n")
	for _, b := range headerBound {
		sb.WriteString(fmt.Sprintf("  __headers[%q] = String(%s);\n", b.bindKey, b.mangled))
	}

	typedKind := stubTypedResponseKind(ctx, s)
	fetchOpts := "credentials: 'include', headers: __headers"
	if typedKind == "Redirect" {
		fetchOpts += ", redirect: 'manual'"
	}

	if len(bodyBound) > 0 {
		sb.WriteString("  __headers['Content-Type'] = 'application/json';\n")
		sb.WriteString("  const __body = {};\n")
		for _, b := range bodyBound {
			sb.WriteString(fmt.Sprintf("  __body[%q] = %s;\n", b.bindKey, b.mangled))
		}
		sb.WriteString(fmt.Sprintf("  const __res = await fetch(__url, { method: %q, %s, body: JSON.stringify(__body) });\n", method, fetchOpts))
	} else {
		sb.WriteString(fmt.Sprintf("  const __res = await fetch(__url, { method: %q, %s });\n", method, fetchOpts))
	}

	switch typedKind {
	case "Redirect":
		sb.WriteString("  if (__res.type === 'opaqueredirect' || (__res.status >= 300 && __res.status < 400)) {\n")
		sb.WriteString("    const __loc = __res.headers.get('Location') || '';\n")
		sb.WriteString("    return [{location: __loc, status: __res.status || 302}, 0];\n")
		sb.WriteString("  }\n")
		sb.WriteString("  if (!__res.ok) { return [null, __res.status === 401 ? 1 : __res.status === 403 ? 2 : __res.status === 404 ? 3 : 4]; }\n")
		sb.WriteString("  const __loc2 = __res.headers.get('Location') || '';\n")
		sb.WriteString("  return [{location: __loc2, status: __res.status}, 0];\n")
	case "Html":
		sb.WriteString("  if (!__res.ok) { return [null, __res.status === 401 ? 1 : __res.status === 403 ? 2 : __res.status === 404 ? 3 : 4]; }\n")
		sb.WriteString("  const __htmlBody = await __res.text();\n")
		sb.WriteString("  return [{body: __htmlBody, status: __res.status}, 0];\n")
	case "File":
		sb.WriteString("  if (!__res.ok) { return [null, __res.status === 401 ? 1 : __res.status === 403 ? 2 : __res.status === 404 ? 3 : 4]; }\n")
		sb.WriteString("  const __blob = await __res.blob();\n")
		sb.WriteString("  const __disp = __res.headers.get('Content-Disposition') || '';\n")
		sb.WriteString("  const __fnMatch = /filename=\"([^\"]+)\"/.exec(__disp);\n")
		sb.WriteString("  const __fname = __fnMatch ? __fnMatch[1] : '';\n")
		sb.WriteString("  return [{data: __blob, filename: __fname, contentType: __res.headers.get('Content-Type') || '', status: __res.status}, 0];\n")
	case "Status":
		sb.WriteString("  if (!__res.ok && __res.status !== 422 && (__res.status < 200 || __res.status >= 500)) { return [null, __res.status === 401 ? 1 : __res.status === 403 ? 2 : __res.status === 404 ? 3 : 4]; }\n")
		sb.WriteString("  const __data = await __res.json();\n")
		sb.WriteString(fmt.Sprintf("  return [{body: __sovaReify(__data.value, %s), status: __res.status}, __data.state];\n", returnDesc))
	default:
		sb.WriteString("  if (!__res.ok) { return [null, __res.status === 401 ? 1 : __res.status === 403 ? 2 : __res.status === 404 ? 3 : 4]; }\n")
		sb.WriteString("  const __data = await __res.json();\n")
		sb.WriteString(fmt.Sprintf("  return [__sovaReify(__data.value, %s), __data.state];\n", returnDesc))
	}
	sb.WriteString("}")
	e.jf.Add(jsgen.Raw(sb.String()))
}

func stubTypedResponseKind(ctx *codegen.EmitContext, s *ir.FuncDeclStmt) string {
	if s.ReturnType == nil || s.ReturnType.Typ == 0 {
		return ""
	}
	ty, ok := ctx.Types.GetByID(s.ReturnType.Typ)
	if !ok || ty == nil || ty.Kind != ir.TK_Struct {
		return ""
	}
	if ty.PackagePath != "std/http" {
		return ""
	}
	switch ty.StructName {
	case "Redirect", "Html", "File", "Status":
		return ty.StructName
	}
	return ""
}

// emitEmbeddedVar emits the inlined-literal form of an `@embed`-decorated top-level const. The file contents are read from disk at codegen time (using the absolute SourcePath the resolver already validated) and serialised into a JS literal — a plain JSON string for text embeds, or a `Uint8Array` instantiated from a base64-decoded string for binary embeds. P1 always inlines; P3 will rewrite this to `import X from "./__embeds/...?text"` once esbuild is wired and can dedup duplicate embeds across modules.
func (e *CodeEmitter) emitEmbeddedVar(ctx *codegen.EmitContext, pkg *ir.PackageContext, s *ir.VarDeclStmt) {
	if s.Embed == nil || len(s.Targets) == 0 || s.Targets[0].Name == nil {
		return
	}
	target := &s.Targets[0]
	name := symName(ctx, target.Name.Sym)
	data, err := os.ReadFile(s.Embed.SourcePath)
	if err != nil {
		return
	}
	orig := symOrigName(ctx, target.Name.Sym)
	if orig != "" {
		e.jf.Add(jsgen.Comment(fmt.Sprintf("@embed %s (%d bytes)", orig, s.Embed.SizeBytes)))
	}
	switch s.Embed.Kind {
	case ir.EmbedKindText:
		encoded, _ := json.Marshal(string(data))
		e.jf.Add(jsgen.Raw(fmt.Sprintf("const %s = %s;", name, string(encoded))))
	case ir.EmbedKindBytes:
		b64 := base64.StdEncoding.EncodeToString(data)
		e.jf.Add(jsgen.Raw(fmt.Sprintf(
			`const %s = Uint8Array.from(atob(%q), c => c.charCodeAt(0));`,
			name, b64,
		)))
	}
}

// emitWiredVarStub emits a frontend fetch-based stub for a wired top-level var/const declaration. The stub is exposed as an async function returning the [value, state] tuple, matching the backend's GET <route> handler shape. For `@reactive wire let` declarations the emitter instead drops a mutable mirror variable, an async loader that primes it from the same GET endpoint, and an `__sovaOnWireVar` subscription that updates the mirror whenever the backend pushes a new value.
func (e *CodeEmitter) emitWiredVarStub(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, s *ir.VarDeclStmt) {
	if s.Wire == nil || len(s.Targets) == 0 || s.Targets[0].Name == nil {
		return
	}
	target := &s.Targets[0]
	funcName := symName(ctx, target.Name.Sym)
	orig := symOrigName(ctx, target.Name.Sym)

	method := s.Wire.Method
	if method == "" {
		method = "GET"
	}
	rawPath := s.Wire.Path

	isReactive := false
	for _, a := range s.Annotations {
		if a.Name.Name == "reactive" {
			isReactive = true
			break
		}
	}

	varDesc := `{kind:"any"}`
	if target.TypeAnn != nil && target.TypeAnn.Typ != 0 {
		varDesc = e.buildTypeDescriptorJSLiteral(ctx, target.TypeAnn.Typ)
	}

	if isReactive {
		var sb strings.Builder
		if orig != "" {
			sb.WriteString(fmt.Sprintf("// @reactive wire let %s (%s %s)\n", orig, method, rawPath))
		}
		sb.WriteString(fmt.Sprintf("const %s = __sovaMakeReactiveWireCell(%q);\n", funcName, orig))
		sb.WriteString("(async () => {\n")
		sb.WriteString("  const __base = (typeof process !== 'undefined' && process.env && process.env.WIRE_BACKEND) || (typeof window !== 'undefined' && window.WIRE_BACKEND) || (typeof window !== 'undefined' && window.location && window.location.origin) || '';\n")
		sb.WriteString(fmt.Sprintf("  try { const __res = await fetch(__base + `%s`, { method: %q, credentials: 'include' });\n", rawPath, method))
		sb.WriteString("    if (__res.ok) { const __data = await __res.json(); ")
		sb.WriteString(fmt.Sprintf("%s.value = __sovaReify(__data.value, %s); }\n", funcName, varDesc))
		sb.WriteString("  } catch (_) {}\n")
		sb.WriteString("})();\n")
		sb.WriteString(fmt.Sprintf("if (typeof __sovaOnWireVar === 'function') { __sovaOnWireVar(%q, function(v) { %s.value = __sovaReify(v, %s); }); }", orig, funcName, varDesc))
		e.jf.Add(jsgen.Raw(sb.String()))
		return
	}

	var sb strings.Builder
	if orig != "" {
		sb.WriteString(fmt.Sprintf("// Wired var %s %s -> %s\n", method, rawPath, orig))
	}
	sb.WriteString(fmt.Sprintf("async function %s() {\n", funcName))
	sb.WriteString("  const __base = (typeof process !== 'undefined' && process.env && process.env.WIRE_BACKEND) || window.WIRE_BACKEND || window.location.origin || '';\n")
	sb.WriteString(fmt.Sprintf("  const __url = __base + `%s`;\n", rawPath))
	sb.WriteString(fmt.Sprintf("  const __res = await fetch(__url, { method: %q, credentials: 'include' });\n", method))
	sb.WriteString("  if (!__res.ok) { return [null, __res.status === 401 ? 1 : __res.status === 403 ? 2 : __res.status === 404 ? 3 : 4]; }\n")
	sb.WriteString("  const __data = await __res.json();\n")
	sb.WriteString(fmt.Sprintf("  return [__sovaReify(__data.value, %s), __data.state];\n", varDesc))
	sb.WriteString("}")
	e.jf.Add(jsgen.Raw(sb.String()))
}
