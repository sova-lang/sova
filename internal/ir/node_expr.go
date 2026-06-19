package ir

import "sova/internal/diag"

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

func (*WhenExpr) exprNode() {}

type WhenCase struct {
	Values []Expr
	Then   Expr
}

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

type EnumValueExpr struct {
	node
	exprBase
	EnumName string
	CaseName string
	EnumSym  SymID
	CaseSym  SymID
}

func (*EnumValueExpr) exprNode() {}

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
