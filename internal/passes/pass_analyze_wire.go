package passes

import (
	"sova/internal/diag"
	"sova/internal/ir"
	"strings"
	"unicode"
)

type PassAnalyzeWire struct{}

func (p *PassAnalyzeWire) Name() string       { return "analyze_wire" }

func (p *PassAnalyzeWire) Scope() PassScope   { return PerBuild }

func (p *PassAnalyzeWire) Requires() []string { return []string{"resolve_typerefs"} }

func (p *PassAnalyzeWire) NoErrors() bool     { return false }

const WireStateCacheKey = "wire_state_typ"

const SessionTypeCacheKey = "session_typ"

const NeedsSessionManagerCacheKey = "needs_session_manager"

const FrontendWireFuncsCacheKey = "frontend_wire_funcs"

const SessionsSessionTypeCacheKey = "sessions_session_typ"

const ReactiveWireVarsCacheKey = "reactive_wire_vars"

func (p *PassAnalyzeWire) Run(pc *PassContext) error {
	type endpoint struct {
		method string
		path   string
	}

	seen := map[endpoint]string{}

	rulesets := map[string]map[string]ir.WireOptValue{}

	for _, pkg := range pc.Pkgs {
		for _, f := range pkg.Files {
			for _, st := range f.Hir.Statements {
				if rs, ok := st.(*ir.WireRulesetStmt); ok {
					rulesets[rs.Name] = rs.Options
				}
			}
		}
	}

	resolveRuleset := func(spec *ir.WireSpec, span diag.TextSpan) {
		if spec == nil || spec.Ruleset == "" {
			return
		}

		opts, ok := rulesets[spec.Ruleset]
		if !ok {
			pc.Diag.Report(diag.ErrUnknownWireRuleset, span, spec.Ruleset)
			return
		}

		if spec.Options == nil {
			spec.Options = map[string]ir.WireOptValue{}
		}

		for k, v := range opts {
			if _, has := spec.Options[k]; !has {
				spec.Options[k] = v
			}
		}
	}

	var wireStateTyp ir.TypID
	hasWired := false
	needsManager := false
	var frontendWires []*ir.FuncDeclStmt
	var reactiveWireVars []*ir.VarDeclStmt
	sessionsFreeFuncNames := map[string]bool{
		"all": true, "byId": true, "firstByUser": true, "allByUser": true,
		"current": true, "broadcast": true,
		"onConnect": true, "onDisconnect": true,
		"onRoomJoin": true, "onRoomLeave": true,
	}

	for _, pkg := range pc.Pkgs {
		for _, f := range pkg.Files {
			fileSide := f.Hir.Side.Kind
			if fileSide == ir.SideFrontend {
				p.checkNoSessionOnFrontend(pc, f.Hir.Statements)
			}

			for _, st := range f.Hir.Statements {
				if fn, ok := st.(*ir.FuncDeclStmt); ok && fn.IsWired {
					resolveRuleset(fn.Wire, fn.Span())
					if !hasWired {
						wireStateTyp = p.ensureWireState(pc)
						p.ensureSessionType(pc)
						hasWired = true
					}

					effSide := fileSide
					if fn.Side != nil {
						effSide = fn.Side.Kind
					}

					p.validateWired(pc, pkg, f, fn)
					p.deriveRoute(pc, pkg, fn)
					if effSide == ir.SideFrontend {
						needsManager = true
						frontendWires = append(frontendWires, fn)
					}

					p.resolveWireTransport(pc, fn, effSide)
					if fn.Wire != nil && (fn.Wire.Transport == "ws" || fn.Wire.Transport == "sse") {
						needsManager = true
					}

					if fn.Wire != nil && fn.Wire.Transport == "raw" {
						p.validateRawWireSignature(pc, fn)
						if p.rawWireUsesSession(fn) {
							fn.Wire.UsesSession = true
							needsManager = true
						}
					}

					if fn.Wire != nil && fn.Wire.Method != "" && fn.Wire.Path != "" && effSide != ir.SideFrontend {
						key := endpoint{method: fn.Wire.Method, path: fn.Wire.Path}

						if owner, taken := seen[key]; taken {
							pc.Diag.Report(diag.ErrWireRouteConflict, fn.Name.Span, fn.Wire.Method, fn.Wire.Path, owner)
						} else {
							seen[key] = fn.Name.Name
						}
					}

					fn.IsAsync = true
					_ = wireStateTyp
					continue
				}

				if vd, ok := st.(*ir.VarDeclStmt); ok && vd.IsWired {
					if !hasWired {
						wireStateTyp = p.ensureWireState(pc)
						p.ensureSessionType(pc)
						hasWired = true
					}

					if !vd.IsConst {
						needsManager = true
					}

					isReactive := hasAnnotation(vd.Annotations, "reactive")
					if isReactive {
						if vd.IsConst {
							pc.Diag.Report(diag.ErrReactiveOnConst, vd.Span(), targetName(vd))
						}

						needsManager = true
						reactiveWireVars = append(reactiveWireVars, vd)
					}

					resolveRuleset(vd.Wire, vd.Span())
					p.deriveVarRoute(pkg, vd)
					if vd.Wire != nil && vd.Wire.Method != "" && vd.Wire.Path != "" {
						key := endpoint{method: vd.Wire.Method, path: vd.Wire.Path}

						if owner, taken := seen[key]; taken && len(vd.Targets) > 0 && vd.Targets[0].Name != nil {
							pc.Diag.Report(diag.ErrWireRouteConflict, vd.Targets[0].Name.Span, vd.Wire.Method, vd.Wire.Path, owner)
						} else if len(vd.Targets) > 0 && vd.Targets[0].Name != nil {
							seen[key] = vd.Targets[0].Name.Name
						}
					}

					if len(vd.Targets) > 0 && vd.Targets[0].Name != nil && vd.Targets[0].TypeAnn != nil && vd.Targets[0].TypeAnn.Typ != 0 {
						if isReactive {
							pkg.Syms.SetType(vd.Targets[0].Name.Sym, vd.Targets[0].TypeAnn.Typ)
						} else {
							tupleTyp := pc.Types.TupleOf(
								ir.TupleField{Name: "value", Type: vd.Targets[0].TypeAnn.Typ},
								ir.TupleField{Name: "state", Type: wireStateTyp},
							)
							funcTyp := pc.Types.AsyncFuncOf(nil, tupleTyp)
							pkg.Syms.SetType(vd.Targets[0].Name.Sym, funcTyp)
						}
					} else if len(vd.Targets) > 0 && vd.Targets[0].Name != nil {
						pc.Diag.Report(diag.ErrWireVarNeedsType, vd.Targets[0].Name.Span, vd.Targets[0].Name.Name)
					}
				}
			}
		}
	}

	if !needsManager {
		for _, pkg := range pc.Pkgs {
			sessionsAliases := map[ir.SymID]bool{}

			for _, f := range pkg.Files {
				for _, st := range f.Hir.Statements {
					if imp, ok := st.(*ir.ImportStmt); ok && imp.Path.String() == "sessions" {
						if sym, ok := pkg.Scopes.LookupOnlyCurrent(pkg.Root, imp.Alias); ok {
							sessionsAliases[sym] = true
						}
					}
				}
			}

			if len(sessionsAliases) == 0 {
				continue
			}

			for _, f := range pkg.Files {
				for _, st := range f.Hir.Statements {
					if p.stmtReferencesSessionsAPI(st, sessionsAliases, sessionsFreeFuncNames) {
						needsManager = true
						break
					}
				}

				if needsManager {
					break
				}
			}

			if needsManager {
				break
			}
		}
	}

	if cfg, ok := pc.Cache["build_config"].(interface{ TestModeValue() bool }); ok && cfg.TestModeValue() {
		needsManager = true
	}

	pc.Cache[NeedsSessionManagerCacheKey] = needsManager
	pc.Cache[FrontendWireFuncsCacheKey] = frontendWires
	pc.Cache[ReactiveWireVarsCacheKey] = reactiveWireVars
	p.attachFrontendWireMethods(pc, frontendWires)
	p.checkOffSessionCalls(pc, frontendWires)
	return nil
}

