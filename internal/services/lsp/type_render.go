package lsp

import (
	"strings"

	"sova/internal/ir"
)

func formatType(tt *ir.TypeTable, id ir.TypID) string {
	return formatTypeInner(tt, id, map[ir.TypID]bool{})
}

func formatTypeInner(tt *ir.TypeTable, id ir.TypID, seen map[ir.TypID]bool) string {
	if id == 0 {
		return "<unknown>"
	}

	if id == tt.TypError() {
		return "<unresolved>"
	}

	if seen[id] {
		return "..."
	}

	seen[id] = true
	defer delete(seen, id)

	ty, ok := tt.GetByID(id)
	if !ok {
		return "<unknown>"
	}

	switch ty.Kind {
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
	case ir.TK_Option:
		return "option<" + formatTypeInner(tt, ty.ElemType, seen) + ">"
	case ir.TK_Slice:
		return "[]" + formatTypeInner(tt, ty.ElemType, seen)
	case ir.TK_Array:
		return "[" + intToString(ty.Dim) + "]" + formatTypeInner(tt, ty.ElemType, seen)
	case ir.TK_Map:
		return "map<" + formatTypeInner(tt, ty.KeyType, seen) + ", " + formatTypeInner(tt, ty.ValueType, seen) + ">"
	case ir.TK_Tuple:
		var parts []string
		for _, f := range ty.Fields {
			if f.Name != "" {
				parts = append(parts, f.Name+": "+formatTypeInner(tt, f.Type, seen))
			} else {
				parts = append(parts, formatTypeInner(tt, f.Type, seen))
			}
		}

		return "(" + strings.Join(parts, ", ") + ")"
	case ir.TK_Chan:
		return "chan<" + formatTypeInner(tt, ty.ElemType, seen) + ">"
	case ir.TK_Function:
		var parts []string
		for _, p := range ty.ParamTypes {
			label := ""
			if p.Name.Name != "" {
				label = p.Name.Name + ": "
			}

			parts = append(parts, label+formatTypeInner(tt, p.Type.Typ, seen))
		}

		prefix := "func"
		if ty.IsAsync {
			prefix = "async func"
		}

		head := prefix + "(" + strings.Join(parts, ", ") + ")"
		if ty.ReturnType == 0 || ty.ReturnType == tt.TypNone() {
			return head
		}

		return head + ": " + formatTypeInner(tt, ty.ReturnType, seen)
	case ir.TK_Struct:
		return qualifyName(ty.PackagePath, ty.StructName)
	case ir.TK_Enum:
		return qualifyName(ty.PackagePath, ty.EnumName)
	case ir.TK_Interface:
		return qualifyName(ty.PackagePath, ty.InterfaceName)
	}

	if key := string(ty.Key); key != "" && !strings.HasPrefix(key, "!") {
		return key
	}

	return "<unresolved>"
}

func qualifyName(pkgPath, name string) string {
	if pkgPath == "" {
		return name
	}

	alias := pkgPath
	if idx := strings.LastIndex(alias, "/"); idx >= 0 {
		alias = alias[idx+1:]
	}

	if alias == "" {
		return name
	}

	return alias + "." + name
}

func intToString(n int64) string {
	if n == 0 {
		return "0"
	}

	neg := false
	if n < 0 {
		neg = true
		n = -n
	}

	var b strings.Builder
	digits := make([]byte, 0, 20)
	for n > 0 {
		digits = append(digits, byte('0'+n%10))
		n /= 10
	}

	for i := len(digits) - 1; i >= 0; i-- {
		b.WriteByte(digits[i])
	}

	if neg {
		return "-" + b.String()
	}

	return b.String()
}
