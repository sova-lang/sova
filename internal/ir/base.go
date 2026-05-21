package ir

import (
	"reflect"

	"sova/internal/diag"
)

// IsNilStmt reports whether st is either an untyped nil interface or a typed-nil
// pointer wrapped in the Stmt interface. Both forms appear when the parser
// builds HIR from partial source (e.g., the LSP recompiling mid-keystroke), so
// every pass and LSP walker must guard against them.
func IsNilStmt(st Stmt) bool {
	if st == nil {
		return true
	}
	v := reflect.ValueOf(st)
	return v.Kind() == reflect.Ptr && v.IsNil()
}

// IsNilExpr is the expression-shaped counterpart to IsNilStmt.
func IsNilExpr(expr Expr) bool {
	if expr == nil {
		return true
	}
	v := reflect.ValueOf(expr)
	return v.Kind() == reflect.Ptr && v.IsNil()
}

// BlockStmts returns b.Stmts, or nil if b itself is nil. Used by every pass
// and walker that descends into function/method/if/for bodies so a missing
// `*BlockStmt` doesn't crash the walk.
func BlockStmts(b *BlockStmt) []Stmt {
	if b == nil {
		return nil
	}
	return b.Stmts
}

// BlockID returns b.ID(), or the zero NodeID if b is nil.
func BlockID(b *BlockStmt) NodeID {
	if b == nil {
		return 0
	}
	return b.ID()
}

type NodeID uint64  // NodeID is a unique identifier for each node in the intermediate representation.
type SymID uint64   // SymID is a unique identifier for each symbol in the intermediate representation.
type TypID uint64   // TypID is a unique identifier for each type in the intermediate representation.
type ScopeID uint64 // ScopeID is a unique identifier for each scope in the intermediate representation.

// Node is the base interface for all nodes in the intermediate representation.
type Node interface {
	ID() NodeID          // ID returns the unique identifier for the node.
	Span() diag.TextSpan // Span returns the text span of the node for diagnostics.
}

// Expr is the base interface for all expressions in the intermediate representation.
type Expr interface {
	Node
	exprNode()
	GetType() TypID // GetType returns the type of the expression.
	SetType(TypID)  // SetType sets the type of the expression.
}

// Stmt is the base interface for all statements in the intermediate representation.
type Stmt interface {
	Node
	stmtNode()
}

// Decl is the base interface for all declarations in the intermediate representation.
type Decl interface {
	Node
	declNode()
}

// NameRef is a reference to a named entity (e.g., variable, function) in the source code.
// It holds the original source code name and a future symbol ID that will be resolved later.
// It is used to track references to named entities to allow easier renaming in the codegen pass.
type NameRef struct {
	Name      string        // Name is the original name of the entity as it appears in the source code.
	Sym       SymID         // Sym is the symbol ID that will be resolved later.
	Span      diag.TextSpan // Span is the text span of the name reference for diagnostics.
	Qualifier string        // Qualifier is an optional package alias for cross-package references like `pkg.Foo`. Empty for unqualified names.
}

// IdAlloc is a simple ID allocator for generating unique IDs.
type IdAlloc struct {
	next uint64 // next is the next ID to be allocated.
}

// NewIdAlloc returns a new instance of IdAlloc.
func NewIdAlloc() *IdAlloc {
	return &IdAlloc{
		next: 1, // Start from 1 to avoid zero ID.
	}
}

// Next returns the next unique ID.
func (a *IdAlloc) Next() uint64 {
	id := a.next
	a.next++
	return id
}

// PreparsedFile represents a pre-parsed source file after ANTLR converted the source code into HIR.
type PreparsedFile struct {
	Filename string // filename is the name of the source file. If empty, no filename was given.
	Content  string // content is the source code of the file.
	Hir      *File  // the file in the high-level intermediate representation (HIR).
}

// PackageContext holds the context for a single package being compiled.
type PackageContext struct {
	Path   PackagePath      // Path is basically the idenifier of the package.
	Files  []*PreparsedFile // files are all files in the package.
	Syms   *SymbolArena     // Syms is the symbol arena for the package, holding all symbols.
	Scopes *ScopeGraph      // Scopes is the scope graph for the package, holding all scopes.
	Types  *TypeTable       // Types is the type table for the package, holding all types.
	Root   ScopeID          // Root is the root scope ID of the package.
}
