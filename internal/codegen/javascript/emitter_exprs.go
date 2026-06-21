package javascript

import (
	"fmt"
	"sova/internal/codegen"
	"sova/internal/codegen/javascript/jsgen"
	"sova/internal/ir"
	"strings"
)

func (e *CodeEmitter) classMemberLookup(ctx *codegen.EmitContext, sym ir.SymID) (string, bool, bool) {
	if e.currentTypeDecl == nil || e.suppressThisKeyword || sym == 0 {
		return "", false, false
	}

	for _, field := range e.currentTypeDecl.Fields {
		if field.Name.Sym == sym {
			return field.Name.Name, false, true
		}
	}

	for _, m := range e.currentTypeDecl.Methods {
		if m.Func != nil && m.Func.Name.Sym == sym {
			return symName(ctx, m.Func.Name.Sym), true, true
		}
	}

	return "", false, false
}

func escapeJSTemplateText(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			b.WriteString("\\\\")
		case '`':
			b.WriteString("\\`")
		case '$':
			if i+1 < len(s) && s[i+1] == '{' {
				b.WriteString("\\${")
				i++
				continue
			}

			b.WriteByte('$')
		default:
			b.WriteByte(s[i])
		}
	}

	return b.String()
}

func (e *CodeEmitter) buildExpr(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, expr ir.Expr) *jsgen.Statement {
	switch x := expr.(type) {
	case *ir.WhenExpr:
		return e.buildWhenExpr(ctx, pkg, f, x)
	case *ir.UnaryExpr:
		return jsgen.Unary(string(x.Op), e.buildExpr(ctx, pkg, f, x.Expr))
	case *ir.PrefixUnaryExpr:
		return jsgen.Unary(string(x.Op), e.buildExpr(ctx, pkg, f, x.Expr))
	case *ir.PostfixUnaryExpr:
		return e.buildExpr(ctx, pkg, f, x.Expr).Op(string(x.Op))
	case *ir.BinaryExpr:
		return e.buildBinaryExpr(ctx, pkg, f, x)
	case *ir.CoalesceExpr:
		leftCond := e.buildExpr(ctx, pkg, f, x.Left)
		leftThen := e.buildExpr(ctx, pkg, f, x.Left)
		defaultExpr := e.buildExpr(ctx, pkg, f, x.Default)
		return leftCond.Op("!==").Add(jsgen.Null()).Op("?").Add(leftThen).Op(":").Add(defaultExpr).Parens()
	case *ir.TenaryExpr:
		cond := e.buildExpr(ctx, pkg, f, x.Cond)
		thenExpr := e.buildExpr(ctx, pkg, f, x.Then)
		elseExpr := e.buildExpr(ctx, pkg, f, x.Else)
		return cond.Op("?").Add(thenExpr).Op(":").Add(elseExpr)
	case *ir.GroupedExpr:
		return e.buildExpr(ctx, pkg, f, x.Expr).Parens()
	case *ir.OptionUnwrapExpr:
		return e.buildExpr(ctx, pkg, f, x.Expr)
	case *ir.AsExpr:
		return e.buildAsExpr(ctx, pkg, f, x)
	case *ir.InstanceofExpr:
		return e.buildInstanceofExpr(ctx, pkg, f, x)
	case *ir.AssignmentExpr:
		return e.buildAssignmentExpr(ctx, pkg, f, x)
	case *ir.IndexExpr:
		return e.buildExpr(ctx, pkg, f, x.Expr).Index(e.buildExpr(ctx, pkg, f, x.Index))
	case *ir.SliceRangeExpr:
		return e.buildSliceRangeExpr(ctx, pkg, f, x)
	case *ir.FieldAccessExpr:
		return e.buildFieldAccessExpr(ctx, pkg, f, x)
	case *ir.VarRef:
		return e.buildVarRef(ctx, pkg, x)
	case *ir.RangeExpr:
		return e.buildRangeExpr(ctx, pkg, f, x.Start, x.End, x.Inc)
	case *ir.FuncCallExpr:
		return e.buildFuncCallExpr(ctx, pkg, f, x)
	case *ir.FuncLitExpr:
		return e.buildFuncLitExpr(ctx, pkg, f, x)
	case *ir.LitInt:
		return jsgen.Lit(x.Value)
	case *ir.LitFloat:
		return jsgen.Lit(x.Value)
	case *ir.LitString:
		return jsgen.Lit(x.Value)
	case *ir.LitChar:
		return jsgen.Lit(string(x.Value))
	case *ir.LitBool:
		return jsgen.Lit(x.Value)
	case *ir.LitNone:
		return jsgen.Null()
	case *ir.ArrayLiteral:
		return e.buildArrayLiteral(ctx, pkg, f, x)
	case *ir.MapLiteral:
		return e.buildMapLiteral(ctx, pkg, f, x)
	case *ir.TupleLiteral:
		return e.buildArrayLiteral(ctx, pkg, f, &ir.ArrayLiteral{Elems: x.Elems})
	case *ir.StringTemplateExpr:
		return e.buildStringTemplateExpr(ctx, pkg, f, x)
	case *ir.SessionExpr:
		return jsgen.Comment("[error] @-session is backend-only")
	case *ir.ComposableCallExpr:
		return e.buildComposableCallExpr(ctx, pkg, f, x)
	case *ir.ChanInitExpr:
		return e.buildChanInitExpr(ctx, pkg, f, x)
	case *ir.NewExpr:
		return e.buildNewExpr(ctx, pkg, f, x)
	}

	return jsgen.Comment("Unknown expression type")
}

