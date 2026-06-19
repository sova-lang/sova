package passes

import (
	"sova/internal/diag"
	"sova/internal/ir"
)

const SynthRegistryCacheKey = "synth_registry"

const SynthEmittedRegistryPrefix = "synth_reg:"

const synthExpansionDepthLimit = 16

type PassExpandSynths struct{}

func (p *PassExpandSynths) Name() string       { return "expand_synths" }

func (p *PassExpandSynths) Scope() PassScope   { return PerBuild }

func (p *PassExpandSynths) Requires() []string { return []string{"check_imports"} }

func (p *PassExpandSynths) NoErrors() bool     { return false }

func (p *PassExpandSynths) Run(pc *PassContext) error {
	registry := p.buildRegistry(pc)
	pc.Cache[SynthRegistryCacheKey] = registry
	if len(registry) == 0 {
		return nil
	}

	exp := &synthExpander{pc: pc, registry: registry}

	for round := 0; round < synthExpansionDepthLimit; round++ {
		sites := exp.collectSites()
		if !exp.runRound(sites) {
			return nil
		}
	}

	exp.diagnoseRemaining(exp.collectSites())
	return nil
}

func (p *PassExpandSynths) buildRegistry(pc *PassContext) map[string]*ir.SynthDeclStmt {
	out := map[string]*ir.SynthDeclStmt{}

	for _, pkg := range pc.Pkgs {
		if pkg == nil {
			continue
		}

		for _, f := range pkg.Files {
			if f == nil || f.Hir == nil || f.Hir.Side.Kind != ir.SideSynth {
				continue
			}

			for _, st := range f.Hir.Statements {
				sd, ok := st.(*ir.SynthDeclStmt)
				if !ok || sd.Name.Name == "" {
					continue
				}

				if _, dup := out[sd.Name.Name]; dup {
					pc.Diag.Report(diag.ErrSynthDuplicateName, sd.Name.Span, sd.Name.Name)
					continue
				}

				out[sd.Name.Name] = sd
			}
		}
	}

	return out
}

type synthExpander struct {
	pc       *PassContext
	registry map[string]*ir.SynthDeclStmt
}

type synthBindKind int

const (
	bindUnknown synthBindKind = iota
	bindType
	bindField
	bindFunc
	bindMethod
	bindCtor
	bindParam
	bindLet
)

type synthBind struct {
	kind     synthBindKind
	typeDecl *ir.TypeDeclStmt
	field    *ir.TypeField
	funcDecl *ir.FuncDeclStmt
	method   *ir.TypeMethodDecl
	ctor     *ir.CtorDecl
	param    *ir.FuncParam
	let      *ir.VarDeclStmt
	fileSide ir.SideKind
}

func (b synthBind) annotations() *[]ir.Annotation {
	switch b.kind {
	case bindType:
		return &b.typeDecl.Annotations
	case bindField:
		return &b.field.Annotations
	case bindFunc:
		return &b.funcDecl.Annotations
	case bindMethod:
		return &b.method.Annotations
	case bindCtor:
		return &b.ctor.Annotations
	case bindParam:
		return &b.param.Annotations
	case bindLet:
		return &b.let.Annotations
	}

	return nil
}

func (b synthBind) synthKind() ir.SynthTargetKind {
	switch b.kind {
	case bindType:
		return ir.SynthTargetType
	case bindField:
		return ir.SynthTargetField
	case bindFunc:
		return ir.SynthTargetFunc
	case bindMethod:
		return ir.SynthTargetMethod
	case bindCtor:
		return ir.SynthTargetCtor
	case bindParam:
		return ir.SynthTargetParam
	case bindLet:
		return ir.SynthTargetLet
	}

	return ir.SynthTargetUnknown
}

func (b synthBind) label() string {
	switch b.kind {
	case bindType:
		return b.typeDecl.Name.Name
	case bindField:
		return b.field.Name.Name
	case bindFunc:
		return b.funcDecl.Name.Name
	case bindMethod:
		if b.method.Func != nil {
			return b.method.Func.Name.Name
		}

		return "method"
	case bindCtor:
		return "ctor"
	case bindParam:
		return b.param.Name.Name
	case bindLet:
		return varDeclLabel(b.let)
	}

	return "?"
}

