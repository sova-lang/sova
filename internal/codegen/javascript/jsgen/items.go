package jsgen

import (
	"fmt"
	"strings"
)

type identifier struct {
	name string
}

func (i identifier) render(indent int) string {
	return i.name
}

type literal struct {
	value interface{}
}

func (l literal) render(indent int) string {
	return formatLiteral(l.value)
}

type nullLiteral struct{}

func (n nullLiteral) render(indent int) string {
	return "null"
}

type operator struct {
	op string
}

func (o operator) render(indent int) string {
	switch o.op {
	case ".", "++", "--", "!", ",":
		return o.op
	default:
		return " " + o.op + " "
	}
}

type unaryOp struct {
	op string
}

func (u unaryOp) render(indent int) string {
	return u.op
}

type varDecl struct {
	kind string
	name string
}

func (v varDecl) render(indent int) string {
	return fmt.Sprintf("%s %s", v.kind, v.name)
}

type indexAccess struct {
	expr *Statement
}

func (i indexAccess) render(indent int) string {
	return fmt.Sprintf("[%s]", i.expr.render(indent))
}

type call struct {
	args []*Statement
}

func (c call) render(indent int) string {
	var args []string
	for _, arg := range c.args {
		if arg != nil {
			args = append(args, arg.render(indent))
		}
	}

	return fmt.Sprintf("(%s)", strings.Join(args, ", "))
}

type parens struct {
	inner *Statement
}

func (p parens) render(indent int) string {
	return fmt.Sprintf("(%s)", p.inner.render(indent))
}

type array struct {
	elements []*Statement
}

func (a array) render(indent int) string {
	var elems []string
	for _, elem := range a.elements {
		if elem != nil {
			elems = append(elems, elem.render(indent))
		}
	}

	return fmt.Sprintf("[%s]", strings.Join(elems, ", "))
}

type object struct {
	pairs []KeyValue
}

func (o object) render(indent int) string {
	if len(o.pairs) == 0 {
		return "{}"
	}

	var pairs []string
	for _, kv := range o.pairs {
		value := ""
		if kv.Value != nil {
			value = kv.Value.render(indent + 1)
		}

		pairs = append(pairs, fmt.Sprintf("%s: %s", kv.Key, value))
	}

	if len(pairs) <= 3 {
		return fmt.Sprintf("{ %s }", strings.Join(pairs, ", "))
	}

	var lines []string
	lines = append(lines, "{")
	for _, pair := range pairs {
		lines = append(lines, indentStr(indent+1)+pair+",")
	}

	lines = append(lines, indentStr(indent)+"}")
	return strings.Join(lines, "\n")
}

type returnStmt struct {
	expr []*Statement
}

func (r returnStmt) render(indent int) string {
	if len(r.expr) == 0 {
		return "return"
	}

	if len(r.expr) == 1 {
		return fmt.Sprintf("return %s", r.expr[0].render(indent))
	}

	var exprs []string
	for _, e := range r.expr {
		if e != nil {
			exprs = append(exprs, e.render(indent))
		}
	}

	return fmt.Sprintf("return [%s]", strings.Join(exprs, ", "))
}

type simpleStmt struct {
	text string
}

func (s simpleStmt) render(indent int) string {
	return s.text
}

type throwStmt struct {
	expr *Statement
}

func (t throwStmt) render(indent int) string {
	return fmt.Sprintf("throw %s", t.expr.render(indent))
}

type ifStmt struct {
	cond     *Statement
	thenBody []Code
	elseBody []Code
}

func (i ifStmt) render(indent int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("if (%s) {\n", i.cond.render(indent)))
	for _, stmt := range i.thenBody {
		if stmt != nil {
			sb.WriteString(indentStr(indent + 1))
			sb.WriteString(stmt.render(indent + 1))
			sb.WriteString(";\n")
		}
	}

	sb.WriteString(indentStr(indent) + "}")

	if len(i.elseBody) > 0 {
		sb.WriteString(" else {\n")
		for _, stmt := range i.elseBody {
			if stmt != nil {
				sb.WriteString(indentStr(indent + 1))
				sb.WriteString(stmt.render(indent + 1))
				sb.WriteString(";\n")
			}
		}

		sb.WriteString(indentStr(indent) + "}")
	}

	return sb.String()
}

