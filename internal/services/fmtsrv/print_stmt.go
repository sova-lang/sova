package fmtsrv

import (
	"sova/internal/ir"
)

// printStmt emits a single statement followed by a newline. Top-level callers should follow up with `blankLine()` when separating sibling decls. Inside blocks the statement printer drives its own newlines, so block emitters can just loop.
func (p *Printer) printStmt(s ir.Stmt) {
	if s == nil {
		return
	}
	if p.comments != nil {
		p.comments.flushBefore(p, s.Span())
	}
	switch n := s.(type) {
	case *ir.BlockStmt:
		p.printBlock(n)
		p.writeNewline()
	case *ir.VarDeclStmt:
		p.printVarDecl(n)
	case *ir.ExprStmt:
		p.printExpr(n.Expr)
		p.writeNewline()
	case *ir.FieldAssignmentStmt:
		p.write(n.Receiver.Name)
		for _, f := range n.Fields {
			p.write(".")
			p.write(f.Name)
		}
		p.write(" ")
		p.write(string(n.Op))
		p.write(" ")
		p.printExpr(n.Value)
		p.writeNewline()
	case *ir.MultiAssignmentStmt:
		for i, tgt := range n.Targets {
			if i > 0 {
				p.write(", ")
			}
			if tgt.Name == nil {
				p.write("_")
				continue
			}
			p.write(tgt.Name.Name)
		}
		p.write(" = ")
		p.printExpr(n.Value)
		p.writeNewline()
	case *ir.IfStmt:
		p.write("if ")
		p.printExpr(n.Cond)
		p.write(" ")
		p.printBlock(n.Then)
		for _, eb := range n.ElseIfs {
			p.write(" else if ")
			p.printExpr(eb.Cond)
			p.write(" ")
			p.printBlock(eb.Then)
		}
		if n.Else != nil {
			p.write(" else ")
			p.printBlock(n.Else)
		}
		p.writeNewline()
	case *ir.ReturnStmt:
		p.write("return")
		for i, r := range n.Results {
			if i == 0 {
				p.write(" ")
			} else {
				p.write(", ")
			}
			p.printExpr(r)
		}
		p.writeNewline()
	case *ir.GuardStmt:
		p.write("guard ")
		p.printExpr(n.Cond)
		p.write(" else return")
		for i, r := range n.Returns {
			if i == 0 {
				p.write(" ")
			} else {
				p.write(", ")
			}
			p.printExpr(r)
		}
		p.writeNewline()
	case *ir.ForStmt:
		p.printForStmt(n)
	case *ir.WhileStmt:
		p.write("while ")
		p.printExpr(n.Cond)
		p.write(" ")
		p.printBlock(n.Body)
		p.writeNewline()
	case *ir.BreakStmt:
		p.writeLine("break")
	case *ir.ContinueStmt:
		p.writeLine("continue")
	case *ir.GoStmt:
		p.write("go ")
		if n.Call != nil {
			p.printExpr(n.Call)
			p.writeNewline()
			return
		}
		p.printBlock(n.Body)
		p.writeNewline()
	case *ir.DeferStmt:
		p.write("defer ")
		if n.Call != nil {
			p.printExpr(n.Call)
			p.writeNewline()
			return
		}
		p.printBlock(n.Body)
		p.writeNewline()
	case *ir.SelectStmt:
		p.printSelectStmt(n)
	case *ir.AssertStmt:
		p.write("assert ")
		p.printExpr(n.Expr)
		p.writeNewline()
	case *ir.AsSessionStmt:
		p.write("asSession")
		if n.Name != "" {
			p.write("(\"" + n.Name + "\")")
		}
		p.write(" ")
		p.printBlock(n.Body)
		p.writeNewline()
	case *ir.FuncDeclStmt:
		p.printFuncDecl(n)
	case *ir.TypeDeclStmt:
		p.printTypeDecl(n)
	case *ir.EnumDeclStmt:
		p.printEnumDecl(n)
	case *ir.InterfaceDeclStmt:
		p.printInterfaceDecl(n)
	case *ir.MixinDeclStmt:
		p.printMixinDecl(n)
	case *ir.ExternDeclStmt:
		p.printExternDecl(n)
	case *ir.ImportStmt:
		p.printImportStmt(n)
	case *ir.TestDeclStmt:
		p.printTestDecl(n)
	case *ir.GroupDeclStmt:
		p.printGroupDecl(n)
	case *ir.SetupStmt:
		p.printSetupStmt(n, false)
	case *ir.TeardownStmt:
		p.printSetupStmt(n, true)
	}
}