func (p *PassAnalyzeWire) stmtReferencesSessionsAPI(s ir.Stmt, aliases map[ir.SymID]bool, names map[string]bool) bool {
	if s == nil || len(aliases) == 0 {
		return false
	}

	switch v := s.(type) {
	case *ir.BlockStmt:
		for _, ss := range v.Stmts {
			if p.stmtReferencesSessionsAPI(ss, aliases, names) {
				return true
			}
		}

	case *ir.FuncDeclStmt:
		if v.Body != nil {
			return p.stmtReferencesSessionsAPI(v.Body, aliases, names)
		}

	case *ir.VarDeclStmt:
		return p.exprReferencesSessionsAPI(v.Init, aliases, names)
	case *ir.ExprStmt:
		return p.exprReferencesSessionsAPI(v.Expr, aliases, names)
	case *ir.FieldAssignmentStmt:
		return p.exprReferencesSessionsAPI(v.Value, aliases, names)
	case *ir.MultiAssignmentStmt:
		return p.exprReferencesSessionsAPI(v.Value, aliases, names)
	case *ir.IfStmt:
		if p.exprReferencesSessionsAPI(v.Cond, aliases, names) {
			return true
		}

		if p.stmtReferencesSessionsAPI(v.Then, aliases, names) {
			return true
		}

		for _, eb := range v.ElseIfs {
			if p.exprReferencesSessionsAPI(eb.Cond, aliases, names) || p.stmtReferencesSessionsAPI(eb.Then, aliases, names) {
				return true
			}
		}

		if v.Else != nil && p.stmtReferencesSessionsAPI(v.Else, aliases, names) {
			return true
		}

	case *ir.ReturnStmt:
		for _, r := range v.Results {
			if p.exprReferencesSessionsAPI(r, aliases, names) {
				return true
			}
		}

	case *ir.ForStmt:
		if v.Body != nil {
			return p.stmtReferencesSessionsAPI(v.Body, aliases, names)
		}

	case *ir.WhileStmt:
		if p.exprReferencesSessionsAPI(v.Cond, aliases, names) {
			return true
		}

		if v.Body != nil {
			return p.stmtReferencesSessionsAPI(v.Body, aliases, names)
		}
	}

	return false
}