func (e *CodeEmitter) buildBinaryExpr(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, x *ir.BinaryExpr) *jsgen.Statement {
	if leftTy, ok := ctx.Types.GetByID(x.Left.GetType()); ok && leftTy.Kind == ir.TK_Struct {
		if methodName, isOp := jsOpOverloadName(x.Op); isOp {
			for _, m := range leftTy.Struct.Methods {
				if m.Name == methodName && m.Sym != 0 {
					left := e.buildExpr(ctx, pkg, f, x.Left)
					right := e.buildExpr(ctx, pkg, f, x.Right)
					return left.Dot(symName(ctx, m.Sym)).Call(right)
				}
			}
		}
	}

	if x.Op == ir.OpAdd {
		if leftTy, ok := ctx.Types.GetByID(x.Left.GetType()); ok && leftTy.Kind == ir.TK_Slice {
			if rightTy, rok := ctx.Types.GetByID(x.Right.GetType()); rok && rightTy.Kind == ir.TK_Slice {
				left := e.buildExpr(ctx, pkg, f, x.Left)
				right := e.buildExpr(ctx, pkg, f, x.Right)
				return left.Dot("concat").Call(right)
			}
		}
	}

	left := e.buildExpr(ctx, pkg, f, x.Left)
	right := e.buildExpr(ctx, pkg, f, x.Right)
	return left.Op(string(x.Op)).Add(right)
}

func (e *CodeEmitter) buildAssignmentExpr(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, x *ir.AssignmentExpr) *jsgen.Statement {
	var left *jsgen.Statement
	if name, isMethod, ok := e.classMemberLookup(ctx, x.Left.Sym); ok && !isMethod {
		left = jsgen.Raw("this.").Add(jsgen.Id(name))
	} else if reactiveWireVarOriginalNameJS(ctx, x.Left.Sym) != "" {
		left = jsgen.Id(symName(ctx, x.Left.Sym)).Dot("value")
	} else {
		left = jsgen.Id(symNameWithUnused(ctx, pkg, x.Left.Sym))
	}

	right := e.buildExpr(ctx, pkg, f, x.Right)
	return left.Op(string(x.Op)).Add(right)
}

func (e *CodeEmitter) buildSliceRangeExpr(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, x *ir.SliceRangeExpr) *jsgen.Statement {
	base := e.buildExpr(ctx, pkg, f, x.Expr)
	args := []*jsgen.Statement{}

	if x.Low != nil {
		args = append(args, e.buildExpr(ctx, pkg, f, x.Low))
	} else {
		args = append(args, jsgen.Id("0"))
	}

	if x.High != nil {
		args = append(args, e.buildExpr(ctx, pkg, f, x.High))
	}

	return base.Dot("slice").Call(args...)
}

