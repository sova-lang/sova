package ir

import (
	"sova/internal/diag"
	"strings"
)

// ==== Internal stuff ====

// node is a small internal struct for less boilerplate.
type node struct {
	id   NodeID
	span diag.TextSpan
}

func (n *node) ID() NodeID          { return n.id }
func (n *node) Span() diag.TextSpan { return n.span }

// exprBase is a small internal struct for expression nodes for less boilerplate.
type exprBase struct{ typ TypID }

func (e *exprBase) GetType() TypID  { return e.typ }
func (e *exprBase) SetType(t TypID) { e.typ = t }

// docBase is the shared field embedded by every declarable node so the
// pre-decl doc-comment block (`///` or `/** */`) can be attached at visitor
// time and surfaced later by hover / completion / signature help. The Doc
// string is already trimmed and joined - leading ` *` from block comments
// and leading `///` from line comments have been stripped.
type docBase struct{ doc string }

// GetDoc returns the attached doc comment, or "" if none was present.
func (d *docBase) GetDoc() string { return d.doc }

// SetDoc replaces the attached doc comment. Used by the visitor as it walks
// the hidden-channel tokens preceding each declaration.
func (d *docBase) SetDoc(s string) { d.doc = s }

// ==== File & Header ====

type SideKind int // SideKind represents the kind of a side in a binary expression.

const (
	SideUnknown SideKind = iota
	SideFrontend
	SideBackend
	SideShared
	SideTest
	SideSynth
)

// SideSpec represents the specification on which side the code is supposed to run.
type SideSpec struct {
	Kind   SideKind
	Target string // Optional target for backend microservices. If unused, it is empty.
}

// PackagePath represents a path of a package. Each element is normally joined with a slash ('/'). Each element can be a valid identifier.
type PackagePath []string

// String returns the string representation of the package path.
func (p PackagePath) String() string {
	return strings.Join(p, "/")
}

// File represents one complete Sova script.
type File struct {
	node
	Path       string      // Path is the path to the file in the filesystem. Can be empty, if not specified.
	Package    PackagePath // Package is the package path of the file, e.g., "sova/ir".
	Side       SideSpec    // Side is the side specification of the file, e.g., frontend, backend, or shared.
	Statements []Stmt      // Statements are the statements in the file.
}

func (*File) declNode() {}

// ==== Statements ====

type BlockStmt struct {
	node
	Stmts []Stmt
}

func (*BlockStmt) stmtNode() {}

type VarDeclStmt struct {
	node
	docBase
	IsConst     bool            // LET=false, CONST=true
	Targets     []VarDeclTarget // Variable declaration targets (can be multiple for tuple destructuring)
	Init        Expr
	IsWired     bool         // IsWired marks a top-level declaration as wired: the backend exposes it via a GET endpoint, the frontend gets a fetch-stub.
	Wire        *WireSpec    // Wire carries the resolved transport metadata for wired vars/const.
	Annotations []Annotation // Annotations are the `@name(args)` decorations applied to this declaration. `@reactive wire let` triggers the broadcast-on-mutate path; other annotations may be added by libraries.
}

type VarDeclTarget struct {
	Name    *NameRef // Name is the variable name. Nil for discard '_'
	TypeAnn *TypeRef // Type annotation for the variable. May be nil.
}

func (*VarDeclStmt) stmtNode() {}

type ExprStmt struct {
	node
	Expr Expr
}

func (*ExprStmt) stmtNode() {}

// FieldAssignmentStmt represents an assignment to a field-access target like `this.x = value` or `obj.field += value`.
type FieldAssignmentStmt struct {
	node
	Receiver NameRef
	Fields   []FieldName
	Op       Op
	Value    Expr
}

func (*FieldAssignmentStmt) stmtNode() {}

type MultiAssignmentStmt struct {
	node
	Targets []AssignmentTarget // Left-hand side targets (can include discard '_')
	Value   Expr               // Right-hand side expression (should evaluate to a tuple)
}

type AssignmentTarget struct {
	Name *NameRef // Name is the variable name. Nil for discard '_'
}

func (*MultiAssignmentStmt) stmtNode() {}

type IfStmt struct {
	node
	Cond    Expr
	Then    *BlockStmt
	ElseIfs []ElseIfBranch
	Else    *BlockStmt
}

type ElseIfBranch struct {
	Cond Expr
	Then *BlockStmt
}

func (*IfStmt) stmtNode() {}

type SwitchStmt struct {
	node
	Expr    Expr
	Cases   []SwitchCase
	Default []Stmt
}

type SwitchCase struct {
	Values []Expr
	Stmts  []Stmt
}

func (*SwitchStmt) stmtNode() {}

type BreakStmt struct {
	node
	Depth int // Depth indicates how many nested loops to break out of. 1 means the innermost loop.
}

func (*BreakStmt) stmtNode() {}

type ContinueStmt struct {
	node
	Depth int // Depth indicates how many nested loops to continue. 1 means the innermost loop.
}

func (*ContinueStmt) stmtNode() {}

type ReturnStmt struct {
	node
	Results []Expr // Results are the expressions to return. Empty for void returns. Can have multiple for multi-value returns.
}

func (*ReturnStmt) stmtNode() {}

type GuardStmt struct {
	node
	Cond    Expr   // Cond is the condition expression.
	Returns []Expr // Returns are the expressions to return if the condition is false. Empty for void returns. Can have multiple for multi-value returns.
}