func (b synthBind) iterable(member string) ([]synthBind, bool) {
	switch b.kind {
	case bindType:
		switch member {
		case "fields":
			out := make([]synthBind, 0, len(b.typeDecl.Fields))
			for _, f := range b.typeDecl.Fields {
				if f != nil {
					out = append(out, synthBind{kind: bindField, field: f})
				}
			}

			return out, true
		case "methods":
			out := make([]synthBind, 0, len(b.typeDecl.Methods))
			for _, m := range b.typeDecl.Methods {
				if m != nil {
					out = append(out, synthBind{kind: bindMethod, method: m})
				}
			}

			return out, true
		case "ctors":
			out := make([]synthBind, 0, len(b.typeDecl.Ctors))
			for _, c := range b.typeDecl.Ctors {
				if c != nil {
					out = append(out, synthBind{kind: bindCtor, ctor: c})
				}
			}

			return out, true
		}

	case bindFunc:
		if member == "params" {
			out := make([]synthBind, 0, len(b.funcDecl.Params))
			for _, prm := range b.funcDecl.Params {
				if prm != nil {
					out = append(out, synthBind{kind: bindParam, param: prm})
				}
			}

			return out, true
		}

	case bindMethod:
		if member == "params" && b.method.Func != nil {
			out := make([]synthBind, 0, len(b.method.Func.Params))
			for _, prm := range b.method.Func.Params {
				if prm != nil {
					out = append(out, synthBind{kind: bindParam, param: prm})
				}
			}

			return out, true
		}

	case bindCtor:
		if member == "params" {
			out := make([]synthBind, 0, len(b.ctor.Params))
			for _, prm := range b.ctor.Params {
				if prm != nil {
					out = append(out, synthBind{kind: bindParam, param: prm})
				}
			}

			return out, true
		}
	}

	return nil, false
}

func (b synthBind) stringProperty(name string) (string, bool) {
	switch b.kind {
	case bindType:
		if name == "name" {
			return b.typeDecl.Name.Name, true
		}

	case bindField:
		switch name {
		case "name":
			return b.field.Name.Name, true
		case "type":
			return typeRefString(b.field.Type), true
		}

	case bindFunc:
		if name == "name" {
			return b.funcDecl.Name.Name, true
		}

	case bindMethod:
		if name == "name" && b.method.Func != nil {
			return b.method.Func.Name.Name, true
		}

	case bindParam:
		switch name {
		case "name":
			return b.param.Name.Name, true
		case "type":
			return typeRefString(b.param.Type), true
		}

	case bindLet:
		if name == "name" {
			return varDeclLabel(b.let), true
		}
	}

	return "", false
}

func (b synthBind) boolProperty(name string) (bool, bool) {
	switch b.kind {
	case bindType:
		if name == "isExtern" {
			return b.typeDecl.IsExtern, true
		}

	case bindField:
		switch name {
		case "isShared":
			return b.field.IsShared, true
		case "isPrivate":
			return b.field.Private, true
		}

	case bindFunc:
		switch name {
		case "isAsync":
			return b.funcDecl.IsAsync, true
		case "isWired":
			return b.funcDecl.IsWired, true
		}

	case bindMethod:
		switch name {
		case "isShared":
			return b.method.IsShared, true
		case "isPrivate":
			return b.method.Private, true
		case "isAsync":
			return b.method.Func != nil && b.method.Func.IsAsync, true
		}

	case bindCtor:
		if name == "isShared" {
			return b.ctor.IsShared, true
		}

	case bindParam:
		if name == "isVariadic" {
			return b.param.IsVariadic, true
		}

	case bindLet:
		switch name {
		case "isConst":
			return b.let.IsConst, true
		case "isWired":
			return b.let.IsWired, true
		}
	}

	return false, false
}

func typeRefString(t *ir.TypeRef) string {
	if t == nil {
		return ""
	}

	if t.CustomName != "" {
		if t.CustomQualifier != "" {
			return t.CustomQualifier + "." + t.CustomName
		}

		return t.CustomName
	}

	return ""
}

