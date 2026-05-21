package ir

import (
	"fmt"
	"math/rand"
	"sova/internal/diag"
)

// SymbolKind represents the kind of a symbol in the intermediate representation.
type SymbolKind int

const (
	SK_Unknown  SymbolKind = iota // Unknown symbol kind
	SK_Variable                   // Variable symbol kind
	SK_Function                   // Function symbol kind
	SK_Package                    // Package alias symbol introduced by an `import` statement
)

// SymbolFlags represents flags associated with a symbol.
type SymbolFlags int

const (
	SF_None       SymbolFlags = 0
	SF_Const      SymbolFlags = 1 << iota
	SF_Unused
	SF_TypeMethod
)

// Symbol represents a symbol in the intermediate representation. A symbol is an entity that is declared in any scope like a variable, function, or type.
type Symbol struct {
	ID          SymID       // ID is the unique identifier for the symbol.
	Kind        SymbolKind  // Kind is the kind of the symbol.
	Name        string      // Name is the original name of the symbol as it appears in the source code.
	Owner       ScopeID     // Owner is the scope ID of the symbol, indicating where it is defined.
	Typ         TypID       // Typ is the type of the symbol.
	DeclN       NodeID      // DeclN is the declaration node ID of the symbol.
	Flags       SymbolFlags // Flags are the flags associated with the symbol, indicating properties like constness.
	PackagePath string      // PackagePath is the resolved import path for SK_Package symbols, empty otherwise.
	Doc         string      // Doc is the markdown-ready doc comment attached at declaration site (from `///` or `/** */` blocks). Populated by bind_declare for user code, by registerBuiltinPackages / injectBuiltinsIntoPackage for the language built-ins.
}

// IsConst is a helper function to check if a symbol is constant.
func (s *Symbol) IsConst() bool { return s.Flags&SF_Const != 0 }

// SetDoc records the doc-comment markdown on the symbol identified by `id`.
// Used by bind_declare to copy the visitor-attached doc string onto each
// top-level symbol, and by registerBuiltinPackages to document the
// compiler-synthesised symbols.
func (sa *SymbolArena) SetDoc(id SymID, doc string) {
	if sym, ok := sa.byID[id]; ok {
		sym.Doc = doc
	}
}

// SymbolArena is like a global symbol table that holds all symbols mapped to their IDs.
type SymbolArena struct {
	byID     map[SymID]*Symbol // byID maps symbol IDs to symbols.
	symAlloc *IdAlloc          // symAlloc is an ID allocator for generating symbol IDs.
}

// NewSymbolArena returns a new instance of SymbolArena.
func NewSymbolArena(symAlloc *IdAlloc) *SymbolArena {
	return &SymbolArena{
		byID:     make(map[SymID]*Symbol),
		symAlloc: symAlloc,
	}
}

// NewSymbol creates a new symbol with the given parameters and returns its ID.
func (sa *SymbolArena) NewSymbol(kind SymbolKind, name string, owner ScopeID, typ TypID, decl NodeID, flags ...SymbolFlags) SymID {
	flagsVal := SF_None
	for _, flag := range flags {
		flagsVal |= flag
	}

	id := SymID(sa.symAlloc.Next())
	sa.byID[id] = &Symbol{
		ID: id, Kind: kind, Name: name, Owner: owner, Typ: typ, DeclN: decl, Flags: flagsVal,
	}
	return id
}

// ByID returns the map of symbols indexed by their IDs. This is useful for iterating over all symbols in the arena.
func (sa *SymbolArena) ByID() map[SymID]*Symbol {
	return sa.byID
}

// GetByID returns the symbol associated with the given ID, if it exists.
func (sa *SymbolArena) GetByID(id SymID) (*Symbol, bool) {
	symbol, exists := sa.byID[id]
	return symbol, exists
}

// GetByName returns the symbol associated with the given name in the specified scope, if it exists.
func (sa *SymbolArena) GetByName(name string, scope ScopeID) (SymID, bool) {
	for _, symbol := range sa.byID {
		if symbol.Name == name && symbol.Owner == scope {
			return symbol.ID, true
		}
	}
	return 0, false // Return zero ID if not found.
}

// SetType sets the type of the symbol with the given ID. Returns true if the type was set successfully, false if the symbol does not exist.
func (sa *SymbolArena) SetType(sym SymID, typ TypID) bool {
	symbol, exists := sa.byID[sym]
	if !exists {
		return false // Symbol does not exist.
	}
	symbol.Typ = typ
	return true // Successfully set the type.
}

