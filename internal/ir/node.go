package ir

import (
	"sova/internal/diag"
	"strings"
)

type node struct {
	id   NodeID
	span diag.TextSpan
}

func (n *node) ID() NodeID          { return n.id }

func (n *node) Span() diag.TextSpan { return n.span }

type exprBase struct{ typ TypID }

func (e *exprBase) GetType() TypID  { return e.typ }

func (e *exprBase) SetType(t TypID) { e.typ = t }

type docBase struct{ doc string }

func (d *docBase) GetDoc() string { return d.doc }

func (d *docBase) SetDoc(s string) { d.doc = s }

type SideKind int

const (
	SideUnknown SideKind = iota
	SideFrontend
	SideBackend
	SideShared
	SideTest
	SideSynth
)

type SideSpec struct {
	Kind   SideKind
	Target string
}

type PackagePath []string

func (p PackagePath) String() string {
	return strings.Join(p, "/")
}

type File struct {
	node
	Path       string
	Package    PackagePath
	Side       SideSpec
	Statements []Stmt
}

func (*File) declNode() {}

type BlockStmt struct {
	node
	Stmts []Stmt
}

func (*BlockStmt) stmtNode() {}

type VarDeclStmt struct {
	node
	docBase
	IsConst     bool
	Targets     []VarDeclTarget
	Init        Expr
	IsWired     bool
	Wire        *WireSpec
	Annotations []Annotation
	Embed       *EmbedInfo
	Asset       *AssetInfo
}

type EmbedKind int

const (
	EmbedKindUnknown EmbedKind = iota
	EmbedKindText
	EmbedKindBytes
)

type EmbedInfo struct {
	SourcePath  string
	Kind        EmbedKind
	ContentHash string
	SizeBytes   int64
	Span        diag.TextSpan
}

type AssetInfo struct {
	SourcePath  string
	ContentHash string
	URL         string
	StagedName  string
	SizeBytes   int64
	Span        diag.TextSpan
}

type VarDeclTarget struct {
	Name    *NameRef
	TypeAnn *TypeRef
}

func (*VarDeclStmt) stmtNode() {}

type ExprStmt struct {
	node
	Expr Expr
}

func (*ExprStmt) stmtNode() {}

type FieldAssignmentStmt struct {
	node
	Receiver NameRef
	Fields   []FieldName
	Op       Op
	Value    Expr
}

func (*FieldAssignmentStmt) stmtNode() {}

type IndexAssignmentStmt struct {
	node
	Receiver Expr
	Index    Expr
	Op       Op
	Value    Expr
}

func (*IndexAssignmentStmt) stmtNode() {}

type MultiAssignmentStmt struct {
	node
	Targets []AssignmentTarget
	Value   Expr
}

type AssignmentTarget struct {
	Name *NameRef
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
	Depth int
}

func (*BreakStmt) stmtNode() {}

type ContinueStmt struct {
	node
	Depth int
}

func (*ContinueStmt) stmtNode() {}

type ReturnStmt struct {
	node
	Results []Expr
}

func (*ReturnStmt) stmtNode() {}

type GuardStmt struct {
	node
	Cond    Expr
	Returns []Expr
}

func (*GuardStmt) stmtNode() {}

type ForConditionType int

const (
	ForCondInfinite ForConditionType = iota
	ForCondInt
	ForCondRange
	ForCondIn
)

type ForStmt struct {
	node
	CondType  ForConditionType
	CondInt   *ForCondIntDecl
	CondRange *ForCondRangeDecl
	CondIn    *ForCondInDecl
	Body      *BlockStmt
}

type ForCondIntDecl struct {
	Init *VarDeclStmt
	Cond Expr
	Post Expr
}

type ForCondRangeDecl struct {
	RangeVar   NameRef
	RangeStart Expr
	RangeEnd   Expr
}

type ForCondInDecl struct {
	InFirstVar  NameRef
	InSecondVar *NameRef
	InThirdVar  *NameRef
	IterExpr    Expr
	IterNextSym SymID
}

func (*ForStmt) stmtNode() {}

type WhileStmt struct {
	node
	Cond Expr
	Body *BlockStmt
}

func (*WhileStmt) stmtNode() {}

type FuncDeclStmt struct {
	node
	docBase
	Side        *SideSpec
	Name        NameRef
	TypeParams  []TypeParamDecl
	Params      []*FuncParam
	ReturnType  *TypeRef
	Body        *BlockStmt
	IsAsync     bool
	IsWired     bool
	Wire        *WireSpec
	Annotations []Annotation
}

