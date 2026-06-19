package lsp

import (
	"context"

	"go.lsp.dev/protocol"

	"sova/internal/diag"
	"sova/internal/ir"
)

func (s *Server) DocumentSymbol(ctx context.Context, params *protocol.DocumentSymbolParams) ([]interface{}, error) {
	snap := s.session.Snapshot()
	if snap == nil {
		return nil, nil
	}

	c, _, err := snap.Compile(s.compileSnapshot)
	if err != nil || c == nil {
		return nil, nil
	}

	_, file, _ := lookupFileByURI(c, params.TextDocument.URI)
	if file == nil {
		return nil, nil
	}

	syms := buildDocumentSymbols(c.TypeUniverse, file)
	out := make([]interface{}, 0, len(syms))
	for _, sym := range syms {
		out = append(out, sym)
	}

	return out, nil
}

func buildDocumentSymbols(tt *ir.TypeTable, f *ir.File) []protocol.DocumentSymbol {
	var out []protocol.DocumentSymbol
	for _, st := range f.Statements {
		switch n := st.(type) {
		case *ir.FuncDeclStmt:
			out = append(out, funcDocSymbol(tt, n, false))
		case *ir.VarDeclStmt:
			for _, tgt := range n.Targets {
				if tgt.Name == nil {
					continue
				}

				kind := protocol.SymbolKindVariable
				if n.IsConst {
					kind = protocol.SymbolKindConstant
				}

				detail := ""
				if tgt.TypeAnn != nil && tgt.TypeAnn.Typ != 0 {
					detail = formatType(tt, tgt.TypeAnn.Typ)
				}

				out = append(out, protocol.DocumentSymbol{
					Name:           tgt.Name.Name,
					Detail:         detail,
					Kind:           kind,
					Range:          spanToRange(n.Span()),
					SelectionRange: spanToRange(tgt.Name.Span),
				})
			}

		case *ir.TypeDeclStmt:
			children := make([]protocol.DocumentSymbol, 0, len(n.Methods)+len(n.Ctors)+len(n.Fields))
			for _, fld := range n.Fields {
				typeStr := ""
				if fld.Type != nil && fld.Type.Typ != 0 {
					typeStr = formatType(tt, fld.Type.Typ)
				}

				children = append(children, protocol.DocumentSymbol{
					Name:           fld.Name.Name,
					Detail:         typeStr,
					Kind:           protocol.SymbolKindField,
					Range:          spanToRange(fld.Span()),
					SelectionRange: spanToRange(fld.Name.Span),
				})
			}

			for _, ctor := range n.Ctors {
				children = append(children, protocol.DocumentSymbol{
					Name:           "new",
					Detail:         formatFuncParams(tt, ctor.Params),
					Kind:           protocol.SymbolKindConstructor,
					Range:          spanToRange(ctor.Span()),
					SelectionRange: spanToRange(ctor.Span()),
				})
			}

			for _, m := range n.Methods {
				children = append(children, funcDocSymbol(tt, m.Func, true))
			}

			out = append(out, protocol.DocumentSymbol{
				Name:           n.Name.Name,
				Kind:           protocol.SymbolKindClass,
				Range:          spanToRange(n.Span()),
				SelectionRange: spanToRange(n.Name.Span),
				Children:       children,
			})
		case *ir.EnumDeclStmt:
			out = append(out, protocol.DocumentSymbol{
				Name:           n.Name.Name,
				Kind:           protocol.SymbolKindEnum,
				Range:          spanToRange(n.Span()),
				SelectionRange: spanToRange(n.Name.Span),
			})
		case *ir.InterfaceDeclStmt:
			out = append(out, protocol.DocumentSymbol{
				Name:           n.Name.Name,
				Kind:           protocol.SymbolKindInterface,
				Range:          spanToRange(n.Span()),
				SelectionRange: spanToRange(n.Name.Span),
			})
		case *ir.TestDeclStmt:
			out = append(out, protocol.DocumentSymbol{
				Name:           "test \"" + n.Name + "\"",
				Kind:           protocol.SymbolKindEvent,
				Range:          spanToRange(n.Span()),
				SelectionRange: spanToRange(n.Span()),
			})
		}
	}

	return out
}

func funcDocSymbol(tt *ir.TypeTable, fn *ir.FuncDeclStmt, isMethod bool) protocol.DocumentSymbol {
	kind := protocol.SymbolKindFunction
	if isMethod {
		kind = protocol.SymbolKindMethod
	}

	return protocol.DocumentSymbol{
		Name:           fn.Name.Name,
		Detail:         formatFuncSignature(tt, fn),
		Kind:           kind,
		Range:          spanToRange(fn.Span()),
		SelectionRange: spanToRange(fn.Name.Span),
	}
}

func formatFuncSignature(tt *ir.TypeTable, fn *ir.FuncDeclStmt) string {
	sig := formatFuncParams(tt, fn.Params)
	if fn.ReturnType != nil && fn.ReturnType.Typ != 0 {
		sig += ": " + formatType(tt, fn.ReturnType.Typ)
	}

	return sig
}

func formatFuncParams(tt *ir.TypeTable, params []*ir.FuncParam) string {
	out := "("
	for i, p := range params {
		if i > 0 {
			out += ", "
		}

		out += p.Name.Name
		if p.Type != nil && p.Type.Typ != 0 {
			out += ": " + formatType(tt, p.Type.Typ)
		}
	}

	out += ")"
	return out
}

func pinDeclSpan(s diag.TextSpan, fallback diag.TextSpan) diag.TextSpan {
	if s.StartLn == 0 && s.EndLn == 0 {
		return fallback
	}

	return s
}