func (e *CodeEmitter) buildFieldAccessExpr(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, x *ir.FieldAccessExpr) *jsgen.Statement {
		var base *jsgen.Statement
		var fields []ir.FieldName
		var curType ir.TypID
		if x.ResolvedSym != 0 {
			base = jsgen.Id(symName(ctx, x.ResolvedSym))
			if len(x.Fields) <= 1 {
				return base
			}

			fields = x.Fields[1:]
			for _, group := range [][]*ir.PackageContext{ctx.Pkgs, ctx.TransPkgs} {
				for _, p := range group {
					if p == nil {
						continue
					}

					if sym, ok := p.Syms.GetByID(x.ResolvedSym); ok {
						curType = sym.Typ
						break
					}
				}

				if curType != 0 {
					break
				}
			}
		} else {
			base = e.buildExpr(ctx, pkg, f, x.Expr)
			fields = x.Fields
			curType = x.Expr.GetType()
		}

		isThisReceiver := false
		if vr, ok := x.Expr.(*ir.VarRef); ok {
			if orig, ok := ctx.Names.GetOriginalName(vr.Ref.Sym); ok && orig == "this" {
				isThisReceiver = true
			}
		}

		lastWasMethod := false
		for _, field := range fields {
			ty, ok := ctx.Types.GetByID(curType)
			lastWasMethod = false
			if ok && ty.Kind == ir.TK_Enum {

				isCaseAccess := false
				for _, c := range ty.Enum.Cases {
					if c.Name == field.Name {
						isCaseAccess = true
						enumName := symName(ctx, getEnumSymbol(ctx, pkg, ty.Enum.Name))
						if ty.Enum.IsNumeric {

							base = jsgen.Id(enumName).Dot(field.Name)
						} else {

							base = jsgen.Id(enumName + field.Name)
						}

						break
					}
				}

				if !isCaseAccess {

					foundMethod := false
					for _, m := range ty.Enum.Methods {
						if m.Name == field.Name {
							foundMethod = true

							methodSym := getMethodSymbol(ctx, pkg, ty.Enum.Name, field.Name)
							if methodSym != 0 {
								base = base.Dot(symName(ctx, methodSym))
							} else {
								base = base.Dot(field.Name)
							}

							curType = m.Type
							lastWasMethod = true
							break
						}
					}

					if !foundMethod {
						base = base.Dot(field.Name)

						for _, fld := range ty.Enum.Fields {
							if fld.Name == field.Name {
								curType = fld.Type
								break
							}
						}
					}
				}
			} else if ok && ty.Kind == ir.TK_Struct {
				found := false
				for _, sf := range ty.Struct.Fields {
					if sf.Name == field.Name {
						base = base.Dot(field.Name)
						curType = sf.Type
						found = true
						break
					}
				}

				if !found {
					methodSym := ir.SymID(0)
					methodFuncTyp := ir.TypID(0)
					if x.MethodSym != 0 {
						methodSym = x.MethodSym
						for _, m := range ty.Struct.Methods {
							if m.Sym == x.MethodSym {
								methodFuncTyp = m.FuncTyp
								break
							}
						}
					}

					if methodSym == 0 {
						for _, m := range ty.Struct.Methods {
							if m.Name == field.Name {
								methodSym = m.Sym
								methodFuncTyp = m.FuncTyp
								break
							}
						}
					}

					if methodSym != 0 {
						base = base.Dot(symName(ctx, methodSym))
						curType = methodFuncTyp
						found = true
						lastWasMethod = true
					}
				}

				if !found {
					base = base.Dot(field.Name)
				}
			} else {
				base = base.Dot(field.Name)
			}
		}

		if isThisReceiver && lastWasMethod && len(x.Fields) == 1 {
			if e.suppressThisKeyword {
				base = base.Dot("bind").Call(e.buildExpr(ctx, pkg, f, x.Expr))
			} else {
				base = base.Dot("bind").Call(jsgen.Id("this"))
			}
		}

	return base
}

