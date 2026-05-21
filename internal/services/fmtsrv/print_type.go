package fmtsrv

import (
	"strconv"
	"strings"

	"sova/internal/ir"
)

// printType renders a TypeRef as canonical Sova syntax. Mirrors the grammar in Sova.g4 - `int`, `[]string`, `[10]int`, `option<T>`, `map<K, V>`, `chan<T>`, `(int, bool)`, `func(a: int): bool`, generic `Pair<A, B>`, package-qualified `pkg.Type`. Used by both file-level decls (return types, field types) and tuple-field/expression annotations.
func (p *Printer) printType(t *ir.TypeRef) {
	if t == nil {
		return
	}
	p.write(formatTypeRef(t))
}

func formatTypeRef(t *ir.TypeRef) string {
	if t == nil {
		return ""
	}
	switch t.Kind {
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
		return "option<" + formatTypeRef(t.Elem) + ">"
	case ir.TK_Slice:
		return "[]" + formatTypeRef(t.Elem)
	case ir.TK_Array:
		return "[" + strconv.FormatInt(t.Dim, 10) + "]" + formatTypeRef(t.Elem)
	case ir.TK_Map:
		return "map<" + formatTypeRef(t.Key) + ", " + formatTypeRef(t.Value) + ">"
	case ir.TK_Tuple:
		parts := make([]string, len(t.Tuple))
		for i, f := range t.Tuple {
			if f.Name != "" {
				parts[i] = f.Name + ": " + formatTypeRef(f.Type)
			} else {
				parts[i] = formatTypeRef(f.Type)
			}
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case ir.TK_Chan:
		return "chan<" + formatTypeRef(t.Elem) + ">"
	case ir.TK_Function:
		parts := make([]string, len(t.FuncParams))
		for i, fp := range t.FuncParams {
			label := ""
			if fp.Name != "" {
				label = fp.Name + ": "
			}
			parts[i] = label + formatTypeRef(fp.Type)
		}
		head := "func(" + strings.Join(parts, ", ") + ")"
		if t.FuncReturn != nil {
			head += ": " + formatTypeRef(t.FuncReturn)
		}
		return head
	}
	if t.CustomName == "" {
		return "any"
	}
	name := t.CustomName
	if t.CustomQualifier != "" {
		name = t.CustomQualifier + "." + name
	}
	if len(t.TypeArgs) > 0 {
		args := make([]string, len(t.TypeArgs))
		for i, a := range t.TypeArgs {
			args[i] = formatTypeRef(a)
		}
		name += "<" + strings.Join(args, ", ") + ">"
	}
	return name
}