func (*GuardStmt) stmtNode() {}

type ForConditionType int

const (
	ForCondInfinite ForConditionType = iota // Infinite loop
	ForCondInt                              // Integer-based loop (e.g., for i = 0; i < 10; i++)
	ForCondRange                            // Range-based loop (e.g., for item in collection)
	ForCondIn                               // In-based loop (e.g., for key, value in map
)

type ForStmt struct {
	node
	CondType  ForConditionType  // CondType is the type of the for loop condition.
	CondInt   *ForCondIntDecl   // CondInt is the integer-based loop condition. Used if CondType is ForCondInt.
	CondRange *ForCondRangeDecl // CondRange is the range-based loop condition. Used if CondType is ForCondRange.
	CondIn    *ForCondInDecl    // CondIn is the in-based loop condition. Used if CondType is ForCondIn.
	Body      *BlockStmt        // Body is the body of the for loop.
}

type ForCondIntDecl struct {
	Init *VarDeclStmt // Init is the initialization statement. The variable kind is always let.
	Cond Expr         // Cond is the condition expression.
	Post Expr         // Post is the post-iteration expression.
}

type ForCondRangeDecl struct {
	RangeVar   NameRef // RangeVar is the variable that holds the current item in the iteration. The variable kind is always let.
	RangeStart Expr    // RangeStart is the expression representing the collection to iterate over.
	RangeEnd   Expr    // RangeEnd is the optional end expression for ranges.
}

type ForCondInDecl struct {
	InFirstVar  NameRef  // InFirstVar is the first variable in the in-based loop (e.g., key in map).
	InSecondVar *NameRef // InSecondVar is the second variable in the in-based loop (e.g., value in map, index in array). May be nil.
	InThirdVar  *NameRef // InThirdVar is the third variable in the in-based loop (e.g., index in map). May be nil.
	IterExpr    Expr     // IterExpr is the expression representing the collection to iterate over.
	IterNextSym SymID    // IterNextSym is the resolved `next()` method symbol when iterating a user-defined iterable type (struct/interface with `func next(): option<T>`). Zero when iterating slices/arrays/maps.
}

func (*ForStmt) stmtNode() {}

type WhileStmt struct {
	node
	Cond Expr       // Cond is the condition expression.
	Body *BlockStmt // Body is the body of the while loop.
}

func (*WhileStmt) stmtNode() {}

type FuncDeclStmt struct {
	node
	docBase
	Side        *SideSpec       // Side is the side specification of the function, e.g., frontend, backend, or shared. Might be nil, which will default to the file side.
	Name        NameRef         // Name is the name of the function.
	TypeParams  []TypeParamDecl // TypeParams holds the generic type parameter names + their interface/mixin constraints. Empty for non-generic functions.
	Params      []*FuncParam    // Params are the parameters of the function.
	ReturnType  *TypeRef        // ReturnType is the return type of the function. May be nil for void functions.
	Body        *BlockStmt      // Body is the body of the function.
	IsAsync     bool            // IsAsync is true when the function is async, either explicitly declared or auto-lifted because it transitively calls an async function.
	IsWired     bool            // IsWired marks the function as wired: the backend hosts the implementation, the frontend calls it over the configured transport.
	Wire        *WireSpec       // Wire carries the resolved transport metadata for IsWired functions; nil for non-wired ones.
	Annotations []Annotation    // Annotations are the `@name(args)` decorations applied to this declaration.
}

// TypeParamDecl is a single generic type parameter declaration like `T: Comparable + Hashable with Logger`. ImplementsConstraints are interface names that the type argument must implement; WithConstraints are mixin names that the type argument must mix in. Empty constraint lists make the parameter fully unconstrained.
type TypeParamDecl struct {
	Name                  string
	ImplementsConstraints []NameRef
	WithConstraints       []NameRef
}

// WireGroupStmt is a transient IR node for `wire(opts) { ... }` blocks. The visitor flattens these into their child statements, applying the group's wire spec to each inner declaration without overriding any per-decl options the inner decl set itself. Consumers should not normally see WireGroupStmt after the visitor pass.
type WireGroupStmt struct {
	node
	Wire  *WireSpec
	Stmts []Stmt
}

func (*WireGroupStmt) stmtNode() {}

// WireRulesetStmt declares a named bundle of wire options. Other wire declarations can apply this bundle via the `wire:<name>` syntax.
type WireRulesetStmt struct {
	node
	Name    string
	Options map[string]WireOptValue
}

func (*WireRulesetStmt) stmtNode() {}

// WireSpec describes how a wired function maps onto a transport endpoint. Method and Path may be derived by the analyze_wire pass from the function name and package, or set explicitly via `wire(method: ..., path: ...)`.
type WireSpec struct {
	Method        string                  // Method is the HTTP method (GET, POST, PUT, PATCH, DELETE).
	Path          string                  // Path is the URL path, including parameter placeholders like ":id".
	PathArgs      []string                // PathArgs lists the parameter names that bind to path placeholders, in URL order.
	Options       map[string]WireOptValue // Options carries raw wire(...) options for downstream consumers.
	RequireAuthN  bool                    // RequireAuthN is true when the handler must reject unauthenticated requests with 401 Unauthorized.
	RequiredRoles []string                // RequiredRoles lists role names the session must hold; missing roles produce 403 Forbidden.
	Ruleset       string                  // Ruleset is the optional named ruleset reference; resolved by analyze_wire.
	Transport     string                  // Transport is "http", "ws", or "sse" (resolved from `wire(transport: ...)`). Empty means use the side-default ("http" for backend wires, "ws" for frontend wires). Validated by analyze_wire; an invalid value produces a diagnostic.
}

