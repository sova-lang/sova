package passes

import "sova/internal/ir"

// PassMangle is a pass that mangles names in the code to avoid naming conflicts.
type PassMangle struct{}

func (p *PassMangle) Name() string       { return "mangle" }
func (p *PassMangle) Scope() PassScope   { return PerPackage } // oder build-weit – dann pm.Run über alle pkgs zusammen aggregieren
func (p *PassMangle) Requires() []string { return []string{"infer_types"} }
func (p *PassMangle) NoErrors() bool     { return true }

func (p *PassMangle) Run(pc *PassContext) error {
	type ownerKey struct {
		owner ir.ScopeID
		name  string
	}
	methodSeen := map[ownerKey]int{}
	for id, sym := range pc.Pkg.Syms.ByID() {
		var mangledName string
		if sym.Kind == ir.SK_Function {
			if sym.Flags&ir.SF_TypeMethod != 0 {
				base := sanitizeMethodName(sym.Name)
				key := ownerKey{owner: sym.Owner, name: base}
				idx := methodSeen[key]
				methodSeen[key] = idx + 1
				if idx == 0 {
					mangledName = base
				} else {
					mangledName = base + "__o" + suffixForInt(idx)
				}
			} else {
				mangledName = p.mangleFunctionName(pc, sym)
			}
		} else {
			mangledName = pc.Names.RandName(p.getManglePrefix(sym.Kind))
		}
		pc.Names.Add(id, sym.Name, mangledName)
	}
	return nil
}

func suffixForInt(n int) string {
	if n == 0 {
		return "0"
	}
	out := ""
	for n > 0 {
		out = string('0'+byte(n%10)) + out
		n /= 10
	}
	return out
}

func sanitizeMethodName(n string) string {
	switch n {
	case "op+":
		return "opPlus"
	case "op-":
		return "opMinus"
	case "op*":
		return "opMul"
	case "op/":
		return "opDiv"
	case "op%":
		return "opMod"
	case "op==":
		return "opEq"
	}
	return n
}

func (p *PassMangle) getManglePrefix(kind ir.SymbolKind) string {
	switch kind {
	case ir.SK_Variable:
		return "v"
	case ir.SK_Function:
		return "fn"
	default:
		return "unk"
	}
}

// mangleFunctionName generates a unique mangled name for a function based on its signature.
// This ensures that overloaded functions get distinct names in the generated code.
func (p *PassMangle) mangleFunctionName(pc *PassContext, sym *ir.Symbol) string {
	funcType, ok := pc.Pkg.Types.GetByID(sym.Typ)
	if !ok || funcType.Kind != ir.TK_Function {
		// Fallback to random name if type lookup fails
		return pc.Names.RandName("fn")
	}

	sigSuffix := ""
	for i, param := range funcType.ParamTypes {
		if i > 0 {
			sigSuffix += "_"
		}

		paramTypeName := p.getTypeName(pc, param.Type.Typ)
		sigSuffix += paramTypeName
	}

	baseName := pc.Names.RandName("fn")

	if sigSuffix != "" {
		return baseName + "_" + sigSuffix
	}
	return baseName
}

// getTypeName returns a short string representation of a type for mangling purposes.
func (p *PassMangle) getTypeName(pc *PassContext, typID ir.TypID) string {
	typ, ok := pc.Pkg.Types.GetByID(typID)
	if !ok {
		return "unk"
	}

	switch typ.Kind {
	case ir.TK_PrimitiveInt:
		return "i"
	case ir.TK_PrimitiveFloat:
		return "f"
	case ir.TK_PrimitiveString:
		return "s"
	case ir.TK_PrimitiveBool:
		return "b"
	case ir.TK_PrimitiveChar:
		return "c"
	case ir.TK_PrimitiveAny:
		return "a"
	case ir.TK_Array:
		elemName := p.getTypeName(pc, typ.ElemType)
		return "arr" + elemName
	case ir.TK_Slice:
		elemName := p.getTypeName(pc, typ.ElemType)
		return "sl" + elemName
	case ir.TK_Map:
		keyName := p.getTypeName(pc, typ.KeyType)
		valName := p.getTypeName(pc, typ.ValueType)
		return "map" + keyName + valName
	case ir.TK_Tuple:
		return "tup"
	case ir.TK_Function:
		return "fn"
	case ir.TK_Option:
		elemName := p.getTypeName(pc, typ.ElemType)
		return "opt" + elemName
	default:
		return "unk"
	}
}
