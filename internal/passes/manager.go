package passes

import (
	"fmt"
	"sova/internal/diag"
	"sova/internal/ir"
	"time"
)

type PassManager struct {
	passes map[string]Pass
	order  []string
}

func NewPassManager() *PassManager {
	return &PassManager{passes: make(map[string]Pass)}
}

func (pm *PassManager) Register(p Pass) {
	name := p.Name()
	if _, exists := pm.passes[name]; exists {
		panic("duplicate pass: " + name)
	}

	pm.passes[name] = p
}

func (pm *PassManager) BuildOrder(selected []string) error {
	seen := map[string]bool{}

	temp := map[string]bool{}

	var out []string
	var visit func(n string) error
	visit = func(n string) error {
		if seen[n] {
			return nil
		}

		if temp[n] {
			return fmt.Errorf("cyclic pass dependency at %s", n)
		}

		p, ok := pm.passes[n]
		if !ok {
			return fmt.Errorf("unknown pass: %s", n)
		}

		temp[n] = true
		for _, dep := range p.Requires() {
			if err := visit(dep); err != nil {
				return err
			}
		}

		temp[n] = false
		seen[n] = true
		out = append(out, n)
		return nil
	}

	for _, n := range selected {
		if err := visit(n); err != nil {
			return err
		}
	}

	pm.order = out
	return nil
}

func (pm *PassManager) Run(bag *diag.DiagnosticsBag, pkgs []*ir.PackageContext, types *ir.TypeTable, symAlloc *ir.IdAlloc, scAlloc *ir.IdAlloc, nodeAlloc *ir.IdAlloc, nameMap *ir.NameMap, cache map[string]any) error {
	startAll := time.Now()
	for _, passName := range pm.order {
		p := pm.passes[passName]
		if p.NoErrors() && bag.Errored() {
			return fmt.Errorf("cannot run pass %s: diagnostics bag has errors", passName)
		}

		stepStart := time.Now()

		switch p.Scope() {
		case PerPackage:
			for _, pkg := range pkgs {
				pc := &PassContext{Diag: bag, Pkgs: pkgs, Pkg: pkg, Types: types, SymAlloc: symAlloc, ScAlloc: scAlloc, NodeAlloc: nodeAlloc, Names: nameMap, Cache: cache}

				if err := runPassWithRecover(p, pc, passName); err != nil {
					return err
				}
			}

		case PerFile:
			for _, pkg := range pkgs {
				for _, f := range pkg.Files {
					pc := &PassContext{Diag: bag, Pkgs: pkgs, Pkg: pkg, File: f, Types: types, SymAlloc: symAlloc, ScAlloc: scAlloc, NodeAlloc: nodeAlloc, Names: nameMap, Cache: cache}

					if err := runPassWithRecover(p, pc, passName); err != nil {
						return err
					}
				}
			}

		case PerBuild:
			pc := &PassContext{Diag: bag, Pkgs: pkgs, Types: types, SymAlloc: symAlloc, ScAlloc: scAlloc, NodeAlloc: nodeAlloc, Names: nameMap, Cache: cache}

			if err := runPassWithRecover(p, pc, passName); err != nil {
				return err
			}
		}

		_ = stepStart
	}

	_ = startAll
	return nil
}

func runPassWithRecover(p Pass, pc *PassContext, passName string) (out error) {
	defer func() {
		if r := recover(); r != nil {
			if pc.Diag.Errored() {
				out = fmt.Errorf("[%s] aborted (upstream diagnostic; pass body bailed out)", passName)
				return
			}

			pc.Diag.Report(diag.ErrPassPanic, diag.NoSpan, passName, fmt.Sprint(r))
			out = fmt.Errorf("[%s] panic: %v (a syntax error earlier in the file usually causes this - fix the diagnostics above first)", passName, r)
		}
	}()
	if err := p.Run(pc); err != nil {
		return fmt.Errorf("[%s] %w", passName, err)
	}

	return nil
}