// WireOptValue is the resolved value of a single wire(...) option. Only the field that matches the literal kind is set.
type WireOptValue struct {
	Str  string
	Int  int64
	Bool bool
	Strs []string
	Kind WireOptValueKind
}

// WireOptValueKind enumerates the supported wire option literal kinds.
type WireOptValueKind int

const (
	WireOptUnknown WireOptValueKind = iota
	WireOptString
	WireOptInt
	WireOptBool
	WireOptStringArray
)

func (*FuncDeclStmt) stmtNode() {}

type FuncParam struct {
	node
	IsVariadic  bool         // IsVariadic indicates if the parameter is variadic.
	Name        NameRef      // Name is the name of the parameter.
	Type        *TypeRef     // Type is the type of the parameter.
	Default     Expr         // Default is the default value of the parameter. May be nil.
	Annotations []Annotation // Annotations are the `@name(args)` decorations applied to this parameter (lowered to built-ins by `expand_synths` before any downstream pass sees them).
}

func (*FuncParam) stmtNode() {}

type ExternDeclStmt struct {
	node
	docBase
	Module          *string              // Module is the optional module import (e.g., "lodash"). May be nil.
	Version         string               // Version is the optional Go-module version pin parsed from `extern "path@version"`. Empty when no pin was given or when the surface form did not include `@`. Currently honoured only by the Go backend; the JS side ignores it.
	IsDefaultImport bool                 // IsDefaultImport selects the JS `import X from "mod"` form instead of the namespace `import * as X from "mod"` default; ignored for Go where modules are always namespace-imported.
	Funcs           []*ExternFunc        // Funcs are the extern function declarations.
	Vars            []*ExternVar         // Vars are the extern variable/const declarations.
	Types           []*TypeDeclStmt      // Types are bound host-language type declarations (Go struct or JS class, depending on side).
	Interfaces      []*InterfaceDeclStmt // Interfaces are bound host-language interface declarations (Go-side only in practice).
}

func (*ExternDeclStmt) stmtNode() {}

type ExternFunc struct {
	node
	docBase
	Name       NameRef         // Name is the name of the extern function.
	TypeParams []TypeParamDecl // TypeParams are the generic type parameters of the extern function.
	Params     []*FuncParam    // Params are the parameters of the extern function.
	ReturnType *TypeRef        // ReturnType is the return type of the extern function. May be nil for void functions.
	Mapping    *ExternMapping  // Mapping specifies how to map this function to native code.
	IsAsync    bool            // IsAsync marks the extern function as asynchronous (callers will auto-lift).
}

type ExternVar struct {
	node
	docBase
	IsConst bool           // IsConst indicates if this is a const (true) or let (false).
	Name    NameRef        // Name is the name of the extern variable.
	Type    *TypeRef       // Type is the type of the extern variable.
	Mapping *ExternMapping // Mapping specifies how to map this variable to native code.
}

type ExternMapping struct {
	Simple *string                   // Simple mapping (e.g., "alert"). Nil if shared mapping is used.
	Shared map[SideKind]*SideMapping // Shared mapping with per-side definitions. Nil if simple mapping is used.
}

type SideMapping struct {
	Module     *string // Module is the optional module for this side (e.g., "fmt" for backend). May be nil.
	Version    string  // Version is the optional Go-module version pin parsed from `backend("path@version")`. Empty when no pin was given. Currently honoured only on the backend side.
	NativeFunc string  // NativeFunc is the native function call (e.g., "fmt.Println").
}

// Annotation represents a `@name(args)` decoration attached to a declaration. The Args are arbitrary Sova expressions; a post-parse pass folds them to compile-time constants. ResolvedArgs is populated by that pass and is what downstream consumers (struct-tag emission, route hints, etc.) read.
type Annotation struct {
	Name         NameRef
	Args         []Expr
	ResolvedArgs []AnnotationValue
}

// AnnotationValueKind enumerates the kinds of values an annotation argument can fold to.
type AnnotationValueKind int

const (
	AnnotationValueUnknown AnnotationValueKind = iota
	AnnotationValueString
	AnnotationValueInt
	AnnotationValueBool
)

// AnnotationValue is the resolved value of one annotation argument after const folding.
type AnnotationValue struct {
	Kind AnnotationValueKind
	Str  string
	Int  int64
	Bool bool
}

// TypeDeclStmt represents a user-defined record type declaration. When IsExtern is true the declaration only describes the shape of a host-language type; no code is emitted at the declaration site.
type TypeDeclStmt struct {
	node
	docBase
	Annotations  []Annotation
	Name         NameRef
	TypeParams   []TypeParamDecl // TypeParams holds the generic type parameter names + their interface/mixin constraints. Empty for non-generic types.
	Implements   []NameRef
	MixedIn      []NameRef
	IsExtern     bool
	ExternModule string
	Fields       []*TypeField
	Ctors        []*CtorDecl
	Methods      []*TypeMethodDecl
	Casts        []*CastDecl
}

