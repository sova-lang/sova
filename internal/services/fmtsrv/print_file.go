package fmtsrv

import (
	"fmt"

	"github.com/antlr4-go/antlr/v4"

	"sova/internal/diag"
	"sova/internal/ir"
	"sova/internal/parser"
)

func Source(src string) (string, error) {
	bag := diag.NewBag()
	is := antlr.NewInputStream(src)
	lexer := parser.NewSovaLexer(is)
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(diag.NewAntlrErrorListener("<fmt>", bag))
	tokens := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	psr := parser.NewSovaParser(tokens)
	psr.RemoveErrorListeners()
	psr.AddErrorListener(diag.NewAntlrErrorListener("<fmt>", bag))
	nodeAlloc := ir.NewIdAlloc()
	visitor := ir.NewVisitor("<fmt>", nodeAlloc, bag)
	hir, ok := visitor.Visit(psr.File()).(*ir.File)
	if !ok || hir == nil {
		return src, fmt.Errorf("parse failed")
	}

	if bag.Errored() {
		return src, fmt.Errorf("source has parse errors; formatter left it unchanged")
	}

	return File(hir, src), nil
}

func File(f *ir.File, src string) string {
	p := newPrinter()
	if src != "" {
		p.attachComments(extractComments(src))
	}

	printFile(p, f)
	return p.String()
}

func printFile(p *Printer, f *ir.File) {

	hasPackage := len(f.Package) > 0
	hasSide := f.Side.Kind != 0
	if hasPackage {
		p.write("package " + f.Package.String())
		if hasSide && f.Side.Kind != ir.SideShared {
			p.write(" on " + sideKindLabel(f.Side.Kind))
			if f.Side.Kind == ir.SideBackend && f.Side.Target != "" {
				p.write("(" + f.Side.Target + ")")
			}
		}

		p.writeNewline()
	} else if hasSide {
		p.write("on " + sideKindLabel(f.Side.Kind))
		if f.Side.Kind == ir.SideBackend && f.Side.Target != "" {
			p.write("(" + f.Side.Target + ")")
		}

		p.writeNewline()
	}

	if hasPackage || hasSide {
		p.writeNewline()
	}

	for i, st := range f.Statements {
		if i > 0 {
			p.blankLine()
		}

		p.printStmt(st)
	}

	if p.comments != nil {
		p.comments.flushTrailing(p)
	}
}
