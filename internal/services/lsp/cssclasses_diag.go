package lsp

import (
	"strings"

	"go.lsp.dev/protocol"

	"sova/internal/ir"
	"sova/internal/services/compiler"
)

func cssClassDiagnostics(c *compiler.CompilerContext, f *ir.File) []protocol.Diagnostic {
	if f == nil {
		return nil
	}

	index := projectCSSClassIndex(c)
	if len(index) == 0 {
		return nil
	}

	known := map[string]struct{}{}

	for name := range index {
		known[name] = struct{}{}
	}

	var out []protocol.Diagnostic
	for _, st := range f.Statements {
		walkStmtForClassChecks(c, st, known, &out)
	}

	return out
}

func walkStmtForClassChecks(c *compiler.CompilerContext, s ir.Stmt, known map[string]struct{}, out *[]protocol.Diagnostic) {
	switch n := s.(type) {
	case *ir.BlockStmt:
		for _, ss := range n.Stmts {
			walkStmtForClassChecks(c, ss, known, out)
		}

	case *ir.ExprStmt:
		walkExprForClassChecks(c, n.Expr, known, out)
	case *ir.VarDeclStmt:
		walkExprForClassChecks(c, n.Init, known, out)
	case *ir.IfStmt:
		walkExprForClassChecks(c, n.Cond, known, out)
		if n.Then != nil {
			walkStmtForClassChecks(c, n.Then, known, out)
		}

		for _, eb := range n.ElseIfs {
			walkExprForClassChecks(c, eb.Cond, known, out)
			if eb.Then != nil {
				walkStmtForClassChecks(c, eb.Then, known, out)
			}
		}

		if n.Else != nil {
			walkStmtForClassChecks(c, n.Else, known, out)
		}

	case *ir.ReturnStmt:
		for _, e := range n.Results {
			walkExprForClassChecks(c, e, known, out)
		}

	case *ir.FuncDeclStmt:
		if n.Body != nil {
			walkStmtForClassChecks(c, n.Body, known, out)
		}

	case *ir.TypeDeclStmt:
		for _, m := range n.Methods {
			if m != nil && m.Func != nil && m.Func.Body != nil {
				walkStmtForClassChecks(c, m.Func.Body, known, out)
			}
		}

		for _, ct := range n.Ctors {
			if ct != nil && ct.Body != nil {
				walkStmtForClassChecks(c, ct.Body, known, out)
			}
		}
	}
}

func walkExprForClassChecks(c *compiler.CompilerContext, e ir.Expr, known map[string]struct{}, out *[]protocol.Diagnostic) {
	if e == nil {
		return
	}

	switch n := e.(type) {
	case *ir.FuncCallExpr:
		checkCallForClassArgs(c, n, known, out)
		walkExprForClassChecks(c, n.Callee, known, out)
		for _, arg := range n.Args {
			walkExprForClassChecks(c, arg.Expr, known, out)
		}

	case *ir.BinaryExpr:
		walkExprForClassChecks(c, n.Left, known, out)
		walkExprForClassChecks(c, n.Right, known, out)
	case *ir.GroupedExpr:
		walkExprForClassChecks(c, n.Expr, known, out)
	}
}

func checkCallForClassArgs(c *compiler.CompilerContext, call *ir.FuncCallExpr, known map[string]struct{}, out *[]protocol.Diagnostic) {
	calleeName := calleeNameForUnknownClass(call.Callee)
	if calleeName == "" {
		return
	}

	if fn := findFuncByName(c, calleeName); fn != nil {
		for i, arg := range call.Args {
			param := resolveFuncArgParam(fn, arg, i)
			if param == nil || !paramHasCSSClass(param) {
				continue
			}

			reportUnknownClassesInArg(arg, known, out)
		}

		return
	}

	if td := findTypeByName(c, calleeName); td != nil {
		for i, arg := range call.Args {
			field := resolveTypeArgField(td, arg, i)
			if field == nil || !fieldHasCSSClass(field) {
				continue
			}

			reportUnknownClassesInArg(arg, known, out)
		}
	}
}

func resolveFuncArgParam(fn *ir.FuncDeclStmt, arg ir.FuncCallArg, i int) *ir.FuncParam {
	if arg.Name != "" {
		for _, p := range fn.Params {
			if p != nil && p.Name.Name == arg.Name {
				return p
			}
		}

		return nil
	}

	if i >= len(fn.Params) {
		return nil
	}

	return fn.Params[i]
}

func resolveTypeArgField(td *ir.TypeDeclStmt, arg ir.FuncCallArg, i int) *ir.TypeField {
	if arg.Name != "" {
		for _, f := range td.Fields {
			if f != nil && f.Name.Name == arg.Name {
				return f
			}
		}

		return nil
	}

	if i >= len(td.Fields) {
		return nil
	}

	return td.Fields[i]
}

func reportUnknownClassesInArg(arg ir.FuncCallArg, known map[string]struct{}, out *[]protocol.Diagnostic) {
	lit, ok := arg.Expr.(*ir.LitString)
	if !ok || lit.Value == "" {
		return
	}

	litSpan := lit.Span()
	for _, token := range strings.Fields(lit.Value) {
		if _, ok := known[token]; ok {
			continue
		}

		*out = append(*out, protocol.Diagnostic{
			Severity: protocol.DiagnosticSeverityWarning,
			Source:   "sova-lsp",
			Message:  "unknown CSS class `" + token + "` - does not appear in any project stylesheet",
			Range:    spanToRange(litSpan),
		})
	}
}

func calleeNameForUnknownClass(e ir.Expr) string {
	switch n := e.(type) {
	case *ir.VarRef:
		return n.Ref.Name
	case *ir.FieldAccessExpr:
		if len(n.Fields) > 0 {
			return n.Fields[len(n.Fields)-1].Name
		}
	}

	return ""
}
