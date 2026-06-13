package ir

// SynthTargetKind is the kind of HIR node a `synth` declaration attaches to. The Sova surface is `synth Name on type T { ... }` / `... on field F { ... }` / `... on func G { ... }` / `... on param P { ... }` / `... on let L { ... }`. The kind binds the per-target accessors the synth body will (in later phases) iterate against, and constrains which annotation use sites the expander matches the synth against — a `synth ... on field F { ... }` only fires for annotations attached to fields, never to types or functions.
type SynthTargetKind int

const (
	SynthTargetUnknown SynthTargetKind = iota
	SynthTargetType
	SynthTargetField
	SynthTargetFunc
	SynthTargetParam
	SynthTargetLet
	SynthTargetMethod
	SynthTargetCtor
)

// String renders the Sova-surface keyword the user wrote for this kind. Mirrors `synthTargetKind` in the grammar — keep in sync.
func (k SynthTargetKind) String() string {
	switch k {
	case SynthTargetType:
		return "type"
	case SynthTargetField:
		return "field"
	case SynthTargetFunc:
		return "func"
	case SynthTargetParam:
		return "param"
	case SynthTargetLet:
		return "let"
	case SynthTargetMethod:
		return "method"
	case SynthTargetCtor:
		return "ctor"
	}
	return "?"
}

// SynthDeclStmt is the top-level declaration of a custom annotation: `synth Name(params) on <kind> Bind { ... body ... }`. Lives only in `on synth` files. The expander reads every SynthDeclStmt across the build into a global registry keyed by Name, then walks every annotation use-site in the regular HIR; matching annotations are expanded by evaluating the synth body against the use-site as the target. The body is a list of `SynthBodyItem` values (`emit on`, `emit append to`, `for ... in ...`), interpreted by `expand_synths` against a bind environment seeded with the synth's target.
type SynthDeclStmt struct {
	node
	docBase
	Name   NameRef
	Params []*FuncParam
	Target SynthTarget
	Body   []SynthBodyItem
}

func (*SynthDeclStmt) stmtNode() {}

// SynthTarget captures the `on <kind> <bind>` clause. `BindName` is the symbolic name the user chose for the annotated entity inside the synth body (e.g. the `F` in `on field F`); the expander binds this name to the actual HIR node when running the body.
type SynthTarget struct {
	Kind     SynthTargetKind
	BindName string
}

// SynthBodyItem is the closed sum of clauses that can appear inside a synth body: emit-on (annotation splice), emit-append-to (registry append), and for-loop (iterate a target collection like `T.fields`). All three implement this marker so the body slice can hold them uniformly without boxing through `any`.
type SynthBodyItem interface {
	synthBodyItem()
}

// SynthEmitOn is one `emit on <scope> { ... }` clause: a list of annotations to splice onto whatever the scope name resolves to (the synth's target, a for-loop's iteration variable, or any other bind in the env at the time the clause is interpreted).
type SynthEmitOn struct {
	node
	Scope           string
	AnnotationEmits []Annotation
}

func (*SynthEmitOn) synthBodyItem() {}

// SynthEmitAppend is one `emit append to <registry> { <expr> }` clause: evaluate `<expr>` (with the current bind env's substitutions) and append the result to the named registry. Registries are realised as `[]ir.Expr` slices stashed in `PassContext.Cache` under a stable prefix so downstream tooling (codegen, custom generators) can read them by name.
type SynthEmitAppend struct {
	node
	Registry string
	Fragment Expr
}

func (*SynthEmitAppend) synthBodyItem() {}

// SynthForStmt is one `for <loopVar> in <bind>.<member> [where <pred>] { <body> }` clause: iterate the collection referenced by `<bind>.<member>` (e.g. `T.fields`, `T.methods`, `F.params`), bind each element to `<loopVar>`, optionally filter by `<pred>`, and recursively interpret `<body>` for each surviving element. Within the body, `emit on <loopVar>` resolves to the current iteration element so per-element annotation splicing falls out naturally.
type SynthForStmt struct {
	node
	LoopVar  string
	BindName string
	Member   string
	Where    *SynthBoolExpr
	Body     []SynthBodyItem
}

func (*SynthForStmt) synthBodyItem() {}

// SynthBoolExpr is the minimal predicate language used by `where` clauses: an optional `!` negation in front of a `<bind>.<property>` boolean property access (e.g. `f.isShared`, `!m.isPrivate`). The property set is hard-coded per target kind in the interpreter; comparison operators and conjunction/disjunction are intentionally absent to keep the surface tight — combining filters is done by nesting `for ... where ...` loops.
type SynthBoolExpr struct {
	node
	Negate   bool
	BindName string
	Property string
}

// SynthEmitField is one `emit field <annotations> <modifiers> <name>: <type> [= <default>]` clause: a new field to inject into the target type. Only valid when the surrounding synth's target is `type`; on any other target the interpreter reports a diagnostic and drops the clause. The interpreter clones the field on every expansion so each `@SynthName` use site gets independent field nodes (fresh IDs, no aliasing).
type SynthEmitField struct {
	node
	Field *TypeField
}

func (*SynthEmitField) synthBodyItem() {}

// SynthEmitMethod is one `emit method <annotations> <modifiers> <name>(<params>): <ret> { <body> }` clause: a new method to inject into the target type. Only valid for `on type` synths. Synth params are *not* substituted into the method body's statement tree (only into annotation arg expressions on the method's own decorations and into the method param defaults) — the body's symbol references are resolved by `bind_declare` at the use-site package, so the method can call `this.field`, peer methods on the target type, and anything imported by the target's package, but it cannot directly reference the synth's params. Authors that need param-driven body content should fold the param into an annotation arg (which does substitute) and read it back from there in a future hook.
type SynthEmitMethod struct {
	node
	Method *TypeMethodDecl
}

func (*SynthEmitMethod) synthBodyItem() {}

// SynthEmitCtor is one `emit ctor(<params>) { <body> }` clause: a new constructor to inject into the target type. Same body-substitution semantics as `SynthEmitMethod`. Multiple `emit ctor` clauses are allowed; the target type ends up with one extra ctor per clause per expansion, in the order they appear in the synth body.
type SynthEmitCtor struct {
	node
	Ctor *CtorDecl
}

func (*SynthEmitCtor) synthBodyItem() {}
