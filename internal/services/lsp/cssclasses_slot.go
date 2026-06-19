package lsp

import (
	"strings"

	"sova/internal/ir"
	"sova/internal/services/compiler"
)

type callContext struct {
	callee   string
	argIndex int
	argName  string
}

func findEnclosingCallTextual(src string, offset int) (callContext, bool) {
	depthParen, depthBracket, depthBrace := 0, 0, 0
	commas := 0
	argStart := -1
	i := offset - 1
	if openQuote, ok := openingQuoteOfEnclosingString(src, offset); ok {
		i = openQuote - 1
	}

	for i >= 0 {
		c := src[i]
		switch c {
		case '"', '\'':
			i = skipStringBackward(src, i)
			continue
		case '/':
			if i > 0 && src[i-1] == '*' {
				i = skipBlockCommentBackward(src, i)
				continue
			}

		case ')':
			depthParen++
		case ']':
			depthBracket++
		case '}':
			depthBrace++
		case '(':
			if depthParen > 0 {
				depthParen--
				break
			}

			callee, ok := readCalleeBefore(src, i)
			if !ok {
				return callContext{}, false
			}

			if argStart < 0 {
				argStart = i + 1
			}

			argName := readNamedArgPrefix(src, argStart, offset)
			return callContext{callee: callee, argIndex: commas, argName: argName}, true
		case '[':
			if depthBracket > 0 {
				depthBracket--
			}

		case '{':
			if depthBrace > 0 {
				depthBrace--
			}

		case ',':
			if depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
				if argStart < 0 {
					argStart = i + 1
				}

				commas++
			}

		case ';', '\n':

			if depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
				return callContext{}, false
			}
		}

		i--
	}

	return callContext{}, false
}

func openingQuoteOfEnclosingString(src string, offset int) (int, bool) {
	lineStart := offset
	for lineStart > 0 && src[lineStart-1] != '\n' {
		lineStart--
	}

	openIdx := -1
	openQuote := byte(0)
	for i := lineStart; i < offset; i++ {
		c := src[i]
		if c == '\\' && i+1 < offset {
			i++
			continue
		}

		if openIdx < 0 {
			if c == '"' || c == '\'' {
				openIdx = i
				openQuote = c
			}
		} else if c == openQuote {
			openIdx = -1
			openQuote = 0
		}
	}

	if openIdx < 0 {
		return 0, false
	}

	return openIdx, true
}

func skipStringBackward(src string, end int) int {
	quote := src[end]
	i := end - 1
	for i >= 0 {
		if src[i] == quote {
			if i == 0 || src[i-1] != '\\' {
				return i - 1
			}
		}

		i--
	}

	return i
}

func skipBlockCommentBackward(src string, end int) int {
	i := end - 2
	for i >= 1 {
		if src[i-1] == '/' && src[i] == '*' {
			return i - 2
		}

		i--
	}

	return i
}

func cssClassSlotAt(c *compiler.CompilerContext, src string, offset int) (callContext, bool) {
	ctx, ok := findEnclosingCallTextual(src, offset)
	if !ok {
		return callContext{}, false
	}

	calleeName := unqualifiedCallee(ctx.callee)
	if calleeName == "" {
		return callContext{}, false
	}

	if fn := findFuncByName(c, calleeName); fn != nil {
		if param, ok := lookupFuncParam(fn, ctx); ok && paramHasCSSClass(param) {
			return ctx, true
		}
	}

	if td := findTypeByName(c, calleeName); td != nil {
		if field, ok := lookupTypeField(td, ctx); ok && fieldHasCSSClass(field) {
			return ctx, true
		}
	}

	return callContext{}, false
}

func lookupFuncParam(fn *ir.FuncDeclStmt, ctx callContext) (*ir.FuncParam, bool) {
	if ctx.argName != "" {
		for _, p := range fn.Params {
			if p != nil && p.Name.Name == ctx.argName {
				return p, true
			}
		}

		return nil, false
	}

	if ctx.argIndex < 0 || ctx.argIndex >= len(fn.Params) {
		return nil, false
	}

	return fn.Params[ctx.argIndex], true
}

func lookupTypeField(td *ir.TypeDeclStmt, ctx callContext) (*ir.TypeField, bool) {
	if ctx.argName != "" {
		for _, f := range td.Fields {
			if f != nil && f.Name.Name == ctx.argName {
				return f, true
			}
		}

		return nil, false
	}

	if ctx.argIndex < 0 || ctx.argIndex >= len(td.Fields) {
		return nil, false
	}

	return td.Fields[ctx.argIndex], true
}

func fieldHasCSSClass(f *ir.TypeField) bool {
	for _, a := range f.Annotations {
		if a.Name.Name == "cssClass" {
			return true
		}
	}

	return false
}

func findTypeByName(c *compiler.CompilerContext, name string) *ir.TypeDeclStmt {
	if c == nil || name == "" {
		return nil
	}

	for _, pkg := range c.Packages {
		if pkg == nil {
			continue
		}

		for _, f := range pkg.Files {
			if f == nil || f.Hir == nil {
				continue
			}

			for _, st := range f.Hir.Statements {
				td, ok := st.(*ir.TypeDeclStmt)
				if !ok {
					continue
				}

				if td.Name.Name == name {
					return td
				}
			}
		}
	}

	return nil
}

func unqualifiedCallee(callee string) string {
	if idx := strings.LastIndex(callee, "."); idx >= 0 {
		return callee[idx+1:]
	}

	return callee
}

func findFuncByName(c *compiler.CompilerContext, name string) *ir.FuncDeclStmt {
	if c == nil || name == "" {
		return nil
	}

	for _, pkg := range c.Packages {
		if pkg == nil {
			continue
		}

		for _, f := range pkg.Files {
			if f == nil || f.Hir == nil {
				continue
			}

			for _, st := range f.Hir.Statements {
				fn, ok := st.(*ir.FuncDeclStmt)
				if !ok {
					continue
				}

				if fn.Name.Name == name {
					return fn
				}
			}
		}
	}

	return nil
}

func paramHasCSSClass(p *ir.FuncParam) bool {
	for _, a := range p.Annotations {
		if a.Name.Name == "cssClass" {
			return true
		}
	}

	return false
}

func readNamedArgPrefix(src string, argStart, cursor int) string {
	i := argStart
	for i < cursor && (src[i] == ' ' || src[i] == '\t' || src[i] == '\n' || src[i] == '\r') {
		i++
	}

	nameStart := i
	for i < cursor && isIdentChar(src[i]) {
		i++
	}

	if nameStart == i {
		return ""
	}

	j := i
	for j < cursor && (src[j] == ' ' || src[j] == '\t') {
		j++
	}

	if j >= cursor || src[j] != ':' {
		return ""
	}

	return src[nameStart:i]
}

func readCalleeBefore(src string, parenIdx int) (string, bool) {
	i := parenIdx - 1
	for i >= 0 && (src[i] == ' ' || src[i] == '\t') {
		i--
	}

	end := i + 1
	for i >= 0 && (isIdentChar(src[i]) || src[i] == '.') {
		i--
	}

	start := i + 1
	if start >= end {
		return "", false
	}

	name := src[start:end]
	for _, c := range name {
		if c >= '0' && c <= '9' {
			continue
		}

		if c == '.' {
			continue
		}

		break
	}

	return name, true
}