type synthEnv struct {
	parent *synthEnv
	binds  map[string]synthBind
}

func newSynthEnv(parent *synthEnv) *synthEnv {
	return &synthEnv{parent: parent, binds: map[string]synthBind{}}
}

func (e *synthEnv) lookup(name string) (synthBind, bool) {
	if e == nil {
		return synthBind{}, false
	}

	if v, ok := e.binds[name]; ok {
		return v, true
	}

	return e.parent.lookup(name)
}

func (e *synthExpander) collectSites() []synthBind {
	var out []synthBind
	for _, pkg := range e.pc.Pkgs {
		if pkg == nil {
			continue
		}

		for _, f := range pkg.Files {
			if f == nil || f.Hir == nil || f.Hir.Side.Kind == ir.SideSynth {
				continue
			}

			fs := f.Hir.Side.Kind
			for _, st := range f.Hir.Statements {
				switch s := st.(type) {
				case *ir.TypeDeclStmt:
					out = append(out, synthBind{kind: bindType, typeDecl: s, fileSide: fs})
					for _, fld := range s.Fields {
						if fld != nil {
							out = append(out, synthBind{kind: bindField, field: fld, fileSide: fs})
						}
					}

					for _, m := range s.Methods {
						if m == nil {
							continue
						}

						out = append(out, synthBind{kind: bindMethod, method: m, fileSide: fs})
						if m.Func != nil {
							for _, prm := range m.Func.Params {
								if prm != nil {
									out = append(out, synthBind{kind: bindParam, param: prm, fileSide: fs})
								}
							}
						}
					}

					for _, c := range s.Ctors {
						if c == nil {
							continue
						}

						out = append(out, synthBind{kind: bindCtor, ctor: c, fileSide: fs})
						for _, prm := range c.Params {
							if prm != nil {
								out = append(out, synthBind{kind: bindParam, param: prm, fileSide: fs})
							}
						}
					}

				case *ir.FuncDeclStmt:
					out = append(out, synthBind{kind: bindFunc, funcDecl: s, fileSide: fs})
					for _, prm := range s.Params {
						if prm != nil {
							out = append(out, synthBind{kind: bindParam, param: prm, fileSide: fs})
						}
					}

				case *ir.VarDeclStmt:
					out = append(out, synthBind{kind: bindLet, let: s, fileSide: fs})
				}
			}
		}
	}

	return out
}

func (e *synthExpander) runRound(sites []synthBind) bool {
	changed := false
	for _, s := range sites {
		anns := s.annotations()
		if anns == nil || len(*anns) == 0 {
			continue
		}

		if e.expandSite(s, anns) {
			changed = true
		}
	}

	return changed
}