func (p *PassAnalyzeWire) exprReferencesSessionsAPI(e ir.Expr, aliases map[ir.SymID]bool, names map[string]bool) bool {
	if e == nil {
		return false
	}

	switch x := e.(type) {
	case *ir.FieldAccessExpr:
		if vr, ok := x.Expr.(*ir.VarRef); ok && aliases[vr.Ref.Sym] {
			for _, fld := range x.Fields {
				if names[fld.Name] {
					return true
				}
			}
		}

		return p.exprReferencesSessionsAPI(x.Expr, aliases, names)
	case *ir.FuncCallExpr:
		if p.exprReferencesSessionsAPI(x.Callee, aliases, names) {
			return true
		}

		for _, a := range x.Args {
			if p.exprReferencesSessionsAPI(a.Expr, aliases, names) {
				return true
			}
		}

	case *ir.BinaryExpr:
		return p.exprReferencesSessionsAPI(x.Left, aliases, names) || p.exprReferencesSessionsAPI(x.Right, aliases, names)
	case *ir.UnaryExpr:
		return p.exprReferencesSessionsAPI(x.Expr, aliases, names)
	case *ir.AssignmentExpr:
		return p.exprReferencesSessionsAPI(x.Right, aliases, names)
	case *ir.GroupedExpr:
		return p.exprReferencesSessionsAPI(x.Expr, aliases, names)
	case *ir.TenaryExpr:
		return p.exprReferencesSessionsAPI(x.Cond, aliases, names) || p.exprReferencesSessionsAPI(x.Then, aliases, names) || p.exprReferencesSessionsAPI(x.Else, aliases, names)
	}

	return false
}

func (p *PassAnalyzeWire) checkOffSessionCalls(pc *PassContext, frontendWires []*ir.FuncDeclStmt) {
	if len(frontendWires) == 0 {
		return
	}

	frontendSyms := map[ir.SymID]string{}

	for _, fn := range frontendWires {
		if fn.Name.Sym != 0 {
			frontendSyms[fn.Name.Sym] = fn.Name.Name
		}
	}

	for _, pkg := range pc.Pkgs {
		for _, f := range pkg.Files {
			if f.Hir.Side.Kind == ir.SideFrontend {
				continue
			}

			for _, st := range f.Hir.Statements {
				p.walkOffSessionStmt(pc, st, frontendSyms)
			}
		}
	}
}

func (p *PassAnalyzeWire) walkOffSessionStmt(pc *PassContext, s ir.Stmt, frontendSyms map[ir.SymID]string) {
	if ir.IsNilStmt(s) {
		return
	}

	switch v := s.(type) {
	case *ir.BlockStmt:
		for _, ss := range v.Stmts {
			p.walkOffSessionStmt(pc, ss, frontendSyms)
		}

	case *ir.FuncDeclStmt:
		if v.Body != nil {
			p.walkOffSessionStmt(pc, v.Body, frontendSyms)
		}

	case *ir.VarDeclStmt:
		p.walkOffSessionExpr(pc, v.Init, frontendSyms)
	case *ir.ExprStmt:
		p.walkOffSessionExpr(pc, v.Expr, frontendSyms)
	case *ir.FieldAssignmentStmt:
		p.walkOffSessionExpr(pc, v.Value, frontendSyms)
	case *ir.MultiAssignmentStmt:
		p.walkOffSessionExpr(pc, v.Value, frontendSyms)
	case *ir.IfStmt:
		p.walkOffSessionExpr(pc, v.Cond, frontendSyms)
		p.walkOffSessionStmt(pc, v.Then, frontendSyms)
		for _, eb := range v.ElseIfs {
			p.walkOffSessionExpr(pc, eb.Cond, frontendSyms)
			p.walkOffSessionStmt(pc, eb.Then, frontendSyms)
		}

		p.walkOffSessionStmt(pc, v.Else, frontendSyms)
	case *ir.ReturnStmt:
		for _, r := range v.Results {
			p.walkOffSessionExpr(pc, r, frontendSyms)
		}

	case *ir.ForStmt:
		if v.Body != nil {
			p.walkOffSessionStmt(pc, v.Body, frontendSyms)
		}

	case *ir.WhileStmt:
		p.walkOffSessionExpr(pc, v.Cond, frontendSyms)
		if v.Body != nil {
			p.walkOffSessionStmt(pc, v.Body, frontendSyms)
		}
	}
}

func (p *PassAnalyzeWire) walkOffSessionExpr(pc *PassContext, e ir.Expr, frontendSyms map[ir.SymID]string) {
	if ir.IsNilExpr(e) {
		return
	}

	if call, ok := e.(*ir.FuncCallExpr); ok {
		if vr, ok := call.Callee.(*ir.VarRef); ok && vr.Ref.Sym != 0 {
			if name, hit := frontendSyms[vr.Ref.Sym]; hit {
				pc.Diag.Report(diag.ErrFrontendWireOffSession, call.Span(), name)
			}
		}

		p.walkOffSessionExpr(pc, call.Callee, frontendSyms)
		for _, a := range call.Args {
			p.walkOffSessionExpr(pc, a.Expr, frontendSyms)
		}

		return
	}

	switch x := e.(type) {
	case *ir.BinaryExpr:
		p.walkOffSessionExpr(pc, x.Left, frontendSyms)
		p.walkOffSessionExpr(pc, x.Right, frontendSyms)
	case *ir.UnaryExpr:
		p.walkOffSessionExpr(pc, x.Expr, frontendSyms)
	case *ir.FieldAccessExpr:
		p.walkOffSessionExpr(pc, x.Expr, frontendSyms)
	case *ir.IndexExpr:
		p.walkOffSessionExpr(pc, x.Expr, frontendSyms)
		p.walkOffSessionExpr(pc, x.Index, frontendSyms)
	case *ir.AssignmentExpr:
		p.walkOffSessionExpr(pc, x.Right, frontendSyms)
	case *ir.GroupedExpr:
		p.walkOffSessionExpr(pc, x.Expr, frontendSyms)
	case *ir.TenaryExpr:
		p.walkOffSessionExpr(pc, x.Cond, frontendSyms)
		p.walkOffSessionExpr(pc, x.Then, frontendSyms)
		p.walkOffSessionExpr(pc, x.Else, frontendSyms)
	}
}