func (e *CodeEmitter) buildVarRef(ctx *codegen.EmitContext, pkg *ir.PackageContext, x *ir.VarRef) *jsgen.Statement {
	if orig, ok := ctx.Names.GetOriginalName(x.Ref.Sym); ok && orig == "this" && !e.suppressThisKeyword {
		return jsgen.Id("this")
	}

	if name, isMethod, ok := e.classMemberLookup(ctx, x.Ref.Sym); ok {
		ref := jsgen.Raw("this.").Add(jsgen.Id(name))
		if isMethod {
			return ref.Dot("bind").Call(jsgen.Id("this"))
		}

		return ref
	}

	if reactiveWireVarOriginalNameJS(ctx, x.Ref.Sym) != "" {
		return jsgen.Id(symName(ctx, x.Ref.Sym)).Dot("value")
	}

	return jsgen.Id(symNameWithUnused(ctx, pkg, x.Ref.Sym))
}

func (e *CodeEmitter) buildFuncCallExpr(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, x *ir.FuncCallExpr) *jsgen.Statement {
	if chOp, chRecv, ok := matchChanMethodJS(ctx, x); ok {
		recv := e.buildExpr(ctx, pkg, f, chRecv)
		switch chOp {
		case "send":
			var arg *jsgen.Statement
			if len(x.Args) == 1 {
				arg = e.buildExpr(ctx, pkg, f, x.Args[0].Expr)
			} else {
				arg = jsgen.Raw("undefined")
			}

			e.usesChanRuntime = true
			return jsgen.Raw("(await ").Add(recv).Add(jsgen.Raw(".send(")).Add(arg).Add(jsgen.Raw("))"))
		case "recv":
			e.usesChanRuntime = true
			return jsgen.Raw("(await ").Add(recv).Add(jsgen.Raw(".recv())"))
		case "close":
			e.usesChanRuntime = true
			return recv.Dot("close").Call()
		}
	}

	if intrinsic := lookupBuiltinIntrinsicJS(ctx, x.Callee); intrinsic != "" {
		argCodes := make([]*jsgen.Statement, len(x.Args))
		for i, arg := range x.Args {
			argCodes[i] = e.buildExpr(ctx, pkg, f, arg.Expr)
		}

		if code := emitBuiltinIntrinsicCallJS(intrinsic, argCodes); code != nil {
			return code
		}
	}

	callee := e.buildExpr(ctx, pkg, f, x.Callee)

	var args []*jsgen.Statement
	for _, arg := range x.Args {
		args = append(args, e.buildExpr(ctx, pkg, f, arg.Expr))
	}

	call := callee.Call(args...)
	if x.IsAsync {
		return jsgen.Raw("await ").Add(call)
	}

	return call
}

func (e *CodeEmitter) buildFuncLitExpr(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, x *ir.FuncLitExpr) *jsgen.Statement {
	params := make([]string, len(x.Params))
	for i, param := range x.Params {
		params[i] = symNameWithUnused(ctx, pkg, param.Name.Sym)
	}

	var body []jsgen.Code
	for _, stmt := range x.Body.Stmts {
		body = append(body, e.buildStmtAsCode(ctx, pkg, f, stmt))
	}

	ab := jsgen.Arrow(params...)
	if x.IsAsync {
		ab = ab.Async()
	}

	return ab.Block(body...)
}

func (e *CodeEmitter) buildArrayLiteral(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, x *ir.ArrayLiteral) *jsgen.Statement {
	var elements []*jsgen.Statement
	for _, elem := range x.Elems {
		elements = append(elements, e.buildExpr(ctx, pkg, f, elem))
	}

	return jsgen.Array(elements...)
}

func (e *CodeEmitter) buildMapLiteral(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, x *ir.MapLiteral) *jsgen.Statement {
	var pairs []jsgen.KeyValue
	for _, entry := range x.Entries {
		keyStr := ""
		switch k := entry.Key.(type) {
		case *ir.LitString:
			keyStr = k.Value
		case *ir.LitInt:
			keyStr = string(rune(k.Value))
		case *ir.VarRef:
			keyStr = symNameWithUnused(ctx, pkg, k.Ref.Sym)
		default:
			keyStr = "key"
		}

		pairs = append(pairs, jsgen.Kv(keyStr, e.buildExpr(ctx, pkg, f, entry.Value)))
	}

	return jsgen.Object(pairs...)
}

