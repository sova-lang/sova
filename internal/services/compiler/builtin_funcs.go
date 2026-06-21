package compiler

import "sova/internal/ir"

const BuiltinIntrinsicsCacheKey = "builtin_intrinsics"

const ErrorTypeCacheKey = "builtin_error_typ"

func injectChannelAndErrorBuiltins(c *CompilerContext, pkg *ir.PackageContext) {
	if _, ok := pkg.Scopes.LookupOnlyCurrent(pkg.Root, "error"); ok {
		return
	}

	t := c.TypeUniverse
	intTyp := t.PrimInt()
	stringTyp := t.PrimString()
	errTyp := ensureErrorType(c)

	if _, ok := c.Cache[BuiltinIntrinsicsCacheKey]; !ok {
		c.Cache[BuiltinIntrinsicsCacheKey] = map[ir.SymID]string{}
	}

	intrinsics := c.Cache[BuiltinIntrinsicsCacheKey].(map[ir.SymID]string)

	register := func(name string, params []*ir.FuncParam, ret ir.TypID, doc string) {
		ft := t.FuncOf(params, ret)
		sym := pkg.Syms.NewSymbol(ir.SK_Function, name, pkg.Root, ft, 0)
		pkg.Syms.SetDoc(sym, doc)
		pkg.Scopes.DeclareSymbol(pkg.Root, name, sym, pkg.Syms)
		c.NameMap.Add(sym, name, name)
		intrinsics[sym] = name
	}

	mkParam := func(name string, typ ir.TypID) *ir.FuncParam {
		return &ir.FuncParam{Name: ir.NameRef{Name: name}, Type: &ir.TypeRef{Typ: typ}}
	}

	register("error", []*ir.FuncParam{mkParam("msg", stringTyp)}, errTyp,
		"Constructs a Sova `error` value carrying the given human-readable message. Used as the second element of result tuples that signal failure: `return result, error(\"file not found\")`.\n\n@param msg the failure message\n@returns the error value")
	register("after", []*ir.FuncParam{mkParam("ms", intTyp)}, t.ChanOf(t.TypNone()),
		"Returns a channel that fires once `ms` milliseconds from now. Useful for one-shot timeouts inside `select` blocks.\n\n@param ms the delay in milliseconds\n@returns a channel that yields exactly one tick")
	register("every", []*ir.FuncParam{mkParam("ms", intTyp)}, t.ChanOf(t.TypNone()),
		"Returns a channel that fires every `ms` milliseconds. Ticks continue until the channel is garbage collected.\n\n@param ms the interval in milliseconds\n@returns a channel that yields a tick on every interval")
}

func ensureErrorType(c *CompilerContext) ir.TypID {
	if cached, ok := c.Cache[ErrorTypeCacheKey]; ok {
		return cached.(ir.TypID)
	}

	t := c.TypeUniverse
	stringTyp := t.PrimString()
	fields := []ir.StructFieldInfo{
		{Name: "message", Type: stringTyp},
	}

	typ := t.StructOf("", "error", fields)
	if structTy, ok := t.GetByID(typ); ok {
		structTy.Struct.Fields = fields
	}

	c.Cache[ErrorTypeCacheKey] = typ
	return typ
}
