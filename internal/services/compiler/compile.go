package compiler

import (
	"fmt"
	"log"
	"sort"
	"sova/internal/diag"
	"sova/internal/ir"
	"sova/internal/parser"

	"github.com/antlr4-go/antlr/v4"
)

type PackageLoader func(c *CompilerContext, path string) error

type CompilerContext struct {
	Diag         *diag.DiagnosticsBag
	Packages     map[string]*ir.PackageContext
	NodeAlloc    *ir.IdAlloc
	SymAlloc     *ir.IdAlloc
	ScAlloc      *ir.IdAlloc
	TypeUniverse *ir.TypeTable
	NameMap      *ir.NameMap
	Cache        map[string]any
	Loader       PackageLoader
}

func New() *CompilerContext {
	c := &CompilerContext{
		Diag:         diag.NewBag(),
		Packages:     make(map[string]*ir.PackageContext),
		NodeAlloc:    ir.NewIdAlloc(),
		SymAlloc:     ir.NewIdAlloc(),
		ScAlloc:      ir.NewIdAlloc(),
		TypeUniverse: ir.NewTypeTable(ir.NewIdAlloc()),
		NameMap:      ir.NewNameMap(),
		Cache:        map[string]any{},
	}

	registerBuiltinPackages(c)
	return c
}

func (c *CompilerContext) SetBuildConfig(key string, value any) {
	c.Cache[key] = value
}

func (c *CompilerContext) preparse(file *ir.PreparsedFile) (ok bool) {
	is := antlr.NewInputStream(file.Content)
	lexer := parser.NewSovaLexer(is)
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(diag.NewAntlrErrorListener(file.Filename, c.Diag))
	cts := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	p := parser.NewSovaParser(cts)
	p.RemoveErrorListeners()
	p.AddErrorListener(diag.NewAntlrErrorListener(file.Filename, c.Diag))
	visitor := ir.NewVisitor(file.Filename, c.NodeAlloc, c.Diag)
	visitor.SetTokenStream(cts)
	defer func() {
		if r := recover(); r != nil {
			c.Diag.Report(diag.ErrVisitorPanic, diag.NoSpan, file.Filename, fmt.Sprint(r))
			ok = false
		}
	}()
	rawHir := visitor.Visit(p.File())
	if rawHir == nil {
		log.Printf("Failed to parse file %s\n", file.Filename)
		return false
	}

	hir, hok := rawHir.(*ir.File)
	if !hok {
		log.Printf("Unexpected type for HIR: %T in file %s\n", rawHir, file.Filename)
		return false
	}

	file.Hir = hir
	return true
}

func (c *CompilerContext) AddSource(filename, content string) {
	file := &ir.PreparsedFile{
		Filename: filename,
		Content:  content,
	}

	if !c.preparse(file) {
		c.Diag.Report(diag.ErrPreparsingFailed, diag.NoSpan)
		return
	}

	packageKey := file.Hir.Package.String()
	if packageKey != "std/__globals__" && file.Hir != nil {
		file.Hir.Statements = append([]ir.Stmt{ir.NewGlobalsImport(c.NodeAlloc)}, file.Hir.Statements...)
	}

	if _, ok := c.Packages[packageKey]; !ok {
		sc := ir.NewScopeGraph(c.Diag, c.ScAlloc)
		pkg := &ir.PackageContext{
			Path:   file.Hir.Package,
			Files:  []*ir.PreparsedFile{file},
			Syms:   ir.NewSymbolArena(c.SymAlloc),
			Types:  c.TypeUniverse,
			Scopes: sc,
			Root:   sc.Root,
		}

		c.Packages[packageKey] = pkg
		if packageKey != "std/__globals__" {
			injectChannelAndErrorBuiltins(c, pkg)
		}
	} else {
		c.Packages[packageKey].Files = append(c.Packages[packageKey].Files, file)
	}
}

func (c *CompilerContext) Compile() error {
	if err := c.resolveImports(); err != nil {
		return err
	}

	return c.runPipeline(compilerPipeline())
}

func (c *CompilerContext) Check() error {
	if err := c.resolveImports(); err != nil {
		return err
	}

	return c.runPipeline(checkPipeline())
}

func (c *CompilerContext) resolveImports() error {
	for {
		added := false
		for _, pkg := range c.Packages {
			for _, f := range pkg.Files {
				if f.Hir == nil {
					continue
				}

				for _, st := range f.Hir.Statements {
					imp, ok := st.(*ir.ImportStmt)
					if !ok {
						continue
					}

					key := imp.Path.String()
					if _, loaded := c.Packages[key]; loaded {
						continue
					}

					handled, err := loadStdPackage(c, key)
					if err != nil {
						return err
					}

					if !handled && c.Loader != nil {
						if err := c.Loader(c, key); err != nil {
							return err
						}
					}

					if _, loaded := c.Packages[key]; loaded {
						added = true
					}
				}
			}
		}

		if !added {
			break
		}
	}

	return nil
}

func (c *CompilerContext) runPipeline(pipeline []string) error {
	pm := buildPassManager()
	if err := pm.BuildOrder(pipeline); err != nil {
		return err
	}

	pkgs := c.topoSortPackages()

	return pm.Run(c.Diag, pkgs, c.TypeUniverse, c.SymAlloc, c.ScAlloc, c.NodeAlloc, c.NameMap, c.Cache)
}

func (c *CompilerContext) topoSortPackages() []*ir.PackageContext {
	edges := map[string][]string{}

	inDegree := map[string]int{}

	for path := range c.Packages {
		edges[path] = nil
		inDegree[path] = 0
	}

	for path, pkg := range c.Packages {
		for _, f := range pkg.Files {
			if f.Hir == nil {
				continue
			}

			for _, st := range f.Hir.Statements {
				imp, ok := st.(*ir.ImportStmt)
				if !ok {
					continue
				}

				target := imp.Path.String()
				if _, exists := c.Packages[target]; !exists {
					continue
				}

				edges[target] = append(edges[target], path)
				inDegree[path]++
			}
		}
	}

	var queue []string
	for path, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, path)
		}
	}

	sort.Strings(queue)

	var order []string
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		order = append(order, cur)
		var promoted []string
		for _, next := range edges[cur] {
			inDegree[next]--
			if inDegree[next] == 0 {
				promoted = append(promoted, next)
			}
		}

		sort.Strings(promoted)
		queue = append(queue, promoted...)
	}

	var leftover []string
	for path := range c.Packages {
		if inDegree[path] > 0 {
			leftover = append(leftover, path)
		}
	}

	sort.Strings(leftover)
	order = append(order, leftover...)

	out := make([]*ir.PackageContext, 0, len(order))
	for _, path := range order {
		if pkg, ok := c.Packages[path]; ok {
			out = append(out, pkg)
		}
	}

	return out
}
