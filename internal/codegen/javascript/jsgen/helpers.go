package jsgen

func List(items ...*Statement) *Statement {
	s := New()
	for i, item := range items {
		if i > 0 {
			s.items = append(s.items, operator{op: ","})
		}

		if item != nil {
			s.items = append(s.items, item.items...)
		}
	}

	return s
}

func Block(statements ...Code) []Code {
	return statements
}

func (s *Statement) Assert(typeName string) *Statement {

	return s
}

type File struct {
	statements       []*Statement
	sourceMapBuilder *SourceMapBuilder
	outputFileName   string
}

func NewFile() *File {
	return &File{}
}

func (f *File) EnableSourceMap(outputFileName string) *File {
	f.outputFileName = outputFileName
	f.sourceMapBuilder = NewSourceMapBuilder(outputFileName)
	return f
}

func (f *File) AddSourceContent(sourceFile string, content string) *File {
	if f.sourceMapBuilder != nil {
		f.sourceMapBuilder.AddSourceContent(sourceFile, content)
	}

	return f
}

func (f *File) Add(stmt *Statement) *File {
	f.statements = append(f.statements, stmt)
	return f
}

func (f *File) Render() string {
	code, _ := f.RenderWithSourceMap()
	return code
}

func (f *File) RenderWithSourceMap() (string, *SourceMap) {
	var lines []string

	for _, stmt := range f.statements {
		if stmt != nil {

			if f.sourceMapBuilder != nil && stmt.pos != nil {
				f.sourceMapBuilder.AddMapping(
					stmt.pos.SourceFile,
					stmt.pos.Line,
					stmt.pos.Column,
				)
			}

			rendered := stmt.Render()
			if needsSemicolon(stmt) {
				rendered += ";"
			}

			lines = append(lines, rendered)

			if f.sourceMapBuilder != nil {
				f.sourceMapBuilder.AdvanceGeneratedPosition(rendered)
				if needsSemicolon(stmt) {
					f.sourceMapBuilder.AdvanceGeneratedPosition(";")
				}

				f.sourceMapBuilder.AdvanceGeneratedPosition("\n")
			}
		}
	}

	code := joinLines(lines)

	if f.sourceMapBuilder != nil {
		sourceMap := f.sourceMapBuilder.Build()
		code += "\n//# sourceMappingURL=" + f.outputFileName + ".map"
		return code, sourceMap
	}

	return code, nil
}

func needsSemicolon(stmt *Statement) bool {
	if len(stmt.items) == 0 {
		return false
	}

	lastItem := stmt.items[len(stmt.items)-1]
	switch t := lastItem.(type) {
	case funcDecl, ifStmt, forStmt, whileStmt, comment, blockComment:
		return false
	case rawText:

		text := t.text
		for len(text) > 0 && (text[len(text)-1] == ' ' || text[len(text)-1] == '\t' || text[len(text)-1] == '\n') {
			text = text[:len(text)-1]
		}

		if len(text) == 0 {
			return false
		}

		lastChar := text[len(text)-1]
		switch lastChar {
		case '{', '}', ';', ':', ',':
			return false
		}

		return true
	default:
		return true
	}
}

func joinLines(lines []string) string {
	result := ""
	for i, line := range lines {
		result += line
		if i < len(lines)-1 {
			result += "\n"
		}
	}

	return result
}

func Binary(left *Statement, op string, right *Statement) *Statement {
	return left.Op(op).Add(right)
}

func Unary(op string, expr *Statement) *Statement {
	s := &Statement{items: []item{unaryOp{op: op}}}

	return s.Add(expr)
}

func ConsoleLog(args ...*Statement) *Statement {
	return Id("console").Dot("log").Call(args...)
}

func New_(constructor string, args ...*Statement) *Statement {
	s := New()
	s.items = append(s.items, simpleStmt{text: "new "})
	s.items = append(s.items, identifier{name: constructor})
	s.items = append(s.items, call{args: args})
	return s
}

func Typeof(expr *Statement) *Statement {
	s := New()
	s.items = append(s.items, simpleStmt{text: "typeof "})
	s.items = append(s.items, expr.items...)
	return s
}

func DestructArray(kind string, names []string, expr *Statement) *Statement {
	s := New()
	s.items = append(s.items, simpleStmt{text: kind + " "})
	s.items = append(s.items, simpleStmt{text: "["})

	for i, name := range names {
		if i > 0 {
			s.items = append(s.items, simpleStmt{text: ","})
		}

		if name == "_" {
			s.items = append(s.items, identifier{name: "_"})
		} else {
			s.items = append(s.items, identifier{name: name})
		}
	}

	s.items = append(s.items, simpleStmt{text: "]"})
	s.items = append(s.items, operator{op: "="})
	s.items = append(s.items, expr.items...)

	return s
}

func DestructAssign(names []string, expr *Statement) *Statement {
	s := New()
	s.items = append(s.items, simpleStmt{text: ";["})

	for i, name := range names {
		if i > 0 {
			s.items = append(s.items, simpleStmt{text: ","})
		}

		if name != "" {
			s.items = append(s.items, identifier{name: name})
		}
	}

	s.items = append(s.items, simpleStmt{text: "]"})
	s.items = append(s.items, operator{op: "="})
	s.items = append(s.items, expr.items...)

	return s
}

