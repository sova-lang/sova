package fmtsrv

import (
	"strconv"
	"strings"

	"sova/internal/ir"
)

func (p *Printer) printFuncDecl(n *ir.FuncDeclStmt) {
	for _, ann := range n.Annotations {
		p.printAnnotation(ann)
		p.writeNewline()
	}

	if n.IsWired && n.Wire != nil {
		p.printWireSpec(n.Wire)
		p.write(" ")
	}

	if n.IsAsync {
		p.write("async ")
	}

	p.write("func ")
	p.write(n.Name.Name)
	if len(n.TypeParams) > 0 {
		p.printTypeParams(n.TypeParams)
	}

	p.write("(")
	for i, param := range n.Params {
		if i > 0 {
			p.write(", ")
		}

		p.printFuncParam(param)
	}

	p.write(")")
	if n.ReturnType != nil {
		p.write(": ")
		p.printType(n.ReturnType)
	}

	if n.Side != nil {
		p.write(" ")
		p.printSide(n.Side)
	}

	if n.Body != nil {
		p.write(" ")
		p.printBlock(n.Body)
	}

	p.writeNewline()
}

func (p *Printer) printFuncParam(param *ir.FuncParam) {
	if param.IsVariadic {
		p.write("...")
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

func (p *Printer) printTypeParams(tps []ir.TypeParamDecl) {
	p.write("<")
	for i, tp := range tps {
		if i > 0 {
			p.write(", ")
		}

		p.write(tp.Name)
		if len(tp.ImplementsConstraints) > 0 {
			p.write(": ")
			for j, c := range tp.ImplementsConstraints {
				if j > 0 {
					p.write(" + ")
				}

				p.write(c.Name)
			}
		}

		if len(tp.WithConstraints) > 0 {
			p.write(" with ")
			for j, c := range tp.WithConstraints {
				if j > 0 {
					p.write(" + ")
				}

				p.write(c.Name)
			}
		}
	}

	p.write(">")
}

func (p *Printer) printTypeDecl(n *ir.TypeDeclStmt) {
	for _, ann := range n.Annotations {
		p.printAnnotation(ann)
		p.writeNewline()
	}

	p.write("type ")
	p.write(n.Name.Name)
	if len(n.TypeParams) > 0 {
		p.printTypeParams(n.TypeParams)
	}

	if len(n.Implements) > 0 {
		p.write(" implements ")
		for i, im := range n.Implements {
			if i > 0 {
				p.write(", ")
			}

			p.write(im.Name)
		}
	}

	if len(n.MixedIn) > 0 {
		p.write(" with ")
		for i, m := range n.MixedIn {
			if i > 0 {
				p.write(", ")
			}

			p.write(m.Name)
		}
	}

	p.write(" {")
	p.writeNewline()
	p.withIndent(func() {
		for _, fld := range n.Fields {
			p.printField(fld)
		}

		for _, ctor := range n.Ctors {
			if ctor.IsSynthetic {
				continue
			}

			p.printCtor(ctor)
		}

		for _, m := range n.Methods {
			if m.Private {
				p.write("private ")
			}

			p.printFuncDecl(m.Func)
		}

		for _, c := range n.Casts {
			p.printCast(c)
		}
	})
	p.writeLine("}")
}

func (p *Printer) printField(f *ir.TypeField) {
	for _, ann := range f.Annotations {
		p.printAnnotation(ann)
		p.write(" ")
	}

	if f.Private {
		p.write("private ")
	}

	p.write(f.Name.Name)
	if f.Type != nil {
		p.write(": ")
		p.printType(f.Type)
	}

	if f.Default != nil {
		p.write(" = ")
		p.printExpr(f.Default)
	}

	p.writeNewline()
}

func (p *Printer) printCtor(c *ir.CtorDecl) {
	for _, ann := range c.Annotations {
		p.printAnnotation(ann)
		p.writeNewline()
	}

	p.write("new(")
	for i, param := range c.Params {
		if i > 0 {
			p.write(", ")
		}

		p.printFuncParam(param)
	}

	p.write(") ")
	p.printBlock(c.Body)
	p.writeNewline()
}

func (p *Printer) printCast(c *ir.CastDecl) {
	for _, ann := range c.Annotations {
		p.printAnnotation(ann)
		p.writeNewline()
	}

	p.write("cast(")
	if c.Param != nil {
		p.printFuncParam(c.Param)
	}

	p.write(")")
	if c.ReturnType != nil {
		p.write(": ")
		p.printType(c.ReturnType)
	}

	if c.Body != nil {
		p.write(" ")
		p.printBlock(c.Body)
	}

	p.writeNewline()
}

func (p *Printer) printEnumDecl(n *ir.EnumDeclStmt) {
	p.write("enum ")
	p.write(n.Name.Name)
	p.write(" {")
	p.writeNewline()
	p.withIndent(func() {
		for _, f := range n.Fields {
			p.write(f.Name.Name)
			if f.Type != nil {
				p.write(": ")
				p.printType(f.Type)
			}

			if f.Default != nil {
				p.write(" = ")
				p.printExpr(f.Default)
			}

			p.writeNewline()
		}

		for _, c := range n.Cases {
			p.write(c.Name.Name)
			if len(c.Args) > 0 {
				p.write("(")
				for i, a := range c.Args {
					if i > 0 {
						p.write(", ")
					}

					p.printExpr(a)
				}

				p.write(")")
			}

			if c.Value != nil {
				p.write(" = " + strconv.FormatInt(*c.Value, 10))
			}

			p.writeNewline()
		}

		for _, m := range n.Methods {
			p.printFuncDecl(m)
		}
	})
	p.writeLine("}")
}

func (p *Printer) printInterfaceDecl(n *ir.InterfaceDeclStmt) {
	p.write("interface ")
	p.write(n.Name.Name)
	p.write(" {")
	p.writeNewline()
	p.withIndent(func() {
		for _, sig := range n.Methods {
			p.write("func ")
			p.write(sig.Name.Name)
			p.write("(")
			for i, param := range sig.Params {
				if i > 0 {
					p.write(", ")
				}

				p.printFuncParam(param)
			}

			p.write(")")
			if sig.ReturnType != nil {
				p.write(": ")
				p.printType(sig.ReturnType)
			}

			p.writeNewline()
		}
	})
	p.writeLine("}")
}

func (p *Printer) printMixinDecl(n *ir.MixinDeclStmt) {
	p.write("mixin ")
	p.write(n.Name.Name)
	p.write(" {")
	p.writeNewline()
	p.withIndent(func() {
		for _, fld := range n.Fields {
			p.printField(fld)
		}

		for _, m := range n.Methods {
			if m.Private {
				p.write("private ")
			}

			p.printFuncDecl(m.Func)
		}
	})
	p.writeLine("}")
}

func (p *Printer) printExternDecl(n *ir.ExternDeclStmt) {
	p.write("extern ")
	if n.IsDefaultImport {
		p.write("default ")
	}

	if n.Module != nil {
		p.write(quoteString(*n.Module) + " ")
	}

	p.write("{")
	p.writeNewline()
	p.withIndent(func() {
		for _, fn := range n.Funcs {
			if fn.IsAsync {
				p.write("async ")
			}

			p.write("func ")
			p.write(fn.Name.Name)
			p.write("(")
			for i, param := range fn.Params {
				if i > 0 {
					p.write(", ")
				}

				p.printFuncParam(param)
			}

			p.write(")")
			if fn.ReturnType != nil {
				p.write(": ")
				p.printType(fn.ReturnType)
			}

			p.write(" = ")
			p.printExternMapping(fn.Mapping)
			p.writeNewline()
		}

		for _, v := range n.Vars {
			if v.IsConst {
				p.write("const ")
			} else {
				p.write("let ")
			}

			p.write(v.Name.Name)
			if v.Type != nil {
				p.write(": ")
				p.printType(v.Type)
			}

			p.write(" = ")
			p.printExternMapping(v.Mapping)
			p.writeNewline()
		}

		for _, t := range n.Types {
			p.printTypeDecl(t)
		}

		for _, ifc := range n.Interfaces {
			p.printInterfaceDecl(ifc)
		}
	})
	p.writeLine("}")
}

func (p *Printer) printExternMapping(m *ir.ExternMapping) {
	if m == nil {
		p.write(`""`)
		return
	}

	if m.Simple != nil {
		p.write(quoteString(*m.Simple))
		return
	}

	p.write("{")
	p.writeNewline()
	p.withIndent(func() {
		first := true
		for side, sm := range m.Shared {
			if sm == nil {
				continue
			}

			if !first {
				p.write(",")
				p.writeNewline()
			}

			first = false
			p.write(sideKindLabel(side))
			if sm.Module != nil {
				p.write("(" + quoteString(*sm.Module) + ")")
			}

			p.write(": " + quoteString(sm.NativeFunc))
		}

		p.writeNewline()
	})
	p.write("}")
}

func (p *Printer) printImportStmt(n *ir.ImportStmt) {
	p.write("import " + quoteString(n.Path.String()))
	if n.UsingAll {
		p.write(" using *")
	} else if len(n.UsingList) > 0 {
		p.write(" using {" + strings.Join(n.UsingList, ", ") + "}")
	}

	p.writeNewline()
}

func (p *Printer) printTestDecl(n *ir.TestDeclStmt) {
	p.write("test " + quoteString(n.Name))
	if n.Parallel {
		p.write(" parallel")
	}

	if len(n.Tags) > 0 {
		p.write(" tag: ")
		for i, t := range n.Tags {
			if i > 0 {
				p.write(", ")
			}

			p.write(quoteString(t))
		}
	}

	p.write(" ")
	p.printBlock(n.Body)
	p.writeNewline()
}

func (p *Printer) printGroupDecl(n *ir.GroupDeclStmt) {
	p.write("group " + quoteString(n.Name))
	if n.Parallel {
		p.write(" parallel")
	}

	if len(n.Tags) > 0 {
		p.write(" tag: ")
		for i, t := range n.Tags {
			if i > 0 {
				p.write(", ")
			}

			p.write(quoteString(t))
		}
	}

	p.write(" {")
	p.writeNewline()
	p.withIndent(func() {
		for _, s := range n.Body {
			p.printStmt(s)
		}
	})
	p.writeLine("}")
}

func (p *Printer) printSetupStmt(s ir.Stmt, teardown bool) {
	switch n := s.(type) {
	case *ir.SetupStmt:
		if n.IsAll {
			p.write("setupAll ")
		} else {
			p.write("setup ")
		}

		p.printBlock(n.Body)
		p.writeNewline()
	case *ir.TeardownStmt:
		if n.IsAll {
			p.write("teardownAll ")
		} else {
			p.write("teardown ")
		}

		p.printBlock(n.Body)
		p.writeNewline()
	}

	_ = teardown
}

func (p *Printer) printSide(side *ir.SideSpec) {
	if side == nil {
		return
	}

	p.write("on " + sideKindLabel(side.Kind))
}

func (p *Printer) printWireSpec(w *ir.WireSpec) {
	p.write("wire")
	if w.Ruleset != "" {
		p.write(":" + w.Ruleset)
	}

	var parts []string
	if w.Method != "" {
		parts = append(parts, "method: "+quoteString(w.Method))
	}

	if w.Path != "" {
		parts = append(parts, "path: "+quoteString(w.Path))
	}

	if w.Transport != "" {
		parts = append(parts, "transport: "+quoteString(w.Transport))
	}

	if len(w.RequiredRoles) > 0 {
		quoted := make([]string, len(w.RequiredRoles))
		for i, r := range w.RequiredRoles {
			quoted[i] = quoteString(r)
		}

		parts = append(parts, "authz: ["+strings.Join(quoted, ", ")+"]")
	}

	if !w.RequireAuthN {
		parts = append(parts, "authn: false")
	}

	if len(parts) > 0 {
		p.write("(" + strings.Join(parts, ", ") + ")")
	}
}

func sideKindLabel(k ir.SideKind) string {
	switch k {
	case ir.SideFrontend:
		return "frontend"
	case ir.SideBackend:
		return "backend"
	case ir.SideShared:
		return "shared"
	case ir.SideTest:
		return "test"
	case ir.SideSynth:
		return "synth"
	}

	return "shared"
}