// ImportStmt represents an `import "package/path"` directive that brings another package's symbols into scope under the package's last path segment. An optional `using` clause exposes specific (or all) exported symbols of the imported package into the current file's scope without qualification.
type ImportStmt struct {
	node
	Path      PackagePath
	Alias     string
	UsingAll  bool
	UsingList []string
}

func (*ImportStmt) stmtNode() {}

// TestDeclStmt represents a `test "name" { ... }` declaration in an `on test` file. Collected by the `test_discovery` pass into the build's TestRegistry; the test driver codegen emits one entry per TestDeclStmt registered. Tags are the optional `tag: "slow", "db"` labels attached to the decl; the CLI's `--tag` filter matches against the union of this test's Tags and the Tags of every enclosing group.
type TestDeclStmt struct {
	node
	Name     string
	Body     *BlockStmt
	Parallel bool
	Tags     []string
}

func (*TestDeclStmt) stmtNode() {}

// GroupDeclStmt represents a `group "name" { ... }` container that nests tests, sub-groups, and setup/teardown hooks. Groups serve as both a reporting structure (nested test name path) and a scope for shared setup/teardown. Tags attach to every test descendant of the group for `--tag` filtering.
type GroupDeclStmt struct {
	node
	Name     string
	Body     []Stmt
	Parallel bool
	Tags     []string
}

func (*GroupDeclStmt) stmtNode() {}

// SetupStmt represents a `setup { ... }` or `setupAll { ... }` hook inside a group (or at file top-level). `IsAll == true` means run once per group; false means run before each test in the group.
type SetupStmt struct {
	node
	IsAll bool
	Body  *BlockStmt
}

func (*SetupStmt) stmtNode() {}

// TeardownStmt represents a `teardown { ... }` or `teardownAll { ... }` hook. Same semantics as SetupStmt but runs after the test(s).
type TeardownStmt struct {
	node
	IsAll bool
	Body  *BlockStmt
}

func (*TeardownStmt) stmtNode() {}

// AssertStmt represents the `assert <expr>` keyword. The runner evaluates the expression at runtime and reports a structured failure (file:line + expression source text + sub-expression values) when it is false. Multiple failed asserts in one test accumulate - the test continues until end-of-body.
type AssertStmt struct {
	node
	Expr Expr
}

func (*AssertStmt) stmtNode() {}

// AsSessionStmt represents the `asSession("name") { ... }` block used in E2E tests to scope the current session perspective. An empty Name (`""`) means an anonymous fresh session created on entry to the block. The block body executes on the Go-side test driver with the named session installed as the current `@`; the JS side currently ignores the wrapper.
type AsSessionStmt struct {
	node
	Name string
	Body *BlockStmt
}

func (*AsSessionStmt) stmtNode() {}

// GoStmt represents `go <call>` or `go { ... }` - spawns a task that runs concurrently with the caller. Backend lowers to `go func(){ ... }()`; frontend lowers to `queueMicrotask(async () => { ... })`. Fire-and-forget: no return value, no future, no waiting from the caller.
type GoStmt struct {
	node
	Call Expr       // Call is set when the goStmt body is a single expression-statement (`go fn(args)`); Body is nil in that case.
	Body *BlockStmt // Body is set when the goStmt body is a `{ ... }` block; Call is nil in that case.
}

func (*GoStmt) stmtNode() {}

// DeferStmt represents `defer <stmt>` - registers a statement to run when the enclosing function returns. LIFO order if multiple defers stack. Backend lowers to Go's `defer`; frontend lowers to a synthesised `try { /* body */ } finally { /* defers in LIFO */ }` wrapper around the enclosing function body.
type DeferStmt struct {
	node
	Call Expr       // Call is set when the deferStmt body is a single expression-statement.
	Body *BlockStmt // Body is set when the deferStmt body is a `{ ... }` block.
}

func (*DeferStmt) stmtNode() {}

// SelectStmt represents a Sova `select { ... }` block - wait on multiple channel ops simultaneously. The first ready case wins. If `Default` is non-nil and no case is ready, the default branch runs immediately (non-blocking select). Backend lowers to Go's `select` statement; frontend uses a runtime helper that probes each case once, then races their pending promises with cancellation on first fire.
type SelectStmt struct {
	node
	Cases   []*SelectCase
	Default *BlockStmt
}

func (*SelectStmt) stmtNode() {}

// SelectCaseKind classifies the operation in a single `case <op> => body` arm of a Sova `select` block.
type SelectCaseKind int

const (
	SelectCaseRecvBind    SelectCaseKind = iota // `case v, ok = ch.recv() => body`
	SelectCaseRecvDiscard                       // `case ch.recv() => body` or `case after(d) => body`
	SelectCaseSend                              // `case ch.send(v) => body`
)

// SelectCase represents one `case <op> => body` arm inside a `select` block. The kind determines which fields are meaningful: send cases use ChanExpr+SendValue, recv-bind cases use ChanExpr+Targets, recv-discard cases use ChanExpr only.
type SelectCase struct {
	Span      diag.TextSpan
	Kind      SelectCaseKind
	ChanExpr  Expr
	SendValue Expr
	Targets   []VarDeclTarget
	Body      *BlockStmt
}

// MixinDeclStmt represents a reusable bundle of fields and methods that can be applied to a type via `with`.
type MixinDeclStmt struct {
	node
	docBase
	Name    NameRef
	Fields  []*TypeField
	Methods []*TypeMethodDecl
}

