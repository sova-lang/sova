package jsgen

import (
	"fmt"
	"strings"
)

type Statement struct {
	items []item
	pos   *SourcePosition
}

type item interface {
	render(indent int) string
}

func New() *Statement {
	return &Statement{}
}

func Null() *Statement {
	return &Statement{items: []item{nullLiteral{}}}
}

func Id(name string) *Statement {
	return &Statement{items: []item{identifier{name: name}}}
}

func Lit(v interface{}) *Statement {
	return &Statement{items: []item{literal{value: v}}}
}

func Raw(code string) *Statement {
	return &Statement{items: []item{rawText{text: code}}}
}

func (s *Statement) Raw(code string) *Statement {
	s.items = append(s.items, rawText{text: code})
	return s
}

func Op(op string) *Statement {
	return &Statement{items: []item{operator{op: op}}}
}

func Var(name string) *Statement {
	return &Statement{items: []item{varDecl{kind: "var", name: name}}}
}

func Let(name string) *Statement {
	return &Statement{items: []item{varDecl{kind: "let", name: name}}}
}

func Const(name string) *Statement {
	return &Statement{items: []item{varDecl{kind: "const", name: name}}}
}

func (s *Statement) Add(other *Statement) *Statement {
	if other == nil {
		return s
	}

	s.items = append(s.items, other.items...)
	return s
}

func (s *Statement) Op(op string) *Statement {
	s.items = append(s.items, operator{op: op})
	return s
}

func (s *Statement) Id(name string) *Statement {
	s.items = append(s.items, identifier{name: name})
	return s
}

func (s *Statement) Lit(v interface{}) *Statement {
	s.items = append(s.items, literal{value: v})
	return s
}

func (s *Statement) Dot(name string) *Statement {
	s.items = append(s.items, operator{op: "."})
	s.items = append(s.items, identifier{name: name})
	return s
}

func (s *Statement) Index(expr *Statement) *Statement {
	s.items = append(s.items, indexAccess{expr: expr})
	return s
}

func (s *Statement) Call(args ...*Statement) *Statement {
	s.items = append(s.items, call{args: args})
	return s
}

func (s *Statement) Parens() *Statement {
	return &Statement{items: []item{parens{inner: s}}}
}

func (s *Statement) Pos(line, column int, sourceFile string) *Statement {
	s.pos = &SourcePosition{
		Line:       line,
		Column:     column,
		SourceFile: sourceFile,
	}

	return s
}

func Array(elements ...*Statement) *Statement {
	return &Statement{items: []item{array{elements: elements}}}
}

func Object(pairs ...KeyValue) *Statement {
	return &Statement{items: []item{object{pairs: pairs}}}
}

type KeyValue struct {
	Key   string
	Value *Statement
}

func Kv(key string, value *Statement) KeyValue {
	return KeyValue{Key: key, Value: value}
}

func Return(expr ...*Statement) *Statement {
	return &Statement{items: []item{returnStmt{expr: expr}}}
}

func If(cond *Statement) *IfStatement {
	return &IfStatement{cond: cond}
}

type IfStatement struct {
	cond     *Statement
	thenBody []Code
	elseBody []Code
}

func (i *IfStatement) Block(body ...Code) *IfStatement {
	i.thenBody = body
	return i
}

func (i *IfStatement) Else(body ...Code) *Statement {
	i.elseBody = body
	return &Statement{items: []item{ifStmt{
		cond:     i.cond,
		thenBody: i.thenBody,
		elseBody: i.elseBody,
	}}}
}

func (i *IfStatement) ToStatement() *Statement {
	return &Statement{items: []item{ifStmt{
		cond:     i.cond,
		thenBody: i.thenBody,
	}}}
}

func For(init, cond, post *Statement) *ForStatement {
	return &ForStatement{init: init, cond: cond, post: post}
}

type ForStatement struct {
	init *Statement
	cond *Statement
	post *Statement
	body []Code
}

func (f *ForStatement) Block(body ...Code) *Statement {
	f.body = body
	return &Statement{items: []item{forStmt{
		init: f.init,
		cond: f.cond,
		post: f.post,
		body: f.body,
	}}}
}

func While(cond *Statement) *WhileStatement {
	return &WhileStatement{cond: cond}
}

type WhileStatement struct {
	cond *Statement
	body []Code
}

func (w *WhileStatement) Block(body ...Code) *Statement {
	w.body = body
	return &Statement{items: []item{whileStmt{
		cond: w.cond,
		body: w.body,
	}}}
}

func Func(name string) *FuncBuilder {
	return &FuncBuilder{name: name}
}

type FuncBuilder struct {
	name    string
	params  []string
	body    []Code
	isAsync bool
}

func (f *FuncBuilder) Async() *FuncBuilder {
	f.isAsync = true
	return f
}

func (f *FuncBuilder) Params(params ...string) *FuncBuilder {
	f.params = params
	return f
}

func (f *FuncBuilder) Block(body ...Code) *Statement {
	return &Statement{items: []item{funcDecl{
		name:    f.name,
		params:  f.params,
		body:    body,
		isAsync: f.isAsync,
	}}}
}

func Arrow(params ...string) *ArrowBuilder {
	return &ArrowBuilder{params: params}
}

type ArrowBuilder struct {
	params  []string
	body    []Code
	isAsync bool
}

func (a *ArrowBuilder) Async() *ArrowBuilder {
	a.isAsync = true
	return a
}

func (a *ArrowBuilder) Block(body ...Code) *Statement {
	return &Statement{items: []item{arrowFunc{
		params:  a.params,
		body:    body,
		isAsync: a.isAsync,
	}}}
}

func Break(label ...string) *Statement {
	if len(label) > 0 && label[0] != "" {
		return &Statement{items: []item{simpleStmt{text: "break " + label[0]}}}
	}

	return &Statement{items: []item{simpleStmt{text: "break"}}}
}

func Continue(label ...string) *Statement {
	if len(label) > 0 && label[0] != "" {
		return &Statement{items: []item{simpleStmt{text: "continue " + label[0]}}}
	}

	return &Statement{items: []item{simpleStmt{text: "continue"}}}
}

func Throw(expr *Statement) *Statement {
	return &Statement{items: []item{throwStmt{expr: expr}}}
}

type Code interface {
	render(indent int) string
}

func (s *Statement) Render() string {
	if s == nil {
		return ""
	}

	return s.render(0)
}

func (s *Statement) String() string {
	return s.Render()
}

func (s *Statement) render(indent int) string {
	var parts []string
	for _, it := range s.items {
		parts = append(parts, it.render(indent))
	}

	return strings.Join(parts, "")
}

func Comment(text string) *Statement {
	return &Statement{items: []item{comment{text: text}}}
}

func BlockComment(text string) *Statement {
	return &Statement{items: []item{blockComment{text: text}}}
}

func indentStr(level int) string {
	return strings.Repeat("\t", level)
}

func formatLiteral(v interface{}) string {
	switch val := v.(type) {
	case string:
		escaped := strings.ReplaceAll(val, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
		escaped = strings.ReplaceAll(escaped, "\n", "\\n")
		escaped = strings.ReplaceAll(escaped, "\r", "\\r")
		escaped = strings.ReplaceAll(escaped, "\t", "\\t")
		return fmt.Sprintf(`"%s"`, escaped)
	case bool:
		return fmt.Sprintf("%t", val)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", val)
	case float32, float64:
		return fmt.Sprintf("%v", val)
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", val)
	}
}
