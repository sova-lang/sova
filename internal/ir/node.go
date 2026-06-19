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