func (*MixinDeclStmt) stmtNode() {}

// NewGlobalsImport builds the synthetic `import "std/__globals__" using *` statement the compiler prepends to every non-globals file's HIR so the language built-ins (print, println, len, error, Composable, after, every, ...) resolve without an explicit import.
func NewGlobalsImport(alloc *IdAlloc) *ImportStmt {
	return &ImportStmt{
		node:     node{id: NodeID(alloc.Next())},
		Path:     PackagePath{"std", "__globals__"},
		Alias:    "__globals__",
		UsingAll: true,
	}
}

// NewComposableMixin builds the compiler-injected `mixin Composable { children: []any }` declaration. PassResolveLibs prepends one of these to every package's HIR so user types can `with Composable` without an explicit import.
func NewComposableMixin(alloc *IdAlloc) *MixinDeclStmt {
	mk := func() node { return node{id: NodeID(alloc.Next())} }
	field := &TypeField{
		node: mk(),
		Name: NameRef{Name: "children"},
		Type: &TypeRef{
			node: mk(),
			Kind: TK_Slice,
			Elem: &TypeRef{node: mk(), Kind: TK_PrimitiveAny},
		},
		Default: &ArrayLiteral{
			node:     mk(),
			exprBase: exprBase{},
		},
	}
	return &MixinDeclStmt{
		node:   mk(),
		Name:   NameRef{Name: "Composable"},
		Fields: []*TypeField{field},
	}
}

// InterfaceDeclStmt represents an interface declaration with method signatures only.
type InterfaceDeclStmt struct {
	node
	docBase
	Name         NameRef
	Methods      []*InterfaceMethodSig
	IsExtern     bool
	ExternModule string
}

func (*InterfaceDeclStmt) stmtNode() {}

// TypeAliasStmt is a transparent type alias declared with `using Name = Target`. The alias name is a synonym for the referenced type — both refer to the same TypID after resolution; no new distinct type is created.
type TypeAliasStmt struct {
	node
	docBase
	Name   NameRef  // Name is the alias's local name.
	Target *TypeRef // Target is the underlying type the alias resolves to.
}

func (*TypeAliasStmt) stmtNode() {}

// InterfaceMethodSig represents a single method signature on an interface. `IsShared` opts the contract method into the cross-side conformance rule: a type implementing this method must mark its matching method `shared` so the body is emitted on both sides. Set by the visitor from the per-method `shared` modifier and also implicitly when the interface's declaring file is `on shared`.
type InterfaceMethodSig struct {
	node
	Name       NameRef
	Params     []*FuncParam
	ReturnType *TypeRef
	IsShared   bool
}

// SharedTypeMembers is the codegen-facing summary of which members of a type opt into cross-side emission. Stored under the `shared_type_members` cache key by `pass_analyze_shared_members` and consumed by both code emitters. Lives in `ir` (rather than `passes`) so both codegens can reference it without circular imports.
type SharedTypeMembers struct {
	TypeDecl *TypeDeclStmt
	Fields   []*TypeField
	Methods  []*TypeMethodDecl
	Ctors    []*CtorDecl
	Casts    []*CastDecl
}

// TypeMethodDecl is a method attached to a TypeDeclStmt. It wraps a normal FuncDeclStmt and tracks the receiver symbol. `IsShared` follows the same semantics as `TypeField.IsShared`: it opts the method body into emission on the other side of a one-sided type.
type TypeMethodDecl struct {
	node
	ThisSym     SymID
	Private     bool
	IsShared    bool
	Func        *FuncDeclStmt
	Annotations []Annotation
}

func (*TypeDeclStmt) stmtNode() {}

// TypeField represents a single field on a TypeDeclStmt. `IsShared` opts the field into cross-side emission when the enclosing type lives in a one-sided file (`on backend` / `on frontend`): the other side gets a parallel field on its host class so the shared subset of the type round-trips across the wire as a real class instance. Has no effect on types declared `on shared` — those already exist on both sides in full.
type TypeField struct {
	node
	Annotations []Annotation
	Name        NameRef
	Type        *TypeRef
	Default     Expr
	Private     bool
	IsShared    bool
}

// CastDecl represents a `cast(p: SourceT): Self { … }` declaration inside a TypeDeclStmt body. It is the only cast-overloading hook a type exposes: the compiler may automatically insert a call to it where a value of SourceT appears in a position expecting Self. `IsShared` follows the same semantics as `TypeField.IsShared`.
type CastDecl struct {
	node
	Sym         SymID
	Param       *FuncParam
	ReturnType  *TypeRef
	Body        *BlockStmt
	Annotations []Annotation
	IsShared    bool
}

func (*CastDecl) stmtNode() {}

// CtorDecl represents an explicit constructor declaration inside a TypeDeclStmt body. `IsShared` follows the same semantics as `TypeField.IsShared`.
type CtorDecl struct {
	node
	Sym         SymID
	ThisSym     SymID
	Params      []*FuncParam
	Body        *BlockStmt
	Annotations []Annotation
	IsSynthetic bool // IsSynthetic marks ctors produced by the visitor's field-init synthesis path; they must be dropped before bind/infer when the enclosing type turns out to be IsExtern.
	IsShared    bool
}