func (p *PassAnalyzeWire) validateRawWireSignature(pc *PassContext, fn *ir.FuncDeclStmt) {
	if fn == nil {
		return
	}

	if len(fn.Params) != 2 {
		pc.Diag.Report(diag.ErrWireRawBadSignature, fn.Name.Span, fn.Name.Name, len(fn.Params))
		return
	}

	if !isRawHttpType(fn.Params[0].Type, "Request") || !isRawHttpType(fn.Params[1].Type, "Response") {
		pc.Diag.Report(diag.ErrWireRawBadParamType, fn.Name.Span, fn.Name.Name)
		return
	}

	if fn.ReturnType != nil && fn.ReturnType.Kind != ir.TK_PrimitiveNone {
		pc.Diag.Report(diag.ErrWireRawBadReturn, fn.Name.Span, fn.Name.Name)
	}
}

func (p *PassAnalyzeWire) rawWireUsesSession(fn *ir.FuncDeclStmt) bool {
	if fn == nil || fn.Body == nil {
		return false
	}

	return stmtsReferenceSession(fn.Body.Stmts)
}

func stmtsReferenceSession(stmts []ir.Stmt) bool {
	for _, st := range stmts {
		if stmtReferencesSession(st) {
			return true
		}
	}

	return false
}

func stmtReferencesSession(st ir.Stmt) bool {
	if ir.IsNilStmt(st) {
		return false
	}

	switch s := st.(type) {
	case *ir.BlockStmt:
		return stmtsReferenceSession(s.Stmts)
	case *ir.VarDeclStmt:
		return exprReferencesSession(s.Init)
	case *ir.ExprStmt:
		return exprReferencesSession(s.Expr)
	case *ir.IfStmt:
		if exprReferencesSession(s.Cond) || stmtReferencesSession(s.Then) {
			return true
		}

		for _, eb := range s.ElseIfs {
			if exprReferencesSession(eb.Cond) || stmtReferencesSession(eb.Then) {
				return true
			}
		}

		return stmtReferencesSession(s.Else)
	case *ir.ReturnStmt:
		for _, r := range s.Results {
			if exprReferencesSession(r) {
				return true
			}
		}

	case *ir.ForStmt:
		if s.Body != nil {
			return stmtReferencesSession(s.Body)
		}

	case *ir.WhileStmt:
		if exprReferencesSession(s.Cond) {
			return true
		}

		if s.Body != nil && stmtReferencesSession(s.Body) {
			return true
		}

	case *ir.FieldAssignmentStmt:
		return exprReferencesSession(s.Value)
	case *ir.IndexAssignmentStmt:
		return exprReferencesSession(s.Receiver) || exprReferencesSession(s.Index) || exprReferencesSession(s.Value)
	case *ir.MultiAssignmentStmt:
		return exprReferencesSession(s.Value)
	}

	return false
}

func exprReferencesSession(e ir.Expr) bool {
	if ir.IsNilExpr(e) {
		return false
	}

	switch x := e.(type) {
	case *ir.SessionExpr:
		return true
	case *ir.BinaryExpr:
		return exprReferencesSession(x.Left) || exprReferencesSession(x.Right)
	case *ir.UnaryExpr:
		return exprReferencesSession(x.Expr)
	case *ir.PrefixUnaryExpr:
		return exprReferencesSession(x.Expr)
	case *ir.PostfixUnaryExpr:
		return exprReferencesSession(x.Expr)
	case *ir.AssignmentExpr:
		return exprReferencesSession(x.Right)
	case *ir.GroupedExpr:
		return exprReferencesSession(x.Expr)
	case *ir.AsExpr:
		return exprReferencesSession(x.Expr)
	case *ir.InstanceofExpr:
		return exprReferencesSession(x.Expr)
	case *ir.OptionUnwrapExpr:
		return exprReferencesSession(x.Expr)
	case *ir.TenaryExpr:
		return exprReferencesSession(x.Cond) || exprReferencesSession(x.Then) || exprReferencesSession(x.Else)
	case *ir.CoalesceExpr:
		return exprReferencesSession(x.Left) || exprReferencesSession(x.Default)
	case *ir.IndexExpr:
		return exprReferencesSession(x.Expr) || exprReferencesSession(x.Index)
	case *ir.SliceRangeExpr:
		return exprReferencesSession(x.Expr) || exprReferencesSession(x.Low) || exprReferencesSession(x.High)
	case *ir.FieldAccessExpr:
		return exprReferencesSession(x.Expr)
	case *ir.FuncCallExpr:
		if exprReferencesSession(x.Callee) {
			return true
		}

		for _, a := range x.Args {
			if exprReferencesSession(a.Expr) {
				return true
			}
		}

	case *ir.NewExpr:
		for _, a := range x.Args {
			if exprReferencesSession(a.Expr) {
				return true
			}
		}
	}

	return false
}

func isRawHttpType(t *ir.TypeRef, name string) bool {
	if t == nil {
		return false
	}

	if t.CustomName != name {
		return false
	}

	switch t.CustomQualifier {
	case "http", "std/http", "":
		return true
	}

	return false
}