func DestructObject(kind string, names []string, expr *Statement) *Statement {
	s := New()
	s.items = append(s.items, simpleStmt{text: kind + " "})
	s.items = append(s.items, simpleStmt{text: "{"})

	for i, name := range names {
		if i > 0 {
			s.items = append(s.items, operator{op: ","})
		}

		s.items = append(s.items, identifier{name: name})
	}

	s.items = append(s.items, simpleStmt{text: "}"})
	s.items = append(s.items, operator{op: "="})
	s.items = append(s.items, expr.items...)

	return s
}

func Template(parts ...interface{}) *Statement {
	s := New()
	s.items = append(s.items, templateLiteral{parts: parts})
	return s
}

type templateLiteral struct {
	parts []interface{}
}

func (t templateLiteral) render(indent int) string {
	result := "`"
	for _, part := range t.parts {
		switch p := part.(type) {
		case string:

			escaped := p
			escaped = escapeTemplateString(escaped)
			result += escaped
		case *Statement:
			result += "${" + p.render(indent) + "}"
		}
	}

	result += "`"
	return result
}

func escapeTemplateString(s string) string {
	s = replaceAll(s, "\\", "\\\\")
	s = replaceAll(s, "`", "\\`")
	s = replaceAll(s, "${", "\\${")
	return s
}

func replaceAll(s, old, new string) string {
	result := ""
	for {
		idx := indexOf(s, old)
		if idx == -1 {
			result += s
			break
		}

		result += s[:idx] + new
		s = s[idx+len(old):]
	}

	return result
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}

	return -1
}

func Switch(expr *Statement) *SwitchBuilder {
	return &SwitchBuilder{expr: expr}
}

type SwitchBuilder struct {
	expr  *Statement
	cases []switchCase
}

type switchCase struct {
	value *Statement
	body  []Code
}

func (s *SwitchBuilder) Case(value *Statement, body ...Code) *SwitchBuilder {
	s.cases = append(s.cases, switchCase{value: value, body: body})
	return s
}

func (s *SwitchBuilder) Default(body ...Code) *Statement {
	return &Statement{items: []item{switchStmt{
		expr:        s.expr,
		cases:       s.cases,
		defaultCase: body,
	}}}
}

func (s *SwitchBuilder) ToStatement() *Statement {
	return &Statement{items: []item{switchStmt{
		expr:  s.expr,
		cases: s.cases,
	}}}
}

type switchStmt struct {
	expr        *Statement
	cases       []switchCase
	defaultCase []Code
}

func (s switchStmt) render(indent int) string {
	result := "switch (" + s.expr.render(indent) + ") {\n"

	for _, c := range s.cases {
		result += indentStr(indent) + "case " + c.value.render(indent) + ":\n"
		for _, stmt := range c.body {
			if stmt != nil {
				result += indentStr(indent+1) + stmt.render(indent+1) + ";\n"
			}
		}
	}

	if len(s.defaultCase) > 0 {
		result += indentStr(indent) + "default:\n"
		for _, stmt := range s.defaultCase {
			if stmt != nil {
				result += indentStr(indent+1) + stmt.render(indent+1) + ";\n"
			}
		}
	}

	result += indentStr(indent-1) + "}"
	return result
}

func Try(body ...Code) *TryBuilder {
	return &TryBuilder{tryBody: body}
}

type TryBuilder struct {
	tryBody     []Code
	catchParam  string
	catchBody   []Code
	finallyBody []Code
}

func (t *TryBuilder) Catch(param string, body ...Code) *TryBuilder {
	t.catchParam = param
	t.catchBody = body
	return t
}

func (t *TryBuilder) Finally(body ...Code) *Statement {
	t.finallyBody = body
	return &Statement{items: []item{tryStmt{
		tryBody:     t.tryBody,
		catchParam:  t.catchParam,
		catchBody:   t.catchBody,
		finallyBody: t.finallyBody,
	}}}
}

func (t *TryBuilder) ToStatement() *Statement {
	return &Statement{items: []item{tryStmt{
		tryBody:    t.tryBody,
		catchParam: t.catchParam,
		catchBody:  t.catchBody,
	}}}
}

type tryStmt struct {
	tryBody     []Code
	catchParam  string
	catchBody   []Code
	finallyBody []Code
}

func (t tryStmt) render(indent int) string {
	result := "try {\n"
	for _, stmt := range t.tryBody {
		if stmt != nil {
			result += indentStr(indent+1) + stmt.render(indent+1) + ";\n"
		}
	}

	result += indentStr(indent) + "}"

	if len(t.catchBody) > 0 {
		result += " catch (" + t.catchParam + ") {\n"
		for _, stmt := range t.catchBody {
			if stmt != nil {
				result += indentStr(indent+1) + stmt.render(indent+1) + ";\n"
			}
		}

		result += indentStr(indent) + "}"
	}

	if len(t.finallyBody) > 0 {
		result += " finally {\n"
		for _, stmt := range t.finallyBody {
			if stmt != nil {
				result += indentStr(indent+1) + stmt.render(indent+1) + ";\n"
			}
		}

		result += indentStr(indent) + "}"
	}

	return result
}