func (e *synthExpander) expandSite(site synthBind, anns *[]ir.Annotation) bool {
	hit := false
	for _, a := range *anns {
		if _, ok := e.registry[a.Name.Name]; ok {
			hit = true
			break
		}
	}

	if !hit {
		return false
	}

	out := make([]ir.Annotation, 0, len(*anns))
	for _, a := range *anns {
		sd, ok := e.registry[a.Name.Name]
		if !ok {
			out = append(out, a)
			continue
		}

		if sd.Target.Kind != site.synthKind() {
			e.pc.Diag.Report(diag.ErrSynthTargetMismatch, a.Name.Span, a.Name.Name, sd.Target.Kind.String(), site.label())
			continue
		}

		if !sideAllows(sd.RequiredSide, site.fileSide) {
			e.pc.Diag.Report(diag.ErrSynthSideMismatch, a.Name.Span, a.Name.Name, sideLabel(sd.RequiredSide), sideLabel(site.fileSide))
			continue
		}

		if want, got := len(sd.Params), len(a.Args); want != got {
			e.pc.Diag.Report(diag.ErrSynthArgCountMismatch, a.Name.Span, a.Name.Name, want, got)
			continue
		}

		env := newSynthEnv(nil)
		env.binds[sd.Target.BindName] = site
		for i, prm := range sd.Params {
			if prm == nil || prm.Name.Name == "" {
				continue
			}

			env.binds[prm.Name.Name] = synthBind{kind: bindUnknown}

			env.binds[paramSlotKey(prm.Name.Name)] = synthBind{kind: bindUnknown}

			env.binds[prm.Name.Name] = synthBind{kind: bindUnknown}

			_ = i
		}

		paramSubs := map[string]ir.Expr{}

		byName := map[string]ir.Expr{}

		firstNamedIdx := -1
		for i, expr := range a.Args {
			if i < len(a.ArgNames) && a.ArgNames[i] != "" {
				byName[a.ArgNames[i]] = expr
				if firstNamedIdx < 0 {
					firstNamedIdx = i
				}
			}
		}

		for i, prm := range sd.Params {
			if prm == nil || prm.Name.Name == "" {
				continue
			}

			if v, ok := byName[prm.Name.Name]; ok {
				paramSubs[prm.Name.Name] = v
				continue
			}

			if i < len(a.Args) && a.Args[i] != nil && (firstNamedIdx < 0 || i < firstNamedIdx) {
				if i >= len(a.ArgNames) || a.ArgNames[i] == "" {
					paramSubs[prm.Name.Name] = a.Args[i]
					continue
				}
			}

			if prm.Default != nil {
				paramSubs[prm.Name.Name] = prm.Default
			}
		}

		emitted := e.interpretBody(sd.Body, env, sd.Target.BindName, paramSubs)
		out = append(out, emitted...)
	}

	*anns = out
	return true
}

func paramSlotKey(name string) string { return "@param:" + name }

func sideAllows(required, actual ir.SideKind) bool {
	if required == ir.SideUnknown {
		return true
	}

	if actual == ir.SideShared {
		return required != ir.SideUnknown
	}

	if required == ir.SideShared {
		return actual == ir.SideShared
	}

	return required == actual
}

func (e *synthExpander) interpretBody(items []ir.SynthBodyItem, env *synthEnv, outerBindName string, paramSubs map[string]ir.Expr) []ir.Annotation {
	var local []ir.Annotation
	for _, item := range items {
		switch it := item.(type) {
		case *ir.SynthEmitOn:
			bind, ok := env.lookup(it.Scope)
			if !ok {
				e.pc.Diag.Report(diag.ErrSynthUnknownBind, ir.NameRef{Name: it.Scope}.Span, it.Scope)
				continue
			}

			substituted := e.substituteAnnotations(it.AnnotationEmits, env, paramSubs)
			if it.Scope == outerBindName {
				local = append(local, substituted...)
				continue
			}

			if target := bind.annotations(); target != nil {
				*target = append(*target, substituted...)
			}

		case *ir.SynthEmitAppend:
			if it.Fragment == nil || it.Registry == "" {
				continue
			}

			frag := e.substituteExpr(it.Fragment, env, paramSubs)
			key := SynthEmittedRegistryPrefix + it.Registry
			existing, _ := e.pc.Cache[key].([]ir.Expr)
			e.pc.Cache[key] = append(existing, frag)
		case *ir.SynthForStmt:
			bind, ok := env.lookup(it.BindName)
			if !ok {
				e.pc.Diag.Report(diag.ErrSynthUnknownBind, ir.NameRef{Name: it.BindName}.Span, it.BindName)
				continue
			}

			elems, ok := bind.iterable(it.Member)
			if !ok {
				e.pc.Diag.Report(diag.ErrSynthUnknownMember, ir.NameRef{Name: it.Member}.Span, it.BindName, it.Member)
				continue
			}

			for _, elem := range elems {
				inner := newSynthEnv(env)
				inner.binds[it.LoopVar] = elem
				if it.Where != nil && !e.evalWhere(it.Where, inner) {
					continue
				}

				nested := e.interpretBody(it.Body, inner, outerBindName, paramSubs)
				local = append(local, nested...)
			}

		case *ir.SynthEmitField:
			e.injectField(env, outerBindName, paramSubs, it)
		case *ir.SynthEmitMethod:
			e.injectMethod(env, outerBindName, paramSubs, it)
		case *ir.SynthEmitCtor:
			e.injectCtor(env, outerBindName, paramSubs, it)
		}
	}

	return local
}