func (e *CodeEmitter) buildStringTemplateExpr(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, x *ir.StringTemplateExpr) *jsgen.Statement {
	var sb strings.Builder
	sb.WriteByte('`')
	for _, part := range x.Parts {
		if part.Expr != nil {
			sb.WriteString("${")
			sb.WriteString(e.buildExpr(ctx, pkg, f, part.Expr).Render())
			sb.WriteByte('}')
		} else {
			sb.WriteString(escapeJSTemplateText(part.Lit))
		}
	}
	sb.WriteByte('`')
	return jsgen.Raw(sb.String())
}

func (e *CodeEmitter) buildComposableCallExpr(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, x *ir.ComposableCallExpr) *jsgen.Statement {
	var ctorCall *jsgen.Statement
	calleeSym := composableCalleeSymJS(x.Callee)
	if x.CtorSym != 0 {
		ctorName := symName(ctx, x.CtorSym)
		ctorSym, _ := pkg.Syms.GetByID(x.CtorSym)
		var ctorFunc *ir.Type
		if ctorSym != nil {
			ctorFunc, _ = ctx.Types.GetByID(ctorSym.Typ)
		}

		args := make([]*jsgen.Statement, len(x.Args))
		for i, arg := range x.Args {
			if arg.Expr != nil {
				args[i] = e.buildExpr(ctx, pkg, f, arg.Expr)
			} else if ctorFunc != nil && i < len(ctorFunc.ParamTypes) && ctorFunc.ParamTypes[i].Default != nil {
				args[i] = e.buildExpr(ctx, pkg, f, ctorFunc.ParamTypes[i].Default)
			} else {
				args[i] = jsgen.Null()
			}
		}

		ctorCall = jsgen.Id(ctorName).Call(args...)
	} else if calleeSym != 0 {
		typeName := symName(ctx, calleeSym)
		ctorCall = jsgen.Raw(fmt.Sprintf("new %s()", typeName))
	} else {
		ctorCall = jsgen.Null()
	}

	var sb strings.Builder
	sb.WriteString("((() => { const __c = ")
	sb.WriteString(ctorCall.String())
	sb.WriteString("; ")
	for _, child := range x.Children {
		if child.Expr == nil {
			continue
		}

		childCode := e.buildExpr(ctx, pkg, f, child.Expr)
		sb.WriteString("__c.children.push(")
		sb.WriteString(childCode.String())
		sb.WriteString("); ")
	}

	e.composableDepth++
	for _, child := range x.Children {
		if child.Stmt == nil {
			continue
		}

		stmtCode := e.buildStmtAsCode(ctx, pkg, f, child.Stmt)
		if stmtCode == nil {
			continue
		}

		if stmtStmt, ok := stmtCode.(*jsgen.Statement); ok {
			sb.WriteString(stmtStmt.String())
			sb.WriteString("; ")
		}
	}

	e.composableDepth--
	sb.WriteString("return __c; })())")
	return jsgen.Raw(sb.String())
}

func (e *CodeEmitter) buildChanInitExpr(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, x *ir.ChanInitExpr) *jsgen.Statement {
	e.usesChanRuntime = true
	var capCode *jsgen.Statement
	if x.Capacity != nil {
		capCode = e.buildExpr(ctx, pkg, f, x.Capacity)
	} else {
		capCode = jsgen.Raw("0")
	}

	return jsgen.Raw("new __SovaChan(").Add(capCode).Add(jsgen.Raw(")"))
}

