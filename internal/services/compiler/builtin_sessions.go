package compiler

import (
	"sova/internal/ir"
)

const SessionsPackagePath = "sessions"

const SessionsSessionTypeCacheKey = "sessions_session_typ"

const SessionsBroadcastTypeCacheKey = "sessions_broadcast_typ"

const SessionsPackageCacheKey = "sessions_package"

func registerBuiltinPackages(c *CompilerContext) {
	buildSessionsPackage(c)
}

func buildSessionsPackage(c *CompilerContext) {
	scopes := ir.NewScopeGraph(c.Diag, c.ScAlloc)
	pkg := &ir.PackageContext{
		Path:   ir.PackagePath{SessionsPackagePath},
		Files:  nil,
		Syms:   ir.NewSymbolArena(c.SymAlloc),
		Types:  c.TypeUniverse,
		Scopes: scopes,
		Root:   scopes.Root,
	}

	t := c.TypeUniverse
	stringTyp := t.PrimString()
	intTyp := t.PrimInt()
	boolTyp := t.PrimBool()
	anyTyp := t.PrimAny()
	noneTyp := t.TypNone()
	stringSlice := t.SliceOf(stringTyp)
	claimsMap := t.MapOf(stringTyp, anyTyp)

	sessionFields := []ir.StructFieldInfo{
		{Name: "id", Type: stringTyp},
		{Name: "user", Type: anyTyp},
		{Name: "roles", Type: stringSlice},
		{Name: "claims", Type: claimsMap},
		{Name: "rooms", Type: stringSlice},
		{Name: "connectedAt", Type: intTyp},
		{Name: "isConnected", Type: boolTyp},
	}

	sessionTyp := t.StructOf("sessions", "Session", sessionFields)
	sessionStructTy, _ := t.GetByID(sessionTyp)
	sessionStructTy.Struct.Fields = sessionFields

	mkParam := func(name string, typ ir.TypID) *ir.FuncParam {
		return &ir.FuncParam{
			Name: ir.NameRef{Name: name},
			Type: &ir.TypeRef{Typ: typ},
		}
	}

	mkParamDefault := func(name string, typ ir.TypID, def ir.Expr) *ir.FuncParam {
		return &ir.FuncParam{
			Name:    ir.NameRef{Name: name},
			Type:    &ir.TypeRef{Typ: typ},
			Default: def,
		}
	}

	_ = mkParamDefault

	sessionStructTy.Struct.Methods = []ir.StructMethodInfo{
		{Name: "authenticate", FuncTyp: t.FuncOf([]*ir.FuncParam{mkParam("user", anyTyp), mkParam("claims", claimsMap)}, noneTyp)},
		{Name: "logout", FuncTyp: t.FuncOf(nil, noneTyp)},
		{Name: "addRoles", FuncTyp: t.FuncOf([]*ir.FuncParam{mkParam("roles", stringSlice)}, noneTyp)},
		{Name: "removeRoles", FuncTyp: t.FuncOf([]*ir.FuncParam{mkParam("roles", stringSlice)}, noneTyp)},
		{Name: "setRoles", FuncTyp: t.FuncOf([]*ir.FuncParam{mkParam("roles", stringSlice)}, noneTyp)},
		{Name: "clearRoles", FuncTyp: t.FuncOf(nil, noneTyp)},
		{Name: "hasRole", FuncTyp: t.FuncOf([]*ir.FuncParam{mkParam("role", stringTyp)}, boolTyp)},
		{Name: "isAuthenticated", FuncTyp: t.FuncOf(nil, boolTyp)},
		{Name: "join", FuncTyp: t.FuncOf([]*ir.FuncParam{mkParam("room", stringTyp)}, noneTyp)},
		{Name: "leave", FuncTyp: t.FuncOf([]*ir.FuncParam{mkParam("room", stringTyp)}, noneTyp)},
		{Name: "inRoom", FuncTyp: t.FuncOf([]*ir.FuncParam{mkParam("room", stringTyp)}, boolTyp)},
	}

	sessionSym := pkg.Syms.NewSymbol(ir.SK_Function, "Session", pkg.Root, sessionTyp, 0)
	pkg.Syms.SetDoc(sessionSym, "A connected client's session handle. `@` evaluates to the current handler's `Session`, and any wired function can pass it around like a normal value. Lives in the server's session registry for the lifetime of the connection plus a short reconnect grace window.")
	pkg.Scopes.DeclareSymbol(pkg.Root, "Session", sessionSym, pkg.Syms)

	broadcastFields := []ir.StructFieldInfo{}

	broadcastTyp := t.StructOf("sessions", "Broadcast", broadcastFields)
	broadcastStructTy, _ := t.GetByID(broadcastTyp)
	broadcastStructTy.Struct.Fields = broadcastFields

	predicateTyp := t.FuncOf([]*ir.FuncParam{mkParam("s", sessionTyp)}, boolTyp)
	broadcastStructTy.Struct.Methods = []ir.StructMethodInfo{
		{Name: "toRoom", FuncTyp: t.FuncOf([]*ir.FuncParam{mkParam("room", stringTyp)}, broadcastTyp)},
		{Name: "filter", FuncTyp: t.FuncOf([]*ir.FuncParam{mkParam("predicate", predicateTyp)}, broadcastTyp)},
	}

	broadcastSym := pkg.Syms.NewSymbol(ir.SK_Function, "Broadcast", pkg.Root, broadcastTyp, 0)
	pkg.Syms.SetDoc(broadcastSym, "A narrowing handle for fan-out delivery. Obtained via `sessions.broadcast()`, then chained with `.to(room)` or `.filter(predicate)` to scope the target audience. Every frontend `wire func` shows up as a method on `Broadcast` so the call fans out to every matching session.")
	pkg.Scopes.DeclareSymbol(pkg.Root, "Broadcast", broadcastSym, pkg.Syms)

	sessionOption := t.OptionOf(sessionTyp)
	sessionSlice := t.SliceOf(sessionTyp)
	handlerTyp := t.FuncOf([]*ir.FuncParam{mkParam("s", sessionTyp)}, noneTyp)

	roomHandlerTyp := t.FuncOf([]*ir.FuncParam{mkParam("s", sessionTyp), mkParam("room", stringTyp)}, noneTyp)
	freeFuncs := []struct {
		name   string
		params []*ir.FuncParam
		ret    ir.TypID
		doc    string
	}{
		{"all", nil, sessionSlice, "Returns every currently-registered session.\n\n@returns slice of all live sessions"},
		{"byId", []*ir.FuncParam{mkParam("id", stringTyp)}, sessionOption, "Looks up a session by its server-assigned id. Returns `none` if the id is unknown or expired.\n\n@param id the session id\n@returns the matching session, or none"},
		{"firstByUser", []*ir.FuncParam{mkParam("user", anyTyp)}, sessionOption, "Returns the first authenticated session whose `user` matches. Use when you expect a single session per user.\n\n@param user the authenticated user value to match\n@returns the first matching session, or none"},
		{"allByUser", []*ir.FuncParam{mkParam("user", anyTyp)}, sessionSlice, "Returns every authenticated session whose `user` matches. Use when one user may have multiple concurrent sessions (laptop + phone).\n\n@param user the authenticated user value to match\n@returns slice of all matching sessions"},
		{"current", nil, sessionTyp, "Returns the session bound to the current request. Equivalent to `@`; use this when the contextual form isn't ergonomic (e.g. when assigning to a named local).\n\n@returns the current session"},
		{"broadcast", nil, broadcastTyp, "Returns a `Broadcast` rooted at all live sessions. Chain `.to(room)` / `.filter(predicate)` to narrow, then call the frontend wire-method to push.\n\n@returns a broadcast handle"},
		{"onConnect", []*ir.FuncParam{mkParam("handler", handlerTyp)}, noneTyp, "Registers a handler invoked whenever a new WebSocket session connects.\n\n@param handler called with the connecting session"},
		{"onDisconnect", []*ir.FuncParam{mkParam("handler", handlerTyp)}, noneTyp, "Registers a handler invoked when a WebSocket session disconnects (after the grace timer expires).\n\n@param handler called with the disconnecting session"},
		{"onRoomJoin", []*ir.FuncParam{mkParam("handler", roomHandlerTyp)}, noneTyp, "Registers a handler invoked when a session joins a room via `Session.join(room)`.\n\n@param handler called with the joining session and the room name"},
		{"onRoomLeave", []*ir.FuncParam{mkParam("handler", roomHandlerTyp)}, noneTyp, "Registers a handler invoked when a session leaves a room via `Session.leave(room)` or disconnects.\n\n@param handler called with the leaving session and the room name"},
	}

	for _, f := range freeFuncs {
		ft := t.FuncOf(f.params, f.ret)
		sym := pkg.Syms.NewSymbol(ir.SK_Function, f.name, pkg.Root, ft, 0)
		pkg.Syms.SetDoc(sym, f.doc)
		pkg.Scopes.DeclareSymbol(pkg.Root, f.name, sym, pkg.Syms)
	}

	c.Packages[SessionsPackagePath] = pkg
	c.Cache[SessionsSessionTypeCacheKey] = sessionTyp
	c.Cache[SessionsBroadcastTypeCacheKey] = broadcastTyp
	c.Cache[SessionsPackageCacheKey] = pkg
}
