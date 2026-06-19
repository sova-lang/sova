package ir

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

type SynthDeclStmt struct {
	node
	docBase
	Name         NameRef
	Params       []*FuncParam
	RequiredSide SideKind
	Target       SynthTarget
	Body         []SynthBodyItem
}

func (*SynthDeclStmt) stmtNode() {}

type SynthTarget struct {
	Kind     SynthTargetKind
	BindName string
}

type SynthBodyItem interface {
	synthBodyItem()
}

type SynthEmitOn struct {
	node
	Scope           string
	AnnotationEmits []Annotation
}

func (*SynthEmitOn) synthBodyItem() {}

type SynthEmitAppend struct {
	node
	Registry string
	Fragment Expr
}

func (*SynthEmitAppend) synthBodyItem() {}

type SynthForStmt struct {
	node
	LoopVar  string
	BindName string
	Member   string
	Where    *SynthBoolExpr
	Body     []SynthBodyItem
}

func (*SynthForStmt) synthBodyItem() {}

type SynthBoolExpr struct {
	node
	Negate   bool
	BindName string
	Property string
}

type SynthEmitField struct {
	node
	Field *TypeField
}

func (*SynthEmitField) synthBodyItem() {}

type SynthEmitMethod struct {
	node
	Method *TypeMethodDecl
}

func (*SynthEmitMethod) synthBodyItem() {}

type SynthEmitCtor struct {
	node
	Ctor *CtorDecl
}

func (*SynthEmitCtor) synthBodyItem() {}
