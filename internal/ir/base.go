package ir

import (
	"reflect"

	"sova/internal/diag"
)

func IsNilStmt(st Stmt) bool {
	if st == nil {
		return true
	}

	v := reflect.ValueOf(st)
	return v.Kind() == reflect.Ptr && v.IsNil()
}

func IsNilExpr(expr Expr) bool {
	if expr == nil {
		return true
	}

	v := reflect.ValueOf(expr)
	return v.Kind() == reflect.Ptr && v.IsNil()
}

func BlockStmts(b *BlockStmt) []Stmt {
	if b == nil {
		return nil
	}

	return b.Stmts
}

func BlockID(b *BlockStmt) NodeID {
	if b == nil {
		return 0
	}

	return b.ID()
}

type NodeID uint64
type SymID uint64
type TypID uint64
type ScopeID uint64

type Node interface {
	ID() NodeID
	Span() diag.TextSpan
}

type Expr interface {
	Node
	exprNode()
	GetType() TypID
	SetType(TypID)
}

type Stmt interface {
	Node
	stmtNode()
}

type Decl interface {
	Node
	declNode()
}

type NameRef struct {
	Name      string
	Sym       SymID
	Span      diag.TextSpan
	Qualifier string
}

type IdAlloc struct {
	next uint64
}

func NewIdAlloc() *IdAlloc {
	return &IdAlloc{
		next: 1,
	}
}

func (a *IdAlloc) Next() uint64 {
	id := a.next
	a.next++
	return id
}

type PreparsedFile struct {
	Filename string
	Content  string
	Hir      *File
}

type PackageContext struct {
	Path   PackagePath
	Files  []*PreparsedFile
	Syms   *SymbolArena
	Scopes *ScopeGraph
	Types  *TypeTable
	Root   ScopeID
}