type TypeParamDecl struct {
	Name                  string
	ImplementsConstraints []NameRef
	WithConstraints       []NameRef
}

type WireGroupStmt struct {
	node
	Wire  *WireSpec
	Stmts []Stmt
}

func (*WireGroupStmt) stmtNode() {}

type WireRulesetStmt struct {
	node
	Name    string
	Options map[string]WireOptValue
}

func (*WireRulesetStmt) stmtNode() {}

type WireSpec struct {
	Method        string
	Path          string
	PathArgs      []string
	Options       map[string]WireOptValue
	RequireAuthN  bool
	RequiredRoles []string
	Ruleset       string
	Transport     string
	UsesSession   bool
}

type WireOptValue struct {
	Str  string
	Int  int64
	Bool bool
	Strs []string
	Kind WireOptValueKind
}

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
	IsVariadic  bool
	Name        NameRef
	Type        *TypeRef
	Default     Expr
	Annotations []Annotation
	WireBinding string
	WireBindAs  string
}

func (*FuncParam) stmtNode() {}

type ExternDeclStmt struct {
	node
	docBase
	Module          *string
	Version         string
	IsDefaultImport bool
	Funcs           []*ExternFunc
	Vars            []*ExternVar
	Types           []*TypeDeclStmt
	Interfaces      []*InterfaceDeclStmt
}

func (*ExternDeclStmt) stmtNode() {}

type ExternFunc struct {
	node
	docBase
	Name       NameRef
	TypeParams []TypeParamDecl
	Params     []*FuncParam
	ReturnType *TypeRef
	Mapping    *ExternMapping
	IsAsync    bool
}

type ExternVar struct {
	node
	docBase
	IsConst bool
	Name    NameRef
	Type    *TypeRef
	Mapping *ExternMapping
}

type ExternMapping struct {
	Simple *string
	Shared map[SideKind]*SideMapping
}

type SideMapping struct {
	Module     *string
	Version    string
	NativeFunc string
}

type Annotation struct {
	Name         NameRef
	Args         []Expr
	ArgNames     []string
	ResolvedArgs []AnnotationValue
}

type AnnotationValueKind int

const (
	AnnotationValueUnknown AnnotationValueKind = iota
	AnnotationValueString
	AnnotationValueInt
	AnnotationValueBool
)

type AnnotationValue struct {
	Kind AnnotationValueKind
	Str  string
	Int  int64
	Bool bool
}

type TypeDeclStmt struct {
	node
	docBase
	Annotations  []Annotation
	Name         NameRef
	TypeParams   []TypeParamDecl
	Implements   []NameRef
	MixedIn      []NameRef
	IsExtern     bool
	ExternModule string
	Fields       []*TypeField
	Ctors        []*CtorDecl
	Methods      []*TypeMethodDecl
	Casts        []*CastDecl
}

type ImportStmt struct {
	node
	Path      PackagePath
	Alias     string
	UsingAll  bool
	UsingList []string
}

func (*ImportStmt) stmtNode() {}

type TestDeclStmt struct {
	node
	Name     string
	Body     *BlockStmt
	Parallel bool
	Tags     []string
}

func (*TestDeclStmt) stmtNode() {}

type GroupDeclStmt struct {
	node
	Name     string
	Body     []Stmt
	Parallel bool
	Tags     []string
}

func (*GroupDeclStmt) stmtNode() {}

type SetupStmt struct {
	node
	IsAll bool
	Body  *BlockStmt
}

func (*SetupStmt) stmtNode() {}

type TeardownStmt struct {
	node
	IsAll bool
	Body  *BlockStmt
}

func (*TeardownStmt) stmtNode() {}

type AssertStmt struct {
	node
	Expr Expr
}

func (*AssertStmt) stmtNode() {}

type AsSessionStmt struct {
	node
	Name string
	Body *BlockStmt
}

func (*AsSessionStmt) stmtNode() {}

type GoStmt struct {
	node
	Call Expr
	Body *BlockStmt
}

func (*GoStmt) stmtNode() {}

type DeferStmt struct {
	node
	Call Expr
	Body *BlockStmt
}

func (*DeferStmt) stmtNode() {}

type SelectStmt struct {
	node
	Cases   []*SelectCase
	Default *BlockStmt
}

func (*SelectStmt) stmtNode() {}

type SelectCaseKind int