func (e *CodeEmitter) buildNewExpr(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, x *ir.NewExpr) *jsgen.Statement {
	typeName := symName(ctx, x.TypeName.Sym)
	if x.CtorSym != 0 {
		ctorName := symName(ctx, x.CtorSym)
		ctorPkg := pkg
		if x.Qualifier != "" {
			if found := lookupImportedPackage(ctx, pkg, x.Qualifier); found != nil {
				ctorPkg = found
			}
		}

		ctorSym, _ := ctorPkg.Syms.GetByID(x.CtorSym)
		var ctorFunc *ir.Type
		if ctorSym != nil {
			ctorFunc, _ = ctx.Types.GetByID(ctorSym.Typ)
		}

		args := make([]*jsgen.Statement, len(x.Args))
		for i, arg := range x.Args {
			if arg.Expr != nil {
				args[i] = e.buildExpr(ctx, pkg, f, arg.Expr)
			} else if ctorFunc != nil && i < len(ctorFunc.ParamTypes) && ctorFunc.ParamTypes[i].Default != nil {
				args[i] = e.buildExpr(ctx, pkg, f, ctorFunc.ParamTypes[i].Default)
			} else {
				args[i] = jsgen.Null()
			}
		}

		return jsgen.Id(ctorName).Call(args...)
	}

	return jsgen.Raw(fmt.Sprintf("new %s()", typeName))
}

func (e *CodeEmitter) buildWhenExpr(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, x *ir.WhenExpr) *jsgen.Statement {
	if len(x.Cases) == 0 {
		return e.buildExpr(ctx, pkg, f, x.Default)
	}

	expr := e.buildExpr(ctx, pkg, f, x.Expr)

	result := e.buildExpr(ctx, pkg, f, x.Default)

	for i := len(x.Cases) - 1; i >= 0; i-- {
		c := x.Cases[i]
		thenExpr := e.buildExpr(ctx, pkg, f, c.Then)

		var cond *jsgen.Statement
		for j, val := range c.Values {
			valExpr := e.buildExpr(ctx, pkg, f, val)
			comparison := expr.Op("===").Add(valExpr)

			if j == 0 {
				cond = comparison
			} else {
				cond = cond.Op("||").Add(comparison)
			}
		}

		result = cond.Op("?").Add(thenExpr).Op(":").Add(result).Parens()
	}

	return result
}

func (e *CodeEmitter) buildAsExpr(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, x *ir.AsExpr) *jsgen.Statement {
	src := e.buildExpr(ctx, pkg, f, x.Expr)
	if x.Target == nil || x.Target.Typ == 0 {
		return src
	}

	if wrapName, jsCtor, ok := jsHandleWrapperTarget(ctx, x.Target.Typ); ok {
		if x.Safe {
			body := fmt.Sprintf("(__v => (__v != null && __v.handle != null && typeof globalThis[%q] === 'function' && __v.handle instanceof globalThis[%q]) ? new %s(__v.handle) : null)", jsCtor, jsCtor, wrapName)
			return jsgen.Raw(body).Call(src)
		}

		body := fmt.Sprintf("(__v => (__v == null) ? new %s(null) : new %s(__v.handle))", wrapName, wrapName)
		return jsgen.Raw(body).Call(src)
	}

	if x.Safe {
		if conv := jsSafePrimitiveConversion(ctx, x.Expr.GetType(), x.Target.Typ, "__v"); conv != "" {
			body := fmt.Sprintf("(__v => %s)", conv)
			return jsgen.Raw(body).Call(src)
		}

		check := jsAsExprPredicate(ctx, x.Target.Typ, "__v")
		if check == "" {
			return src
		}

		body := fmt.Sprintf("(__v => (%s) ? __v : null)", check)
		return jsgen.Raw(body).Call(src)
	}

	if conv := jsPrimitiveConversion(ctx, x.Expr.GetType(), x.Target.Typ, "__v"); conv != "" {
		body := fmt.Sprintf("(__v => %s)", conv)
		return jsgen.Raw(body).Call(src)
	}

	return src
}

