package ir

import "sova/internal/diag"

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
}

func (*VarDeclStmt) stmtNode() {}

type VarDeclTarget struct {
	Name    *NameRef
	TypeAnn *TypeRef
}

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

func (*MultiAssignmentStmt) stmtNode() {}

type AssignmentTarget struct {
	Name *NameRef
}

type IfStmt struct {
	node
	Cond    Expr
	Then    *BlockStmt
	ElseIfs []ElseIfBranch
	Else    *BlockStmt
}

func (*IfStmt) stmtNode() {}

type ElseIfBranch struct {
	Cond Expr
	Then *BlockStmt
}

type SwitchStmt struct {
	node
	Expr    Expr
	Cases   []SwitchCase
	Default []Stmt
}

func (*SwitchStmt) stmtNode() {}

type SwitchCase struct {
	Values []Expr
	Stmts  []Stmt
}

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

func (*ForStmt) stmtNode() {}

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

type WhileStmt struct {
	node
	Cond Expr
	Body *BlockStmt
}

func (*WhileStmt) stmtNode() {}

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