// printBlock emits `{` newline, indented body, `}` (no trailing newline - caller adds one).
func (p *Printer) printBlock(b *ir.BlockStmt) {
	if b == nil {
		p.write("{}")
		return
	}
	p.write("{")
	p.writeNewline()
	p.withIndent(func() {
		for _, s := range b.Stmts {
			p.printStmt(s)
		}
	})
	p.write("}")
}

func (p *Printer) printVarDecl(n *ir.VarDeclStmt) {
	for _, ann := range n.Annotations {
		p.printAnnotation(ann)
		p.write(" ")
	}
	if n.Wire != nil {
		p.printWireSpec(n.Wire)
		p.write(" ")
	}
	if n.IsConst {
		p.write("const ")
	} else {
		p.write("let ")
	}
	for i, tgt := range n.Targets {
		if i > 0 {
			p.write(", ")
		}
		if tgt.Name == nil {
			p.write("_")
			continue
		}
		p.write(tgt.Name.Name)
		if tgt.TypeAnn != nil {
			p.write(": ")
			p.printType(tgt.TypeAnn)
		}
	}
	if n.Init != nil {
		p.write(" = ")
		p.printExpr(n.Init)
	}
	p.writeNewline()
}

func (p *Printer) printForStmt(n *ir.ForStmt) {
	p.write("for ")
	switch {
	case n.CondInt != nil:
		if n.CondInt.Init != nil {
			if n.CondInt.Init.IsConst {
				p.write("const ")
			} else {
				p.write("let ")
			}
			for i, tgt := range n.CondInt.Init.Targets {
				if i > 0 {
					p.write(", ")
				}
				if tgt.Name == nil {
					p.write("_")
					continue
				}
				p.write(tgt.Name.Name)
				if tgt.TypeAnn != nil {
					p.write(": ")
					p.printType(tgt.TypeAnn)
				}
			}
			if n.CondInt.Init.Init != nil {
				p.write(" = ")
				p.printExpr(n.CondInt.Init.Init)
			}
		}
		p.write("; ")
		p.printExpr(n.CondInt.Cond)
		p.write("; ")
		p.printExpr(n.CondInt.Post)
	case n.CondIn != nil:
		p.write(n.CondIn.InFirstVar.Name)
		if n.CondIn.InSecondVar != nil {
			p.write(", " + n.CondIn.InSecondVar.Name)
		}
		if n.CondIn.InThirdVar != nil {
			p.write(", " + n.CondIn.InThirdVar.Name)
		}
		p.write(" in ")
		p.printExpr(n.CondIn.IterExpr)
	case n.CondRange != nil:
		if n.CondRange.RangeVar.Name != "" {
			p.write(n.CondRange.RangeVar.Name + " in ")
		}
		p.printExpr(n.CondRange.RangeStart)
		if n.CondRange.RangeEnd != nil {
			p.write("..")
			p.printExpr(n.CondRange.RangeEnd)
		}
	}
	p.write(" ")
	p.printBlock(n.Body)
	p.writeNewline()
}

func (p *Printer) printSelectStmt(n *ir.SelectStmt) {
	p.write("select {")
	p.writeNewline()
	p.withIndent(func() {
		for _, cc := range n.Cases {
			p.write("case ")
			switch cc.Kind {
			case ir.SelectCaseSend:
				p.printExpr(cc.ChanExpr)
				p.write(".send(")
				p.printExpr(cc.SendValue)
				p.write(")")
			case ir.SelectCaseRecvBind:
				for i, tgt := range cc.Targets {
					if i > 0 {
						p.write(", ")
					}
					if tgt.Name == nil {
						p.write("_")
						continue
					}
					p.write(tgt.Name.Name)
				}
				p.write(" = ")
				p.printExpr(cc.ChanExpr)
				p.write(".recv()")
			case ir.SelectCaseRecvDiscard:
				p.printExpr(cc.ChanExpr)
			}
			p.write(" => ")
			p.printBlock(cc.Body)
			p.writeNewline()
		}
		if n.Default != nil {
			p.write("default => ")
			p.printBlock(n.Default)
			p.writeNewline()
		}
	})
	p.writeLine("}")
}

func (p *Printer) printAnnotation(a ir.Annotation) {
	p.write("@" + a.Name.Name)
	if len(a.Args) > 0 {
		p.write("(")
		for i, arg := range a.Args {
			if i > 0 {
				p.write(", ")
			}
			p.printExpr(arg)
		}
		p.write(")")
	}
}
