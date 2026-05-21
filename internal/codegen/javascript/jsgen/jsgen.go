package jsgen

import (
	"fmt"
	"strings"
)

// Statement represents a JavaScript code statement that can be rendered to a string.
type Statement struct {
	items []item
	pos   *SourcePosition // Optional source position for source maps
}

// item represents a single element in a statement (identifier, operator, literal, etc.)
type item interface {
	render(indent int) string
}

// New creates a new empty statement.
func New() *Statement {
	return &Statement{}
}

// Null returns a null literal.
func Null() *Statement {
	return &Statement{items: []item{nullLiteral{}}}
}

// Id creates an identifier reference.
func Id(name string) *Statement {
	return &Statement{items: []item{identifier{name: name}}}
}

// Lit creates a literal value (auto-detects type).
func Lit(v interface{}) *Statement {
	return &Statement{items: []item{literal{value: v}}}
}

// Raw creates a raw JavaScript string (inserted as-is).
func Raw(code string) *Statement {
	return &Statement{items: []item{rawText{text: code}}}
}

// Raw adds raw text to the statement.
func (s *Statement) Raw(code string) *Statement {
	s.items = append(s.items, rawText{text: code})
	return s
}

// Op adds an operator.
func Op(op string) *Statement {
	return &Statement{items: []item{operator{op: op}}}
}

// Var creates a var declaration.
func Var(name string) *Statement {
	return &Statement{items: []item{varDecl{kind: "var", name: name}}}
}

// Let creates a let declaration.
func Let(name string) *Statement {
	return &Statement{items: []item{varDecl{kind: "let", name: name}}}
}

// Const creates a const declaration.
func Const(name string) *Statement {
	return &Statement{items: []item{varDecl{kind: "const", name: name}}}
}

// Add appends another statement's items to this statement.
func (s *Statement) Add(other *Statement) *Statement {
	if other == nil {
		return s
	}
	s.items = append(s.items, other.items...)
	return s
}

// Op adds an operator to the statement.
func (s *Statement) Op(op string) *Statement {
	s.items = append(s.items, operator{op: op})
	return s
}

// Id adds an identifier.
func (s *Statement) Id(name string) *Statement {
	s.items = append(s.items, identifier{name: name})
	return s
}

// Lit adds a literal.
func (s *Statement) Lit(v interface{}) *Statement {
	s.items = append(s.items, literal{value: v})
	return s
}

// Dot adds a dot accessor.
func (s *Statement) Dot(name string) *Statement {
	s.items = append(s.items, operator{op: "."})
	s.items = append(s.items, identifier{name: name})
	return s
}

// Index adds an array/object index access.
func (s *Statement) Index(expr *Statement) *Statement {
	s.items = append(s.items, indexAccess{expr: expr})
	return s
}

// Call adds a function call with arguments.
func (s *Statement) Call(args ...*Statement) *Statement {
	s.items = append(s.items, call{args: args})
	return s
}

// Parens wraps the statement in parentheses.
func (s *Statement) Parens() *Statement {
	return &Statement{items: []item{parens{inner: s}}}
}

// Pos sets the source position for this statement (for source maps).
// line is 1-based, column is 0-based.
func (s *Statement) Pos(line, column int, sourceFile string) *Statement {
	s.pos = &SourcePosition{
		Line:       line,
		Column:     column,
		SourceFile: sourceFile,
	}
	return s
}

// Array creates an array literal.
func Array(elements ...*Statement) *Statement {
	return &Statement{items: []item{array{elements: elements}}}
}

// Object creates an object literal.
func Object(pairs ...KeyValue) *Statement {
	return &Statement{items: []item{object{pairs: pairs}}}
}

// KeyValue represents a key-value pair for objects.
type KeyValue struct {
	Key   string
	Value *Statement
}

// Kv creates a key-value pair for object literals.
func Kv(key string, value *Statement) KeyValue {
	return KeyValue{Key: key, Value: value}
}

// Return creates a return statement.
func Return(expr ...*Statement) *Statement {
	return &Statement{items: []item{returnStmt{expr: expr}}}
}

// If creates an if statement.
func If(cond *Statement) *IfStatement {
	return &IfStatement{cond: cond}
}

// IfStatement represents an if statement with optional else.
type IfStatement struct {
	cond     *Statement
	thenBody []Code
	elseBody []Code
}

// Block sets the then body.
func (i *IfStatement) Block(body ...Code) *IfStatement {
	i.thenBody = body
	return i
}