func (p *PassAnalyzeWire) resolveWireTransport(pc *PassContext, fn *ir.FuncDeclStmt, effSide ir.SideKind) {
	if fn == nil || fn.Wire == nil || fn.Wire.Options == nil {
		return
	}

	opt, ok := fn.Wire.Options["transport"]
	if !ok || opt.Kind != ir.WireOptString {
		return
	}

	transport := strings.ToLower(opt.Str)
	switch transport {
	case "http", "ws", "sse", "raw":
	default:
		pc.Diag.Report(diag.ErrWireInvalidTransport, fn.Name.Span, opt.Str, fn.Name.Name)
		return
	}

	sideOK := true
	sideLabel := "backend"
	if effSide == ir.SideFrontend {
		sideLabel = "frontend"
		if transport == "http" || transport == "raw" {
			sideOK = false
		}
	} else {
		if transport == "sse" {
			sideOK = false
		}
	}

	if !sideOK {
		pc.Diag.Report(diag.ErrWireTransportSideMismatch, fn.Name.Span, transport, fn.Name.Name, sideLabel)
		return
	}

	fn.Wire.Transport = transport
}

func targetName(vd *ir.VarDeclStmt) string {
	if len(vd.Targets) > 0 && vd.Targets[0].Name != nil {
		return vd.Targets[0].Name.Name
	}

	return "<anonymous>"
}

func (p *PassAnalyzeWire) attachFrontendWireMethods(pc *PassContext, frontendWires []*ir.FuncDeclStmt) {
	if len(frontendWires) == 0 {
		return
	}

	sessionTyp, ok := pc.Cache[SessionsSessionTypeCacheKey].(ir.TypID)
	if !ok {
		return
	}

	sessionStruct, ok := pc.Types.GetByID(sessionTyp)
	if !ok {
		return
	}

	broadcastTyp, _ := pc.Cache["sessions_broadcast_typ"].(ir.TypID)
	var broadcastStruct *ir.Type
	if broadcastTyp != 0 {
		broadcastStruct, _ = pc.Types.GetByID(broadcastTyp)
	}

	for _, fn := range frontendWires {
		methodName := fn.Name.Name
		if hasStructMethod(sessionStruct, methodName) {
			continue
		}

		params := make([]*ir.FuncParam, 0, len(fn.Params))
		for _, prm := range fn.Params {
			params = append(params, &ir.FuncParam{
				Name: ir.NameRef{Name: prm.Name.Name},
				Type: prm.Type,
			})
		}

		retTyp := pc.Types.TypNone()
		if fn.ReturnType != nil && fn.ReturnType.Typ != 0 {
			retTyp = fn.ReturnType.Typ
		}

		fnTyp := pc.Types.AsyncFuncOf(params, retTyp)

		sessionStruct.StructMethods = append(sessionStruct.StructMethods, ir.StructMethodInfo{Name: methodName, FuncTyp: fnTyp, Sym: fn.Name.Sym})
		if broadcastStruct != nil && !hasStructMethod(broadcastStruct, methodName) {
			broadcastStruct.StructMethods = append(broadcastStruct.StructMethods, ir.StructMethodInfo{Name: methodName, FuncTyp: fnTyp, Sym: fn.Name.Sym})
		}
	}
}

func hasStructMethod(st *ir.Type, name string) bool {
	if st == nil {
		return false
	}

	for _, m := range st.StructMethods {
		if m.Name == name {
			return true
		}
	}

	return false
}

func (p *PassAnalyzeWire) ensureSessionType(pc *PassContext) ir.TypID {
	if cached, ok := pc.Cache[SessionTypeCacheKey]; ok {
		return cached.(ir.TypID)
	}

	if cached, ok := pc.Cache[SessionsSessionTypeCacheKey]; ok {
		typ := cached.(ir.TypID)
		pc.Cache[SessionTypeCacheKey] = typ
		return typ
	}

	return 0
}

func (p *PassAnalyzeWire) ensureWireState(pc *PassContext) ir.TypID {
	if cached, ok := pc.Cache[WireStateCacheKey]; ok {
		return cached.(ir.TypID)
	}

	cases := []ir.EnumCaseInfo{
		{Name: "Ok", Ordinal: 0, Value: 0},
		{Name: "Unauthorized", Ordinal: 1, Value: 1},
		{Name: "Forbidden", Ordinal: 2, Value: 2},
		{Name: "NotFound", Ordinal: 3, Value: 3},
		{Name: "Error", Ordinal: 4, Value: 4},
	}

	typ := pc.Types.EnumOf("", "WireState", cases, nil, true)
	if enumTy, ok := pc.Types.GetByID(typ); ok {
		enumTy.EnumCases = cases
		enumTy.IsNumeric = true
	}

	pc.Cache[WireStateCacheKey] = typ
	for _, pkg := range pc.Pkgs {
		if _, exists := pkg.Scopes.LookupOnlyCurrent(pkg.Root, "WireState"); exists {
			continue
		}

		sym := pkg.Syms.NewSymbol(ir.SK_Function, "WireState", pkg.Root, typ, 0)
		pkg.Scopes.DeclareSymbol(pkg.Root, "WireState", sym, pkg.Syms)
	}

	return typ
}

func (p *PassAnalyzeWire) upgradeWiredSymType(pc *PassContext, pkg *ir.PackageContext, fn *ir.FuncDeclStmt, wireStateTyp ir.TypID) {
	s, ok := pkg.Syms.GetByID(fn.Name.Sym)
	if !ok {
		return
	}

	ft, ok := pc.Types.GetByID(s.Typ)
	if !ok || ft.Kind != ir.TK_Function {
		return
	}

	innerRet := ft.ReturnType
	if innerRet == 0 {
		innerRet = pc.Types.TypNone()
	}

	tupleTyp := pc.Types.TupleOf(
		ir.TupleField{Name: "value", Type: innerRet},
		ir.TupleField{Name: "state", Type: wireStateTyp},
	)
	newFuncTyp := pc.Types.AsyncFuncOf(ft.ParamTypes, tupleTyp)
	pkg.Syms.SetType(fn.Name.Sym, newFuncTyp)
}

