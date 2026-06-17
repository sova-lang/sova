package javascript

import (
	"fmt"
	"os"
	"path/filepath"
	"sova/internal/codegen"
	"sova/internal/codegen/javascript/jsgen"
	"sova/internal/ir"
)

// CodeEmitter implements the codegen.Emitter interface for JavaScript.
type CodeEmitter struct {
	hk                  *codegen.HoistKit[jsgen.Code]
	jf                  *jsgen.File
	deferredInits       []*jsgen.Statement
	loopDepth           int
	loopLabels          []string
	mangledMainName     string
	mainIsAsync         bool
	currentFunc         *ir.FuncDeclStmt
	currentTypeDecl     *ir.TypeDeclStmt
	discardCounter      int
	suppressThisKeyword bool
	moduleBinds         map[string]moduleBind
	moduleOrder         []string
	composableDepth     int
	inSyntheticCtor     bool
	usesChanRuntime     bool
}

// moduleBind tracks the local identifier the emitter chose for a host-language module and the import form (default vs. namespace).
type moduleBind struct {
	Name      string
	IsDefault bool
}

func (e *CodeEmitter) Init(ctx *codegen.EmitContext) error {
	e.hk = codegen.NewHoistKit[jsgen.Code]("_t")
	e.jf = jsgen.NewFile()

	// Enable source maps
	outputFileName := filepath.Base(ctx.OutPath)
	e.jf.EnableSourceMap(outputFileName)

	return nil
}

func (e *CodeEmitter) Emit(ctx *codegen.EmitContext) error {
	// Add source file contents for source maps
	for _, pkg := range ctx.Pkgs {
		for _, file := range pkg.Files {
			// Add source file content (already available in PreparsedFile)
			if file.Filename != "" && file.Content != "" {
				e.jf.AddSourceContent(file.Filename, file.Content)
			}
		}
	}

	// Pre-register module imports so the emitted JS file starts with an `import` header. We walk all extern decls in user packages and assign each module a local bind name; later `@mod` substitutions reuse the same bind.
	for _, pkg := range ctx.Pkgs {
		for _, file := range pkg.Files {
			if file.Hir.Side.Kind == ir.SideSynth {
				continue
			}
			for _, st := range file.Hir.Statements {
				ext, ok := st.(*ir.ExternDeclStmt)
				if !ok || ext.Module == nil || *ext.Module == "" {
					continue
				}
				e.bindForModule(*ext.Module, ext.IsDefaultImport)
			}
		}
	}
	for _, module := range e.moduleOrder {
		bind := e.moduleBinds[module]
		var importLine string
		if bind.IsDefault {
			importLine = fmt.Sprintf("import %s from %q;", bind.Name, module)
		} else {
			importLine = fmt.Sprintf("import * as %s from %q;", bind.Name, module)
		}
		e.jf.Add(jsgen.Raw(importLine))
	}

	if envInit := dotenvSovaEnvJS(ctx); envInit != "" {
		e.jf.Add(jsgen.Raw(envInit))
	}

	e.jf.Add(jsgen.Raw(sovaReifyRuntime))
	if v, ok := ctx.Cache["needs_session_manager"].(bool); ok && v {
		e.jf.Add(jsgen.Raw(sovaWSClientRuntime))
	}

	if ctx.Types.HasAnyOfKind(ir.TK_Chan) {
		e.jf.Add(jsgen.Raw(SovaChanRuntime))
	}

	if jsTestModeFromCache(ctx) {
		emitJSTestRuntime(e.jf)
	}

	// Emit wired-func stubs from the other side (e.g. backend files): the JS frontend never gets the implementation, but it needs callable fetch stubs so cross-side calls resolve to the gemangelt symbol name. Same loop also emits the shared subset of any cross-side TypeDecl: when a backend type carries `shared` members (Stage 3 of the GORM-friendly Sova design), the JS side gets a parallel class with only the shared fields + methods so the wire layer can hand back real class instances rather than property bags.
	for _, pkg := range ctx.TransPkgs {
		for _, file := range pkg.Files {
			if file.Hir.Side.Kind == ir.SideSynth {
				continue
			}
			for _, st := range file.Hir.Statements {
				switch v := st.(type) {
				case *ir.FuncDeclStmt:
					if v.IsWired {
						e.emitWiredStub(ctx, pkg, file.Hir, v)
					}
				case *ir.VarDeclStmt:
					if v.IsWired {
						e.emitWiredVarStub(ctx, pkg, file.Hir, v)
					}
				case *ir.TypeDeclStmt:
					if v.IsExtern || jsHasBuiltinAnnotation(v.Annotations) {
						continue
					}
					if filtered := sharedSubsetTypeDecl(ctx, pkg, v); filtered != nil {
						e.emitTypeDecl(ctx, pkg, file.Hir, filtered, true)
					}
				}
			}
		}
	}

	// Emit code
	for _, pkg := range ctx.Pkgs {
		for _, file := range pkg.Files {
			if file.Hir.Side.Kind == ir.SideSynth {
				continue
			}
			for _, st := range file.Hir.Statements {
				if codegen.TopLevelStmtPrunable(ctx, st) {
					continue
				}
				e.emitStmt(ctx, pkg, file.Hir, st, true)
			}

			for _, init := range e.deferredInits {
				e.jf.Add(init)
			}
			e.deferredInits = nil
		}
	}

	if jsTestModeFromCache(ctx) {
		e.emitJSTestImplFuncs(ctx)
	}

	if e.mangledMainName != "" && !jsTestModeFromCache(ctx) {
		e.jf.Add(jsgen.Comment("Call main function"))
		if e.mainIsAsync {
			e.jf.Add(jsgen.Raw(fmt.Sprintf("(async () => { await %s(); })()", e.mangledMainName)))
		} else {
			e.jf.Add(jsgen.Id(e.mangledMainName).Call())
		}
	}

	code, sourceMap := e.jf.RenderWithSourceMap()

	if err := os.MkdirAll(filepath.Dir(ctx.OutPath), 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(ctx.OutPath, []byte(code), 0o644); err != nil {
		return err
	}

	if sourceMap != nil {
		sourceMapJSON, err := sourceMap.ToJSON()
		if err != nil {
			return err
		}

		sourceMapPath := ctx.OutPath + ".map"
		if err := os.WriteFile(sourceMapPath, []byte(sourceMapJSON), 0o644); err != nil {
			return err
		}
	}

	return nil
}
