package fmtsrv

import (
	"strconv"
	"strings"

	"sova/internal/ir"
)

// printExpr emits a single expression. Operator-precedence is preserved via the `GroupedExpr` wrappers the parser already produces - we don't try to reconstruct minimal parens. Whatever the parser nested inside `( ... )` gets re-emitted with parens; everything else flows flat.
func (p *Printer) printExpr(e ir.Expr) {
	if e == nil {
		return
	}
	switch n := e.(type) {
	case *ir.LitInt:
		p.write(strconv.FormatInt(n.Value, 10))
	case *ir.LitFloat:
		p.write(strconv.FormatFloat(n.Value, 'g', -1, 64))
	case *ir.LitString:
		p.write(quoteString(n.Value))
	case *ir.LitChar:
		p.write("'" + escapeRune(n.Value) + "'")
	case *ir.LitBool:
		if n.Value {
			p.write("true")
		} else {
			p.write("false")
		}
	case *ir.LitNone:
		p.write("none")
	case *ir.VarRef:
		if n.Ref.Qualifier != "" {
			p.write(n.Ref.Qualifier + "." + n.Ref.Name)
			return
		}
		p.write(n.Ref.Name)
	case *ir.FieldAccessExpr:
		p.printExpr(n.Expr)
		for _, f := range n.Fields {
			p.write(".")
			p.write(f.Name)
		}
	case *ir.IndexExpr:
		p.printExpr(n.Expr)
		p.write("[")
		p.printExpr(n.Index)
		p.write("]")
	case *ir.GroupedExpr:
		p.write("(")
		p.printExpr(n.Expr)
		p.write(")")
	case *ir.BinaryExpr:
		p.printExpr(n.Left)
		p.write(" ")
		p.write(string(n.Op))
		p.write(" ")
		p.printExpr(n.Right)
	case *ir.UnaryExpr:
		p.write(string(n.Op))
		p.printExpr(n.Expr)
	case *ir.PrefixUnaryExpr:
		p.write(string(n.Op))
		p.printExpr(n.Expr)
	case *ir.PostfixUnaryExpr:
		p.printExpr(n.Expr)
		p.write(string(n.Op))
	case *ir.AssignmentExpr:
		p.write(n.Left.Name)
		p.write(" ")
		p.write(string(n.Op))
		p.write(" ")
		p.printExpr(n.Right)
	case *ir.TenaryExpr:
		p.printExpr(n.Cond)
		p.write(" ? ")
		p.printExpr(n.Then)
		p.write(" : ")
		p.printExpr(n.Else)
	case *ir.CoalesceExpr:
		p.printExpr(n.Left)
		p.write(" ?? ")
		p.printExpr(n.Default)
	case *ir.RangeExpr:
		p.printExpr(n.Start)
		p.write("..")
		p.printExpr(n.End)
		if n.Inc != nil {
			p.write(" step ")
			p.printExpr(n.Inc)
		}
	case *ir.FuncCallExpr:
		p.printExpr(n.Callee)
		if len(n.TypeArgs) > 0 {
			parts := make([]string, len(n.TypeArgs))
			for i, t := range n.TypeArgs {
				parts[i] = formatTypeRef(t)
			}
			p.write("<" + strings.Join(parts, ", ") + ">")
		}
		p.write("(")
		for i, a := range n.Args {
			if i > 0 {
				p.write(", ")
			}
			if a.Name != "" {
				p.write(a.Name + ": ")
			}
			p.printExpr(a.Expr)
		}
		p.write(")")
	case *ir.NewExpr:
		p.write("new ")
		if n.Qualifier != "" {
			p.write(n.Qualifier + ".")
		}
		p.write(n.TypeName.Name)
		p.write("(")
		for i, a := range n.Args {
			if i > 0 {
				p.write(", ")
			}
			if a.Name != "" {
				p.write(a.Name + ": ")
			}
			p.printExpr(a.Expr)
		}
		p.write(")")
	case *ir.ChanInitExpr:
		p.write("chan<" + formatTypeRef(n.ElemType) + ">(")
		if n.Capacity != nil {
			p.printExpr(n.Capacity)
		}
		p.write(")")
	case *ir.FuncLitExpr:
		p.write("func(")
		for i, param := range n.Params {
			if i > 0 {
				p.write(", ")
			}
			p.write(param.Name.Name)
			if param.Type != nil {
				p.write(": ")
				p.printType(param.Type)
			}
			if param.Default != nil {
				p.write(" = ")
				p.printExpr(param.Default)
			}
		}
		p.write(")")
		if n.ReturnType != nil {
			p.write(": ")
			p.printType(n.ReturnType)
		}
		p.write(" ")
		p.printBlock(n.Body)
	case *ir.ArrayLiteral:
		p.write("[")
		for i, el := range n.Elems {
			if i > 0 {
				p.write(", ")
			}
			p.printExpr(el)
		}
		p.write("]")
	case *ir.MapLiteral:
		if len(n.Entries) == 0 {
			p.write("{}")
			return
		}
		p.write("{")
		for i, kv := range n.Entries {
			if i > 0 {
				p.write(", ")
			}
			p.printExpr(kv.Key)
			p.write(": ")
			p.printExpr(kv.Value)
		}
		p.write("}")
	case *ir.TupleLiteral:
		p.write("(")
		for i, el := range n.Elems {
			if i > 0 {
				p.write(", ")
			}
			p.printExpr(el)
		}
		p.write(")")
	case *ir.StringTemplateExpr:
		p.write("`")
		for _, part := range n.Parts {
			if part.Expr != nil {
				p.write("${")
				p.printExpr(part.Expr)
				p.write("}")
			} else {
				p.write(part.Lit)
			}
		}
		p.write("`")
	case *ir.WhenExpr:
		p.write("when ")
		p.printExpr(n.Expr)
		p.write(" {")
		p.writeNewline()
		p.withIndent(func() {
			for _, c := range n.Cases {
				for i, v := range c.Values {
					if i > 0 {
						p.write(", ")
					}
					p.printExpr(v)
				}
				p.write(" => ")
				p.printExpr(c.Then)
				p.writeNewline()
			}
			if n.Default != nil {
				p.write("else => ")
				p.printExpr(n.Default)
				p.writeNewline()
			}
		})
		p.write("}")
	case *ir.SessionExpr:
		p.write("@")
	}
}

// quoteString re-emits a string literal as a `"..."` form, escaping the bytes that need escaping for round-trip safety.
func quoteString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func escapeRune(r rune) string {
	switch r {
	case '\'':
		return `\'`
	case '\\':
		return `\\`
	case '\n':
		return `\n`
	case '\t':
		return `\t`
	case '\r':
		return `\r`
	}
	return string(r)
}