// Scope represents a semantic scope in the intermediate representation.
type Scope struct {
	ID      ScopeID            // ID is the unique identifier for the scope.
	Parent  ScopeID            // Parent is the ID of the parent scope, if any. If 0, then this scope is the root scope.
	Entries map[string][]SymID // Entries maps names to symbol IDs, representing the symbols defined in this scope.
}

// ScopeGraph represents the graph of scopes in the intermediate representation. It used to build up the symbol table for different scopes allowing lookups for semantic analysis.
type ScopeGraph struct {
	diag       *diag.DiagnosticsBag // diag is the diagnostics bag for reporting errors and warnings.
	byID       map[ScopeID]*Scope   // byID maps scope IDs to scopes.
	nodeScope  map[NodeID]ScopeID   // nodeScope maps node IDs to their enclosing scope IDs.
	scopeAlloc *IdAlloc             // scopeAlloc is an ID allocator for generating scope IDs.
	Root       ScopeID              // Root is the ID of the root scope, which is created when the ScopeGraph is initialized.
}

// NewScopeGraph creates a new instance of ScopeGraph.
func NewScopeGraph(diag *diag.DiagnosticsBag, alloc *IdAlloc) *ScopeGraph {
	sg := &ScopeGraph{
		diag:       diag,
		byID:       make(map[ScopeID]*Scope),
		nodeScope:  make(map[NodeID]ScopeID),
		scopeAlloc: alloc,
	}
	sg.Root = sg.NewScope()
	return sg
}

// NewScope creates a new scope and returns its ID.
func (sg *ScopeGraph) NewScope(parent ...ScopeID) ScopeID {
	if len(parent) <= 0 && len(sg.byID) > 0 {
		return sg.Root // If no parent is given, return the root scope.
	}

	id := ScopeID(sg.scopeAlloc.Next())
	sc := &Scope{
		ID:      id,
		Parent:  0,
		Entries: make(map[string][]SymID),
	}
	if len(parent) > 0 {
		sc.Parent = parent[0]
	}
	sg.byID[id] = sc
	return id
}

// BindNode binds a node to its enclosing scope.
func (sg *ScopeGraph) BindNode(node NodeID, scope ScopeID) {
	sg.nodeScope[node] = scope
}

// EnclosingScope returns the enclosing scope for a given node ID.
func (sg *ScopeGraph) EnclosingScope(node NodeID) (ScopeID, bool) {
	scope, exists := sg.nodeScope[node]
	return scope, exists
}

// DeclareSymbol declares a given symbol in the specified scope.
// For functions, multiple symbols with the same name are allowed (overloading).
// For other symbols, redeclaration is an error.
func (sg *ScopeGraph) DeclareSymbol(scope ScopeID, name string, sym SymID, sa *SymbolArena) bool {
	s := sg.byID[scope]
	if s == nil {
		panic(fmt.Errorf("declaring symbol %s in non-existing scope %d", name, scope))
	}

	if name == "_" { // special symbol, declares discard, may be redeclared
		s.Entries[name] = append(s.Entries[name], sym)
		return true
	}

	newSymbol, _ := sa.GetByID(sym)

	// Check if name already exists
	if existingSyms, exists := s.Entries[name]; exists {
		// Function overloading: allow multiple functions with the same name
		if newSymbol.Kind == SK_Function {
			// Check if all existing symbols are also functions
			for _, existingSym := range existingSyms {
				existingSymbol, _ := sa.GetByID(existingSym)
				if existingSymbol.Kind != SK_Function {
					sg.diag.Report(diag.ErrRedeclaration, diag.NoSpan, name, scope)
					return false
				}
			}
			// All existing symbols are functions, allow the overload
			s.Entries[name] = append(s.Entries[name], sym)
			return true
		} else {
			// Non-function redeclaration is an error
			sg.diag.Report(diag.ErrRedeclaration, diag.NoSpan, name, scope)
			return false
		}
	}

	s.Entries[name] = append(s.Entries[name], sym)
	return true
}

// LookupOnlyCurrent checks if a symbol is declared in the current scope only, returning the symbol ID if found, or 0 if not found.
func (sg *ScopeGraph) LookupOnlyCurrent(scope ScopeID, name string) (SymID, bool) {
	s := sg.byID[scope]
	if s == nil {
		return 0, false
	}

	syms, exists := s.Entries[name]
	if !exists || len(syms) == 0 {
		return 0, false
	}

	return syms[0], exists // Return the first symbol found with the given name because the only symbol with multiple entries can be the discard symbol "_".
}