// EnumDeclStmt represents an enum declaration.
type EnumDeclStmt struct {
	node
	docBase
	Name    NameRef         // Name is the name of the enum.
	Fields  []*EnumFieldDef // Fields are the payload fields (nil for numeric enums).
	Cases   []*EnumCase     // Cases are the enum cases.
	Methods []*FuncDeclStmt // Methods are the methods defined on the enum.
}

func (*EnumDeclStmt) stmtNode() {}

// EnumFieldDef represents a field in a payload enum.
type EnumFieldDef struct {
	node
	Name    NameRef  // Name is the field name.
	Type    *TypeRef // Type is the field type.
	Default Expr     // Default is the optional default value.
}

// EnumCase represents a single enum case.
type EnumCase struct {
	node
	Name    NameRef // Name is the case name.
	Args    []Expr  // Args are the constructor arguments (for payload enums).
	Value   *int64  // Value is the explicit value (for numeric enums, nil = auto).
	Ordinal int     // Ordinal is the computed ordinal (0-indexed).
}

// EnumValueExpr represents accessing an enum case (e.g., Color.Red).
type EnumValueExpr struct {
	node
	exprBase
	EnumName string // EnumName is the enum type name.
	CaseName string // CaseName is the case name.
	EnumSym  SymID  // EnumSym is the symbol of the enum.
	CaseSym  SymID  // CaseSym is the symbol of the case.
}

func (*EnumValueExpr) exprNode() {}

// ==== Expressions ====

type Op string // Op represents an operator in an expression.

const (
	OpUnknown Op = "" // Unknown operator
	// Arithmetic
	OpAdd Op = "+"  // Addition
	OpSub Op = "-"  // Subtraction
	OpMul Op = "*"  // Multiplication
	OpDiv Op = "/"  // Division
	OpMod Op = "%"  // Modulo
	OpInc Op = "++" // Increment
	OpDec Op = "--" // Decrement
	// Bitwise
	OpAnd Op = "&"  // Bitwise AND
	OpOr  Op = "|"  // Bitwise OR
	OpXor Op = "^"  // Bitwise XOR
	OpShl Op = "<<" // Bitwise Shift Left
	OpShr Op = ">>" // Bitwise Shift Right
	OpNot Op = "~"  // Bitwise NOT
	// Logical
	OpLAnd Op = "&&" // Logical AND
	OpLOr  Op = "||" // Logical OR
	OpLNot Op = "!"  // Logical NOT
	// Comparison
	OpEq  Op = "==" // Equal
	OpNeq Op = "!=" // Not Equal
	OpLt  Op = "<"  // Less Than
	OpLte Op = "<=" // Less Than or Equal
	OpGt  Op = ">"  // Greater Than
	OpGte Op = ">=" // Greater Than or Equal
	// Assignment
	OpAssign Op = "="   // Assignment
	OpAddEq  Op = "+="  // Addition Assignment
	OpSubEq  Op = "-="  // Subtraction Assignment
	OpMulEq  Op = "*="  // Multiplication Assignment
	OpDivEq  Op = "/="  // Division Assignment
	OpModEq  Op = "%="  // Modulo Assignment
	OpAndEq  Op = "&="  // Bitwise AND Assignment
	OpOrEq   Op = "|="  // Bitwise OR Assignment
	OpXorEq  Op = "^="  // Bitwise XOR Assignment
	OpShlEq  Op = "<<=" // Bitwise Shift Left Assignment
	OpShrEq  Op = ">>=" // Bitwise Shift Right Assignment
)

type WhenExpr struct {
	node
	exprBase
	Expr    Expr       // Expr is the expression to evaluate.
	Cases   []WhenCase // Cases are the cases to match against.
	Default Expr       // Default is the default expression if no case matches.
}

type WhenCase struct {
	Values []Expr // Values are the values to match against.
	Then   Expr   // Then is the expression to evaluate if a value matches.
}

func (*WhenExpr) exprNode() {}

type UnaryExpr struct {
	node
	exprBase
	Op   Op   // Op is the operator of the unary expression.
	Expr Expr // Expr is the expression the operator is applied to.
}

func (*UnaryExpr) exprNode() {}

type PrefixUnaryExpr struct {
	node
	exprBase
	Op   Op   // Op is the operator of the prefix unary expression. (currently only '++' and '--' are supported)
	Expr Expr // Expr is the expression the operator is applied to.
}

func (*PrefixUnaryExpr) exprNode() {}

type PostfixUnaryExpr struct {
	node
	exprBase
	Op   Op   // Op is the operator of the postfix unary expression. (currently only '++' and '--' are supported)
	Expr Expr // Expr is the expression the operator is
}

func (*PostfixUnaryExpr) exprNode() {}

type BinaryExpr struct {
	node
	exprBase
	Left  Expr // Left is the left-hand side expression.
	Op    Op   // Op is the operator of the binary expression.
	Right Expr // Right is the right-hand side expression.
}

func (*BinaryExpr) exprNode() {}

type CoalesceExpr struct {
	node
	exprBase
	Left    Expr // Left is the option expression.
	Default Expr // Default is the value to use if Left is none.
}

func (*CoalesceExpr) exprNode() {}

type TenaryExpr struct {
	node
	exprBase
	Cond Expr // Cond is the condition expression.
	Then Expr // Then is the expression if the condition is true.
	Else Expr // Else is the expression if the condition is false.
}