func (e *synthExpander) outerTypeBind(env *synthEnv, outerBindName string) *ir.TypeDeclStmt {
	bind, ok := env.lookup(outerBindName)
	if !ok || bind.kind != bindType || bind.typeDecl == nil {
		return nil
	}

	return bind.typeDecl
}

func (e *synthExpander) injectField(env *synthEnv, outerBindName string, paramSubs map[string]ir.Expr, it *ir.SynthEmitField) {
	td := e.outerTypeBind(env, outerBindName)
	if td == nil || it.Field == nil {
		if it.Field != nil {
			e.pc.Diag.Report(diag.ErrSynthMemberOnNonType, it.Field.Name.Span, "field", outerBindName)
		}

		return
	}

	fld := ir.CloneTypeField(it.Field, e.pc.NodeAlloc)
	fld.Annotations = e.substituteAnnotations(it.Field.Annotations, env, paramSubs)
	if it.Field.Default != nil {
		fld.Default = e.substituteExpr(it.Field.Default, env, paramSubs)
	}

	td.Fields = append(td.Fields, fld)
}

func (e *synthExpander) injectMethod(env *synthEnv, outerBindName string, paramSubs map[string]ir.Expr, it *ir.SynthEmitMethod) {
	td := e.outerTypeBind(env, outerBindName)
	if td == nil || it.Method == nil {
		if it.Method != nil && it.Method.Func != nil {
			e.pc.Diag.Report(diag.ErrSynthMemberOnNonType, it.Method.Func.Name.Span, "method", outerBindName)
		}

		return
	}

	m := ir.CloneTypeMethodDecl(it.Method, e.pc.NodeAlloc)
	m.Annotations = e.substituteAnnotations(it.Method.Annotations, env, paramSubs)
	if m.Func != nil && it.Method.Func != nil {
		for i, srcParam := range it.Method.Func.Params {
			if i >= len(m.Func.Params) || srcParam == nil || m.Func.Params[i] == nil {
				continue
			}

			m.Func.Params[i].Annotations = e.substituteAnnotations(srcParam.Annotations, env, paramSubs)
			if srcParam.Default != nil {
				m.Func.Params[i].Default = e.substituteExpr(srcParam.Default, env, paramSubs)
			}
		}

		if m.Func.Body != nil {
			e.substituteStmt(m.Func.Body, env, paramSubs)
		}
	}

	td.Methods = append(td.Methods, m)
}

func (e *synthExpander) substituteStmt(st ir.Stmt, env *synthEnv, paramSubs map[string]ir.Expr) {
	if ir.IsNilStmt(st) {
		return
	}

	switch s := st.(type) {
	case *ir.BlockStmt:
		for _, sub := range s.Stmts {
			e.substituteStmt(sub, env, paramSubs)
		}

	case *ir.ReturnStmt:
		for i := range s.Results {
			s.Results[i] = e.substituteExpr(s.Results[i], env, paramSubs)
		}

	case *ir.VarDeclStmt:
		if s.Init != nil {
			s.Init = e.substituteExpr(s.Init, env, paramSubs)
		}

	case *ir.ExprStmt:
		if s.Expr != nil {
			s.Expr = e.substituteExpr(s.Expr, env, paramSubs)
		}

	case *ir.FieldAssignmentStmt:
		if s.Value != nil {
			s.Value = e.substituteExpr(s.Value, env, paramSubs)
		}

	case *ir.MultiAssignmentStmt:
		if s.Value != nil {
			s.Value = e.substituteExpr(s.Value, env, paramSubs)
		}

	case *ir.IndexAssignmentStmt:
		if s.Value != nil {
			s.Value = e.substituteExpr(s.Value, env, paramSubs)
		}

		if s.Index != nil {
			s.Index = e.substituteExpr(s.Index, env, paramSubs)
		}

	case *ir.IfStmt:
		if s.Cond != nil {
			s.Cond = e.substituteExpr(s.Cond, env, paramSubs)
		}

		if s.Then != nil {
			e.substituteStmt(s.Then, env, paramSubs)
		}

		for i := range s.ElseIfs {
			if s.ElseIfs[i].Cond != nil {
				s.ElseIfs[i].Cond = e.substituteExpr(s.ElseIfs[i].Cond, env, paramSubs)
			}

			if s.ElseIfs[i].Then != nil {
				e.substituteStmt(s.ElseIfs[i].Then, env, paramSubs)
			}
		}

		if s.Else != nil {
			e.substituteStmt(s.Else, env, paramSubs)
		}

	case *ir.ForStmt:
		if s.Body != nil {
			e.substituteStmt(s.Body, env, paramSubs)
		}

	case *ir.WhileStmt:
		if s.Cond != nil {
			s.Cond = e.substituteExpr(s.Cond, env, paramSubs)
		}

		if s.Body != nil {
			e.substituteStmt(s.Body, env, paramSubs)
		}
	}
}