// Lookup checks if the symbol is declared the scope chain up, returning the symbol ID if found, or 0 if not found.
func (sg *ScopeGraph) Lookup(scope ScopeID, name string) (SymID, bool) {
	cur := scope
	for cur != 0 {
		if id, ok := sg.LookupOnlyCurrent(cur, name); ok {
			return id, true
		}
		cur = sg.byID[cur].Parent
	}
	return 0, false
}

// LookupAll checks if symbols are declared in the scope chain, returning all matching symbol IDs.
// This is used for function overload resolution.
func (sg *ScopeGraph) LookupAll(scope ScopeID, name string) []SymID {
	cur := scope
	for cur != 0 {
		s := sg.byID[cur]
		if s != nil {
			if syms, exists := s.Entries[name]; exists && len(syms) > 0 {
				return syms
			}
		}
		cur = sg.byID[cur].Parent
	}
	return nil
}

// ParentScope returns the parent scope of the given scope ID.
func (sg *ScopeGraph) ParentScope(scope ScopeID) (ScopeID, bool) {
	s := sg.byID[scope]
	if s == nil {
		return 0, false
	}
	return s.Parent, true
}

// ScopeStack allows for managing the current scope during semantic analysis.
type ScopeStack struct {
	stack []ScopeID
}

// NewScopeStack creates a new ScopeStack.
func NewScopeStack() *ScopeStack {
	return &ScopeStack{}
}

// Push pushes the current scope ID onto the stack.
func (ss *ScopeStack) Push(id ScopeID) {
	ss.stack = append(ss.stack, id)
}

// Pop pops the current scope ID from the stack.
func (ss *ScopeStack) Pop() {
	ss.stack = ss.stack[:len(ss.stack)-1]
}

// Current returns the current scope ID.
func (ss *ScopeStack) Current() ScopeID {
	if len(ss.stack) == 0 {
		return 0
	}
	return ss.stack[len(ss.stack)-1]
}

// NameMap maps symbol IDs to their mangled and original names.
type NameMap struct {
	toMangled  map[SymID]string // toMangled maps symbol IDs to their mangled names.
	toOriginal map[SymID]string // toOriginal maps symbol IDs to their original names.
	usedNames  map[string]bool  // usedNames keeps track of all used names to avoid duplicates.
}

const validMangleChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// NewNameMap creates a new NameMap.
func NewNameMap() *NameMap {
	return &NameMap{
		toMangled:  make(map[SymID]string),
		toOriginal: make(map[SymID]string),
		usedNames:  make(map[string]bool),
	}
}

// RandName generates a random unused mangled name. If a prefix is provided, it will be used as the base for the mangled name.
func (nm *NameMap) RandName(prefix ...string) string {
	prefixStr := ""
	if len(prefix) > 0 {
		prefixStr = prefix[0] + "__"
	}

	count := 0
	l := 8
	var name string
	for {
		name = prefixStr + nm.randString(l)
		if _, exists := nm.usedNames[name]; !exists {
			break
		}

		count++
		if count > 1000 {
			l += 2 // Increase length if too many attempts
		}
	}
	nm.usedNames[name] = true
	return name
}

func (nm *NameMap) randString(length int) string {
	result := make([]byte, length)
	for i := range result {
		result[i] = validMangleChars[rand.Intn(len(validMangleChars))]
	}
	return string(result)
}

// Add adds a new symbol ID with its original and mangled names to the NameMap.
func (nm *NameMap) Add(sym SymID, original, mangled string) {
	nm.toOriginal[sym] = original
	nm.toMangled[sym] = mangled
}

// GetMangledName returns the mangled name for a given symbol ID.
func (nm *NameMap) GetMangledName(sym SymID) (string, bool) {
	name, exists := nm.toMangled[sym]
	return name, exists
}

// GetOriginalName returns the original name for a given symbol ID.
func (nm *NameMap) GetOriginalName(sym SymID) (string, bool) {
	name, exists := nm.toOriginal[sym]
	return name, exists
}

// ReplaceMangledName replaces the mangled name for a given symbol ID with a new name.
func (nm *NameMap) ReplaceMangledName(sym SymID, newName string) {
	nm.toMangled[sym] = newName
}
