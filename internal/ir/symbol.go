package ir

import (
	"fmt"
	"math/rand"
	"sova/internal/diag"
)

type SymbolKind int

const (
	SK_Unknown SymbolKind = iota
	SK_Variable
	SK_Function
	SK_Package
)

type SymbolFlags int

const (
	SF_None  SymbolFlags = 0
	SF_Const SymbolFlags = 1 << iota
	SF_Unused
	SF_TypeMethod
	SF_Reachable
)

type Symbol struct {
	ID          SymID
	Kind        SymbolKind
	Name        string
	Owner       ScopeID
	Typ         TypID
	DeclN       NodeID
	Flags       SymbolFlags
	PackagePath string
	Doc         string
}

func (s *Symbol) IsConst() bool { return s.Flags&SF_Const != 0 }

func (sa *SymbolArena) SetDoc(id SymID, doc string) {
	if sym, ok := sa.byID[id]; ok {
		sym.Doc = doc
	}
}

type SymbolArena struct {
	byID     map[SymID]*Symbol
	symAlloc *IdAlloc
}

func NewSymbolArena(symAlloc *IdAlloc) *SymbolArena {
	return &SymbolArena{
		byID:     make(map[SymID]*Symbol),
		symAlloc: symAlloc,
	}
}

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

func (sa *SymbolArena) ByID() map[SymID]*Symbol {
	return sa.byID
}

func (sa *SymbolArena) GetByID(id SymID) (*Symbol, bool) {
	symbol, exists := sa.byID[id]
	return symbol, exists
}

func (sa *SymbolArena) GetByName(name string, scope ScopeID) (SymID, bool) {
	for _, symbol := range sa.byID {
		if symbol.Name == name && symbol.Owner == scope {
			return symbol.ID, true
		}
	}

	return 0, false
}

func (sa *SymbolArena) SetType(sym SymID, typ TypID) bool {
	symbol, exists := sa.byID[sym]
	if !exists {
		return false
	}

	symbol.Typ = typ
	return true
}

type Scope struct {
	ID      ScopeID
	Parent  ScopeID
	Entries map[string][]SymID
}

type ScopeGraph struct {
	diag       *diag.DiagnosticsBag
	byID       map[ScopeID]*Scope
	nodeScope  map[NodeID]ScopeID
	scopeAlloc *IdAlloc
	Root       ScopeID
}

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

func (sg *ScopeGraph) NewScope(parent ...ScopeID) ScopeID {
	if len(parent) <= 0 && len(sg.byID) > 0 {
		return sg.Root
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

func (sg *ScopeGraph) BindNode(node NodeID, scope ScopeID) {
	sg.nodeScope[node] = scope
}

func (sg *ScopeGraph) EnclosingScope(node NodeID) (ScopeID, bool) {
	scope, exists := sg.nodeScope[node]
	return scope, exists
}

func (sg *ScopeGraph) DeclareSymbol(scope ScopeID, name string, sym SymID, sa *SymbolArena) bool {
	s := sg.byID[scope]
	if s == nil {
		panic(fmt.Errorf("declaring symbol %s in non-existing scope %d", name, scope))
	}

	if name == "_" {
		s.Entries[name] = append(s.Entries[name], sym)
		return true
	}

	newSymbol, _ := sa.GetByID(sym)

	if existingSyms, exists := s.Entries[name]; exists {

		if newSymbol.Kind == SK_Function {

			for _, existingSym := range existingSyms {
				existingSymbol, _ := sa.GetByID(existingSym)
				if existingSymbol.Kind != SK_Function {
					sg.diag.Report(diag.ErrRedeclaration, diag.NoSpan, name, scope)
					return false
				}
			}

			s.Entries[name] = append(s.Entries[name], sym)
			return true
		} else {

			sg.diag.Report(diag.ErrRedeclaration, diag.NoSpan, name, scope)
			return false
		}
	}

	s.Entries[name] = append(s.Entries[name], sym)
	return true
}

func (sg *ScopeGraph) LookupOnlyCurrent(scope ScopeID, name string) (SymID, bool) {
	s := sg.byID[scope]
	if s == nil {
		return 0, false
	}

	syms, exists := s.Entries[name]
	if !exists || len(syms) == 0 {
		return 0, false
	}

	return syms[0], exists
}

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

func (sg *ScopeGraph) ParentScope(scope ScopeID) (ScopeID, bool) {
	s := sg.byID[scope]
	if s == nil {
		return 0, false
	}

	return s.Parent, true
}

type ScopeStack struct {
	stack []ScopeID
}

func NewScopeStack() *ScopeStack {
	return &ScopeStack{}
}

func (ss *ScopeStack) Push(id ScopeID) {
	ss.stack = append(ss.stack, id)
}

func (ss *ScopeStack) Pop() {
	ss.stack = ss.stack[:len(ss.stack)-1]
}

func (ss *ScopeStack) Current() ScopeID {
	if len(ss.stack) == 0 {
		return 0
	}

	return ss.stack[len(ss.stack)-1]
}

type NameMap struct {
	toMangled  map[SymID]string
	toOriginal map[SymID]string
	usedNames  map[string]bool
}

const validMangleChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func NewNameMap() *NameMap {
	return &NameMap{
		toMangled:  make(map[SymID]string),
		toOriginal: make(map[SymID]string),
		usedNames:  make(map[string]bool),
	}
}

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
			l += 2
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

func (nm *NameMap) Add(sym SymID, original, mangled string) {
	nm.toOriginal[sym] = original
	nm.toMangled[sym] = mangled
}

func (nm *NameMap) GetMangledName(sym SymID) (string, bool) {
	name, exists := nm.toMangled[sym]
	return name, exists
}

func (nm *NameMap) GetOriginalName(sym SymID) (string, bool) {
	name, exists := nm.toOriginal[sym]
	return name, exists
}

func (nm *NameMap) ReplaceMangledName(sym SymID, newName string) {
	nm.toMangled[sym] = newName
}