const (
	SelectCaseRecvBind SelectCaseKind = iota
	SelectCaseRecvDiscard
	SelectCaseSend
)

type SelectCase struct {
	Span      diag.TextSpan
	Kind      SelectCaseKind
	ChanExpr  Expr
	SendValue Expr
	Targets   []VarDeclTarget
	Body      *BlockStmt
}

type MixinDeclStmt struct {
	node
	docBase
	Name    NameRef
	Fields  []*TypeField
	Methods []*TypeMethodDecl
}

func (*MixinDeclStmt) stmtNode() {}

func NewGlobalsImport(alloc *IdAlloc) *ImportStmt {
	return &ImportStmt{
		node:     node{id: NodeID(alloc.Next())},
		Path:     PackagePath{"std", "__globals__"},
		Alias:    "__globals__",
		UsingAll: true,
	}
}

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

type InterfaceDeclStmt struct {
	node
	docBase
	Name         NameRef
	TypeParams   []TypeParamDecl
	Methods      []*InterfaceMethodSig
	IsExtern     bool
	ExternModule string
}

func (*InterfaceDeclStmt) stmtNode() {}

type TypeAliasStmt struct {
	node
	docBase
	Name   NameRef
	Target *TypeRef
}

func (*TypeAliasStmt) stmtNode() {}

type InterfaceMethodSig struct {
	node
	Name       NameRef
	Params     []*FuncParam
	ReturnType *TypeRef
	IsShared   bool
}

type SharedTypeMembers struct {
	TypeDecl *TypeDeclStmt
	Fields   []*TypeField
	Methods  []*TypeMethodDecl
	Ctors    []*CtorDecl
	Casts    []*CastDecl
}

type TypeMethodDecl struct {
	node
	ThisSym     SymID
	Private     bool
	IsShared    bool
	Func        *FuncDeclStmt
	Annotations []Annotation
}

func (*TypeDeclStmt) stmtNode() {}

type TypeField struct {
	node
	Annotations []Annotation
	Name        NameRef
	Type        *TypeRef
	Default     Expr
	Private     bool
	IsShared    bool
	Embed       *EmbedInfo
}

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

type CtorDecl struct {
	node
	Sym         SymID
	ThisSym     SymID
	Params      []*FuncParam
	Body        *BlockStmt
	Annotations []Annotation
	IsSynthetic bool
	IsShared    bool
}

type EnumDeclStmt struct {
	node
	docBase
	Name    NameRef
	Fields  []*EnumFieldDef
	Cases   []*EnumCase
	Methods []*FuncDeclStmt
}

func (*EnumDeclStmt) stmtNode() {}

type EnumFieldDef struct {
	node
	Name    NameRef
	Type    *TypeRef
	Default Expr
}

type EnumCase struct {
	node
	Name    NameRef
	Args    []Expr
	Value   *int64
	Ordinal int
}

type EnumValueExpr struct {
	node
	exprBase
	EnumName string
	CaseName string
	EnumSym  SymID
	CaseSym  SymID
}

func (*EnumValueExpr) exprNode() {}

type Op string

const (
	OpUnknown Op = ""

	OpAdd Op = "+"
	OpSub Op = "-"
	OpMul Op = "*"
	OpDiv Op = "/"
	OpMod Op = "%"
	OpInc Op = "++"
	OpDec Op = "--"

	OpAnd Op = "&"
	OpOr  Op = "|"
	OpXor Op = "^"
	OpShl Op = "<<"
	OpShr Op = ">>"
	OpNot Op = "~"

	OpLAnd Op = "&&"
	OpLOr  Op = "||"
	OpLNot Op = "!"

	OpEq  Op = "=="
	OpNeq Op = "!="
	OpLt  Op = "<"
	OpLte Op = "<="
	OpGt  Op = ">"
	OpGte Op = ">="

	OpAssign Op = "="
	OpAddEq  Op = "+="
	OpSubEq  Op = "-="
	OpMulEq  Op = "*="
	OpDivEq  Op = "/="
	OpModEq  Op = "%="
	OpAndEq  Op = "&="
	OpOrEq   Op = "|="
	OpXorEq  Op = "^="
	OpShlEq  Op = "<<="
	OpShrEq  Op = ">>="
)

type WhenExpr struct {
	node
	exprBase
	Expr    Expr
	Cases   []WhenCase
	Default Expr
}

type WhenCase struct {
	Values []Expr
	Then   Expr
}

func (*WhenExpr) exprNode() {}

type UnaryExpr struct {
	node
	exprBase
	Op   Op
	Expr Expr
}