func (*TenaryExpr) exprNode() {}

type GroupedExpr struct {
	node
	exprBase
	Expr Expr // Expr is the inner expression.
}

func (*GroupedExpr) exprNode() {}

// AsExpr represents a runtime type cast: `expr as T` or `expr as? T`. The
// panicking form (`Safe=false`) evaluates to `T` and aborts at runtime if the
// underlying value does not match; the optional form (`Safe=true`) evaluates
// to `option<T>` and yields none on mismatch. Used to bridge `any`-typed
// values (e.g. session-bound `@.user`) back into concrete types without
// silently relying on Go-side type assertions.
type AsExpr struct {
	node
	exprBase
	Expr   Expr     // Expr is the source expression being cast.
	Target *TypeRef // Target is the destination type annotation.
	Safe   bool     // Safe is true for `as?` (returns option<T>) and false for `as` (panics on mismatch).
}

func (*AsExpr) exprNode() {}

// OptionUnwrapExpr is the postfix `expr!` form: asserts that an `option<T>` value is not `none` and yields the underlying `T`. Codegen lowers to a pointer dereference (Go) or identity (JS). Today the assertion is unchecked at runtime - dereferencing a `none` will produce a host-language nil-pointer panic / null-access; a future tightening could insert an explicit `none` check.
//
// IsNoOp is set by the type checker when the operand is already a non-option (typically because flow-sensitive `if x == none` narrowing has already unwrapped it). Codegen then emits the operand unchanged.
type OptionUnwrapExpr struct {
	node
	exprBase
	Expr    Expr // Expr is the option-typed source.
	IsNoOp  bool
}

func (*OptionUnwrapExpr) exprNode() {}

type AssignmentExpr struct {
	node
	exprBase
	Left  NameRef // Left is the variable being assigned to.
	Op    Op      // Op is the assignment operator (e.g., '=', '+=', '-=', etc.).
	Right Expr    // Right is the expression being assigned to the variable.
}

func (*AssignmentExpr) exprNode() {}

type IndexExpr struct {
	node
	exprBase
	Expr  Expr // Expr is the expression being indexed (e.g., an array or map).
	Index Expr // Index is the index expression (e.g., an integer for arrays or a key for maps).
}

func (*IndexExpr) exprNode() {}

type FieldName struct {
	Name string
	Span diag.TextSpan
}

type FieldAccessExpr struct {
	node
	exprBase
	Expr        Expr        // Expr is the expression being accessed (e.g., a struct or object).
	Fields      []FieldName // Fields is the list of field names being accessed in sequence. The first name is the first field, the second name is the field of the first field, and so on. (e.g., obj.field1.field2) translates to Fields = [field1, field2]
	ResolvedSym SymID       // ResolvedSym, when non-zero, identifies a cross-package member; the codegen emits its mangled name directly instead of a member access chain.
}

func (*FieldAccessExpr) exprNode() {}

type VarRef struct {
	node
	exprBase
	Ref NameRef
}

func (*VarRef) exprNode() {}

type RangeExpr struct {
	node
	exprBase
	Start Expr // Start is the start expression of the range.
	End   Expr // End is the end expression of the range.
	Inc   Expr // Inc is the increment expression of the range. May be nil for default increment (1).
}

func (*RangeExpr) exprNode() {}

type FuncCallArg struct {
	Name string // Name is the parameter name if this is a named argument. Empty for positional arguments.
	Expr Expr   // Expr is the argument expression.
}

type FuncCallExpr struct {
	node
	exprBase
	Callee   Expr          // Callee is the function being called.
	Args     []FuncCallArg // Args are the arguments passed to the function (positional and/or named).
	TypeArgs []*TypeRef    // TypeArgs holds explicit generic type arguments from `foo<int>(...)` syntax. Empty when the caller relies on inference.
	IsAsync  bool          // IsAsync is true when the callee is an async function; the JS code generator emits `await` for such calls.
}

func (*FuncCallExpr) exprNode() {}

// SessionExpr is the `@` shorthand inside a wired backend function body. It resolves to the implicit per-request Session value injected by the wire handler.
type SessionExpr struct {
	node
	exprBase
}

func (*SessionExpr) exprNode() {}

// ComposableCallExpr represents the `TypeName(args) { children }` syntax where TypeName resolves to a composable type. It is similar to a NewExpr but with a children list that gets appended to the constructed instance's `children` field after construction. Each child is either an expression (evaluated to a composable value, or convertible via cast) or a control-flow construct whose body yields child values in source order.
type ComposableCallExpr struct {
	node
	exprBase
	Callee    Expr
	Args      []FuncCallArg
	Children  []ComposableChild
	CtorSym   SymID
	TargetTyp TypID
}

func (*ComposableCallExpr) exprNode() {}

// ComposableChildKind enumerates the shapes of a single child slot in a composable block.
type ComposableChildKind int

const (
	ComposableChildExpr   ComposableChildKind = iota // a value-producing expression
	ComposableChildIf                                // an `if` / `else if` / `else` tree producing 0+ children per branch
	ComposableChildFor                               // a `for` loop, each iteration produces 0+ children
	ComposableChildWhile                             // a `while` loop, each iteration produces 0+ children
	ComposableChildSwitch                            // a `switch`/`when` statement, each branch produces 0+ children
)

