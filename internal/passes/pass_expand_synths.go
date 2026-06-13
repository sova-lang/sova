package passes

import (
	"sova/internal/diag"
	"sova/internal/ir"
)

// SynthRegistryCacheKey holds `map[string]*ir.SynthDeclStmt` — every `synth` declaration the build saw, keyed by the synth's user-facing name. Populated by `PassExpandSynths` once at the start of its run; downstream tooling (the LSP, `sova synth`, future passes) consumes it through the cache rather than re-walking packages.
const SynthRegistryCacheKey = "synth_registry"

// SynthEmittedRegistryPrefix is the cache-key prefix under which `emit append to <name>` clauses accumulate fragments. Each registry is stored as `[]ir.Expr` under `SynthEmittedRegistryPrefix + name`; codegen and external generators read these slices by name to materialise things like a routing table or a serialised metadata blob without re-running the expander.
const SynthEmittedRegistryPrefix = "synth_reg:"

// synthExpansionDepthLimit caps how many consecutive expansion rounds may fire over the whole build before the expander bails. Each round walks every annotation site once; a chain like `@A` → `@B` → `@C` → builtin is three rounds. The cap is generous enough that any real chain succeeds while still catching obvious cycles (`synth A on field F { emit on F { @A } }`).
const synthExpansionDepthLimit = 16

// PassExpandSynths interprets `synth` declarations and rewrites every annotation use-site in the regular HIR to the concrete annotations the synth body emits. Targets covered: `type`, `field`, `func`, `method`, `ctor`, `param`, `let`. Body clauses understood: `emit on <bind>` (annotation splice; the bind resolves to the synth target or any for-loop iteration variable), `emit append to <reg> { <expr> }` (append the substituted expression to a named registry slice in the compiler cache), `for <var> in <bind>.<member> [where <pred>] { <body> }` (iterate a target collection — `T.fields`, `T.methods`, `T.ctors`, `F.params` — with an optional boolean property filter and a recursively-evaluated body). Synth params and loop variables are substituted into emitted annotation arg expressions via the same walker, so `@structTag("col:" + f.name)` works whether `f` is a synth param or a `for f in T.fields` binding. The build-wide round counter is the only fixpoint mechanism; a runaway-loop synth (a self-emitter) hits the cap and is reported via `ErrSynthRecursionLimit` with the use-site annotation dropped.
//
// The pass runs after import resolution but before `bind_declare`. By the time the binder, name resolver, type inferrer, and `fold_annotations` get the HIR, every user-written `@CustomName` has been substituted with the literal annotations the synth body emits — those downstream passes see no synth-related state at all, so the integration cost on the rest of the pipeline is zero.
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

// buildRegistry walks every `on synth` package for top-level `SynthDeclStmt`s and indexes them by their user-facing name. Conflicts (two synths with the same name) produce a diagnostic at the second declaration's source position; the first wins so subsequent expansions stay deterministic. Synth declarations in non-synth files are skipped silently — the grammar accepts them anywhere but the language only sanctions `on synth` packages.
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

// synthBindKind is the kind of HIR entity a name in the expander's env points to. Stays inside the package because the env is purely an interpreter-local construct; nothing outside `expand_synths` needs to reason about a "bind".
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

// synthBind is one entry in the interpreter env: a tagged pointer to an HIR node the synth body is currently aware of. Exactly one of the typed fields is populated, selected by `kind`. Kept as a value type so `env.binds` map updates don't aliasing-leak.
type synthBind struct {
	kind     synthBindKind
	typeDecl *ir.TypeDeclStmt
	field    *ir.TypeField
	funcDecl *ir.FuncDeclStmt
	method   *ir.TypeMethodDecl
	ctor     *ir.CtorDecl
	param    *ir.FuncParam
	let      *ir.VarDeclStmt
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

// label is a diagnostic-friendly identifier for the bind, used in error messages where the user needs to know which annotation site is being talked about. Walks the declaration chain just enough to produce something like `User.id` for a field or `User.display` for a method.
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

// iterable returns the sub-binds of a member collection (e.g. `T.fields`). The returned slice is fresh; mutating it doesn't mutate the IR. Returns ok=false when the member name isn't a known collection for the bind's kind — the caller turns that into a diagnostic.
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

// stringProperty resolves a `<bind>.<name>` access that is expected to yield a string (used inside annotation arg expressions). Currently exposed: `name` on everything that has one, plus `type` on fields (the textual type) and `member` placeholder for future expansion. Returns ok=false when the property isn't known so the substituter can fall back to a clone-as-is path (which fold_annotations will then reject as non-const, surfacing a useful error to the user).
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

// boolProperty resolves a `<bind>.<name>` access that yields a bool (used by `where` predicates). Property names are camelCase Sova-style: `isShared`, `isPrivate`, `isExtern`, `isAsync`, `isWired`, `isConst`, `isVariadic`. Unknown names return ok=false; the caller treats that as "predicate is unsatisfied" so a `where f.notAProperty` silently drops every element, but the surface error surfaces because the loop body emits nothing.
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

// typeRefString renders a TypeRef to its source-form name. Best-effort: for custom types we use CustomName (qualifier prepended if present); for everything else we currently return the empty string. Good enough for the common GORM-style use of "name + type token" — and when a synth genuinely needs richer type info, the right move is to thread the typed `TypID` through (which only stabilises after `infer_types`, well past where this pass runs).
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

// collectSites enumerates every annotatable HIR node across all non-synth packages. Each entry is a `synthBind` whose `annotations()` returns a pointer into the underlying IR slice, so the interpreter can replace/append entries in place. The traversal mirrors the grammar's annotation positions: type decls, their fields/methods/ctors, those members' params, and top-level funcs/lets with their params.
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
			for _, st := range f.Hir.Statements {
				switch s := st.(type) {
				case *ir.TypeDeclStmt:
					out = append(out, synthBind{kind: bindType, typeDecl: s})
					for _, fld := range s.Fields {
						if fld != nil {
							out = append(out, synthBind{kind: bindField, field: fld})
						}
					}
					for _, m := range s.Methods {
						if m == nil {
							continue
						}
						out = append(out, synthBind{kind: bindMethod, method: m})
						if m.Func != nil {
							for _, prm := range m.Func.Params {
								if prm != nil {
									out = append(out, synthBind{kind: bindParam, param: prm})
								}
							}
						}
					}
					for _, c := range s.Ctors {
						if c == nil {
							continue
						}
						out = append(out, synthBind{kind: bindCtor, ctor: c})
						for _, prm := range c.Params {
							if prm != nil {
								out = append(out, synthBind{kind: bindParam, param: prm})
							}
						}
					}
				case *ir.FuncDeclStmt:
					out = append(out, synthBind{kind: bindFunc, funcDecl: s})
					for _, prm := range s.Params {
						if prm != nil {
							out = append(out, synthBind{kind: bindParam, param: prm})
						}
					}
				case *ir.VarDeclStmt:
					out = append(out, synthBind{kind: bindLet, let: s})
				}
			}
		}
	}
	return out
}