func (p *PassAnalyzeWire) deriveRoute(pc *PassContext, pkg *ir.PackageContext, fn *ir.FuncDeclStmt) {
	if fn.Wire == nil {
		fn.Wire = &ir.WireSpec{Options: map[string]ir.WireOptValue{}}
	}

	fn.Wire.RequireAuthN = true
	if v, ok := fn.Wire.Options["authn"]; ok && v.Kind == ir.WireOptBool {
		fn.Wire.RequireAuthN = v.Bool
	}

	if v, ok := fn.Wire.Options["authz"]; ok && v.Kind == ir.WireOptStringArray {
		fn.Wire.RequiredRoles = v.Strs
	}

	overrideMethod := ""
	overridePath := ""
	if m, ok := fn.Wire.Options["method"]; ok && m.Kind == ir.WireOptString {
		overrideMethod = strings.ToUpper(m.Str)
	}

	if pth, ok := fn.Wire.Options["path"]; ok && pth.Kind == ir.WireOptString {
		overridePath = pth.Str
	}

	method, verbLen, plural := classifyVerb(fn.Name.Name)
	if overrideMethod != "" {
		method = overrideMethod
	}

	fn.Wire.Method = method

	resource := fn.Name.Name[verbLen:]
	if resource == "" {
		resource = fn.Name.Name
	}

	resourceKebab := camelToKebab(resource)
	if plural && method == "GET" {
		resourceKebab = pluralize(resourceKebab)
	}

	p.bindParamAnnotations(pc, fn)

	var pathArgs []string
	for _, param := range fn.Params {
		switch param.WireBinding {
		case "path":
			pathArgs = append(pathArgs, paramBindName(param))
		case "query", "header", "cookie", "body":
		case "":
			if isPathArgName(param.Name.Name) {
				pathArgs = append(pathArgs, param.Name.Name)
			}
		}
	}

	fn.Wire.PathArgs = pathArgs

	if overridePath != "" {
		validatePathBindings(pc, fn, overridePath)
		placeholders := map[string]bool{}

		for _, ph := range extractPathPlaceholders(overridePath) {
			placeholders[ph] = true
		}

		filtered := pathArgs[:0]
		for _, name := range pathArgs {
			if placeholders[name] {
				filtered = append(filtered, name)
			}
		}

		fn.Wire.PathArgs = filtered
		fn.Wire.Path = overridePath
		return
	}

	parts := []string{"/api"}

	for _, seg := range pkg.Path {
		parts = append(parts, seg)
	}

	parts = append(parts, resourceKebab)
	path := strings.Join(parts, "/")
	path = strings.ReplaceAll(path, "//", "/")
	for _, arg := range pathArgs {
		path += "/:" + arg
	}

	fn.Wire.Path = path
}

func (p *PassAnalyzeWire) deriveVarRoute(pkg *ir.PackageContext, vd *ir.VarDeclStmt) {
	if vd.Wire == nil {
		vd.Wire = &ir.WireSpec{Options: map[string]ir.WireOptValue{}}
	}

	vd.Wire.RequireAuthN = true
	if v, ok := vd.Wire.Options["authn"]; ok && v.Kind == ir.WireOptBool {
		vd.Wire.RequireAuthN = v.Bool
	}

	if v, ok := vd.Wire.Options["authz"]; ok && v.Kind == ir.WireOptStringArray {
		vd.Wire.RequiredRoles = v.Strs
	}

	overrideMethod := "GET"
	if m, ok := vd.Wire.Options["method"]; ok && m.Kind == ir.WireOptString {
		overrideMethod = strings.ToUpper(m.Str)
	}

	vd.Wire.Method = overrideMethod

	if len(vd.Targets) == 0 || vd.Targets[0].Name == nil {
		return
	}

	name := vd.Targets[0].Name.Name
	resourceKebab := camelToKebab(name)

	if pth, ok := vd.Wire.Options["path"]; ok && pth.Kind == ir.WireOptString {
		vd.Wire.Path = pth.Str
		return
	}

	parts := []string{"/api"}

	for _, seg := range pkg.Path {
		parts = append(parts, seg)
	}

	parts = append(parts, resourceKebab)
	vd.Wire.Path = strings.Join(parts, "/")
}

func classifyVerb(name string) (string, int, bool) {
	type rule struct {
		prefix string
		method string
		plural bool
	}

	rules := []rule{
		{"findAll", "GET", true},
		{"list", "GET", true},
		{"fetch", "GET", false},
		{"find", "GET", false},
		{"get", "GET", false},
		{"create", "POST", false},
		{"add", "POST", false},
		{"update", "PUT", false},
		{"set", "PUT", false},
		{"patch", "PATCH", false},
		{"edit", "PATCH", false},
		{"delete", "DELETE", false},
		{"remove", "DELETE", false},
	}

	for _, r := range rules {
		if strings.HasPrefix(name, r.prefix) && len(name) > len(r.prefix) && unicode.IsUpper(rune(name[len(r.prefix)])) {
			return r.method, len(r.prefix), r.plural
		}
	}

	return "POST", 0, false
}

