package javascript

import (
	"fmt"
	"os"
	"path/filepath"
	"sova/internal/codegen"
	"sova/internal/codegen/javascript/jsgen"
	"sova/internal/ir"
)

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

type moduleBind struct {
	Name      string
	IsDefault bool
}

func (e *CodeEmitter) Init(ctx *codegen.EmitContext) error {
	e.hk = codegen.NewHoistKit[jsgen.Code]("_t")
	e.jf = jsgen.NewFile()

	outputFileName := filepath.Base(ctx.OutPath)
	e.jf.EnableSourceMap(outputFileName)

	return nil
}

func (e *CodeEmitter) Emit(ctx *codegen.EmitContext) error {

	for _, pkg := range ctx.Pkgs {
		for _, file := range pkg.Files {

			if file.Filename != "" && file.Content != "" {
				e.jf.AddSourceContent(file.Filename, file.Content)
			}
		}
	}

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