// runRound processes every site's annotation list once. Returns true if any site mutated, which the outer fixpoint loop uses as its "another round needed" signal. Each match against the registry is consumed in this round; the same `@SynthName` annotation can't fire twice in one round because the matched entry is removed from the list as it's expanded.
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

// expandSite walks one annotation list and replaces each synth-match with the synth body's emit-on-target output. Non-synth annotations pass through. Returns true iff the list was actually rewritten.
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
		for i, prm := range sd.Params {
			if prm == nil || prm.Name.Name == "" {
				continue
			}
			paramSubs[prm.Name.Name] = a.Args[i]
		}
		emitted := e.interpretBody(sd.Body, env, sd.Target.BindName, paramSubs)
		out = append(out, emitted...)
	}
	*anns = out
	return true
}

// paramSlotKey is reserved for a future change that wants to put synth-param substitutions inside the env itself (instead of a side-table). Kept as a no-op marker today so the rename lands cleanly.
func paramSlotKey(name string) string { return "@param:" + name }

// interpretBody runs a synth body in env and returns the annotations that should be spliced onto the use-site's annotation list (those emitted on `outerBindName`). All other emissions — to other targets via `emit on <otherBind>`, or to registries via `emit append to` — happen as side effects on the IR or the cache.
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

// outerTypeBind resolves the surrounding synth's target bind by name and returns the bound TypeDeclStmt if (and only if) the target is a type. Used as the gate for member-injection clauses (`emit field`, `emit method`, `emit ctor`) — those only make sense on `on type T` synths, and we refuse to inject anywhere else so the user gets a clean diagnostic instead of mysterious downstream breakage.
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
	}
	td.Methods = append(td.Methods, m)
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

// cloneCtorDecl produces a fresh CtorDecl with new node IDs for the params and body. Mirrors `CloneTypeField` / `CloneTypeMethodDecl` (which both live in `ir/clone.go`); kept local here because the broader IR package doesn't currently need a `CloneCtorDecl` export, and pulling it across the package boundary just for one synth callsite would be unnecessary surface widening.
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

// cloneTypeRefShallow copies a TypeRef's identity-bearing fields without recursing into composite shapes. Sufficient for synth-injected param types in V1 (the common case is named primitives and named user types — `int`, `string`, `User`); generics / tuples / function types in injected param positions stay shallow-cloned and will share node-IDs with the synth body, which is benign because nothing downstream mutates type-ref nodes after `infer_types` reads them.
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

// substituteExpr deep-clones an expression with two kinds of substitution applied: (1) `*ir.VarRef` whose name is a synth param key gets replaced by a deep clone of the corresponding use-site arg; (2) `*ir.FieldAccessExpr` of shape `<loopVar>.<property>` where `<loopVar>` is bound in env to a sub-bind (a field, method, param, …) gets replaced by a `*ir.LitString` carrying the property's resolved value. Anything else recurses or clones verbatim via `ir.CloneExpr`. This is what makes `@structTag("col:" + f.name)` produce a real const-foldable concatenation after expansion.
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
	}
	return ir.CloneExpr(ex, e.pc.NodeAlloc)
}

// diagnoseRemaining is the post-fixpoint cleanup. After `synthExpansionDepthLimit` rounds, any annotation still matching a registered synth is part of a cycle (or chain too deep for the cap); each is reported with `ErrSynthRecursionLimit` and removed so the downstream passes don't crash on a "phantom" synth annotation.
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

// varDeclLabel returns a diagnostic-friendly identifier for a `let`/`const` declaration. Single-target decls use the bare name; tuple-destructuring decls join the bound names with `,` so the diagnostic still points at something the user wrote.
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