func camelToKebab(s string) string {
	if s == "" {
		return s
	}

	var b strings.Builder
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteByte('-')
		}

		b.WriteRune(unicode.ToLower(r))
	}

	return b.String()
}

func pluralize(s string) string {
	if s == "" {
		return s
	}

	if strings.HasSuffix(s, "s") {
		return s
	}

	if strings.HasSuffix(s, "y") {
		return s[:len(s)-1] + "ies"
	}

	return s + "s"
}

func isPathArgName(name string) bool {
	switch name {
	case "id", "key", "slug", "name":
		return true
	}

	return false
}

func paramBindName(p *ir.FuncParam) string {
	if p.WireBindAs != "" {
		return p.WireBindAs
	}

	return p.Name.Name
}

func (p *PassAnalyzeWire) bindParamAnnotations(pc *PassContext, fn *ir.FuncDeclStmt) {
	for _, param := range fn.Params {
		for _, anno := range param.Annotations {
			binding := ""
			switch anno.Name.Name {
			case "query":
				binding = "query"
			case "path":
				binding = "path"
			case "header":
				binding = "header"
			case "cookie":
				binding = "cookie"
			case "body":
				binding = "body"
			default:
				continue
			}

			if param.WireBinding != "" && param.WireBinding != binding {
				pc.Diag.Report(diag.ErrWireConflictingParamBinding, param.Name.Span, param.Name.Name, param.WireBinding, binding)
				continue
			}

			param.WireBinding = binding
			if len(anno.ResolvedArgs) > 0 && anno.ResolvedArgs[0].Kind == ir.AnnotationValueString {
				param.WireBindAs = anno.ResolvedArgs[0].Str
			}
		}
	}
}

func validatePathBindings(pc *PassContext, fn *ir.FuncDeclStmt, path string) {
	placeholders := extractPathPlaceholders(path)
	declared := map[string]bool{}

	for _, ph := range placeholders {
		declared[ph] = true
	}

	for _, param := range fn.Params {
		if param.WireBinding != "path" {
			continue
		}

		name := paramBindName(param)
		if !declared[name] {
			pc.Diag.Report(diag.ErrWirePathParamMissing, param.Name.Span, name, path)
		}
	}
}

func extractPathPlaceholders(path string) []string {
	var out []string
	i := 0
	for i < len(path) {
		if path[i] != ':' {
			i++
			continue
		}

		i++
		start := i
		for i < len(path) {
			c := path[i]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
				i++
				continue
			}

			break
		}

		if i > start {
			out = append(out, path[start:i])
		}
	}

	return out
}

func (p *PassAnalyzeWire) validateWired(pc *PassContext, pkg *ir.PackageContext, f *ir.PreparsedFile, fn *ir.FuncDeclStmt) {
	side := f.Hir.Side.Kind
	if fn.Side != nil {
		side = fn.Side.Kind
	}

	if side == ir.SideShared {
		pc.Diag.Report(diag.ErrWireOnShared, fn.Name.Span, fn.Name.Name)
	}

	for _, param := range fn.Params {
		if param.Type == nil || !p.isTransferable(pc, param.Type.Typ) {
			pc.Diag.Report(diag.ErrWireNonTransferableType, param.Name.Span, fn.Name.Name+"."+param.Name.Name)
		}
	}

	if fn.ReturnType != nil && fn.ReturnType.Typ != 0 && fn.ReturnType.Typ != pc.Types.TypNone() {
		if !p.isTransferable(pc, fn.ReturnType.Typ) {
			pc.Diag.Report(diag.ErrWireNonTransferableType, fn.Name.Span, fn.Name.Name+" return")
		}
	}
}

func (p *PassAnalyzeWire) isTransferable(pc *PassContext, t ir.TypID) bool {
	if t == 0 {
		return false
	}

	ty, ok := pc.Types.GetByID(t)
	if !ok {
		return false
	}

	switch ty.Kind {
	case ir.TK_PrimitiveInt, ir.TK_PrimitiveFloat, ir.TK_PrimitiveBool, ir.TK_PrimitiveString, ir.TK_PrimitiveChar, ir.TK_PrimitiveByte, ir.TK_PrimitiveAny, ir.TK_PrimitiveNone:
		return true
	case ir.TK_Option, ir.TK_Slice, ir.TK_Array:
		return p.isTransferable(pc, ty.ElemType)
	case ir.TK_Map:
		return p.isTransferable(pc, ty.KeyType) && p.isTransferable(pc, ty.ValueType)
	case ir.TK_Tuple:
		for _, fld := range ty.Fields {
			if !p.isTransferable(pc, fld.Type) {
				return false
			}
		}

		return true
	case ir.TK_Struct, ir.TK_Enum:
		return true
	}

	return false
}

func (p *PassAnalyzeWire) upgradeSymTypeToAsync(pc *PassContext, pkg *ir.PackageContext, sym ir.SymID) {
	s, ok := pkg.Syms.GetByID(sym)
	if !ok {
		return
	}

	ft, ok := pc.Types.GetByID(s.Typ)
	if !ok || ft.Kind != ir.TK_Function {
		return
	}

	newTyp := pc.Types.AsyncFuncOf(ft.ParamTypes, ft.ReturnType)
	pkg.Syms.SetType(sym, newTyp)
}

func (p *PassAnalyzeWire) checkNoSessionOnFrontend(pc *PassContext, stmts []ir.Stmt) {
	for _, s := range stmts {
		p.walkSessionStmt(pc, s)
	}
}

