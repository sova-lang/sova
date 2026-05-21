package passes

import (
	"fmt"
	"sort"
	"sova/internal/codegen"
	"sova/internal/diag"
	"sova/internal/ir"
	"strings"
)

type InitStepKind int

const (
	InitExpr InitStepKind = iota
	InitBlock
	InitIf
	InitFor
	InitSwitch
	InitWhile
	InitReturn
)

func (k InitStepKind) String() string {
	switch k {
	case InitExpr:
		return "InitExpr"
	case InitBlock:
		return "InitBlock"
	case InitIf:
		return "InitIf"
	case InitFor:
		return "InitFor"
	case InitSwitch:
		return "InitSwitch"
	case InitWhile:
		return "InitWhile"
	case InitReturn:
		return "InitReturn"
	default:
		return "Init"
	}
}

type InitStep struct {
	ID   ir.NodeID
	Kind InitStepKind
	Pkg  *ir.PackageContext
	File *ir.File
	Stmt ir.Stmt
	Seq  int
}

// PassInitOrder is a pass that determines the correct initialization order of package-level expressions and code
// passages like if statements to ensure proper execution order in the generated init function.
type PassInitOrder struct{}

func (p *PassInitOrder) Name() string       { return "init_order" }
func (p *PassInitOrder) Scope() PassScope   { return PerBuild }
func (p *PassInitOrder) Requires() []string { return []string{"infer_types"} }
func (p *PassInitOrder) NoErrors() bool     { return false }

func (p *PassInitOrder) Run(pc *PassContext) error {
	// Placeholder for later import resolving: key is pkg path and value is list of pkg paths it depends on
	var buildImportDeps = map[string][]string{}
	var steps []*InitStep

	for _, pkg := range pc.Pkgs {
		for _, f := range pkg.Files {
			seq := 0
			for _, st := range f.Hir.Statements {
				add := func(s ir.Stmt, kind InitStepKind) {
					steps = append(steps, &InitStep{
						ID: s.ID(), Kind: kind, Pkg: pkg, File: f.Hir, Stmt: s,
						Seq: seq,
					})
					seq++
				}

				switch s := st.(type) {
				case *ir.BlockStmt:
					add(s, InitBlock)
				case *ir.IfStmt:
					add(s, InitIf)
				case *ir.SwitchStmt:
					add(s, InitSwitch)
				case *ir.ReturnStmt:
					add(s, InitReturn)
				case *ir.ExprStmt:
					add(s, InitExpr)
				case *ir.WhileStmt:
					add(s, InitWhile)
				}
			}
		}
	}

	if len(steps) == 0 {
		return nil
	}

	idx := make(map[ir.NodeID]int, len(steps))
	for i, st := range steps {
		idx[st.ID] = i
	}

	adj := make([][]int, len(steps))
	indeg := make([]int, len(steps))
	addEdge := func(a, b int) {
		adj[a] = append(adj[a], b)
		indeg[b]++
	}

	sort.SliceStable(steps, func(i, j int) bool {
		pi := steps[i].File.Package.String()
		pj := steps[j].File.Package.String()
		if pi == pj {
			if steps[i].File.Path == steps[j].File.Path {
				return steps[i].Seq < steps[j].Seq
			}
			return steps[i].File.Path < steps[j].File.Path
		}
		return pi < pj
	})
	for i := 0; i+1 < len(steps); i++ {
		if steps[i].File == steps[i+1].File {
			addEdge(i, i+1)
		}
	}

	if len(buildImportDeps) > 0 {
		firstStepOfPkg := map[string]int{}
		lastStepOfPkg := map[string]int{}
		for i, s := range steps {
			p := s.File.Package.String()
			if _, ok := firstStepOfPkg[p]; !ok {
				firstStepOfPkg[p] = i
			}
			lastStepOfPkg[p] = i
		}
		for p, deps := range buildImportDeps {
			for _, q := range deps {
				pi, okP := lastStepOfPkg[p]
				qj, okQ := firstStepOfPkg[q]
				if okP && okQ {
					addEdge(pi, qj)
				}
			}
		}
	}

	orderIdx, cycleIdx, ok := topoWithCycle(adj, indeg)
	if !ok {
		labels := make([]string, 0, len(cycleIdx))
		for _, i := range cycleIdx {
			st := steps[i]
			sp := st.Stmt.Span()
			kind := st.Kind.String()
			lbl := fmt.Sprintf("%s/%s:%d (%s)",
				st.File.Package.String(), st.File.Path, sp.StartLn, kind)
			labels = append(labels, lbl)
		}

		firstSpan := steps[cycleIdx[0]].Stmt.Span()
		pc.Diag.Report(diag.ErrTopLevelCycle, firstSpan, strings.Join(labels, " -> "))
		return nil
	}

	plan := make([]*codegen.InitPlanEntry, len(orderIdx))
	for k, i := range orderIdx {
		s := steps[i]
		plan[k] = &codegen.InitPlanEntry{
			Stmt: s.Stmt,
			Pkg:  s.Pkg,
			File: s.File,
		}
	}

	pc.Cache["init_plan"] = plan
	return nil
}

func topoWithCycle(adj [][]int, indeg []int) (order []int, cycle []int, ok bool) {
	n := len(adj)
	q := make([]int, 0, n)
	idg := make([]int, n)
	copy(idg, indeg)

	for i := range n {
		if idg[i] == 0 {
			q = append(q, i)
		}
	}
	for len(q) > 0 {
		v := q[0]
		q = q[1:]
		order = append(order, v)
		for _, w := range adj[v] {
			idg[w]--
			if idg[w] == 0 {
				q = append(q, w)
			}
		}
	}
	if len(order) == n {
		return order, nil, true
	}

	rest := make([]bool, n)
	for i := range n {
		if idg[i] > 0 {
			rest[i] = true
		}
	}

	vis := make([]int8, n)
	var stack []int
	var found []int

	var dfs func(int) bool
	dfs = func(v int) bool {
		vis[v] = 1
		stack = append(stack, v)
		for _, w := range adj[v] {
			if !rest[w] {
				continue
			}
			if vis[w] == 0 {
				if dfs(w) {
					return true
				}
			} else if vis[w] == 1 {
				pos := -1
				for i := len(stack) - 1; i >= 0; i-- {
					if stack[i] == w {
						pos = i
						break
					}
				}
				if pos >= 0 {
					found = append([]int{}, stack[pos:]...)
					found = append(found, w)
				}
				return true
			}
		}
		stack = stack[:len(stack)-1]
		vis[v] = 2
		return false
	}

	for i := 0; i < n; i++ {
		if rest[i] && vis[i] == 0 {
			if dfs(i) {
				break
			}
		}
	}
	if len(found) == 0 {
		return order, nil, false
	}
	return order, found, false
}