func (e *synthExpander) injectCtor(env *synthEnv, outerBindName string, paramSubs map[string]ir.Expr, it *ir.SynthEmitCtor) {
	td := e.outerTypeBind(env, outerBindName)
	if td == nil || it.Ctor == nil {
		if it.Ctor != nil {
			e.pc.Diag.Report(diag.ErrSynthMemberOnNonType, it.Ctor.Span(), "ctor", outerBindName)
		}

		return
	}

	c := cloneCtorDecl(it.Ctor, e.pc.NodeAlloc)
	c.Annotations = e.substituteAnnotations(it.Ctor.Annotations, env, paramSubs)
	for i, srcParam := range it.Ctor.Params {
		if i >= len(c.Params) || srcParam == nil || c.Params[i] == nil {
			continue
		}

		c.Params[i].Annotations = e.substituteAnnotations(srcParam.Annotations, env, paramSubs)
		if srcParam.Default != nil {
			c.Params[i].Default = e.substituteExpr(srcParam.Default, env, paramSubs)
		}
	}

	td.Ctors = append(td.Ctors, c)
}

func cloneCtorDecl(c *ir.CtorDecl, alloc *ir.IdAlloc) *ir.CtorDecl {
	if c == nil {
		return nil
	}

	out := &ir.CtorDecl{
		IsSynthetic: c.IsSynthetic,
		IsShared:    c.IsShared,
	}

	for _, p := range c.Params {
		if p == nil {
			continue
		}

		out.Params = append(out.Params, &ir.FuncParam{
			IsVariadic: p.IsVariadic,
			Name:       ir.NameRef{Name: p.Name.Name, Span: p.Name.Span},
			Type:       cloneTypeRefShallow(p.Type),
			Default:    ir.CloneExpr(p.Default, alloc),
		})
	}

	if c.Body != nil {
		if cloned, ok := ir.CloneStmt(c.Body, alloc).(*ir.BlockStmt); ok {
			out.Body = cloned
		}
	}

	return out
}

func cloneTypeRefShallow(t *ir.TypeRef) *ir.TypeRef {
	if t == nil {
		return nil
	}

	cp := *t
	return &cp
}

func (e *synthExpander) evalWhere(w *ir.SynthBoolExpr, env *synthEnv) bool {
	if w == nil {
		return true
	}

	bind, ok := env.lookup(w.BindName)
	if !ok {
		return false
	}

	v, ok := bind.boolProperty(w.Property)
	if !ok {
		return false
	}

	if w.Negate {
		return !v
	}

	return v
}

func (e *synthExpander) substituteAnnotations(annos []ir.Annotation, env *synthEnv, paramSubs map[string]ir.Expr) []ir.Annotation {
	if len(annos) == 0 {
		return nil
	}

	out := make([]ir.Annotation, 0, len(annos))
	for _, a := range annos {
		na := ir.Annotation{Name: a.Name}

		if len(a.Args) > 0 {
			na.Args = make([]ir.Expr, 0, len(a.Args))
			for _, arg := range a.Args {
				na.Args = append(na.Args, e.substituteExpr(arg, env, paramSubs))
			}
		}

		out = append(out, na)
	}

	return out
}