func (e *CodeEmitter) buildInstanceofExpr(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, x *ir.InstanceofExpr) *jsgen.Statement {
	src := e.buildExpr(ctx, pkg, f, x.Expr)
	if x.Target == nil || x.Target.Typ == 0 {
		return jsgen.Raw("false")
	}

	if _, jsCtor, ok := jsHandleWrapperTarget(ctx, x.Target.Typ); ok {
		body := fmt.Sprintf("(__v => __v != null && __v.handle != null && typeof globalThis[%q] === 'function' && __v.handle instanceof globalThis[%q])", jsCtor, jsCtor)
		return jsgen.Raw(body).Call(src)
	}

	if check := jsAsExprPredicate(ctx, x.Target.Typ, "__v"); check != "" {
		body := fmt.Sprintf("(__v => __v != null && (%s))", check)
		return jsgen.Raw(body).Call(src)
	}

	return jsgen.Raw("false")
}

func jsHandleWrapperTarget(ctx *codegen.EmitContext, typID ir.TypID) (mangled, jsCtor string, ok bool) {
	ty, found := ctx.Types.GetByID(typID)
	if !found || ty.Kind != ir.TK_Struct {
		return "", "", false
	}

	hasHandle := false
	for _, sf := range ty.Struct.Fields {
		if sf.Name == "handle" && sf.Type == ctx.Types.PrimAny() {
			hasHandle = true
			break
		}
	}

	if !hasHandle {
		return "", "", false
	}

	for _, group := range [][]*ir.PackageContext{ctx.Pkgs, ctx.TransPkgs} {
		for _, p := range group {
			if p == nil {
				continue
			}

			for sym, s := range p.Syms.ByID() {
				if s == nil || s.Typ != typID || s.Name != ty.Struct.Name {
					continue
				}

				if name, found := ctx.Names.GetMangledName(sym); found {
					return name, ty.Struct.Name, true
				}
			}
		}
	}

	return "", "", false
}

func jsSafePrimitiveConversion(ctx *codegen.EmitContext, srcTyp, dstTyp ir.TypID, varName string) string {
	str := ctx.Types.PrimString()
	in := ctx.Types.PrimInt()
	fl := ctx.Types.PrimFloat()
	bl := ctx.Types.PrimBool()
	ch := ctx.Types.PrimChar()
	if srcTyp == 0 || dstTyp == 0 || srcTyp == dstTyp {
		return ""
	}

	if dstTyp == str {
		if srcTyp == in || srcTyp == fl || srcTyp == bl || srcTyp == ch {
			return fmt.Sprintf("String(%s)", varName)
		}

		return ""
	}

	if srcTyp == str {
		switch dstTyp {
		case in:
			return fmt.Sprintf("((() => { const __n = parseInt(%s, 10); return Number.isNaN(__n) ? null : (__n | 0); })())", varName)
		case fl:
			return fmt.Sprintf("((() => { const __n = Number(%s); return Number.isNaN(__n) ? null : __n; })())", varName)
		case bl:
			return fmt.Sprintf("((() => { if (%s === 'true') return true; if (%s === 'false') return false; return null; })())", varName, varName)
		}

		return ""
	}

	if srcTyp == in && dstTyp == fl {
		return fmt.Sprintf("Number(%s)", varName)
	}

	if srcTyp == fl && dstTyp == in {
		return fmt.Sprintf("(%s | 0)", varName)
	}

	if srcTyp == in && dstTyp == ch {
		return fmt.Sprintf("String.fromCharCode(%s)", varName)
	}

	if srcTyp == ch && dstTyp == in {
		return fmt.Sprintf("(%s).charCodeAt(0)", varName)
	}

	return ""
}

func jsPrimitiveConversion(ctx *codegen.EmitContext, srcTyp, dstTyp ir.TypID, varName string) string {
	str := ctx.Types.PrimString()
	in := ctx.Types.PrimInt()
	fl := ctx.Types.PrimFloat()
	bl := ctx.Types.PrimBool()
	ch := ctx.Types.PrimChar()
	if srcTyp == 0 || dstTyp == 0 || srcTyp == dstTyp {
		return ""
	}

	if dstTyp == str {
		if srcTyp == in || srcTyp == fl || srcTyp == bl {
			return fmt.Sprintf("String(%s)", varName)
		}

		if srcTyp == ch {
			return fmt.Sprintf("String(%s)", varName)
		}

		return ""
	}

	if srcTyp == str {
		if dstTyp == in {
			return fmt.Sprintf("(parseInt(%s, 10) | 0)", varName)
		}

		if dstTyp == fl {
			return fmt.Sprintf("Number(%s)", varName)
		}

		if dstTyp == bl {
			return fmt.Sprintf("(%s === 'true')", varName)
		}

		return ""
	}

	if srcTyp == in && dstTyp == fl {
		return fmt.Sprintf("Number(%s)", varName)
	}

	if srcTyp == fl && dstTyp == in {
		return fmt.Sprintf("(%s | 0)", varName)
	}

	if srcTyp == in && dstTyp == ch {
		return fmt.Sprintf("String.fromCharCode(%s)", varName)
	}

	if srcTyp == ch && dstTyp == in {
		return fmt.Sprintf("(%s).charCodeAt(0)", varName)
	}

	return ""
}