// Else sets the else body.
func (i *IfStatement) Else(body ...Code) *Statement {
	i.elseBody = body
	return &Statement{items: []item{ifStmt{
		cond:     i.cond,
		thenBody: i.thenBody,
		elseBody: i.elseBody,
	}}}
}

// ToStatement converts to a statement (without else).
func (i *IfStatement) ToStatement() *Statement {
	return &Statement{items: []item{ifStmt{
		cond:     i.cond,
		thenBody: i.thenBody,
	}}}
}

// For creates a for loop.
func For(init, cond, post *Statement) *ForStatement {
	return &ForStatement{init: init, cond: cond, post: post}
}

// ForStatement represents a for loop.
type ForStatement struct {
	init *Statement
	cond *Statement
	post *Statement
	body []Code
}

// Block sets the loop body.
func (f *ForStatement) Block(body ...Code) *Statement {
	f.body = body
	return &Statement{items: []item{forStmt{
		init: f.init,
		cond: f.cond,
		post: f.post,
		body: f.body,
	}}}
}

// While creates a while loop.
func While(cond *Statement) *WhileStatement {
	return &WhileStatement{cond: cond}
}

// WhileStatement represents a while loop.
type WhileStatement struct {
	cond *Statement
	body []Code
}

// Block sets the loop body.
func (w *WhileStatement) Block(body ...Code) *Statement {
	w.body = body
	return &Statement{items: []item{whileStmt{
		cond: w.cond,
		body: w.body,
	}}}
}

// Func creates a function declaration.
func Func(name string) *FuncBuilder {
	return &FuncBuilder{name: name}
}

// FuncBuilder builds function declarations.
type FuncBuilder struct {
	name    string
	params  []string
	body    []Code
	isAsync bool
}

// Async marks the function as async, causing the renderer to prefix `async ` to the declaration.
func (f *FuncBuilder) Async() *FuncBuilder {
	f.isAsync = true
	return f
}

// Params sets the function parameters.
func (f *FuncBuilder) Params(params ...string) *FuncBuilder {
	f.params = params
	return f
}

// Block sets the function body.
func (f *FuncBuilder) Block(body ...Code) *Statement {
	return &Statement{items: []item{funcDecl{
		name:    f.name,
		params:  f.params,
		body:    body,
		isAsync: f.isAsync,
	}}}
}

// Arrow creates an arrow function expression.
func Arrow(params ...string) *ArrowBuilder {
	return &ArrowBuilder{params: params}
}

// ArrowBuilder builds arrow functions.
type ArrowBuilder struct {
	params  []string
	body    []Code
	isAsync bool
}

// Async marks the arrow function as `async`, so the renderer prefixes
// `async ` to the expression. Used for closures whose body contains awaited
// calls - propagate_async sets ir.FuncLitExpr.IsAsync and the JS emitter
// threads that through here.
func (a *ArrowBuilder) Async() *ArrowBuilder {
	a.isAsync = true
	return a
}

// Block sets the arrow function body.
func (a *ArrowBuilder) Block(body ...Code) *Statement {
	return &Statement{items: []item{arrowFunc{
		params:  a.params,
		body:    body,
		isAsync: a.isAsync,
	}}}
}

// Break creates a break statement. Optionally accepts a label for labeled breaks.
func Break(label ...string) *Statement {
	if len(label) > 0 && label[0] != "" {
		return &Statement{items: []item{simpleStmt{text: "break " + label[0]}}}
	}
	return &Statement{items: []item{simpleStmt{text: "break"}}}
}

// Continue creates a continue statement. Optionally accepts a label for labeled continues.
func Continue(label ...string) *Statement {
	if len(label) > 0 && label[0] != "" {
		return &Statement{items: []item{simpleStmt{text: "continue " + label[0]}}}
	}
	return &Statement{items: []item{simpleStmt{text: "continue"}}}
}

// Throw creates a throw statement.
func Throw(expr *Statement) *Statement {
	return &Statement{items: []item{throwStmt{expr: expr}}}
}

// Code represents anything that can be rendered as JavaScript code.
type Code interface {
	render(indent int) string
}

// Render converts the statement to a JavaScript string.
func (s *Statement) Render() string {
	if s == nil {
		return ""
	}
	return s.render(0)
}

// String implements the Stringer interface for Statement.
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

// Comment adds a single-line comment.
func Comment(text string) *Statement {
	return &Statement{items: []item{comment{text: text}}}
}

// BlockComment adds a multi-line comment.
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