func (e *synthExpander) substituteExpr(ex ir.Expr, env *synthEnv, paramSubs map[string]ir.Expr) ir.Expr {
	if ex == nil {
		return nil
	}

	switch v := ex.(type) {
	case *ir.VarRef:
		if sub, ok := paramSubs[v.Ref.Name]; ok {
			return ir.CloneExpr(sub, e.pc.NodeAlloc)
		}

		return ir.CloneExpr(v, e.pc.NodeAlloc)
	case *ir.FieldAccessExpr:
		if vr, ok := v.Expr.(*ir.VarRef); ok && len(v.Fields) == 1 {
			if bind, ok := env.lookup(vr.Ref.Name); ok && bind.kind != bindUnknown {
				if str, ok := bind.stringProperty(v.Fields[0].Name); ok {
					return &ir.LitString{Value: str}
				}
			}
		}

		return ir.CloneExpr(v, e.pc.NodeAlloc)
	case *ir.BinaryExpr:
		return &ir.BinaryExpr{
			Op:    v.Op,
			Left:  e.substituteExpr(v.Left, env, paramSubs),
			Right: e.substituteExpr(v.Right, env, paramSubs),
		}

	case *ir.GroupedExpr:
		return &ir.GroupedExpr{Expr: e.substituteExpr(v.Expr, env, paramSubs)}

	case *ir.StringTemplateExpr:
		out := &ir.StringTemplateExpr{}

		for _, part := range v.Parts {
			out.Parts = append(out.Parts, ir.StringTemplatePart{
				Lit:  part.Lit,
				Expr: e.substituteExpr(part.Expr, env, paramSubs),
			})
		}

		return out
	case *ir.TenaryExpr:
		return &ir.TenaryExpr{
			Cond: e.substituteExpr(v.Cond, env, paramSubs),
			Then: e.substituteExpr(v.Then, env, paramSubs),
			Else: e.substituteExpr(v.Else, env, paramSubs),
		}

	case *ir.UnaryExpr:
		return &ir.UnaryExpr{Op: v.Op, Expr: e.substituteExpr(v.Expr, env, paramSubs)}

	case *ir.CoalesceExpr:
		return &ir.CoalesceExpr{
			Left:    e.substituteExpr(v.Left, env, paramSubs),
			Default: e.substituteExpr(v.Default, env, paramSubs),
		}

	case *ir.FuncCallExpr:
		out := &ir.FuncCallExpr{Callee: e.substituteExpr(v.Callee, env, paramSubs)}

		for _, a := range v.Args {
			out.Args = append(out.Args, ir.FuncCallArg{Name: a.Name, Expr: e.substituteExpr(a.Expr, env, paramSubs)})
		}

		return out
	}

	return ir.CloneExpr(ex, e.pc.NodeAlloc)
}

func (e *synthExpander) diagnoseRemaining(sites []synthBind) {
	for _, site := range sites {
		anns := site.annotations()
		if anns == nil {
			continue
		}

		filtered := (*anns)[:0]
		for _, a := range *anns {
			if _, ok := e.registry[a.Name.Name]; ok {
				e.pc.Diag.Report(diag.ErrSynthRecursionLimit, a.Name.Span, a.Name.Name, synthExpansionDepthLimit)
				continue
			}

			filtered = append(filtered, a)
		}

		*anns = filtered
	}
}

func varDeclLabel(s *ir.VarDeclStmt) string {
	if s == nil || len(s.Targets) == 0 {
		return "?"
	}

	if len(s.Targets) == 1 {
		if s.Targets[0].Name != nil {
			return s.Targets[0].Name.Name
		}

		return "_"
	}

	parts := make([]string, 0, len(s.Targets))
	for _, t := range s.Targets {
		if t.Name != nil {
			parts = append(parts, t.Name.Name)
		} else {
			parts = append(parts, "_")
		}
	}

	joined := ""
	for i, p := range parts {
		if i > 0 {
			joined += ","
		}

		joined += p
	}

	return joined
}