func jsAsExprPredicate(ctx *codegen.EmitContext, typID ir.TypID, varName string) string {
	ty, ok := ctx.Types.GetByID(typID)
	if !ok {
		return ""
	}

	switch ty.Kind {
	case ir.TK_PrimitiveAny:
		return ""
	case ir.TK_PrimitiveString:
		return fmt.Sprintf("typeof %s === 'string'", varName)
	case ir.TK_PrimitiveInt, ir.TK_PrimitiveFloat:
		return fmt.Sprintf("typeof %s === 'number'", varName)
	case ir.TK_PrimitiveBool:
		return fmt.Sprintf("typeof %s === 'boolean'", varName)
	case ir.TK_PrimitiveChar:
		return fmt.Sprintf("typeof %s === 'string' && %s.length === 1", varName, varName)
	case ir.TK_Array, ir.TK_Slice:
		return fmt.Sprintf("Array.isArray(%s)", varName)
	case ir.TK_Map:
		return fmt.Sprintf("%s !== null && typeof %s === 'object' && !Array.isArray(%s)", varName, varName, varName)
	case ir.TK_Struct, ir.TK_Enum:
		return fmt.Sprintf("%s !== null && typeof %s === 'object'", varName, varName)
	}

	return ""
}

func jsTypeLabel(ctx *codegen.EmitContext, typID ir.TypID) string {
	ty, ok := ctx.Types.GetByID(typID)
	if !ok {
		return "<unknown>"
	}

	switch ty.Kind {
	case ir.TK_PrimitiveString:
		return "string"
	case ir.TK_PrimitiveInt:
		return "int"
	case ir.TK_PrimitiveFloat:
		return "float"
	case ir.TK_PrimitiveBool:
		return "bool"
	case ir.TK_PrimitiveChar:
		return "char"
	case ir.TK_Array, ir.TK_Slice:
		return "array"
	case ir.TK_Map:
		return "map"
	case ir.TK_Struct:
		return ty.Struct.Name
	case ir.TK_Enum:
		return ty.Enum.Name
	}

	return string(ty.Key)
}

func (e *CodeEmitter) buildRangeExpr(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, start, end, inc ir.Expr) *jsgen.Statement {
	startExpr := e.buildExpr(ctx, pkg, f, start)
	endExpr := e.buildExpr(ctx, pkg, f, end)

	incExpr := jsgen.Lit(1)
	if inc != nil {
		incExpr = e.buildExpr(ctx, pkg, f, inc)
	}

	rangeFunc := jsgen.Arrow().Block(
		jsgen.Const("result").Op("=").Add(jsgen.Array()),
		jsgen.For(
			jsgen.Let("i").Op("=").Add(startExpr),
			jsgen.Id("i").Op("<").Add(endExpr),
			jsgen.Id("i").Op("+=").Add(incExpr),
		).Block(
			jsgen.Id("result").Dot("push").Call(jsgen.Id("i")),
		),
		jsgen.Return(jsgen.Id("result")),
	)

	return rangeFunc.Parens().Call()
}

func jsOpOverloadName(op ir.Op) (string, bool) {
	switch op {
	case ir.OpAdd, ir.OpSub, ir.OpMul, ir.OpDiv, ir.OpMod, ir.OpEq:
		return "op" + string(op), true
	}

	return "", false
}
