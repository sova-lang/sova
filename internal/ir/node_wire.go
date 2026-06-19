package ir

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