func (*UnaryExpr) exprNode() {}

type PrefixUnaryExpr struct {
	node
	exprBase
	Op   Op
	Expr Expr
}

func (*PrefixUnaryExpr) exprNode() {}

type PostfixUnaryExpr struct {
	node
	exprBase
	Op   Op
	Expr Expr
}

func (*PostfixUnaryExpr) exprNode() {}

type BinaryExpr struct {
	node
	exprBase
	Left  Expr
	Op    Op
	Right Expr
}

func (*BinaryExpr) exprNode() {}

type CoalesceExpr struct {
	node
	exprBase
	Left    Expr
	Default Expr
}

func (*CoalesceExpr) exprNode() {}

type TenaryExpr struct {
	node
	exprBase
	Cond Expr
	Then Expr
	Else Expr
}

func (*TenaryExpr) exprNode() {}

type GroupedExpr struct {
	node
	exprBase
	Expr Expr
}

func (*GroupedExpr) exprNode() {}

type AsExpr struct {
	node
	exprBase
	Expr   Expr
	Target *TypeRef
	Safe   bool
}

func (*AsExpr) exprNode() {}

type InstanceofExpr struct {
	node
	exprBase
	Expr   Expr
	Target *TypeRef
}

func (*InstanceofExpr) exprNode() {}

type OptionUnwrapExpr struct {
	node
	exprBase
	Expr   Expr
	IsNoOp bool
}

func (*OptionUnwrapExpr) exprNode() {}

type AssignmentExpr struct {
	node
	exprBase
	Left  NameRef
	Op    Op
	Right Expr
}

func (*AssignmentExpr) exprNode() {}

type IndexExpr struct {
	node
	exprBase
	Expr  Expr
	Index Expr
}

func (*IndexExpr) exprNode() {}

type SliceRangeExpr struct {
	node
	exprBase
	Expr Expr
	Low  Expr
	High Expr
}

func (*SliceRangeExpr) exprNode() {}

type FieldName struct {
	Name string
	Span diag.TextSpan
}

type FieldAccessExpr struct {
	node
	exprBase
	Expr        Expr
	Fields      []FieldName
	ResolvedSym SymID
	MethodSym   SymID
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
	Start Expr
	End   Expr
	Inc   Expr
}

func (*RangeExpr) exprNode() {}

type FuncCallArg struct {
	Name string
	Expr Expr
}

type FuncCallExpr struct {
	node
	exprBase
	Callee   Expr
	Args     []FuncCallArg
	TypeArgs []*TypeRef
	IsAsync  bool
}

func (*FuncCallExpr) exprNode() {}

type SessionExpr struct {
	node
	exprBase
}

func (*SessionExpr) exprNode() {}

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

type ComposableChildKind int

const (
	ComposableChildExpr ComposableChildKind = iota
	ComposableChildIf
	ComposableChildFor
	ComposableChildWhile
	ComposableChildSwitch
)

type ComposableChild struct {
	Kind ComposableChildKind
	Expr Expr
	Stmt Stmt
}

type NewExpr struct {
	node
	exprBase
	Qualifier string
	TypeName  NameRef
	TypeArgs  []*TypeRef
	Args      []FuncCallArg
	CtorSym   SymID
}

func (*NewExpr) exprNode() {}

type ChanInitExpr struct {
	node
	exprBase
	ElemType *TypeRef
	Capacity Expr
}

func (*ChanInitExpr) exprNode() {}

type FuncLitExpr struct {
	node
	exprBase
	Params     []*FuncParam
	ReturnType *TypeRef
	Body       *BlockStmt
	IsAsync    bool
}

func (*FuncLitExpr) exprNode() {}

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

type StringTemplatePart struct {
	Lit  string
	Expr Expr
}

type StringTemplateExpr struct {
	node
	exprBase
	Parts []StringTemplatePart
}

func (*StringTemplateExpr) exprNode() {}

type TypeRef struct {
	node
	Kind TypeKind
	Typ  TypID

	Elem *TypeRef
	Dim  int64

	Key   *TypeRef
	Value *TypeRef

	Tuple []TupleFieldRef

	FuncParams []FuncTypeParamRef
	FuncReturn *TypeRef

	CustomName      string
	CustomQualifier string
	TypeArgs        []*TypeRef
}

type FuncTypeParamRef struct {
	Name string
	Type *TypeRef
}

type TupleFieldRef struct {
	Name string
	Type *TypeRef
}