func (p *PassAnalyzeWire) walkSessionStmt(pc *PassContext, s ir.Stmt) {
	if ir.IsNilStmt(s) {
		return
	}

	switch v := s.(type) {
	case *ir.BlockStmt:
		for _, ss := range v.Stmts {
			p.walkSessionStmt(pc, ss)
		}

	case *ir.VarDeclStmt:
		p.walkSessionExpr(pc, v.Init)
	case *ir.ExprStmt:
		p.walkSessionExpr(pc, v.Expr)
	case *ir.FieldAssignmentStmt:
		p.walkSessionExpr(pc, v.Value)
	case *ir.MultiAssignmentStmt:
		p.walkSessionExpr(pc, v.Value)
	case *ir.IfStmt:
		p.walkSessionExpr(pc, v.Cond)
		p.walkSessionStmt(pc, v.Then)
		for _, eb := range v.ElseIfs {
			p.walkSessionExpr(pc, eb.Cond)
			p.walkSessionStmt(pc, eb.Then)
		}

		p.walkSessionStmt(pc, v.Else)
	case *ir.SwitchStmt:
		p.walkSessionExpr(pc, v.Expr)
		for _, c := range v.Cases {
			for _, val := range c.Values {
				p.walkSessionExpr(pc, val)
			}

			for _, ss := range c.Stmts {
				p.walkSessionStmt(pc, ss)
			}
		}

		for _, ss := range v.Default {
			p.walkSessionStmt(pc, ss)
		}

	case *ir.ReturnStmt:
		for _, r := range v.Results {
			p.walkSessionExpr(pc, r)
		}

	case *ir.GuardStmt:
		p.walkSessionExpr(pc, v.Cond)
		for _, r := range v.Returns {
			p.walkSessionExpr(pc, r)
		}

	case *ir.ForStmt:
		if v.CondInt != nil && v.CondInt.Init != nil {
			p.walkSessionExpr(pc, v.CondInt.Init.Init)
			p.walkSessionExpr(pc, v.CondInt.Cond)
			p.walkSessionExpr(pc, v.CondInt.Post)
		}

		if v.CondRange != nil {
			p.walkSessionExpr(pc, v.CondRange.RangeStart)
			p.walkSessionExpr(pc, v.CondRange.RangeEnd)
		}

		if v.CondIn != nil {
			p.walkSessionExpr(pc, v.CondIn.IterExpr)
		}

		p.walkSessionStmt(pc, v.Body)
	case *ir.WhileStmt:
		p.walkSessionExpr(pc, v.Cond)
		p.walkSessionStmt(pc, v.Body)
	case *ir.FuncDeclStmt:
		p.walkSessionStmt(pc, v.Body)
	}
}

func (p *PassAnalyzeWire) walkSessionExpr(pc *PassContext, e ir.Expr) {
	if e == nil {
		return
	}

	switch x := e.(type) {
	case *ir.SessionExpr:
		pc.Diag.Report(diag.ErrSessionOnFrontend, x.Span())
	case *ir.BinaryExpr:
		p.walkSessionExpr(pc, x.Left)
		p.walkSessionExpr(pc, x.Right)
	case *ir.UnaryExpr:
		p.walkSessionExpr(pc, x.Expr)
	case *ir.PrefixUnaryExpr:
		p.walkSessionExpr(pc, x.Expr)
	case *ir.PostfixUnaryExpr:
		p.walkSessionExpr(pc, x.Expr)
	case *ir.AssignmentExpr:
		p.walkSessionExpr(pc, x.Right)
	case *ir.IndexExpr:
		p.walkSessionExpr(pc, x.Expr)
		p.walkSessionExpr(pc, x.Index)
	case *ir.FieldAccessExpr:
		p.walkSessionExpr(pc, x.Expr)
	case *ir.FuncCallExpr:
		p.walkSessionExpr(pc, x.Callee)
		for _, a := range x.Args {
			p.walkSessionExpr(pc, a.Expr)
		}

	case *ir.NewExpr:
		for _, a := range x.Args {
			p.walkSessionExpr(pc, a.Expr)
		}

	case *ir.RangeExpr:
		p.walkSessionExpr(pc, x.Start)
		p.walkSessionExpr(pc, x.End)
		p.walkSessionExpr(pc, x.Inc)
	case *ir.TenaryExpr:
		p.walkSessionExpr(pc, x.Cond)
		p.walkSessionExpr(pc, x.Then)
		p.walkSessionExpr(pc, x.Else)
	case *ir.CoalesceExpr:
		p.walkSessionExpr(pc, x.Left)
		p.walkSessionExpr(pc, x.Default)
	case *ir.GroupedExpr:
		p.walkSessionExpr(pc, x.Expr)
	case *ir.WhenExpr:
		p.walkSessionExpr(pc, x.Expr)
		p.walkSessionExpr(pc, x.Default)
		for _, c := range x.Cases {
			for _, val := range c.Values {
				p.walkSessionExpr(pc, val)
			}

			p.walkSessionExpr(pc, c.Then)
		}

	case *ir.StringTemplateExpr:
		for _, part := range x.Parts {
			p.walkSessionExpr(pc, part.Expr)
		}

	case *ir.ArrayLiteral:
		for _, el := range x.Elems {
			p.walkSessionExpr(pc, el)
		}

	case *ir.MapLiteral:
		for _, en := range x.Entries {
			p.walkSessionExpr(pc, en.Key)
			p.walkSessionExpr(pc, en.Value)
		}

	case *ir.TupleLiteral:
		for _, el := range x.Elems {
			p.walkSessionExpr(pc, el)
		}
	}
}