// ComposableChild is one slot in a composable block. Exactly one of the Expr/Stmt fields is set depending on Kind.
type ComposableChild struct {
	Kind ComposableChildKind
	Expr Expr
	Stmt Stmt
}

// NewExpr instantiates a user-defined type via the `new TypeName(args...)` syntax. Qualifier is set for cross-package constructions like `new pkg.User(...)`.
type NewExpr struct {
	node
	exprBase
	Qualifier string
	TypeName  NameRef
	TypeArgs  []*TypeRef // TypeArgs are the explicit `<T1, T2, ...>` arguments at the instantiation site. Used to substitute `TypeParam` slots in ctor / cast / method signatures during call-site type checking.
	Args      []FuncCallArg
	CtorSym   SymID
}

func (*NewExpr) exprNode() {}

// ChanInitExpr represents `chan<T>()` (unbuffered) or `chan<T>(N)` (buffered with capacity N). Backend lowers to `make(chan T)` / `make(chan T, N)`; frontend lowers to a `__sovaChan(capacity)` runtime call.
type ChanInitExpr struct {
	node
	exprBase
	ElemType *TypeRef // ElemType is the channel's element type.
	Capacity Expr     // Capacity is the optional buffered-capacity expression; nil for unbuffered.
}

func (*ChanInitExpr) exprNode() {}

type FuncLitExpr struct {
	node
	exprBase
	Params     []*FuncParam // Params are the parameters of the function literal.
	ReturnType *TypeRef     // ReturnType is the return type of the function literal. May be nil for void functions.
	Body       *BlockStmt   // Body is the body of the function literal.
	IsAsync    bool         // IsAsync is set by propagate_async when the body calls an async function. Causes the JS emitter to render `async (params) => { ... }` so awaited calls inside are syntactically valid.
}

func (*FuncLitExpr) exprNode() {}

// ---- Literale ----
type LitInt struct {
	node
	exprBase
	Value int64
}

func (*LitInt) exprNode() {}

type LitFloat struct {
	node
	exprBase
	Value float64
}

func (*LitFloat) exprNode() {}

// without quotes
type LitString struct {
	node
	exprBase
	Value string
}

func (*LitString) exprNode() {}

type LitChar struct {
	node
	exprBase
	Value rune
}

func (*LitChar) exprNode() {}

type LitBool struct {
	node
	exprBase
	Value bool
}

func (*LitBool) exprNode() {}

type LitNone struct {
	node
	exprBase
}

func (*LitNone) exprNode() {}

type ArrayLiteral struct {
	node
	exprBase
	Elems []Expr
}

func (*ArrayLiteral) exprNode() {}

type MapEntry struct{ Key, Value Expr }
type MapLiteral struct {
	node
	exprBase
	Entries []MapEntry
}

func (*MapLiteral) exprNode() {}

type TupleLiteral struct {
	node
	exprBase
	Elems []Expr
}

func (*TupleLiteral) exprNode() {}

// StringTemplatePart is one segment of a template string: either a literal chunk (Expr == nil) or an interpolated expression (Lit == "").
type StringTemplatePart struct {
	Lit  string
	Expr Expr
}

// StringTemplateExpr represents a backtick-quoted template string with embedded ${expr} interpolations.
type StringTemplateExpr struct {
	node
	exprBase
	Parts []StringTemplatePart
}

func (*StringTemplateExpr) exprNode() {}

// ==== Type Stuff ====

// TypeRef is a predecessor of the type. It is the same type but in the context of the source code.
type TypeRef struct {
	node
	Kind TypeKind // Kind is the kind of the type.
	Typ  TypID    // Typ is the type ID of the type in the symbol table. This will be filled in the type pass (later).

	// option/slice/array
	Elem *TypeRef // Elem is the element type for option/slice/array types.
	Dim  int64    // Dim is the dimension of the type. For option types, this is always 0. For slice/array types, this is the number of dimensions (e.g., 1 for a slice, 2 for a 2D array).

	// map/umap
	Key   *TypeRef // Key is the key type for map/umap types.
	Value *TypeRef // Value is the value type for map/umap types.

	// tuple
	Tuple []TupleFieldRef // Tuple is the field type for tuple types.

	// function type (`func(...): Ret`) - used when Kind == TK_Function
	FuncParams []FuncTypeParamRef // FuncParams lists the parameter types of the function type.
	FuncReturn *TypeRef           // FuncReturn is the return type of the function type; nil means `none`.

	// custom (enum, struct, etc.)
	CustomName      string     // CustomName is the name of the custom type (e.g., enum name).
	CustomQualifier string     // CustomQualifier is the optional package alias for qualified references like `pkg.Foo`. Empty for unqualified types.
	TypeArgs        []*TypeRef // TypeArgs holds the generic type arguments at the reference site, e.g. for `List<int>` this is `[int-typeref]`. Empty for non-generic references.
}

// FuncTypeParamRef is a single parameter slot inside a function-type annotation `func(name: T, ...)`. Name is optional; only Type is significant for type-equality.
type FuncTypeParamRef struct {
	Name string   // Name is the optional parameter label.
	Type *TypeRef // Type is the parameter type.
}

// TupleFieldRef is a reference to a field in a tuple type.
type TupleFieldRef struct {
	Name string   // Name is the name of the field. Can be empty, if the field is unnamed.
	Type *TypeRef // Type is the type of the field.
}