type forStmt struct {
	init *Statement
	cond *Statement
	post *Statement
	body []Code
}

func (f forStmt) render(indent int) string {
	var sb strings.Builder

	initStr := ""
	if f.init != nil {
		initStr = f.init.render(indent)
	}

	condStr := ""
	if f.cond != nil {
		condStr = f.cond.render(indent)
	}

	postStr := ""
	if f.post != nil {
		postStr = f.post.render(indent)
	}

	if f.cond == nil && f.post == nil {
		sb.WriteString(fmt.Sprintf("for (%s) {\n", initStr))
	} else {
		sb.WriteString(fmt.Sprintf("for (%s; %s; %s) {\n", initStr, condStr, postStr))
	}

	for _, stmt := range f.body {
		if stmt != nil {
			sb.WriteString(indentStr(indent + 1))
			sb.WriteString(stmt.render(indent + 1))
			sb.WriteString(";\n")
		}
	}

	sb.WriteString(indentStr(indent) + "}")

	return sb.String()
}

type whileStmt struct {
	cond *Statement
	body []Code
}

func (w whileStmt) render(indent int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("while (%s) {\n", w.cond.render(indent)))
	for _, stmt := range w.body {
		if stmt != nil {
			sb.WriteString(indentStr(indent + 1))
			sb.WriteString(stmt.render(indent + 1))
			sb.WriteString(";\n")
		}
	}

	sb.WriteString(indentStr(indent) + "}")

	return sb.String()
}

type funcDecl struct {
	name    string
	params  []string
	body    []Code
	isAsync bool
}

func (f funcDecl) render(indent int) string {
	var sb strings.Builder

	prefix := "function"
	if f.isAsync {
		prefix = "async function"
	}

	sb.WriteString(fmt.Sprintf("%s %s(%s) {\n",
		prefix, f.name, strings.Join(f.params, ", ")))

	for _, stmt := range f.body {
		if stmt != nil {
			sb.WriteString(indentStr(indent + 1))
			sb.WriteString(stmt.render(indent + 1))
			sb.WriteString(";\n")
		}
	}

	sb.WriteString(indentStr(indent) + "}")

	return sb.String()
}

type arrowFunc struct {
	params  []string
	body    []Code
	isAsync bool
}

func (a arrowFunc) render(indent int) string {
	var sb strings.Builder

	if a.isAsync {
		sb.WriteString("async ")
	}

	if len(a.params) == 1 {
		sb.WriteString(a.params[0])
	} else {
		sb.WriteString(fmt.Sprintf("(%s)", strings.Join(a.params, ", ")))
	}

	sb.WriteString(" => ")

	if len(a.body) == 1 {
		sb.WriteString("{\n")
		sb.WriteString(indentStr(indent + 1))
		sb.WriteString(a.body[0].render(indent + 1))
		sb.WriteString(";\n")
		sb.WriteString(indentStr(indent) + "}")
	} else {
		sb.WriteString("{\n")
		for _, stmt := range a.body {
			if stmt != nil {
				sb.WriteString(indentStr(indent + 1))
				sb.WriteString(stmt.render(indent + 1))
				sb.WriteString(";\n")
			}
		}

		sb.WriteString(indentStr(indent) + "}")
	}

	return sb.String()
}

type comment struct {
	text string
}

func (c comment) render(indent int) string {
	return fmt.Sprintf("// %s", c.text)
}

type blockComment struct {
	text string
}

func (b blockComment) render(indent int) string {
	lines := strings.Split(b.text, "\n")
	if len(lines) == 1 {
		return fmt.Sprintf("/* %s */", b.text)
	}

	var sb strings.Builder
	sb.WriteString("/*\n")
	for _, line := range lines {
		sb.WriteString(indentStr(indent))
		sb.WriteString(" * ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	sb.WriteString(indentStr(indent))
	sb.WriteString(" */")
	return sb.String()
}

type rawText struct {
	text string
}

func (r rawText) render(indent int) string {
	return r.text
}
