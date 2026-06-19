package ir

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

func (*FuncDeclStmt) stmtNode() {}

type TypeParamDecl struct {
	Name                  string
	ImplementsConstraints []NameRef
	WithConstraints       []NameRef
}

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

type TypeMethodDecl struct {
	node
	ThisSym     SymID
	Private     bool
	IsShared    bool
	Func        *FuncDeclStmt
	Annotations []Annotation
}

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

type SharedTypeMembers struct {
	TypeDecl *TypeDeclStmt
	Fields   []*TypeField
	Methods  []*TypeMethodDecl
	Ctors    []*CtorDecl
	Casts    []*CastDecl
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

type InterfaceMethodSig struct {
	node
	Name       NameRef
	Params     []*FuncParam
	ReturnType *TypeRef
	IsShared   bool
}

type TypeAliasStmt struct {
	node
	docBase
	Name   NameRef
	Target *TypeRef
}

func (*TypeAliasStmt) stmtNode() {}

type MixinDeclStmt struct {
	node
	docBase
	Name    NameRef
	Fields  []*TypeField
	Methods []*TypeMethodDecl
}

func (*MixinDeclStmt) stmtNode() {}

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
