package golang

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sova/internal/codegen"
	"sova/internal/ir"
	"strconv"
	"strings"

	"github.com/dave/jennifer/jen"
	"golang.org/x/tools/imports"
)

//go:embed dev_helpers.go.tpl
var devHelpersTemplate string

//go:embed prod_helpers.go.tpl
var prodHelpersTemplate string

// CodeEmitter implements the codegen.Emitter interface.
type CodeEmitter struct {
	hk              *codegen.HoistKit[jen.Code]
	jf              *jen.File
	deferredInits   []jen.Code
	loopDepth       int
	loopLabels      []string
	mangledMainName string
	currentFunc     *ir.FuncDeclStmt
	typeDecls       map[ir.TypID]*ir.TypeDeclStmt
	wiredFuncs      []*ir.FuncDeclStmt
	wiredVars       []*ir.VarDeclStmt
	composableDepth int
	externImports   map[string]string
}

func (e *CodeEmitter) Init(ctx *codegen.EmitContext) error {
	e.hk = codegen.NewHoistKit[jen.Code]("_t")
	e.jf = jen.NewFile("main")
	e.loopLabels = make([]string, 0)
	e.typeDecls = map[ir.TypID]*ir.TypeDeclStmt{}
	e.externImports = map[string]string{}
	for _, pkg := range ctx.Pkgs {
		for _, file := range pkg.Files {
			for _, st := range file.Hir.Statements {
				if td, ok := st.(*ir.TypeDeclStmt); ok && td.Name.Sym != 0 {
					if sym, ok := pkg.Syms.GetByID(td.Name.Sym); ok && sym.Typ != 0 {
						e.typeDecls[sym.Typ] = td
					}
				}
			}
		}
	}
	return nil
}

func (e *CodeEmitter) Emit(ctx *codegen.EmitContext) error {
	testMode := testModeFromCache(ctx)
	e.withScope(func() {
		block := e.jf.Group

		emitSovaErrorType(block)

		if !testMode {
			emitTestHarnessStubs(block)
		}

		_, hasWire := ctx.Cache["wire_state_typ"]
		if hasWire || testMode {
			e.emitWireStateDecl(ctx, block)
		}

		if testMode {
			emitTestRuntime(block)
		}

		for _, pkg := range ctx.Pkgs {
			for _, file := range pkg.Files {
				for _, st := range file.Hir.Statements {
					e.emitStmt(ctx, pkg, file.Hir, block, st, true)
				}
			}
		}

		if testMode {
			e.emitTestImplFuncs(ctx, block)
		}

		if len(ctx.InitPlan) > 0 || len(e.deferredInits) > 0 {
			block.Add(jen.Func().Id("init").Params().BlockFunc(func(g *jen.Group) {
				for _, init := range e.deferredInits {
					g.Add(init).Line()
				}

				for _, initStmt := range ctx.InitPlan {
					e.emitStmt(ctx, initStmt.Pkg, initStmt.File, g, initStmt.Stmt, false)
				}
			}))
		}

		block.Func().Id("main").Params().BlockFunc(func(g *jen.Group) {
			if testMode {
				emitTestDriverMain(ctx, g)
				return
			}
			if e.mangledMainName != "" {
				g.Id(e.mangledMainName).Call()
			}
			if len(e.wiredFuncs) > 0 || len(e.wiredVars) > 0 {
				e.emitWireServerBoot(ctx, g)
			} else {
				e.emitDevOnlyBoot(ctx, g)
			}
		})
	})

	if err := os.MkdirAll(filepath.Dir(ctx.OutPath), 0o755); err != nil {
		return err
	}

	rendered := fmt.Appendf(nil, "%#v", e.jf)
	fixed, fixErr := imports.Process(ctx.OutPath, rendered, &imports.Options{
		Comments:   true,
		TabIndent:  true,
		TabWidth:   8,
		FormatOnly: false,
	})
	if fixErr == nil {
		rendered = fixed
	}
	if err := os.WriteFile(ctx.OutPath, rendered, 0o644); err != nil {
		return err
	}

	outDir := filepath.Dir(ctx.OutPath)
	devPath := filepath.Join(outDir, "dev_helpers.go")
	prodPath := filepath.Join(outDir, "prod_helpers.go")
	if prodModeFromCache(ctx) {
		_ = os.Remove(devPath)
		if err := os.WriteFile(prodPath, []byte(prodHelpersTemplate), 0o644); err != nil {
			return err
		}
	} else {
		_ = os.Remove(prodPath)
		if err := os.WriteFile(devPath, []byte(devHelpersTemplate), 0o644); err != nil {
			return err
		}
	}

	modPath := filepath.Join(outDir, "go.mod")
	modContent := emittedGoMod(ctx)
	if err := os.WriteFile(modPath, []byte(modContent), 0o644); err != nil {
		return err
	}
	sumAnchor := goSumAnchorPath(ctx)
	if sumAnchor != "" {
		if data, err := os.ReadFile(sumAnchor); err == nil {
			_ = os.WriteFile(filepath.Join(outDir, "go.sum"), data, 0o644)
		}
	}
	if needsGoModTidy(ctx) {
		if err := goModTidy(outDir); err != nil {
			return fmt.Errorf("go mod tidy in %s: %w", outDir, err)
		}
	}
	if sumAnchor != "" {
		if data, err := os.ReadFile(filepath.Join(outDir, "go.sum")); err == nil {
			if mkErr := os.MkdirAll(filepath.Dir(sumAnchor), 0o755); mkErr == nil {
				_ = os.WriteFile(sumAnchor, data, 0o644)
			}
		}
	}
	return nil
}

// goModTidy runs `go mod tidy` in the given directory so that every dependency in the generated `go.mod` (the WebSocket module when the build needs the session manager, plus every extern-pinned Go module) resolves into the module cache and into a populated `go.sum`. Hidden by default behind a feature gate (see `needsGoModTidy`) so legacy stdlib-only builds keep their fast no-network path.
func goModTidy(dir string) error {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = dir
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func needsSessionManagerFromCache(ctx *codegen.EmitContext) bool {
	if ctx.Cache == nil {
		return false
	}
	v, ok := ctx.Cache["needs_session_manager"].(bool)
	return ok && v
}

// needsGoModTidy reports whether the generated module has any non-stdlib `require` entries that would benefit from a `go mod tidy` invocation. True when the session manager is active OR when any extern declaration pinned a Go module via `extern "path@version"` / `backend("path@version")`. The two-state gate keeps the existing fast path for builds that don't reach for the network.
func needsGoModTidy(ctx *codegen.EmitContext) bool {
	if needsSessionManagerFromCache(ctx) {
		return true
	}
	return len(externGoModulesFromCache(ctx)) > 0
}

// externGoModulesFromCache returns the `(modulePath -> version)` map produced by PassAggregateExternModules. Returns an empty map (never nil) when the cache entry is missing so callers can range over it unconditionally.
func externGoModulesFromCache(ctx *codegen.EmitContext) map[string]string {
	if ctx.Cache == nil {
		return map[string]string{}
	}
	raw, ok := ctx.Cache["extern_go_modules"]
	if !ok || raw == nil {
		return map[string]string{}
	}
	m, ok := raw.(map[string]string)
	if !ok {
		return map[string]string{}
	}
	return m
}

// emittedGoMod returns the contents of the synthetic `go.mod` for the output module. The base module declaration is always emitted; the `gorilla/websocket` require line is appended when the build needs the session manager; and any extern-pinned Go modules (declared via `extern "path@version"` / `backend("path@version")` and aggregated by `PassAggregateExternModules`) are appended after, sorted alphabetically for a stable file content across builds.
func emittedGoMod(ctx *codegen.EmitContext) string {
	needsManager := false
	if ctx.Cache != nil {
		if v, ok := ctx.Cache["needs_session_manager"].(bool); ok {
			needsManager = v
		}
	}
	var b strings.Builder
	b.WriteString("module sovaapp\n\ngo 1.23\n")
	if needsManager {
		b.WriteString("\nrequire github.com/gorilla/websocket v1.5.3\n")
	}
	pins := externGoModulesFromCache(ctx)
	if len(pins) > 0 {
		paths := make([]string, 0, len(pins))
		for p := range pins {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		b.WriteString("\n")
		for _, p := range paths {
			fmt.Fprintf(&b, "require %s %s\n", p, pins[p])
		}
	}
	return b.String()
}

// goSumAnchorPath returns the persistent location of the `go.sum` file Sova keeps across builds (`<source-root>/.sova/go.sum`). The output directory itself is treated as a wipeable build artefact, so we round-trip the sum file through the workspace's `.sova/` directory (which the package manager already owns) to keep reproducibility intact between clean rebuilds. Returns the empty string when no source-root information is available, in which case the round-trip is skipped and `go mod tidy` regenerates the sum from scratch.
func goSumAnchorPath(ctx *codegen.EmitContext) string {
	if ctx.Cache == nil {
		return ""
	}
	raw, ok := ctx.Cache["build_config"]
	if !ok {
		return ""
	}
	src, ok := raw.(interface{ SourceDirectory() string })
	if !ok {
		return ""
	}
	root := strings.TrimSpace(src.SourceDirectory())
	if root == "" {
		return ""
	}
	return filepath.Join(root, ".sova", "go.sum")
}

func prodModeFromCache(ctx *codegen.EmitContext) bool {
	if ctx.Cache == nil {
		return false
	}
	raw, ok := ctx.Cache["build_config"]
	if !ok {
		return false
	}
	if cfg, ok := raw.(interface{ ProdModeValue() bool }); ok {
		return cfg.ProdModeValue()
	}
	return false
}

func (e *CodeEmitter) emitBlock(ctx *codegen.EmitContext, pkg *ir.PackageContext, file *ir.File, block *jen.Group, stmts []ir.Stmt) {
	e.withScope(func() {
		block.BlockFunc(func(newBlock *jen.Group) {
			for _, st := range stmts {
				e.emitStmt(ctx, pkg, file, newBlock, st, false)
			}
		})
	})
}

func (e *CodeEmitter) emitStmt(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, block *jen.Group, st ir.Stmt, topLevel bool) {
	switch s := st.(type) {
	case *ir.WireRulesetStmt:
		return
	case *ir.TestDeclStmt, *ir.GroupDeclStmt, *ir.SetupStmt, *ir.TeardownStmt:
		// The test driver consumes these declarations via the TestRegistry view; the regular statement emitter ignores them so an `on test` file does not produce stray top-level code.
		return
	case *ir.AssertStmt:
		e.emitAssertStmt(ctx, pkg, f, block, s)
		return
	case *ir.AsSessionStmt:
		e.emitAsSessionStmt(ctx, pkg, f, block, s)
		return
	case *ir.GoStmt:
		e.emitGoStmt(ctx, pkg, f, block, s)
		return
	case *ir.DeferStmt:
		e.emitDeferStmt(ctx, pkg, f, block, s)
		return
	case *ir.SelectStmt:
		e.emitSelectStmt(ctx, pkg, f, block, s)
		return
	case *ir.BlockStmt:
		if topLevel {
			return
		}

		e.emitBlock(ctx, pkg, f, block, s.Stmts)
	case *ir.VarDeclStmt:
		if s.IsWired && topLevel {
			e.wiredVars = append(e.wiredVars, s)
			e.emitWiredVarGetter(ctx, pkg, f, block, s)
			e.emitWiredVarHandler(ctx, pkg, f, block, s)
			return
		}
		if len(s.Targets) == 1 {
			target := &s.Targets[0]
			if topLevel {
				if _, isFuncLit := s.Init.(*ir.FuncLitExpr); isFuncLit && target.Name != nil {
					name := symNameWithUnused(ctx, pkg, target.Name.Sym)
					ty := typeToGoWithContext(ctx, pkg, ctx.Types, typeOfSym(pkg, target.Name.Sym))

					e.withStmt(block, func() jen.Code {
						jv := jen.Var().Id(name).Add(ty)
						orig := symOrigName(ctx, target.Name.Sym)
						if orig != "" {
							jv.Commentf("Original name: %s", orig)
						}
						return jv
					})

					rhs := e.buildExpr(ctx, pkg, f, s.Init)
					e.deferredInits = append(e.deferredInits, jen.Id(name).Op("=").Add(rhs))
					return
				}
			}

			e.withStmt(block, func() jen.Code {
				if target.Name == nil {
					return jen.Id("_").Op("=").Add(e.buildExpr(ctx, pkg, f, s.Init))
				}

				name := symNameWithUnused(ctx, pkg, target.Name.Sym)

				var rhs jen.Code = nil
				asConst := false
				if s.Init != nil {
					rhs = e.buildExpr(ctx, pkg, f, s.Init)

					if isExprConstant(s.Init) {
						asConst = true
					}

					// Handle implicit lifting: T -> *T (option<T>)
					targetType := typeOfSym(pkg, target.Name.Sym)
					initType := s.Init.GetType()
					if targetType != 0 && initType != 0 {
						targetTy, _ := ctx.Types.GetByID(targetType)
						initTy, _ := ctx.Types.GetByID(initType)
						// If target is option and init is not option/none, we need to lift
						if targetTy != nil && targetTy.Kind == ir.TK_Option &&
							initTy != nil && initTy.Kind != ir.TK_Option && initTy.Kind != ir.TK_PrimitiveNone {
							// Wrap in a helper function that returns a pointer
							tempVar := e.hk.NewTemp()
							rhs = jen.Func().Params().Op("*").Add(typeToGoWithContext(ctx, pkg, ctx.Types, targetTy.ElemType)).Block(
								jen.Id(tempVar).Op(":=").Add(rhs),
								jen.Return(jen.Op("&").Id(tempVar)),
							).Call()
							asConst = false // Can't use const with pointer creation
						}
						// Numeric primitive coercion (byte ↔ int, etc.) via explicit Go conversion.
						if rhs != nil {
							if conv := goNumericConversionWrapper(targetType, initType, ctx.Types, rhs); conv != nil {
								rhs = conv
								asConst = false
							}
						}
						// When boxing a typed primitive into `any`, wrap the literal in the target Go type so the interface stores e.g. `int64` rather than Go's default `int`. Without this, a subsequent `v.(int64)` type assertion on the boxed value would miss.
						if rhs != nil && targetTy != nil && targetTy.Kind == ir.TK_PrimitiveAny {
							if conv := goAnyBoxWrapper(initType, ctx.Types, rhs); conv != nil {
								rhs = conv
								asConst = false
							}
						}
					}
				}

				ty := typeToGoWithContext(ctx, pkg, ctx.Types, typeOfSym(pkg, target.Name.Sym))

				jv := jen.Var()
				if asConst && s.IsConst {
					jv = jen.Const()
				}

				jv = jv.Id(name).Add(ty).Op("=").Add(rhs)

				orig := symOrigName(ctx, target.Name.Sym)
				if orig != "" {
					jv.Commentf("Original name: %s", orig)
				}

				return jv
			})
		} else {
			hasNonDiscard := false
			for _, target := range s.Targets {
				if target.Name != nil {
					name := symNameWithUnused(ctx, pkg, target.Name.Sym)
					if name != "_" {
						hasNonDiscard = true
						break
					}
				}
			}

			if hasNonDiscard {
				tupleVarName := "__tuple_tmp_" + e.hk.NewTemp()

				e.withStmt(block, func() jen.Code {
					rhs := e.buildExpr(ctx, pkg, f, s.Init)
					return jen.Id(tupleVarName).Op(":=").Add(rhs)
				})

				e.withStmt(block, func() jen.Code {
					var names []jen.Code
					var values []jen.Code

					for i, target := range s.Targets {
						if target.Name == nil {
							names = append(names, jen.Id("_"))
						} else {
							names = append(names, jen.Id(symNameWithUnused(ctx, pkg, target.Name.Sym)))
						}

						elemAccess := jen.Id(tupleVarName).Index(jen.Lit(i))

						if target.Name != nil {
							elemType := typeOfSym(pkg, target.Name.Sym)
							if elemType != 0 {
								elemAccess = elemAccess.Assert(typeToGoWithContext(ctx, pkg, ctx.Types, elemType))
							}
						}

						values = append(values, elemAccess)
					}

					return jen.List(names...).Op(":=").List(values...)
				})
			}
		}
	case *ir.FuncDeclStmt:
		if hasBuiltinAnnotation(s.Annotations) {
			return
		}
		if !s.IsWired {
			side := ir.SideShared
			if s.Side != nil {
				side = s.Side.Kind
			} else if f != nil {
				side = f.Side.Kind
			}
			if side == ir.SideFrontend {
				return
			}
		}
		if s.IsWired {
			e.wiredFuncs = append(e.wiredFuncs, s)
		}
		e.withStmt(block, func() jen.Code {
			funcName := symName(ctx, s.Name.Sym)
			orig := symOrigName(ctx, s.Name.Sym)

			if orig == "main" && len(s.Params) == 0 && e.mangledMainName == "" {
				e.mangledMainName = funcName
			}

			params := make([]jen.Code, 0, len(s.Params)+1)
			if s.IsWired {
				params = append(params, jen.Id("__session").Op("*").Id("fn____Session"))
			}
			for _, param := range s.Params {
				paramName := symNameWithUnused(ctx, pkg, param.Name.Sym)
				paramType := typeToGoWithContext(ctx, pkg, ctx.Types, param.Type.Typ)
				params = append(params, jen.Id(paramName).Add(paramType))
			}

			funcDecl := jen.Func().Id(funcName).Params(params...)

			if s.ReturnType.Typ != ctx.Types.TypNone() {
				returnType := typeToGoWithContext(ctx, pkg, ctx.Types, s.ReturnType.Typ)
				funcDecl = funcDecl.Add(returnType)
			}

			prevFunc := e.currentFunc
			e.currentFunc = s

			fDecl := funcDecl.BlockFunc(func(g *jen.Group) {
				e.emitBlock(ctx, pkg, f, g, s.Body.Stmts)
			})

			e.currentFunc = prevFunc
			f := fDecl

			if orig != "" {
				f.Commentf("Original name: %s", orig)
			}
			return f
		})
		if s.IsWired {
			e.emitWiredHandler(ctx, pkg, f, block, s)
			if s.Wire != nil && s.Wire.Transport == "ws" && needsSessionManagerFromCache(ctx) {
				e.emitWiredWSAdapter(ctx, pkg, f, block, s)
			}
		}
	case *ir.ExternDeclStmt:
		targetSide := ir.SideBackend
		if f.Side.Kind == ir.SideFrontend {
			return
		} else if f.Side.Kind == ir.SideShared {
			targetSide = ir.SideBackend
		}

		for _, fn := range s.Funcs {
			var sideMapping *ir.SideMapping
			var externModule *string

			if fn.Mapping.Simple != nil {
				sideMapping = &ir.SideMapping{
					NativeFunc: *fn.Mapping.Simple,
					Module:     nil,
				}
				externModule = s.Module
			} else if fn.Mapping.Shared != nil {
				mapping, exists := fn.Mapping.Shared[targetSide]
				if !exists {
					continue
				}
				sideMapping = mapping
				externModule = s.Module
			} else {
				continue
			}

			if sideMapping.Module != nil && *sideMapping.Module != "" {
				e.registerExternImport(*sideMapping.Module, sideMapping.NativeFunc)
			} else if externModule != nil && *externModule != "" {
				e.registerExternImport(*externModule, sideMapping.NativeFunc)
			}

			e.withStmt(block, func() jen.Code {
				funcName := symName(ctx, fn.Name.Sym)
				orig := symOrigName(ctx, fn.Name.Sym)

				params := make([]jen.Code, len(fn.Params))
				paramNames := make([]jen.Code, len(fn.Params))
				for i, param := range fn.Params {
					paramName := param.Name.Name
					paramType := typeToGoWithContext(ctx, pkg, ctx.Types, param.Type.Typ)
					params[i] = jen.Id(paramName).Add(paramType)
					paramNames[i] = jen.Id(paramName)
				}

				funcDecl := jen.Func().Id(funcName).Params(params...)

				var returnType jen.Code
				hasReturn := fn.ReturnType != nil && fn.ReturnType.Typ != ctx.Types.TypNone()
				if hasReturn {
					returnType = typeToGoWithContext(ctx, pkg, ctx.Types, fn.ReturnType.Typ)
					funcDecl = funcDecl.Add(returnType)
				}

				nativeCall := sideMapping.NativeFunc
				modulePath := ""
				if sideMapping.Module != nil {
					nativeCall = e.replaceModPlaceholder(nativeCall, *sideMapping.Module)
					modulePath = *sideMapping.Module
				} else if externModule != nil {
					nativeCall = e.replaceModPlaceholder(nativeCall, *externModule)
					modulePath = *externModule
				}

				origNameForMock := orig
				if origNameForMock == "" {
					origNameForMock = fn.Name.Name
				}
				mockableName := origNameForMock
				if pkg != nil && pkg.Path.String() == "std/testing" {
					mockableName = ""
				}
				testMode := isTestMode(ctx)

				result := funcDecl.BlockFunc(func(g *jen.Group) {
					if testMode && mockableName != "" {
						mockArgs := []jen.Code{jen.Lit(mockableName)}
						mockArgs = append(mockArgs, paramNames...)
						if hasReturn {
							g.If(jen.List(jen.Id("__mockV"), jen.Id("__mockHas"), jen.Id("__mockReg")).Op(":=").Id("__sovaMockHook").Call(mockArgs...), jen.Id("__mockReg")).Block(
								jen.If(jen.Id("__mockHas")).Block(
									jen.Return(jen.Id("__mockV").Assert(returnType)),
								),
								jen.Var().Id("__mockZero").Add(returnType),
								jen.Return(jen.Id("__mockZero")),
							)
						} else {
							g.If(jen.List(jen.Id("_"), jen.Id("_"), jen.Id("__mockReg")).Op(":=").Id("__sovaMockHook").Call(mockArgs...), jen.Id("__mockReg")).Block(
								jen.Return(),
							)
						}
					}
					callExpr := e.buildNativeCallWithModule(nativeCall, modulePath, paramNames)
					if hasReturn {
						g.Return(callExpr)
					} else {
						g.Add(callExpr)
					}
				})

				if orig != "" {
					result.Commentf("Original name: %s", orig)
				}
				return result
			})
		}

		for _, v := range s.Vars {
			var sideMapping *ir.SideMapping
			var externModule *string

			if v.Mapping.Simple != nil {
				sideMapping = &ir.SideMapping{
					NativeFunc: *v.Mapping.Simple,
					Module:     nil,
				}
				externModule = s.Module
			} else if v.Mapping.Shared != nil {
				mapping, exists := v.Mapping.Shared[targetSide]
				if !exists {
					continue
				}
				sideMapping = mapping
				externModule = s.Module
			} else {
				continue
			}

			if sideMapping.Module != nil && *sideMapping.Module != "" {
				e.registerExternImport(*sideMapping.Module, sideMapping.NativeFunc)
			} else if externModule != nil && *externModule != "" {
				e.registerExternImport(*externModule, sideMapping.NativeFunc)
			}

			e.withStmt(block, func() jen.Code {
				varName := symName(ctx, v.Name.Sym)
				orig := symOrigName(ctx, v.Name.Sym)
				varType := typeToGoWithContext(ctx, pkg, ctx.Types, v.Type.Typ)

				nativeRef := sideMapping.NativeFunc
				modulePath := ""
				if sideMapping.Module != nil {
					nativeRef = e.replaceModPlaceholder(nativeRef, *sideMapping.Module)
					modulePath = *sideMapping.Module
				} else if externModule != nil {
					nativeRef = e.replaceModPlaceholder(nativeRef, *externModule)
					modulePath = *externModule
				}

				nativeExpr := e.buildNativeRefWithModule(nativeRef, modulePath)

				var result *jen.Statement
				if v.IsConst {
					result = jen.Var().Id(varName).Add(varType).Op("=").Add(nativeExpr)
				} else {
					result = jen.Var().Id(varName).Add(varType).Op("=").Add(nativeExpr)
				}

				if orig != "" {
					result.Commentf("Original name: %s", orig)
				}
				return result
			})
		}
	case *ir.MixinDeclStmt:
		_ = s
	case *ir.ImportStmt:
		_ = s
	case *ir.InterfaceDeclStmt:
		if s.IsExtern {
			return
		}
		ifaceName := symName(ctx, s.Name.Sym)
		ifaceSym := s.Name.Sym
		_ = ifaceSym
		var methods []jen.Code
		for _, sig := range s.Methods {
			methodName := symName(ctx, sig.Name.Sym)
			params := make([]jen.Code, len(sig.Params))
			for i, param := range sig.Params {
				params[i] = typeToGoWithContext(ctx, pkg, ctx.Types, param.Type.Typ)
			}
			method := jen.Id(methodName).Params(params...)
			if sig.ReturnType != nil && sig.ReturnType.Typ != 0 && sig.ReturnType.Typ != ctx.Types.TypNone() {
				method = method.Add(typeToGoWithContext(ctx, pkg, ctx.Types, sig.ReturnType.Typ))
			}
			methods = append(methods, method)
		}
		e.withStmt(block, func() jen.Code {
			return jen.Type().Id(ifaceName).Interface(methods...)
		})
	case *ir.TypeDeclStmt:
		if s.IsExtern {
			return
		}
		if hasBuiltinAnnotation(s.Annotations) {
			return
		}
		typeName := symName(ctx, s.Name.Sym)
		structFields := []jen.Code{}
		for _, ref := range s.MixedIn {
			if ref.Sym == 0 {
				continue
			}
			symPkg := pkg
			if ref.Qualifier != "" {
				if found := lookupImportedPackage(ctx, pkg, ref.Qualifier); found != nil {
					symPkg = found
				}
			}
			embedSym, ok := symPkg.Syms.GetByID(ref.Sym)
			if !ok || embedSym.Typ == 0 {
				continue
			}
			embedTy, ok := ctx.Types.GetByID(embedSym.Typ)
			if !ok || embedTy.Kind != ir.TK_Struct {
				continue
			}
			if embedTy.IsExtern {
				structFields = append(structFields, jen.Qual(embedTy.ExternModule, embedTy.StructName))
			} else {
				structFields = append(structFields, jen.Id(symName(ctx, ref.Sym)))
			}
		}
		for _, field := range s.Fields {
			fieldType := typeToGoWithContext(ctx, pkg, ctx.Types, field.Type.Typ)
			fieldDecl := jen.Id(goExportedName(field.Name.Name)).Add(fieldType)
			tag := buildStructTag(field.Annotations)
			if tag == nil {
				tag = map[string]string{}
			}
			if _, ok := tag["json"]; !ok && !strings.HasPrefix(field.Name.Name, "__") {
				tag["json"] = field.Name.Name
			}
			if len(tag) > 0 {
				fieldDecl = fieldDecl.Tag(tag)
			}
			structFields = append(structFields, fieldDecl)
			if fieldHasReactiveAnnotation(field.Annotations) {
				obsType := jen.Index().Func().Params(
					typeToGoWithContext(ctx, pkg, ctx.Types, field.Type.Typ),
					typeToGoWithContext(ctx, pkg, ctx.Types, field.Type.Typ),
				)
				obsField := jen.Id("__obs" + goExportedName(field.Name.Name)).Add(obsType)
				structFields = append(structFields, obsField)
			}
		}
		e.withStmt(block, func() jen.Code {
			return jen.Type().Id(typeName).Struct(structFields...)
		})
		for _, field := range s.Fields {
			if !fieldHasReactiveAnnotation(field.Annotations) {
				continue
			}
			fldRef := field
			e.withStmt(block, func() jen.Code {
				fieldType := typeToGoWithContext(ctx, pkg, ctx.Types, fldRef.Type.Typ)
				fieldName := goExportedName(fldRef.Name.Name)
				setName := "set" + fieldName
				return jen.Func().Params(jen.Id("this").Op("*").Id(typeName)).Id(setName).Params(jen.Id("v").Add(fieldType)).BlockFunc(func(g *jen.Group) {
					g.Id("__old").Op(":=").Id("this").Dot(fieldName)
					g.Id("this").Dot(fieldName).Op("=").Id("v")
					g.For(jen.List(jen.Id("_"), jen.Id("__o")).Op(":=").Range().Id("this").Dot("__obs" + fieldName)).Block(
						jen.Id("__o").Call(jen.Id("__old"), jen.Id("v")),
					)
				})
			})
			e.withStmt(block, func() jen.Code {
				fieldName := goExportedName(fldRef.Name.Name)
				obsName := "observe" + fieldName
				fnType := jen.Func().Params(
					typeToGoWithContext(ctx, pkg, ctx.Types, fldRef.Type.Typ),
					typeToGoWithContext(ctx, pkg, ctx.Types, fldRef.Type.Typ),
				)
				obsField := "__obs" + fieldName
				return jen.Func().Params(jen.Id("this").Op("*").Id(typeName)).Id(obsName).Params(jen.Id("__fn").Add(fnType)).Func().Params().Any().BlockFunc(func(g *jen.Group) {
					g.Id("__idx").Op(":=").Qual("", "len").Call(jen.Id("this").Dot(obsField))
					g.Id("this").Dot(obsField).Op("=").Append(jen.Id("this").Dot(obsField), jen.Id("__fn"))
					g.Return(jen.Func().Params().Any().BlockFunc(func(rg *jen.Group) {
						rg.If(jen.Id("__idx").Op(">=").Qual("", "len").Call(jen.Id("this").Dot(obsField))).Block(
							jen.Return(jen.Nil()),
						)
						rg.Id("this").Dot(obsField).Op("=").Append(
							jen.Id("this").Dot(obsField).Index(jen.Empty(), jen.Id("__idx")),
							jen.Id("this").Dot(obsField).Index(jen.Id("__idx").Op("+").Lit(1), jen.Empty()).Op("..."),
						)
						rg.Return(jen.Nil())
					}))
				})
			})
		}
		for _, method := range s.Methods {
			methodRef := method
			e.withStmt(block, func() jen.Code {
				fn := methodRef.Func
				methodName := symName(ctx, fn.Name.Sym)
				params := make([]jen.Code, len(fn.Params))
				for i, param := range fn.Params {
					paramName := symNameWithUnused(ctx, pkg, param.Name.Sym)
					params[i] = jen.Id(paramName).Add(typeToGoWithContext(ctx, pkg, ctx.Types, param.Type.Typ))
				}
				receiver := jen.Id("this").Op("*").Id(typeName)
				funcDecl := jen.Func().Params(receiver).Id(methodName).Params(params...)
				hasReturn := fn.ReturnType != nil && fn.ReturnType.Typ != 0 && fn.ReturnType.Typ != ctx.Types.TypNone()
				if hasReturn {
					funcDecl = funcDecl.Add(typeToGoWithContext(ctx, pkg, ctx.Types, fn.ReturnType.Typ))
				}
				return funcDecl.BlockFunc(func(g *jen.Group) {
					prevFunc := e.currentFunc
					e.currentFunc = fn
					defer func() { e.currentFunc = prevFunc }()
					for _, st := range fn.Body.Stmts {
						e.emitStmt(ctx, pkg, f, g, st, false)
					}
				})
			})
		}
		hasUserToString := false
		hasUserHashCode := false
		for _, m := range s.Methods {
			if m.Func.Name.Name == "toString" {
				hasUserToString = true
			}
			if m.Func.Name.Name == "hashCode" {
				hasUserHashCode = true
			}
		}
		if !hasUserToString {
			decl := s
			e.withStmt(block, func() jen.Code {
				return jen.Func().Params(jen.Id("this").Op("*").Id(typeName)).Id("toString").Params().String().BlockFunc(func(g *jen.Group) {
					var format strings.Builder
					format.WriteString(decl.Name.Name)
					format.WriteString("{")
					var args []jen.Code
					for i, field := range decl.Fields {
						if i > 0 {
							format.WriteString(", ")
						}
						format.WriteString(field.Name.Name)
						format.WriteString(": %v")
						args = append(args, jen.Id("this").Dot(goExportedName(field.Name.Name)))
					}
					format.WriteString("}")
					call := []jen.Code{jen.Lit(format.String())}
					call = append(call, args...)
					g.Return(jen.Qual("fmt", "Sprintf").Call(call...))
				})
			})
		}
		if !hasUserHashCode {
			decl := s
			e.withStmt(block, func() jen.Code {
				return jen.Func().Params(jen.Id("this").Op("*").Id(typeName)).Id("hashCode").Params().Int64().BlockFunc(func(g *jen.Group) {
					g.Var().Id("h").Int64().Op("=").Lit(int64(5381))
					var format strings.Builder
					format.WriteString(decl.Name.Name)
					var args []jen.Code
					for _, field := range decl.Fields {
						format.WriteString("|%v")
						args = append(args, jen.Id("this").Dot(goExportedName(field.Name.Name)))
					}
					call := []jen.Code{jen.Lit(format.String())}
					call = append(call, args...)
					g.Id("repr").Op(":=").Qual("fmt", "Sprintf").Call(call...)
					g.For(jen.List(jen.Id("_"), jen.Id("c")).Op(":=").Range().Id("repr")).Block(
						jen.Id("h").Op("=").Parens(jen.Parens(jen.Id("h").Op("<<").Lit(5)).Op("+").Id("h")).Op("+").Int64().Call(jen.Id("c")),
					)
					g.Return(jen.Id("h"))
				})
			})
		}
		for _, ctor := range s.Ctors {
			ctorRef := ctor
			decl := s
			e.withStmt(block, func() jen.Code {
				ctorName := symName(ctx, ctorRef.Sym)
				params := make([]jen.Code, len(ctorRef.Params))
				for i, param := range ctorRef.Params {
					paramName := symNameWithUnused(ctx, pkg, param.Name.Sym)
					params[i] = jen.Id(paramName).Add(typeToGoWithContext(ctx, pkg, ctx.Types, param.Type.Typ))
				}
				returnType := jen.Op("*").Id(typeName)
				return jen.Func().Id(ctorName).Params(params...).Add(returnType).BlockFunc(func(g *jen.Group) {
					var inits []jen.Code
					for _, field := range decl.Fields {
						if field.Default != nil {
							inits = append(inits, jen.Id(goExportedName(field.Name.Name)).Op(":").Add(e.buildExpr(ctx, pkg, f, field.Default)))
						}
					}
					g.Id("this").Op(":=").Op("&").Id(typeName).Values(inits...)
					for _, st := range ctorRef.Body.Stmts {
						e.emitStmt(ctx, pkg, f, g, st, false)
					}
					g.Return(jen.Id("this"))
				})
			})
		}
		for _, cast := range s.Casts {
			castRef := cast
			e.withStmt(block, func() jen.Code {
				castName := symName(ctx, castRef.Sym)
				paramName := symNameWithUnused(ctx, pkg, castRef.Param.Name.Sym)
				paramType := typeToGoWithContext(ctx, pkg, ctx.Types, castRef.Param.Type.Typ)
				returnTyp := castRef.Param.Type.Typ
				if castRef.ReturnType != nil && castRef.ReturnType.Typ != 0 {
					returnTyp = castRef.ReturnType.Typ
				} else {
					returnTyp = typeOfSym(pkg, s.Name.Sym)
				}
				returnType := typeToGoWithContext(ctx, pkg, ctx.Types, returnTyp)
				return jen.Func().Id(castName).Params(jen.Id(paramName).Add(paramType)).Add(returnType).BlockFunc(func(g *jen.Group) {
					for _, st := range castRef.Body.Stmts {
						e.emitStmt(ctx, pkg, f, g, st, false)
					}
				})
			})
		}

	case *ir.EnumDeclStmt:
		enumName := symName(ctx, s.Name.Sym)
		enumTyp, _ := ctx.Types.GetByID(typeOfSym(pkg, s.Name.Sym))

		if enumTyp != nil && enumTyp.IsNumeric {
			// Numeric enum: type alias + constants
			e.withStmt(block, func() jen.Code {
				return jen.Type().Id(enumName).Int64()
			})

			e.withStmt(block, func() jen.Code {
				return jen.Const().DefsFunc(func(g *jen.Group) {
					for _, c := range enumTyp.EnumCases {
						g.Id(enumName + c.Name).Id(enumName).Op("=").Id(enumName).Call(jen.Lit(c.Value))
					}
				})
			})
		} else if enumTyp != nil {
			// Payload enum: struct type + instances + methods
			e.withStmt(block, func() jen.Code {
				fields := []jen.Code{
					jen.Id("__ordinal").Int64(),
					jen.Id("__name").String(),
				}
				for _, fld := range enumTyp.EnumFields {
					fields = append(fields,
						jen.Id(fld.Name).Add(typeToGoWithContext(ctx, pkg, ctx.Types, fld.Type)))
				}
				return jen.Type().Id(enumName).Struct(fields...)
			})

			// Generate case instances
			for i, c := range s.Cases {
				caseIndex := i
				caseDef := c
				e.withStmt(block, func() jen.Code {
					args := []jen.Code{
						jen.Lit(int64(caseIndex)),  // ordinal
						jen.Lit(caseDef.Name.Name), // name
					}
					for _, arg := range caseDef.Args {
						args = append(args, e.buildExpr(ctx, pkg, f, arg))
					}
					// Fill defaults for missing args
					for j := len(caseDef.Args); j < len(s.Fields); j++ {
						if s.Fields[j].Default != nil {
							args = append(args, e.buildExpr(ctx, pkg, f, s.Fields[j].Default))
						}
					}

					return jen.Var().Id(enumName + caseDef.Name.Name).Op("=").
						Op("&").Id(enumName).Values(args...)
				})
			}

			// Generate Values slice for iteration
			e.withStmt(block, func() jen.Code {
				var vals []jen.Code
				for _, c := range s.Cases {
					vals = append(vals, jen.Id(enumName+c.Name.Name))
				}
				return jen.Var().Id(enumName + "Values").Op("=").
					Index().Op("*").Id(enumName).Values(vals...)
			})

			// Generate String method (toString)
			e.withStmt(block, func() jen.Code {
				return jen.Func().Params(jen.Id("e").Op("*").Id(enumName)).
					Id("String").Params().String().Block(
					jen.Return(jen.Id("e").Dot("__name")),
				)
			})

			// Generate HashCode method
			e.withStmt(block, func() jen.Code {
				return jen.Func().Params(jen.Id("e").Op("*").Id(enumName)).
					Id("HashCode").Params().Int64().Block(
					jen.Return(jen.Id("e").Dot("__ordinal")),
				)
			})

			// Emit user-defined methods with receiver
			for _, method := range s.Methods {
				methodDef := method
				e.withStmt(block, func() jen.Code {
					methodName := symName(ctx, methodDef.Name.Sym)

					params := make([]jen.Code, len(methodDef.Params))
					for i, param := range methodDef.Params {
						paramName := symNameWithUnused(ctx, pkg, param.Name.Sym)
						paramType := typeToGoWithContext(ctx, pkg, ctx.Types, param.Type.Typ)
						params[i] = jen.Id(paramName).Add(paramType)
					}

					funcDecl := jen.Func().Params(jen.Id("this").Op("*").Id(enumName)).
						Id(methodName).Params(params...)

					if methodDef.ReturnType.Typ != ctx.Types.TypNone() {
						returnType := typeToGoWithContext(ctx, pkg, ctx.Types, methodDef.ReturnType.Typ)
						funcDecl = funcDecl.Add(returnType)
					}

					return funcDecl.BlockFunc(func(g *jen.Group) {
						e.emitBlock(ctx, pkg, f, g, methodDef.Body.Stmts)
					})
				})
			}
		}
	case *ir.ExprStmt:
		if topLevel { // Top-level expressions are not allowed in Go
			return
		}

		if e.composableDepth > 0 {
			e.withStmt(block, func() jen.Code {
				return jen.Id("__c").Dot("Children").Op("=").Qual("", "append").Call(
					jen.Id("__c").Dot("Children"),
					e.buildExpr(ctx, pkg, f, s.Expr),
				)
			})
			return
		}

		e.withStmt(block, func() jen.Code {
			return e.buildExpr(ctx, pkg, f, s.Expr)
		})
	case *ir.FieldAssignmentStmt:
		if topLevel {
			return
		}
		e.withStmt(block, func() jen.Code {
			var recvName string
			if s.Receiver.Name == "this" {
				recvName = "this"
			} else {
				recvName = symName(ctx, s.Receiver.Sym)
			}
			if s.Op == ir.OpAssign && len(s.Fields) == 1 {
				fld := s.Fields[0]
				if reactive := isReactiveFieldOf(ctx, pkg, s.Receiver.Sym, fld.Name); reactive {
					setterName := "set" + goExportedName(fld.Name)
					return jen.Id(recvName).Dot(setterName).Call(e.buildExpr(ctx, pkg, f, s.Value))
				}
			}
			target := jen.Id(recvName)
			for _, fld := range s.Fields {
				target = target.Dot(goExportedName(fld.Name))
			}
			rhs := e.buildExpr(ctx, pkg, f, s.Value)
			if s.Op == ir.OpAssign && s.Value != nil {
				targetTyp := fieldAssignmentTargetType(ctx, pkg, s)
				srcTyp := s.Value.GetType()
				if targetTyp != 0 && srcTyp != 0 {
					if targetTy, ok := ctx.Types.GetByID(targetTyp); ok && targetTy.Kind == ir.TK_Option {
						if srcTy, ok2 := ctx.Types.GetByID(srcTyp); ok2 && srcTy.Kind != ir.TK_Option && srcTy.Kind != ir.TK_PrimitiveNone {
							tempVar := e.hk.NewTemp()
							rhs = jen.Func().Params().Op("*").Add(typeToGoWithContext(ctx, pkg, ctx.Types, targetTy.ElemType)).Block(
								jen.Id(tempVar).Op(":=").Add(rhs),
								jen.Return(jen.Op("&").Id(tempVar)),
							).Call()
						}
					}
				}
			}
			return target.Op(string(s.Op)).Add(rhs)
		})
	case *ir.MultiAssignmentStmt:
		if topLevel {
			return
		}

		hasNonDiscard := false
		for _, target := range s.Targets {
			if target.Name != nil {
				name := symNameWithUnused(ctx, pkg, target.Name.Sym)
				if name != "_" {
					hasNonDiscard = true
					break
				}
			}
		}

		if !hasNonDiscard {
			return
		}

		if len(s.Targets) == 1 {
			target := s.Targets[0]
			if target.Name == nil {
				return
			}
			lhsName := symNameWithUnused(ctx, pkg, target.Name.Sym)
			e.withStmt(block, func() jen.Code {
				return jen.Id(lhsName).Op("=").Add(e.buildExpr(ctx, pkg, f, s.Value))
			})
			if origName := reactiveWireVarOriginalName(ctx, target.Name.Sym); origName != "" {
				e.withStmt(block, func() jen.Code {
					return jen.Id("__sovaPushWireVar").Call(jen.Lit(origName), jen.Id(lhsName))
				})
			}
			return
		}

		tupleVarName := "__tuple_tmp_" + e.hk.NewTemp()

		e.withStmt(block, func() jen.Code {
			rhs := e.buildExpr(ctx, pkg, f, s.Value)
			return jen.Id(tupleVarName).Op(":=").Add(rhs)
		})

		e.withStmt(block, func() jen.Code {
			var names []jen.Code
			var values []jen.Code

			for i, target := range s.Targets {
				if target.Name == nil {
					names = append(names, jen.Id("_"))
				} else {
					names = append(names, jen.Id(symNameWithUnused(ctx, pkg, target.Name.Sym)))
				}

				elemAccess := jen.Id(tupleVarName).Index(jen.Lit(i))

				if target.Name != nil {
					elemType := typeOfSym(pkg, target.Name.Sym)
					if elemType != 0 {
						elemAccess = elemAccess.Assert(typeToGoWithContext(ctx, pkg, ctx.Types, elemType))
					}
				}

				values = append(values, elemAccess)
			}

			return jen.List(names...).Op("=").List(values...)
		})
	case *ir.IfStmt:
		if topLevel { // Top-level expressions are not allowed in Go
			return
		}

		e.withStmt(block, func() jen.Code {
			ifs := jen.If(e.buildExpr(ctx, pkg, f, s.Cond)).BlockFunc(func(g *jen.Group) {
				e.emitBlock(ctx, pkg, f, g, s.Then.Stmts)
			})

			for _, elif := range s.ElseIfs {
				ifs.Else().If(e.buildExpr(ctx, pkg, f, elif.Cond)).BlockFunc(func(g *jen.Group) {
					e.emitBlock(ctx, pkg, f, g, elif.Then.Stmts)
				})
			}

			if s.Else != nil {
				ifs.Else().BlockFunc(func(g *jen.Group) {
					e.emitBlock(ctx, pkg, f, g, s.Else.Stmts)
				})
			}

			return ifs
		})
	case *ir.SwitchStmt:
		if topLevel {
			return
		}

		e.withStmt(block, func() jen.Code {
			return jen.Switch(e.buildExpr(ctx, pkg, f, s.Expr)).BlockFunc(func(g *jen.Group) {
				for _, caseStmt := range s.Cases {
					g.CaseFunc(func(cg *jen.Group) {
						for _, caseExpr := range caseStmt.Values {
							cg.Add(e.buildExpr(ctx, pkg, f, caseExpr))
						}
					}).BlockFunc(func(bg *jen.Group) {
						e.emitBlock(ctx, pkg, f, bg, caseStmt.Stmts)
					})
				}

				if len(s.Default) > 0 {
					g.Default().BlockFunc(func(bg *jen.Group) {
						e.emitBlock(ctx, pkg, f, bg, s.Default)
					})
				}
			})
		})
	case *ir.ReturnStmt:
		if topLevel { // Top-level expressions are not allowed in Go
			return
		}

		e.withStmt(block, func() jen.Code {
			if len(s.Results) == 0 {
				return jen.Return()
			} else if len(s.Results) == 1 {
				expr := e.buildExpr(ctx, pkg, f, s.Results[0])

				// Handle implicit lifting for return values
				if e.currentFunc != nil && e.currentFunc.ReturnType != nil {
					returnType := e.currentFunc.ReturnType.Typ
					resultType := s.Results[0].GetType()
					if returnType != 0 && resultType != 0 {
						returnTy, _ := ctx.Types.GetByID(returnType)
						resultTy, _ := ctx.Types.GetByID(resultType)
						// If return type is option and result is not option/none, we need to lift
						if returnTy != nil && returnTy.Kind == ir.TK_Option &&
							resultTy != nil && resultTy.Kind != ir.TK_Option && resultTy.Kind != ir.TK_PrimitiveNone {
							// Wrap in a helper function that returns a pointer
							tempVar := e.hk.NewTemp()
							expr = jen.Func().Params().Op("*").Add(typeToGoWithContext(ctx, pkg, ctx.Types, returnTy.ElemType)).Block(
								jen.Id(tempVar).Op(":=").Add(expr),
								jen.Return(jen.Op("&").Id(tempVar)),
							).Call()
						}
					}
				}

				return jen.Return(expr)
			} else {
				var returnType *ir.Type
				if e.currentFunc != nil && e.currentFunc.ReturnType != nil {
					returnType, _ = ctx.Types.GetByID(e.currentFunc.ReturnType.Typ)
				}

				var exprs []jen.Code
				for _, result := range s.Results {
					exprs = append(exprs, e.buildExpr(ctx, pkg, f, result))
				}

				if returnType != nil && returnType.Kind == ir.TK_Tuple {
					return jen.Return(jen.Index().Any().Values(exprs...))
				} else {
					return jen.Return(jen.List(exprs...))
				}
			}
		})
	case *ir.GuardStmt:
		if topLevel { // Top-level expressions are not allowed in Go
			return
		}

		e.withStmt(block, func() jen.Code {
			cond := e.buildExpr(ctx, pkg, f, s.Cond)
			isVarOption := false
			if s.Cond.GetType() == ctx.Types.PrimBool() {
				cond = jen.Op("!").Add(cond)
			} else if _, ok := s.Cond.(*ir.VarRef); ok { // Assume option type
				cond = jen.Id("(").Add(cond).Op("==").Nil().Id(")")
				isVarOption = true
			}

			ifCode := jen.If(cond).BlockFunc(func(g *jen.Group) {
				if len(s.Returns) == 0 {
					g.Return()
				} else if len(s.Returns) == 1 {
					g.Return(e.buildExpr(ctx, pkg, f, s.Returns[0]))
				} else {
					var exprs []jen.Code
					for _, ret := range s.Returns {
						exprs = append(exprs, e.buildExpr(ctx, pkg, f, ret))
					}
					g.Return(jen.List(exprs...))
				}
			})

			if isVarOption {
				vr := s.Cond.(*ir.VarRef)
				newMangledName := ctx.Names.RandName("_opt_guarded_")
				mangledName := symNameWithUnused(ctx, pkg, vr.Ref.Sym)
				orig := symOrigName(ctx, vr.Ref.Sym)

				unwrapped := jen.Id(newMangledName).Op(":=").Op("*").Add(jen.Id(mangledName))
				if orig != "" {
					unwrapped.Commentf("Original name: %s", orig)
				}

				ifCode.Line().Add(
					unwrapped,
				)

				ctx.Names.ReplaceMangledName(vr.Ref.Sym, newMangledName)
			}

			return ifCode
		})
	case *ir.BreakStmt:
		if topLevel { // Top-level expressions are not allowed in Go
			return
		}

		e.withStmt(block, func() jen.Code {
			if s.Depth > 1 {
				label := e.getLoopLabel(s.Depth)
				if label != "" {
					return jen.Break().Id(label)
				}
			}

			return jen.Break()
		})
	case *ir.ContinueStmt:
		if topLevel { // Top-level expressions are not allowed in Go
			return
		}

		e.withStmt(block, func() jen.Code {
			if s.Depth > 1 {
				label := e.getLoopLabel(s.Depth)
				if label != "" {
					return jen.Continue().Id(label)
				}
			}

			return jen.Continue()
		})
	case *ir.WhileStmt:
		if topLevel {
			return
		}

		e.withStmt(block, func() jen.Code {
			loopLevel := len(e.loopLabels) + 1
			needsLabel := e.loopNeedsLabel(s.Body.Stmts, loopLevel)

			label := e.pushLoop()
			defer e.popLoop()

			forLoop := jen.For().BlockFunc(func(g *jen.Group) {
				cond := e.buildExpr(ctx, pkg, f, s.Cond)
				g.If(jen.Op("!").Parens(cond)).Block(
					jen.Break(),
				)

				e.emitBlock(ctx, pkg, f, g, s.Body.Stmts)
			})

			if needsLabel {
				return jen.Id(label).Op(":").Add(forLoop)
			}
			return forLoop
		})
	case *ir.ForStmt:
		if topLevel {
			return
		}

		e.withStmt(block, func() jen.Code {
			loopLevel := len(e.loopLabels) + 1
			needsLabel := e.loopNeedsLabel(s.Body.Stmts, loopLevel)

			label := e.pushLoop()
			defer e.popLoop()

			var forLoop *jen.Statement
			var prepend *jen.Statement = nil
			if s.CondType == ir.ForCondInfinite {
				forLoop = jen.For().BlockFunc(func(g *jen.Group) {
					e.emitBlock(ctx, pkg, f, g, s.Body.Stmts)
				})
			} else if s.CondType == ir.ForCondRange {
				rangeCollectionVar := e.hk.NewTemp()
				prepend = jen.Id(rangeCollectionVar).Op(":=").Add(e.buildRangeExpr(ctx, pkg, f, ctx.Types.PrimFloat(), s.CondRange.RangeStart, s.CondRange.RangeEnd, nil))

				rangeIterVar := symNameWithUnused(ctx, pkg, s.CondRange.RangeVar.Sym)
				rangeIterOrig := symOrigName(ctx, s.CondRange.RangeVar.Sym)

				if rangeIterVar == "_" {
					forLoop = jen.For(jen.Range().Id(rangeCollectionVar)).BlockFunc(func(g *jen.Group) {
						if rangeIterOrig != "" {
							g.Commentf("Original name: %s", rangeIterOrig)
						}
						e.emitBlock(ctx, pkg, f, g, s.Body.Stmts)
					})
				} else {
					forLoop = jen.For(jen.Id(rangeIterVar).Op(":=").Range().Id(rangeCollectionVar)).BlockFunc(func(g *jen.Group) {
						if rangeIterOrig != "" {
							g.Commentf("Original name: %s", rangeIterOrig)
						}
						e.emitBlock(ctx, pkg, f, g, s.Body.Stmts)
					})
				}
			} else if s.CondType == ir.ForCondIn {
				inFirstVar := symNameWithUnused(ctx, pkg, s.CondIn.InFirstVar.Sym)
				inFirstOrig := symOrigName(ctx, s.CondIn.InFirstVar.Sym)
				inSecondVar := ""
				inSecondOrig := ""
				inThirdVar := ""
				inThirdOrig := ""
				if s.CondIn.InSecondVar != nil {
					inSecondVar = symNameWithUnused(ctx, pkg, s.CondIn.InSecondVar.Sym)
					inSecondOrig = symOrigName(ctx, s.CondIn.InSecondVar.Sym)
				}
				if s.CondIn.InThirdVar != nil {
					inThirdVar = symNameWithUnused(ctx, pkg, s.CondIn.InThirdVar.Sym)
					inThirdOrig = symOrigName(ctx, s.CondIn.InThirdVar.Sym)
				}

				isMap := ctx.Types.IsTypeOfKind(s.CondIn.IterExpr.GetType(), ir.TK_Map)
				isIterable := s.CondIn.IterNextSym != 0
				useIndex := (inSecondVar != "" && !isMap && !isIterable) || (inThirdVar != "" && isMap) || (isIterable && inSecondVar != "")
				indexVar := e.hk.NewTemp()
				if useIndex {
					prepend = jen.Var().Id(indexVar).Int64().Op("=").Lit(-1)
				}

				var iterOptTemp string
				var iterNextMethod string
				if isIterable {
					iterNextMethod = symName(ctx, s.CondIn.IterNextSym)
					iterOptTemp = e.hk.NewTemp()
					forLoop = jen.For()
				} else if isMap {
					if inSecondVar != "" {
						if inFirstVar == "_" && inSecondVar == "_" {
							forLoop = jen.For(jen.Range().Add(e.buildExpr(ctx, pkg, f, s.CondIn.IterExpr)))
						} else {
							forLoop = jen.For(jen.Id(inFirstVar).Op(",").Id(inSecondVar).Op(":=").Range().Add(e.buildExpr(ctx, pkg, f, s.CondIn.IterExpr)))
						}
					} else {
						if inFirstVar == "_" {
							forLoop = jen.For(jen.Range().Add(e.buildExpr(ctx, pkg, f, s.CondIn.IterExpr)))
						} else {
							forLoop = jen.For(jen.Id(inFirstVar).Op(",").Id("_").Op(":=").Range().Add(e.buildExpr(ctx, pkg, f, s.CondIn.IterExpr)))
						}
					}
				} else {
					if inFirstVar == "_" {
						forLoop = jen.For(jen.Range().Add(e.buildExpr(ctx, pkg, f, s.CondIn.IterExpr)))
					} else {
						forLoop = jen.For(jen.Id("_").Op(",").Id(inFirstVar).Op(":=").Range().Add(e.buildExpr(ctx, pkg, f, s.CondIn.IterExpr)))
					}
				}

				forLoop = forLoop.BlockFunc(func(g *jen.Group) {
					if isIterable {
						g.Id(iterOptTemp).Op(":=").Add(e.buildExpr(ctx, pkg, f, s.CondIn.IterExpr)).Dot(iterNextMethod).Call()
						g.If(jen.Id(iterOptTemp).Op("==").Nil()).Block(jen.Break())
						if inFirstVar != "_" {
							g.Id(inFirstVar).Op(":=").Op("*").Id(iterOptTemp)
							g.Id("_").Op("=").Id(inFirstVar)
						}
					}
					if inFirstOrig != "" {
						g.Commentf("Original name: %s for var %s", inFirstOrig, inFirstVar)
					}
					if inSecondOrig != "" {
						g.Commentf("Original name: %s for var %s", inSecondOrig, inSecondVar)
					}
					if inThirdOrig != "" {
						g.Commentf("Original name: %s for var %s", inThirdOrig, inThirdVar)
					}

					if useIndex {
						g.Id(indexVar).Op("++")
						if inSecondVar != "" && inSecondVar != "_" && !isMap {
							g.Id(inSecondVar).Op(":=").Id(indexVar)
							g.Id("_").Op("=").Id(inSecondVar)
						}
					}

					e.emitBlock(ctx, pkg, f, g, s.Body.Stmts)
				})
			} else if s.CondType == ir.ForCondInt {
				initVarName := symNameWithUnused(ctx, pkg, s.CondInt.Init.Targets[0].Name.Sym)
				initVarOrig := symOrigName(ctx, s.CondInt.Init.Targets[0].Name.Sym)

				forLoop = jen.For(
					jen.Id(initVarName).Op(":=").Add(e.buildExpr(ctx, pkg, f, s.CondInt.Init.Init)),
					e.buildExpr(ctx, pkg, f, s.CondInt.Cond),
					e.buildExpr(ctx, pkg, f, s.CondInt.Post),
				).BlockFunc(func(g *jen.Group) {
					if initVarOrig != "" {
						g.Commentf("Original name: %s", initVarOrig)
					}
					e.emitBlock(ctx, pkg, f, g, s.Body.Stmts)
				})
			} else {
				panic("unsupported for loop condition type")
			}

			if needsLabel {
				forLoop = jen.Id(label).Op(":").Add(forLoop)
			}

			if prepend != nil {
				forLoop = prepend.Line().Add(forLoop)
			}

			return forLoop
		})
	case *ir.TypeAliasStmt:
		// Aliases are erased at the type-resolution stage; no Go declaration needed.
	default:
		panic(fmt.Sprintf("go codegen: unhandled statement type %T", st))
	}
}

func (e *CodeEmitter) buildExpr(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, expr ir.Expr) *jen.Statement {
	switch x := expr.(type) {
	case *ir.WhenExpr:
		return jen.Func().Params().Add(typeToGoWithContext(ctx, pkg, ctx.Types, x.GetType())).BlockFunc(func(g *jen.Group) {
			g.Switch(e.buildExpr(ctx, pkg, f, x.Expr)).BlockFunc(func(g *jen.Group) {
				for _, caseStmt := range x.Cases {
					g.CaseFunc(func(cg *jen.Group) {
						for _, caseExpr := range caseStmt.Values {
							cg.Add(e.buildExpr(ctx, pkg, f, caseExpr))
						}
					}).Block(jen.Return(e.buildExpr(ctx, pkg, f, caseStmt.Then)))
				}

				g.Default().Block(jen.Return(e.buildExpr(ctx, pkg, f, x.Default)))
			})
		}).Call()
	case *ir.UnaryExpr:
		op := string(x.Op)
		return jen.Op(op).Add(e.buildExpr(ctx, pkg, f, x.Expr))
	case *ir.PrefixUnaryExpr:
		return jen.Func().Params().Add(typeToGoWithContext(ctx, pkg, ctx.Types, x.GetType())).BlockFunc(func(g *jen.Group) {
			target := e.hk.NewTemp()
			if v, ok := x.Expr.(*ir.VarRef); ok {
				target = symName(ctx, v.Ref.Sym)
			} else {
				g.Id(target).Op("=").Add(jen.Id(target))
			}

			switch x.Op {
			case ir.OpInc:
				g.Id(target).Op("=").Id(target).Op("+").Lit(1)
			case ir.OpDec:
				g.Id(target).Op("=").Id(target).Op("-").Lit(1)
			}

			g.Return(jen.Id(target))
		}).Call()
	case *ir.PostfixUnaryExpr:
		return jen.Func().Params().Add(typeToGoWithContext(ctx, pkg, ctx.Types, x.GetType())).BlockFunc(func(g *jen.Group) {
			target := e.hk.NewTemp()
			if v, ok := x.Expr.(*ir.VarRef); ok {
				target = symName(ctx, v.Ref.Sym)
			} else {
				g.Id(target).Op("=").Add(jen.Id(target))
			}

			orig := e.hk.NewTemp()
			g.Id(orig).Op(":=").Id(target)

			switch x.Op {
			case ir.OpInc:
				g.Id(target).Op("=").Id(target).Op("+").Lit(1)
			case ir.OpDec:
				g.Id(target).Op("=").Id(target).Op("-").Lit(1)
			}

			g.Return(jen.Id(orig))
		}).Call()
	case *ir.BinaryExpr:
		if leftTy, ok := ctx.Types.GetByID(x.Left.GetType()); ok && leftTy.Kind == ir.TK_Struct {
			if methodName, isOp := opOverloadName(x.Op); isOp {
				for _, m := range leftTy.StructMethods {
					if m.Name == methodName && m.Sym != 0 {
						left := e.buildExpr(ctx, pkg, f, x.Left)
						right := e.buildExpr(ctx, pkg, f, x.Right)
						return left.Dot(symName(ctx, m.Sym)).Call(right)
					}
				}
			}
		}
		if x.GetType() == ctx.Types.PrimString() && (x.Op == ir.OpAdd) {
			left := e.buildExpr(ctx, pkg, f, x.Left)
			leftFmtKey := fmtSprintfKey(x.Left.GetType(), ctx.Types)
			right := e.buildExpr(ctx, pkg, f, x.Right)
			rightFmtKey := fmtSprintfKey(x.Right.GetType(), ctx.Types)
			fmtKey := leftFmtKey + rightFmtKey
			return jen.Qual("fmt", "Sprintf").Call(
				jen.Lit(fmtKey),
				left,
				right,
			)
		}
		if x.Op == ir.OpAdd {
			if leftTy, ok := ctx.Types.GetByID(x.Left.GetType()); ok && leftTy.Kind == ir.TK_Slice {
				if rightTy, rok := ctx.Types.GetByID(x.Right.GetType()); rok && rightTy.Kind == ir.TK_Slice {
					left := e.buildExpr(ctx, pkg, f, x.Left)
					right := e.buildExpr(ctx, pkg, f, x.Right)
					return jen.Append(left, right.Op("..."))
				}
			}
		}

		left := e.buildExpr(ctx, pkg, f, x.Left)
		right := e.buildExpr(ctx, pkg, f, x.Right)
		op := string(x.Op)
		return jen.Parens(left).Op(op).Parens(right)
	case *ir.CoalesceExpr:
		leftExpr := e.buildExpr(ctx, pkg, f, x.Left)
		defaultExpr := e.buildExpr(ctx, pkg, f, x.Default)

		return jen.Func().Params().Add(typeToGoWithContext(ctx, pkg, ctx.Types, x.GetType())).BlockFunc(func(g *jen.Group) {
			temp := e.hk.NewTemp()
			g.Id(temp).Op(":=").Add(leftExpr)
			g.If(jen.Id(temp).Op("!=").Nil()).Block(
				jen.Return(jen.Op("*").Id(temp)),
			).Else().Block(
				jen.Return(defaultExpr),
			)
		}).Call()
	case *ir.TenaryExpr:
		cond := e.buildExpr(ctx, pkg, f, x.Cond)
		then := e.buildExpr(ctx, pkg, f, x.Then)
		elsee := e.buildExpr(ctx, pkg, f, x.Else)

		return jen.Func().Params().Add(typeToGoWithContext(ctx, pkg, ctx.Types, x.GetType())).BlockFunc(func(g *jen.Group) {
			g.If(cond).Block(
				jen.Return(then),
			).Else().Block(
				jen.Return(elsee),
			)
		}).Call()
	case *ir.GroupedExpr:
		return jen.Parens(e.buildExpr(ctx, pkg, f, x.Expr))
	case *ir.OptionUnwrapExpr:
		return jen.Parens(jen.Op("*").Add(jen.Parens(e.buildExpr(ctx, pkg, f, x.Expr))))
	case *ir.AsExpr:
		if x.Target == nil || x.Target.Typ == 0 {
			return e.buildExpr(ctx, pkg, f, x.Expr)
		}
		if x.Safe {
			if conv := goSafePrimitiveConversion(ctx, pkg, f, e, x); conv != nil {
				return conv
			}
			return jen.Func().Params().Add(typeToGoWithContext(ctx, pkg, ctx.Types, x.GetType())).Block(
				jen.List(jen.Id("__v"), jen.Id("__ok")).Op(":=").Add(e.buildExpr(ctx, pkg, f, x.Expr)).Assert(typeToGoWithContext(ctx, pkg, ctx.Types, x.Target.Typ)),
				jen.If(jen.Id("__ok")).Block(
					jen.Return(jen.Op("&").Id("__v")),
				),
				jen.Return(jen.Nil()),
			).Call()
		}
		if conv := goPrimitiveConversion(ctx, pkg, f, e, x); conv != nil {
			return conv
		}
		return jen.Func().Params().Add(typeToGoWithContext(ctx, pkg, ctx.Types, x.Target.Typ)).Block(
			jen.List(jen.Id("__v"), jen.Id("_")).Op(":=").Add(e.buildExpr(ctx, pkg, f, x.Expr)).Assert(typeToGoWithContext(ctx, pkg, ctx.Types, x.Target.Typ)),
			jen.Return(jen.Id("__v")),
		).Call()
	case *ir.AssignmentExpr:
		return jen.Func().Params().Add(typeToGoWithContext(ctx, pkg, ctx.Types, x.GetType())).BlockFunc(func(group *jen.Group) {
			right := e.buildExpr(ctx, pkg, f, x.Right)
			left := symName(ctx, x.Left.Sym)

			if x.Op == ir.OpAssign {
				group.Id(left).Op("=").Add(right)
			} else {
				op := string(x.Op[:len(x.Op)-1]) // Trim the '=' from the operator
				temp := e.hk.NewTemp()
				group.Id(temp).Op(":=").Id(left).Op(op).Add(right)
				group.Id(left).Op("=").Id(temp)
			}

			if reactiveWireVarOriginalName(ctx, x.Left.Sym) != "" {
				group.Id("__sovaPushWireVar").Call(jen.Lit(reactiveWireVarOriginalName(ctx, x.Left.Sym)), jen.Id(left))
			}

			group.Return(jen.Id(left))
		}).Call()
	case *ir.IndexExpr:
		return jen.Parens(e.buildExpr(ctx, pkg, f, x.Expr)).Index(e.buildExpr(ctx, pkg, f, x.Index))
	case *ir.FieldAccessExpr:
		if x.ResolvedSym != 0 {
			return jen.Id(symName(ctx, x.ResolvedSym))
		}
		base := e.buildExpr(ctx, pkg, f, x.Expr)
		cur := base
		curType := x.Expr.GetType()
		// Option-unwrap flow narrowing: the expr's cached type was overwritten to the unwrapped element type inside an `if x != none` branch, but the underlying Go storage is still the wider `option<T>` (one extra pointer). Detect by comparing against the receiver symbol's declared type and insert one deref per option layer the narrowing peeled off.
		if vr, ok := x.Expr.(*ir.VarRef); ok && vr.Ref.Sym != 0 {
			if sym, ok := pkg.Syms.GetByID(vr.Ref.Sym); ok {
				symType := sym.Typ
				for symType != curType {
					ty, ok := ctx.Types.GetByID(symType)
					if !ok || ty.Kind != ir.TK_Option {
						break
					}
					cur = jen.Parens(jen.Op("*").Add(cur))
					symType = ty.ElemType
				}
			}
		}
		// Also strip any option layers that survive on the inferred type itself (e.g. accessing through an `option<T>` value directly without narrowing).
		for {
			ty, ok := ctx.Types.GetByID(curType)
			if !ok || ty.Kind != ir.TK_Option {
				break
			}
			cur = jen.Parens(jen.Op("*").Add(cur))
			curType = ty.ElemType
		}
		for _, fld := range x.Fields {
			ty, ok := ctx.Types.GetByID(curType)
			if ok && ty.Kind == ir.TK_Struct {
				found := false
				for _, sf := range ty.StructFields {
					if sf.Name == fld.Name {
						var fieldName string
						switch {
						case ty.IsExtern:
							fieldName = fld.Name
						case sf.IsPromoted && sf.PromotedFromExtern:
							fieldName = fld.Name
						case ty.StructName == "__Session":
							fieldName = sessionFieldNameToGo(fld.Name)
						default:
							fieldName = goExportedName(fld.Name)
						}
						cur = jen.Add(cur).Dot(fieldName)
						curType = sf.Type
						found = true
						break
					}
				}
				if !found {
					for _, m := range ty.StructMethods {
						if m.Name == fld.Name {
							switch {
							case ty.IsExtern:
								cur = jen.Add(cur).Dot(fld.Name)
							case m.IsPromoted && m.PromotedFromExtern:
								cur = jen.Add(cur).Dot(fld.Name)
							case m.Sym != 0:
								cur = jen.Add(cur).Dot(symName(ctx, m.Sym))
							default:
								cur = jen.Add(cur).Dot(m.Name)
							}
							curType = m.FuncTyp
							found = true
							break
						}
					}
				}
				if !found {
					cur = jen.Add(cur).Dot(fld.Name)
				}
				continue
			}
			if ok && ty.Kind == ir.TK_Interface {
				for _, m := range ty.InterfaceMethods {
					if m.Name == fld.Name {
						curType = m.FuncTyp
						break
					}
				}
				cur = jen.Add(cur).Dot(fld.Name)
				continue
			}
			if ok && ty.Kind == ir.TK_Enum {
				// Check if this is a case access (e.g., Color.Red)
				isCaseAccess := false
				for _, c := range ty.EnumCases {
					if c.Name == fld.Name {
						isCaseAccess = true
						// Generate enum constant name: EnumNameCaseName
						enumName := symName(ctx, getEnumSymbol(ctx, pkg, ty.EnumName))
						cur = jen.Id(enumName + fld.Name)
						break
					}
				}

				// If not a case, use dot notation for fields and methods
				if !isCaseAccess {
					// Check if this is a method and use the mangled name
					isMethod := false
					for _, method := range ty.EnumMethods {
						if method.Name == fld.Name {
							isMethod = true
							curType = method.Type
							// Look up the mangled method name
							methodSym := getMethodSymbol(ctx, pkg, ty.EnumName, fld.Name)
							if methodSym != 0 {
								cur = jen.Add(cur).Dot(symName(ctx, methodSym))
							} else {
								cur = jen.Add(cur).Dot(fld.Name)
							}
							break
						}
					}
					// If not a method, it's a field - use the original field name
					if !isMethod {
						cur = jen.Add(cur).Dot(fld.Name)
						// Update curType to the field type
						for _, field := range ty.EnumFields {
							if field.Name == fld.Name {
								curType = field.Type
								break
							}
						}
					}
				}
			} else {
				cur = jen.Add(cur).Index(jen.Lit(fld.Name))
			}
		}
		return cur
	case *ir.RangeExpr:
		return e.buildRangeExpr(ctx, pkg, f, x.GetType(), x.Start, x.End, x.Inc)
	case *ir.FuncCallExpr:
		if chOp, chRecv, ok := matchChanMethod(ctx, x); ok {
			recvCode := e.buildExpr(ctx, pkg, f, chRecv)
			switch chOp {
			case "send":
				if len(x.Args) == 1 {
					return jen.Func().Params().Block(recvCode.Clone().Op("<-").Add(e.buildExpr(ctx, pkg, f, x.Args[0].Expr))).Call()
				}
			case "recv":
				return jen.Func().Params().Index().Any().Block(
					jen.List(jen.Id("__v"), jen.Id("__ok")).Op(":=").Op("<-").Add(recvCode),
					jen.Return(jen.Index().Any().Values(jen.Id("__v"), jen.Id("__ok"))),
				).Call()
			case "close":
				return jen.Func().Params().Block(jen.Id("close").Call(recvCode)).Call()
			}
		}
		if intrinsic := lookupBuiltinIntrinsic(ctx, x.Callee); intrinsic != "" {
			argCodes := make([]jen.Code, len(x.Args))
			for i, arg := range x.Args {
				argCodes[i] = e.buildExpr(ctx, pkg, f, arg.Expr)
			}
			argTypes := make([]ir.TypID, len(x.Args))
			for i, arg := range x.Args {
				if arg.Expr != nil {
					argTypes[i] = arg.Expr.GetType()
				}
			}
			if code := emitBuiltinIntrinsicCall(ctx, intrinsic, argCodes, argTypes); code != nil {
				return code
			}
		}
		callee := e.buildExpr(ctx, pkg, f, x.Callee)

		calleeType := x.Callee.GetType()
		funcTypeDef, _ := ctx.Types.GetByID(calleeType)

		var args []jen.Code
		if funcTypeDef != nil && funcTypeDef.Kind == ir.TK_Function {
			paramCount := len(funcTypeDef.ParamTypes)
			args = make([]jen.Code, paramCount)

			for i := 0; i < paramCount; i++ {
				if i < len(x.Args) && x.Args[i].Expr != nil {
					args[i] = e.buildExpr(ctx, pkg, f, x.Args[i].Expr)
				} else if funcTypeDef.ParamTypes[i].Default != nil {
					args[i] = e.buildExpr(ctx, pkg, f, funcTypeDef.ParamTypes[i].Default)
				} else {
					args[i] = jen.Null()
				}
			}
		} else {
			args = make([]jen.Code, len(x.Args))
			for i, arg := range x.Args {
				args[i] = e.buildExpr(ctx, pkg, f, arg.Expr)
			}
		}

		return callee.Call(args...)
	case *ir.FuncLitExpr:
		params := make([]jen.Code, len(x.Params))
		for i, param := range x.Params {
			paramName := symName(ctx, param.Name.Sym)
			paramType := typeToGoWithContext(ctx, pkg, ctx.Types, param.Type.Typ)
			params[i] = jen.Id(paramName).Add(paramType)
		}

		funcStmt := jen.Func().Params(params...)
		if x.ReturnType != nil && x.ReturnType.Typ != 0 && x.ReturnType.Typ != ctx.Types.TypNone() {
			funcStmt = funcStmt.Add(typeToGoWithContext(ctx, pkg, ctx.Types, x.ReturnType.Typ))
		}
		return funcStmt.BlockFunc(func(g *jen.Group) {
			e.emitBlock(ctx, pkg, f, g, x.Body.Stmts)
		})
	case *ir.LitInt:
		return jen.Op(strconv.FormatInt(x.Value, 10))
	case *ir.LitFloat:
		return jen.Lit(x.Value)
	case *ir.LitBool:
		if x.Value {
			return jen.True()
		}

		return jen.False()
	case *ir.LitString:
		return jen.Lit(x.Value)
	case *ir.LitChar:
		return jen.Lit(string(x.Value))
	case *ir.LitNone:
		return jen.Nil()
	case *ir.VarRef:
		// Special handling for "this" in enum methods
		if orig, ok := ctx.Names.GetOriginalName(x.Ref.Sym); ok && orig == "this" {
			return jen.Id("this")
		}
		return jen.Id(symName(ctx, x.Ref.Sym))
	case *ir.ArrayLiteral:
		elements := make([]jen.Code, len(x.Elems))
		for i, elem := range x.Elems {
			elements[i] = e.buildExpr(ctx, pkg, f, elem)
		}

		return typeToGoWithContext(ctx, pkg, ctx.Types, x.GetType()).(*jen.Statement).Values(elements...)
	case *ir.MapLiteral:
		dict := jen.Dict{}
		for _, entry := range x.Entries {
			key := e.buildExpr(ctx, pkg, f, entry.Key)
			value := e.buildExpr(ctx, pkg, f, entry.Value)
			dict[key] = value
		}

		return typeToGoWithContext(ctx, pkg, ctx.Types, x.GetType()).(*jen.Statement).Values(dict)
	case *ir.TupleLiteral:
		if len(x.Elems) == 0 {
			return jen.Index().Any().Values()
		}

		var elements []jen.Code
		for _, elem := range x.Elems {
			elements = append(elements, e.buildExpr(ctx, pkg, f, elem))
		}

		return jen.Index().Any().Values(elements...)

	case *ir.StringTemplateExpr:
		var format strings.Builder
		var args []jen.Code
		for _, part := range x.Parts {
			if part.Expr != nil {
				format.WriteString(fmtSprintfKey(part.Expr.GetType(), ctx.Types))
				args = append(args, e.buildExpr(ctx, pkg, f, part.Expr))
			} else {
				format.WriteString(strings.ReplaceAll(part.Lit, "%", "%%"))
			}
		}
		call := []jen.Code{jen.Lit(format.String())}
		call = append(call, args...)
		return jen.Qual("fmt", "Sprintf").Call(call...)

	case *ir.SessionExpr:
		return jen.Id("__session")
	case *ir.ComposableCallExpr:
		return e.buildComposableCall(ctx, pkg, f, x)
	case *ir.ChanInitExpr:
		elem := typeToGoWithContext(ctx, pkg, ctx.Types, x.ElemType.Typ)
		if x.Capacity != nil {
			return jen.Make(jen.Chan().Add(elem), e.buildExpr(ctx, pkg, f, x.Capacity))
		}
		return jen.Make(jen.Chan().Add(elem))
	case *ir.NewExpr:
		typeName := symName(ctx, x.TypeName.Sym)
		if x.CtorSym != 0 {
			ctorName := symName(ctx, x.CtorSym)
			ctorPkg := pkg
			if x.Qualifier != "" {
				if found := lookupImportedPackage(ctx, pkg, x.Qualifier); found != nil {
					ctorPkg = found
				}
			}
			ctorSym, _ := ctorPkg.Syms.GetByID(x.CtorSym)
			var ctorFunc *ir.Type
			if ctorSym != nil {
				ctorFunc, _ = ctx.Types.GetByID(ctorSym.Typ)
			}
			args := make([]jen.Code, len(x.Args))
			for i, arg := range x.Args {
				if arg.Expr != nil {
					args[i] = e.buildExpr(ctx, pkg, f, arg.Expr)
				} else if ctorFunc != nil && i < len(ctorFunc.ParamTypes) && ctorFunc.ParamTypes[i].Default != nil {
					args[i] = e.buildExpr(ctx, pkg, f, ctorFunc.ParamTypes[i].Default)
				} else {
					args[i] = jen.Nil()
				}
			}
			return jen.Id(ctorName).Call(args...)
		}
		var inits []jen.Code
		if decl, ok := e.typeDecls[x.GetType()]; ok {
			for _, field := range decl.Fields {
				if field.Default != nil {
					inits = append(inits, jen.Id(goExportedName(field.Name.Name)).Op(":").Add(e.buildExpr(ctx, pkg, f, field.Default)))
				}
			}
		}
		return jen.Op("&").Id(typeName).Values(inits...)

	default:
		break
	}

	return jen.Nil() // Fallback to nil for unsupported expressions
}

// buildComposableCall emits a composable expression as an immediately-invoked function: it constructs the target via the matched ctor, appends each child to `Children`, and returns the instance. Returning an IIFE keeps the result usable in expression position (assignment, argument, nested composable, etc.) while still allowing the statement-level appends.
func (e *CodeEmitter) buildComposableCall(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, x *ir.ComposableCallExpr) *jen.Statement {
	resultType := typeToGoWithContext(ctx, pkg, ctx.Types, x.TargetTyp)

	var ctorCall jen.Code
	calleeSym := composableCalleeSymGo(x.Callee)
	if x.CtorSym != 0 {
		ctorName := symName(ctx, x.CtorSym)
		ctorSym, _ := pkg.Syms.GetByID(x.CtorSym)
		var ctorFunc *ir.Type
		if ctorSym != nil {
			ctorFunc, _ = ctx.Types.GetByID(ctorSym.Typ)
		}
		args := make([]jen.Code, len(x.Args))
		for i, arg := range x.Args {
			if arg.Expr != nil {
				args[i] = e.buildExpr(ctx, pkg, f, arg.Expr)
			} else if ctorFunc != nil && i < len(ctorFunc.ParamTypes) && ctorFunc.ParamTypes[i].Default != nil {
				args[i] = e.buildExpr(ctx, pkg, f, ctorFunc.ParamTypes[i].Default)
			} else {
				args[i] = jen.Nil()
			}
		}
		ctorCall = jen.Id(ctorName).Call(args...)
	} else if calleeSym != 0 {
		typeName := symName(ctx, calleeSym)
		ctorCall = jen.Op("&").Id(typeName).Values()
	} else {
		ctorCall = jen.Nil()
	}

	return jen.Func().Params().Add(resultType).BlockFunc(func(g *jen.Group) {
		g.Id("__c").Op(":=").Add(ctorCall)
		appendArgs := []jen.Code{jen.Id("__c").Dot("Children")}
		hasAppends := false
		for _, child := range x.Children {
			if child.Expr != nil {
				appendArgs = append(appendArgs, e.buildExpr(ctx, pkg, f, child.Expr))
				hasAppends = true
			}
		}
		if hasAppends {
			g.Id("__c").Dot("Children").Op("=").Qual("", "append").Call(appendArgs...)
		}
		e.composableDepth++
		for _, child := range x.Children {
			if child.Stmt == nil {
				continue
			}
			e.emitStmt(ctx, pkg, f, g, child.Stmt, false)
		}
		e.composableDepth--
		g.Return(jen.Id("__c"))
	}).Call()
}

// composableCalleeSymGo extracts the resolved type symbol from a composable call's callee, mirroring the inference-time helper.
func composableCalleeSymGo(callee ir.Expr) ir.SymID {
	switch c := callee.(type) {
	case *ir.VarRef:
		return c.Ref.Sym
	case *ir.FieldAccessExpr:
		if c.ResolvedSym != 0 {
			return c.ResolvedSym
		}
	}
	return 0
}

func (e *CodeEmitter) withStmt(block *jen.Group, core func() jen.Code) {
	e.hk.Begin()
	coreStmt := core()
	pre, post := e.hk.End()

	if len(pre)+len(post) == 0 {
		block.Add(coreStmt).Line()
		return
	}

	for _, p := range pre {
		block.Add(p).Line()
	}

	block.Add(coreStmt).Line()

	for _, p := range post {
		block.Add(p).Line()
	}
}

func (e *CodeEmitter) buildRangeExpr(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, ty ir.TypID, start, end, inc ir.Expr) *jen.Statement {
	return jen.Func().Params().Add(typeToGoWithContext(ctx, pkg, ctx.Types, ty)).BlockFunc(func(g *jen.Group) {
		resArr := e.hk.NewTemp()
		g.Id(resArr).Op(":=").Make(typeToGoWithContext(ctx, pkg, ctx.Types, ty), jen.Lit(0))

		iterVar := e.hk.NewTemp()
		g.Id(iterVar).Op(":=").Add(e.buildExpr(ctx, pkg, f, start))

		g.For().BlockFunc(func(bg *jen.Group) {
			bg.Id(resArr).Op("=").Append(jen.Id(resArr), jen.Id(iterVar))

			bg.If(jen.Id(iterVar).Op("==").Add(e.buildExpr(ctx, pkg, f, end))).Block(
				jen.Break(),
			)

			if inc == nil {
				bg.Id(iterVar).Op("=").Id(iterVar).Op("+").Lit(1)
			} else {
				bg.Id(iterVar).Op("=").Id(iterVar).Op("+").Add(e.buildExpr(ctx, pkg, f, inc))
			}
		})

		g.Return(jen.Id(resArr))
	}).Call()
}

// ---- Helpers ---- \\

func (e *CodeEmitter) withScope(body func()) {
	e.hk.PushScope()
	defer e.hk.PopScope()
	body()
}

// pushLoop pushes a new loop label onto the stack and returns the label name.
func (e *CodeEmitter) pushLoop() string {
	label := fmt.Sprintf("loop%d", e.loopDepth)
	e.loopLabels = append(e.loopLabels, label)
	e.loopDepth++
	return label
}

// popLoop pops the current loop label from the stack.
func (e *CodeEmitter) popLoop() {
	if e.loopDepth > 0 {
		e.loopDepth--
		e.loopLabels = e.loopLabels[:len(e.loopLabels)-1]
	}
}

// getLoopLabel returns the label for a loop at the specified depth (1 = innermost).
func (e *CodeEmitter) getLoopLabel(depth int) string {
	if depth < 1 || depth > len(e.loopLabels) {
		return "" // Invalid depth
	}
	idx := len(e.loopLabels) - depth
	return e.loopLabels[idx]
}

// loopNeedsLabel checks if a loop at loopLevel needs a label.
// A loop needs a label if any break/continue from a nested position targets it with depth > 1.
func (e *CodeEmitter) loopNeedsLabel(stmts []ir.Stmt, loopLevel int) bool {
	return e.scanForTargetedBreaks(stmts, loopLevel, loopLevel)
}

// scanForTargetedBreaks scans statements for break/continue that target loopLevel.
// currentLevel is the nesting level of the statements being scanned.
func (e *CodeEmitter) scanForTargetedBreaks(stmts []ir.Stmt, loopLevel int, currentLevel int) bool {
	for _, st := range stmts {
		switch s := st.(type) {
		case *ir.BreakStmt:
			targetLevel := currentLevel - s.Depth + 1
			if targetLevel == loopLevel && s.Depth > 1 {
				return true
			}
		case *ir.ContinueStmt:
			targetLevel := currentLevel - s.Depth + 1
			if targetLevel == loopLevel && s.Depth > 1 {
				return true
			}
		case *ir.BlockStmt:
			if e.scanForTargetedBreaks(s.Stmts, loopLevel, currentLevel) {
				return true
			}
		case *ir.IfStmt:
			if e.scanForTargetedBreaks(s.Then.Stmts, loopLevel, currentLevel) {
				return true
			}
			for _, elif := range s.ElseIfs {
				if e.scanForTargetedBreaks(elif.Then.Stmts, loopLevel, currentLevel) {
					return true
				}
			}
			if s.Else != nil && e.scanForTargetedBreaks(s.Else.Stmts, loopLevel, currentLevel) {
				return true
			}
		case *ir.SwitchStmt:
			for _, c := range s.Cases {
				if e.scanForTargetedBreaks(c.Stmts, loopLevel, currentLevel) {
					return true
				}
			}
			if s.Default != nil && e.scanForTargetedBreaks(s.Default, loopLevel, currentLevel) {
				return true
			}
		case *ir.WhileStmt:
			if e.scanForTargetedBreaks(s.Body.Stmts, loopLevel, currentLevel+1) {
				return true
			}
		case *ir.ForStmt:
			if e.scanForTargetedBreaks(s.Body.Stmts, loopLevel, currentLevel+1) {
				return true
			}
		}
	}
	return false
}

// fieldHasReactiveAnnotation reports whether the given annotation list carries an `@reactive` marker. Reactive fields get an extra observer slice plus auto-generated Set/Observe methods.
func fieldHasReactiveAnnotation(annos []ir.Annotation) bool {
	for _, a := range annos {
		if a.Name.Name == "reactive" {
			return true
		}
	}
	return false
}

// hasBuiltinAnnotation reports whether the declaration carries an `@builtin` marker - used on the placeholder declarations in `std/__globals__.sova` so codegen skips emitting Go/JS for them (the compiler emits the host-language implementation at the call site instead).
func hasBuiltinAnnotation(annos []ir.Annotation) bool {
	for _, a := range annos {
		if a.Name.Name == "builtin" {
			return true
		}
	}
	return false
}

// fieldAssignmentTargetType walks the receiver+field chain of a `this.a.b.c = expr` style statement and returns the TypID of the final field. Returns 0 when any link can't be resolved (anonymous receiver, missing struct field, etc.) so the caller can fall back to emitting the assignment without option-lifting.
func fieldAssignmentTargetType(ctx *codegen.EmitContext, pkg *ir.PackageContext, s *ir.FieldAssignmentStmt) ir.TypID {
	if s.Receiver.Sym == 0 || len(s.Fields) == 0 {
		return 0
	}
	recvSym, ok := pkg.Syms.GetByID(s.Receiver.Sym)
	if !ok {
		return 0
	}
	cur := recvSym.Typ
	for _, fld := range s.Fields {
		ty, ok := ctx.Types.GetByID(cur)
		if !ok || ty.Kind != ir.TK_Struct {
			return 0
		}
		found := false
		for _, sf := range ty.StructFields {
			if sf.Name == fld.Name {
				cur = sf.Type
				found = true
				break
			}
		}
		if !found {
			return 0
		}
	}
	return cur
}

// isReactiveFieldOf returns true when receiverSym refers to a value whose type has a field named fieldName carrying the `@reactive` annotation. Used at codegen to rewrite `recv.field = value` into `recv.setField(value)` so observers fire on direct writes.
func isReactiveFieldOf(ctx *codegen.EmitContext, pkg *ir.PackageContext, receiverSym ir.SymID, fieldName string) bool {
	if receiverSym == 0 {
		return false
	}
	sym, ok := pkg.Syms.GetByID(receiverSym)
	if !ok || sym.Typ == 0 {
		return false
	}
	ty, ok := ctx.Types.GetByID(sym.Typ)
	if !ok || ty.Kind != ir.TK_Struct {
		return false
	}
	for _, f := range ty.StructFields {
		if f.Name == fieldName {
			return f.IsReactive
		}
	}
	return false
}

// goExportedName uppercases the first rune of a Sova-side identifier so the resulting Go-side struct field/method name is exported. Empty input or already-uppercase input is returned unchanged.
func goExportedName(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	if r[0] >= 'A' && r[0] <= 'Z' {
		return s
	}
	if r[0] >= 'a' && r[0] <= 'z' {
		r[0] = r[0] - 'a' + 'A'
	}
	return string(r)
}

// buildStructTag turns a slice of resolved annotations into a Go struct tag map suitable for jen.Statement.Tag. Returns nil when there are no annotations with resolved arguments (jen skips the tag rendering for nil).
func buildStructTag(annos []ir.Annotation) map[string]string {
	if len(annos) == 0 {
		return nil
	}
	out := map[string]string{}
	for _, a := range annos {
		if a.Name.Name == "" || len(a.ResolvedArgs) == 0 {
			continue
		}
		parts := make([]string, 0, len(a.ResolvedArgs))
		for _, v := range a.ResolvedArgs {
			switch v.Kind {
			case ir.AnnotationValueString:
				parts = append(parts, v.Str)
			case ir.AnnotationValueInt:
				parts = append(parts, strconv.FormatInt(v.Int, 10))
			case ir.AnnotationValueBool:
				if v.Bool {
					parts = append(parts, "true")
				} else {
					parts = append(parts, "false")
				}
			}
		}
		if len(parts) == 0 {
			continue
		}
		out[a.Name.Name] = strings.Join(parts, ",")
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func lookupImportedPackage(ctx *codegen.EmitContext, currentPkg *ir.PackageContext, alias string) *ir.PackageContext {
	for _, pkg := range ctx.Pkgs {
		if pkg == currentPkg || len(pkg.Path) == 0 {
			continue
		}
		if pkg.Path[len(pkg.Path)-1] == alias {
			return pkg
		}
	}
	for _, pkg := range ctx.TransPkgs {
		if len(pkg.Path) == 0 {
			continue
		}
		if pkg.Path[len(pkg.Path)-1] == alias {
			return pkg
		}
	}
	return nil
}

func symName(ctx *codegen.EmitContext, sym ir.SymID) string {
	if name, ok := ctx.Names.GetMangledName(sym); ok {
		return name
	}
	panic("unresolved symbol: " + fmt.Sprint(sym))
}

func symNameWithUnused(ctx *codegen.EmitContext, pkg *ir.PackageContext, sym ir.SymID) string {
	if symbol, ok := pkg.Syms.GetByID(sym); ok {
		if symbol.Flags&ir.SF_Unused != 0 {
			return "_"
		}
	}
	return symName(ctx, sym)
}

func symOrigName(ctx *codegen.EmitContext, sym ir.SymID) string {
	if name, ok := ctx.Names.GetOriginalName(sym); ok {
		return name
	}
	return ""
}

// getEnumSymbol looks up the symbol ID for an enum type by its name.
// findTypeSymbolAcrossPkgs locates the SymID that declares a struct/interface/enum with `name` in the package whose path matches `pkgPath`. Used by Go codegen so that `*Item` references in package A's emitted code that refer to package B's `type Item` resolve to B's mangled name (not A's). Empty `pkgPath` falls back to the legacy any-package search to keep compiler-internal globals (error/Session/Broadcast/WireState) discoverable.
func findTypeSymbolAcrossPkgs(ctx *codegen.EmitContext, currentPkg *ir.PackageContext, pkgPath, name string) ir.SymID {
	if pkgPath != "" {
		var pools []*ir.PackageContext
		if currentPkg != nil && currentPkg.Path.String() == pkgPath {
			pools = append(pools, currentPkg)
		}
		for _, other := range ctx.Pkgs {
			if other.Path.String() == pkgPath {
				pools = append(pools, other)
			}
		}
		for _, other := range ctx.TransPkgs {
			if other.Path.String() == pkgPath {
				pools = append(pools, other)
			}
		}
		for _, p := range pools {
			if sym := findSymInPkg(ctx, p, name); sym != 0 {
				return sym
			}
		}
		return 0
	}
	if currentPkg != nil {
		if sym := findSymInPkg(ctx, currentPkg, name); sym != 0 {
			return sym
		}
	}
	for _, other := range ctx.Pkgs {
		if other == currentPkg {
			continue
		}
		if sym := findSymInPkg(ctx, other, name); sym != 0 {
			return sym
		}
	}
	for _, other := range ctx.TransPkgs {
		if sym := findSymInPkg(ctx, other, name); sym != 0 {
			return sym
		}
	}
	return 0
}

func getEnumSymbol(ctx *codegen.EmitContext, pkg *ir.PackageContext, enumName string) ir.SymID {
	if sym := findSymInPkg(ctx, pkg, enumName); sym != 0 {
		return sym
	}
	for _, other := range ctx.Pkgs {
		if other == pkg {
			continue
		}
		if sym := findSymInPkg(ctx, other, enumName); sym != 0 {
			return sym
		}
	}
	return 0
}

func findSymInPkg(ctx *codegen.EmitContext, pkg *ir.PackageContext, enumName string) ir.SymID {
	for sym, s := range pkg.Syms.ByID() {
		if s.Kind != ir.SK_Function {
			continue
		}
		if orig, ok := ctx.Names.GetOriginalName(sym); ok && orig == enumName {
			return sym
		}
	}
	return 0
}

// getMethodSymbol looks up the symbol ID for an enum method by enum name and method name.
func getMethodSymbol(ctx *codegen.EmitContext, pkg *ir.PackageContext, enumName string, methodName string) ir.SymID {
	// Search through all symbols to find the method
	for sym := ir.SymID(1); ; sym++ {
		s, ok := pkg.Syms.GetByID(sym)
		if !ok {
			break
		}
		if s.Kind == ir.SK_Function {
			if orig, ok := ctx.Names.GetOriginalName(sym); ok && orig == methodName {
				// Check if this method belongs to the enum
				// The method's scope should be the enum's scope
				return sym
			}
		}
	}
	return 0
}

func typeOfVar(pkg *ir.PackageContext, v *ir.VarDeclStmt) ir.TypID {
	if len(v.Targets) == 1 && v.Targets[0].TypeAnn != nil && v.Targets[0].TypeAnn.Typ != 0 {
		return v.Targets[0].TypeAnn.Typ
	}
	if len(v.Targets) == 1 && v.Targets[0].Name != nil {
		if s, ok := pkg.Syms.GetByID(v.Targets[0].Name.Sym); ok {
			return s.Typ
		}
	}
	return 0
}

func typeOfSym(pkg *ir.PackageContext, sym ir.SymID) ir.TypID {
	if s, ok := pkg.Syms.GetByID(sym); ok {
		return s.Typ
	}
	return 0
}

func typeToGo(tt *ir.TypeTable, id ir.TypID) jen.Code {
	return typeToGoWithContext(nil, nil, tt, id)
}

// goNumericConversionWrapper returns expr wrapped with an explicit Go conversion when dstTyp/srcTyp are different numeric primitives that need coercion (e.g. int ↔ byte). Returns nil when no wrap is needed - either because the types already match or because they're not both numeric primitives.
func goNumericConversionWrapper(dstTyp, srcTyp ir.TypID, tt *ir.TypeTable, expr jen.Code) jen.Code {
	if dstTyp == 0 || srcTyp == 0 || dstTyp == srcTyp {
		return nil
	}
	dstName := goNumericPrimitiveName(dstTyp, tt)
	srcName := goNumericPrimitiveName(srcTyp, tt)
	if dstName == "" || srcName == "" || dstName == srcName {
		return nil
	}
	return jen.Id(dstName).Call(expr)
}

// goAnyBoxWrapper wraps `expr` in an explicit Go conversion when a value of `srcTyp` is being assigned into an `any` slot, so the resulting interface stores the precise Sova primitive type (e.g. `int64` rather than Go's default untyped-int promotion to `int`). Returns nil for non-primitive source types so the caller leaves the expression alone.
func goAnyBoxWrapper(srcTyp ir.TypID, tt *ir.TypeTable, expr jen.Code) jen.Code {
	if srcTyp == 0 {
		return nil
	}
	name := goNumericPrimitiveName(srcTyp, tt)
	if name == "" {
		return nil
	}
	return jen.Id(name).Call(expr)
}

func goNumericPrimitiveName(id ir.TypID, tt *ir.TypeTable) string {
	switch id {
	case tt.PrimInt():
		return "int64"
	case tt.PrimFloat():
		return "float64"
	case tt.PrimByte():
		return "byte"
	}
	return ""
}

// goSafePrimitiveConversion lowers a Sova `value as? T` between primitive types into an explicit Go conversion that yields `option<T>` (i.e. `*T`). Lossless conversions (e.g. `int -> float`) wrap the result in a non-nil pointer. Parse-style conversions (`string -> int`, `string -> float`) return `nil` when the parse fails so the caller can pattern-match on `none`. Returns nil for non-primitive pairs so the caller falls back to the runtime type-assertion path.
func goSafePrimitiveConversion(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, e *CodeEmitter, x *ir.AsExpr) *jen.Statement {
	tt := ctx.Types
	str := tt.PrimString()
	in := tt.PrimInt()
	fl := tt.PrimFloat()
	bl := tt.PrimBool()
	ch := tt.PrimChar()
	bt := tt.PrimByte()
	srcTy := x.Expr.GetType()
	dstTy := x.Target.Typ
	if srcTy == 0 || dstTy == 0 || srcTy == dstTy {
		return nil
	}
	isPrim := func(t ir.TypID) bool {
		return t == str || t == in || t == fl || t == bl || t == ch || t == bt
	}
	if !isPrim(srcTy) || !isPrim(dstTy) {
		return nil
	}
	src := e.buildExpr(ctx, pkg, f, x.Expr)
	dstGo := typeToGoWithContext(ctx, pkg, tt, dstTy)
	wrapInfallible := func(value jen.Code) *jen.Statement {
		return jen.Func().Params().Op("*").Add(dstGo).Block(
			jen.Id("__v").Op(":=").Add(value),
			jen.Return(jen.Op("&").Id("__v")),
		).Call()
	}
	switch {
	case dstTy == str && srcTy == in:
		return wrapInfallible(jen.Qual("strconv", "FormatInt").Call(src, jen.Lit(10)))
	case dstTy == str && srcTy == fl:
		return wrapInfallible(jen.Qual("strconv", "FormatFloat").Call(src, jen.LitRune('f'), jen.Lit(-1), jen.Lit(64)))
	case dstTy == str && srcTy == bl:
		return wrapInfallible(jen.Qual("strconv", "FormatBool").Call(src))
	case dstTy == str && srcTy == ch:
		return wrapInfallible(jen.Id("string").Call(src))
	case srcTy == str && dstTy == in:
		return jen.Func().Params().Op("*").Int64().Block(
			jen.List(jen.Id("__n"), jen.Id("__err")).Op(":=").Qual("strconv", "ParseInt").Call(src, jen.Lit(10), jen.Lit(64)),
			jen.If(jen.Id("__err").Op("!=").Nil()).Block(jen.Return(jen.Nil())),
			jen.Return(jen.Op("&").Id("__n")),
		).Call()
	case srcTy == str && dstTy == fl:
		return jen.Func().Params().Op("*").Float64().Block(
			jen.List(jen.Id("__f"), jen.Id("__err")).Op(":=").Qual("strconv", "ParseFloat").Call(src, jen.Lit(64)),
			jen.If(jen.Id("__err").Op("!=").Nil()).Block(jen.Return(jen.Nil())),
			jen.Return(jen.Op("&").Id("__f")),
		).Call()
	case srcTy == str && dstTy == bl:
		return jen.Func().Params().Op("*").Bool().Block(
			jen.List(jen.Id("__b"), jen.Id("__err")).Op(":=").Qual("strconv", "ParseBool").Call(src),
			jen.If(jen.Id("__err").Op("!=").Nil()).Block(jen.Return(jen.Nil())),
			jen.Return(jen.Op("&").Id("__b")),
		).Call()
	case srcTy == in && dstTy == fl:
		return wrapInfallible(jen.Id("float64").Call(src))
	case srcTy == fl && dstTy == in:
		return wrapInfallible(jen.Id("int64").Call(src))
	case srcTy == in && dstTy == ch:
		return wrapInfallible(jen.Id("rune").Call(src))
	case srcTy == ch && dstTy == in:
		return wrapInfallible(jen.Id("int64").Call(src))
	case srcTy == in && dstTy == bt:
		return wrapInfallible(jen.Id("byte").Call(src))
	case srcTy == bt && dstTy == in:
		return wrapInfallible(jen.Id("int64").Call(src))
	}
	return nil
}

// goPrimitiveConversion lowers a Sova `value as T` between primitive types into the explicit Go conversion (mirrors the JS side). Returns nil for any non-primitive or unsupported pair so the caller falls back to the standard type-assertion path. None of the emitted forms panic: `strconv.Parse*` errors are silently swallowed and the zero value is returned, matching the no-throw policy.
func goPrimitiveConversion(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, e *CodeEmitter, x *ir.AsExpr) *jen.Statement {
	if x.Safe {
		return nil
	}
	tt := ctx.Types
	str := tt.PrimString()
	in := tt.PrimInt()
	fl := tt.PrimFloat()
	bl := tt.PrimBool()
	ch := tt.PrimChar()
	srcTy := x.Expr.GetType()
	dstTy := x.Target.Typ
	if srcTy == 0 || dstTy == 0 || srcTy == dstTy {
		return nil
	}
	isPrim := func(t ir.TypID) bool { return t == str || t == in || t == fl || t == bl || t == ch }
	if !isPrim(srcTy) || !isPrim(dstTy) {
		return nil
	}
	src := e.buildExpr(ctx, pkg, f, x.Expr)
	switch {
	case dstTy == str && srcTy == in:
		return jen.Qual("strconv", "FormatInt").Call(src, jen.Lit(10))
	case dstTy == str && srcTy == fl:
		return jen.Qual("strconv", "FormatFloat").Call(src, jen.LitRune('f'), jen.Lit(-1), jen.Lit(64))
	case dstTy == str && srcTy == bl:
		return jen.Qual("strconv", "FormatBool").Call(src)
	case dstTy == str && srcTy == ch:
		return jen.Id("string").Call(src)
	case srcTy == str && dstTy == in:
		return jen.Func().Params().Int64().Block(
			jen.List(jen.Id("__n"), jen.Id("_")).Op(":=").Qual("strconv", "ParseInt").Call(src, jen.Lit(10), jen.Lit(64)),
			jen.Return(jen.Id("__n")),
		).Call()
	case srcTy == str && dstTy == fl:
		return jen.Func().Params().Float64().Block(
			jen.List(jen.Id("__f"), jen.Id("_")).Op(":=").Qual("strconv", "ParseFloat").Call(src, jen.Lit(64)),
			jen.Return(jen.Id("__f")),
		).Call()
	case srcTy == str && dstTy == bl:
		return jen.Func().Params().Bool().Block(
			jen.List(jen.Id("__b"), jen.Id("_")).Op(":=").Qual("strconv", "ParseBool").Call(src),
			jen.Return(jen.Id("__b")),
		).Call()
	case srcTy == in && dstTy == fl:
		return jen.Id("float64").Call(src)
	case srcTy == fl && dstTy == in:
		return jen.Id("int64").Call(src)
	case srcTy == in && dstTy == ch:
		return jen.Id("rune").Call(src)
	case srcTy == ch && dstTy == in:
		return jen.Id("int64").Call(src)
	}
	return nil
}

func typeToGoWithContext(ctx *codegen.EmitContext, pkg *ir.PackageContext, tt *ir.TypeTable, id ir.TypID) jen.Code {
	if id == 0 {
		return jen.Id("any")
	}
	if ty, ok := tt.GetByID(id); ok {
		switch ty.Kind {
		case ir.TK_PrimitiveInt:
			return jen.Id("int64")
		case ir.TK_PrimitiveFloat:
			return jen.Id("float64")
		case ir.TK_PrimitiveBool:
			return jen.Id("bool")
		case ir.TK_PrimitiveString:
			return jen.Id("string")
		case ir.TK_PrimitiveChar:
			return jen.Id("rune")
		case ir.TK_PrimitiveByte:
			return jen.Id("byte")
		case ir.TK_Option:
			return jen.Op("*").Add(typeToGoWithContext(ctx, pkg, tt, ty.ElemType))
		case ir.TK_Slice:
			return jen.Index().Add(typeToGoWithContext(ctx, pkg, tt, ty.ElemType))
		case ir.TK_Array:
			return jen.Index(jen.Lit(ty.Dim)).Add(typeToGoWithContext(ctx, pkg, tt, ty.ElemType))
		case ir.TK_Map:
			return jen.Map(typeToGoWithContext(ctx, pkg, tt, ty.KeyType)).Add(typeToGoWithContext(ctx, pkg, tt, ty.ValueType))
		case ir.TK_Tuple:
			return jen.Index().Any()
		case ir.TK_Function:
			params := make([]jen.Code, len(ty.ParamTypes))
			for i, param := range ty.ParamTypes {
				params[i] = typeToGoWithContext(ctx, pkg, tt, param.Type.Typ)
			}
			if ty.ReturnType == 0 || ty.ReturnType == tt.TypNone() {
				return jen.Func().Params(params...)
			}
			returnType := typeToGoWithContext(ctx, pkg, tt, ty.ReturnType)
			return jen.Func().Params(params...).Add(returnType)
		case ir.TK_Enum:
			// Try to get the mangled enum name using context
			enumName := ty.EnumName
			if ctx != nil && pkg != nil {
				enumSym := getEnumSymbol(ctx, pkg, ty.EnumName)
				if enumSym != 0 {
					enumName = symName(ctx, enumSym)
				}
			}
			if ty.IsNumeric {
				return jen.Id(enumName)
			}
			return jen.Op("*").Id(enumName)
		case ir.TK_TypeParam:
			return jen.Id("any")
		case ir.TK_Chan:
			return jen.Chan().Add(typeToGoWithContext(ctx, pkg, tt, ty.ElemType))
		case ir.TK_Struct:
			if ty.IsExtern {
				if ty.ExternValue {
					return jen.Qual(ty.ExternModule, ty.StructName)
				}
				return jen.Op("*").Qual(ty.ExternModule, ty.StructName)
			}
			if ctx != nil && ctx.Cache != nil {
				if sessTyp, ok := ctx.Cache["sessions_session_typ"].(ir.TypID); ok && sessTyp == id {
					return jen.Op("*").Id("fn____Session")
				}
				if bcTyp, ok := ctx.Cache["sessions_broadcast_typ"].(ir.TypID); ok && bcTyp == id {
					return jen.Op("*").Id("fn____Broadcast")
				}
				if errTyp, ok := ctx.Cache["builtin_error_typ"].(ir.TypID); ok && errTyp == id {
					return jen.Op("*").Id("sovaError")
				}
			}
			structName := ty.StructName
			if ctx != nil {
				if sym := findTypeSymbolAcrossPkgs(ctx, pkg, ty.PackagePath, ty.StructName); sym != 0 {
					structName = symName(ctx, sym)
				}
			}
			return jen.Op("*").Id(structName)
		case ir.TK_Interface:
			if ty.IsExtern {
				return jen.Qual(ty.ExternModule, ty.InterfaceName)
			}
			ifaceName := ty.InterfaceName
			if ctx != nil {
				if sym := findTypeSymbolAcrossPkgs(ctx, pkg, ty.PackagePath, ty.InterfaceName); sym != 0 {
					ifaceName = symName(ctx, sym)
				}
			}
			return jen.Id(ifaceName)
		default:
			break
		}
	}
	return jen.Id("any")
}

func isExprConstant(expr ir.Expr) bool {
	switch expr.(type) {
	case *ir.LitInt, *ir.LitFloat, *ir.LitBool, *ir.LitString, *ir.LitChar, *ir.LitNone:
		return true
	default:
		return false
	}
}

func fmtSprintfKey(ty ir.TypID, tt *ir.TypeTable) string {
	if ty == 0 {
		return "%v"
	}
	if t, ok := tt.GetByID(ty); ok {
		switch t.Kind {
		case ir.TK_PrimitiveInt:
			return "%d"
		case ir.TK_PrimitiveFloat:
			return "%f"
		case ir.TK_PrimitiveBool:
			return "%t"
		case ir.TK_PrimitiveString:
			return "%s"
		case ir.TK_PrimitiveChar:
			return "%c"
		case ir.TK_PrimitiveByte:
			return "%d"
		case ir.TK_Option, ir.TK_Slice, ir.TK_Array, ir.TK_Map, ir.TK_Tuple:
			return "%v"
		default:
			break
		}
	}
	return "%v"
}

func (e *CodeEmitter) replaceModPlaceholder(nativeCall string, module string) string {
	result := ""
	i := 0
	for i < len(nativeCall) {
		if i+4 <= len(nativeCall) && nativeCall[i:i+4] == "@mod" {
			result += module
			i += 4
		} else {
			result += string(nativeCall[i])
			i++
		}
	}
	return result
}

func (e *CodeEmitter) buildNativeCall(nativeCall string, params []jen.Code) jen.Code {
	return e.buildNativeCallWithModule(nativeCall, "", params)
}

// buildNativeCallWithModule emits a Go call expression for a native function reference. When `modulePath` is non-empty and `nativeCall` is a dotted identifier whose first segment matches the module's last path segment (e.g. `uuid.NewString` for module `github.com/google/uuid`), the package selector is rewritten to use the full module import path. This is what makes third-party extern bindings produce a correctly-resolved `jen.Qual("github.com/google/uuid", "NewString")` rather than the broken `jen.Qual("uuid", "NewString")` (which goimports would then attempt to resolve against a non-existent `"uuid"` import path). When `modulePath` is empty (stdlib externs and the inline-expression escape hatch), behaviour is identical to the original two-arg form.
func (e *CodeEmitter) buildNativeCallWithModule(nativeCall string, modulePath string, params []jen.Code) jen.Code {
	if !isDottedIdentGo(nativeCall) {
		return jen.Parens(jen.Op(nativeCall)).Call(params...)
	}
	parts := splitDottedIdent(nativeCall)
	if modulePath != "" && len(parts) >= 2 && parts[0] == lastPathSegment(modulePath) {
		parts[0] = modulePath
	}
	if len(parts) == 1 {
		return jen.Id(parts[0]).Call(params...)
	}
	base := jen.Qual(parts[0], parts[1])
	for i := 2; i < len(parts); i++ {
		base = base.Dot(parts[i])
	}
	return base.Call(params...)
}

func (e *CodeEmitter) buildNativeRef(nativeRef string) jen.Code {
	return e.buildNativeRefWithModule(nativeRef, "")
}

// buildNativeRefWithModule mirrors buildNativeCallWithModule for the value-reference path: it rewrites the leading package alias of a dotted identifier to the full module import path when one is known, so `jen.Qual` registers the correct import. See buildNativeCallWithModule for the rationale.
func (e *CodeEmitter) buildNativeRefWithModule(nativeRef string, modulePath string) jen.Code {
	if !isDottedIdentGo(nativeRef) {
		return jen.Parens(jen.Op(nativeRef))
	}
	parts := splitDottedIdent(nativeRef)
	if modulePath != "" && len(parts) >= 2 && parts[0] == lastPathSegment(modulePath) {
		parts[0] = modulePath
	}
	if len(parts) == 1 {
		return jen.Id(parts[0])
	}
	base := jen.Qual(parts[0], parts[1])
	for i := 2; i < len(parts); i++ {
		base = base.Dot(parts[i])
	}
	return base
}

// splitDottedIdent splits a dotted Go identifier chain (`pkg.Type.Field`) into its segments. Caller must already have verified the input is a dotted identifier via `isDottedIdentGo`; the function does no validation of its own.
func splitDottedIdent(s string) []string {
	parts := []string{}
	current := ""
	for _, ch := range s {
		if ch == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

// registerExternImport hooks an extern's native body up to the Go output's import block. `modulePath` is the Go import path; `nativeCall` is the raw native expression (either a dotted identifier like `pkg.Func` or an inline `func(...) {...}` literal). The function does two things: it registers the conventional package-name alias with jen via `ImportName`, and — because jen only materialises an import when it sees a `Qual` reference in the AST — it also scans `nativeCall` for `<alias>.<Symbol>` occurrences and emits a synthetic `var _ = pkg.Symbol` at file scope. Without that synthetic reference, inline `func(...) {...}` bodies (which jen renders as opaque text) would never trigger an import for third-party paths like `github.com/redis/go-redis/v9`, and the resulting Go output would fail to compile with `undefined: redis`.
func (e *CodeEmitter) registerExternImport(modulePath string, nativeCall string) {
	if modulePath == "" {
		return
	}
	bodyAlias := aliasFromBody(nativeCall, modulePath)
	existing, seen := e.externImports[modulePath]
	switch {
	case bodyAlias != "" && !seen:
		e.jf.ImportName(modulePath, bodyAlias)
		e.externImports[modulePath] = bodyAlias
	case bodyAlias != "" && seen && existing == "":
		e.jf.ImportName(modulePath, bodyAlias)
		e.externImports[modulePath] = bodyAlias
	case bodyAlias == "" && !seen:
		fallback := lastPathSegment(modulePath)
		e.jf.ImportName(modulePath, fallback)
		e.externImports[modulePath] = ""
	}
	aliasForRefs := bodyAlias
	if aliasForRefs == "" {
		aliasForRefs = e.externImports[modulePath]
	}
	if aliasForRefs == "" {
		aliasForRefs = lastPathSegment(modulePath)
	}
	for _, sym := range firstExportedRefs(nativeCall, aliasForRefs) {
		e.jf.Add(jen.Var().Id("_").Op("=").Qual(modulePath, sym))
	}
}

// aliasFromBody scans an inline native body for `<ident>.<Symbol>` package selectors (both call form and type/value form) and picks the most frequent `<ident>` that is a plausible alias for `modulePath`. Plausibility is judged by `aliasPlausibleForModule`: the candidate must appear as a segment of the module path (after stripping the conventional `/vN` major-version suffix and any `go-` prefix that Go libraries often carry). This rejects unrelated stdlib qualifiers like `context.Background(...)` or `time.Duration(...)` that frequently show up in extern bodies and would otherwise be mistaken for the alias of the wrapped module. Returns the empty string when no plausible candidate is found, in which case the caller falls back to a path-based default.
func aliasFromBody(body string, modulePath string) string {
	if body == "" {
		return ""
	}
	counts := map[string]int{}
	for _, ref := range allPackageRefs(body) {
		if !aliasPlausibleForModule(ref.alias, modulePath) {
			continue
		}
		counts[ref.alias]++
	}
	var best string
	bestN := 0
	for k, v := range counts {
		if v > bestN {
			best = k
			bestN = v
		}
	}
	return best
}

// allPackageRefs returns every `<ident>.<UpperSymbol>` occurrence in `body`, whether followed by `(` (call), `)` (type assertion), or anything else (field/method-value reference). Used by alias detection to consider every plausible package selector, not only call-form.
func allPackageRefs(body string) []packageCallRef {
	out := []packageCallRef{}
	if body == "" {
		return out
	}
	i := 0
	for i < len(body) {
		if !isLetterOrUnderscoreByte(body[i]) {
			i++
			continue
		}
		start := i
		for i < len(body) && isIdentRuneByte(body[i]) {
			i++
		}
		if i >= len(body) || body[i] != '.' {
			continue
		}
		j := i + 1
		if j >= len(body) {
			continue
		}
		if !(body[j] >= 'A' && body[j] <= 'Z') {
			continue
		}
		k := j + 1
		for k < len(body) && isIdentRuneByte(body[k]) {
			k++
		}
		if start > 0 && body[start-1] == '.' {
			continue
		}
		alias := body[start:i]
		if isGoReservedOrCommonVar(alias) {
			continue
		}
		identStart := body[start]
		if !(identStart >= 'a' && identStart <= 'z') && identStart != '_' {
			continue
		}
		out = append(out, packageCallRef{alias: alias, sym: body[j:k]})
	}
	return out
}

// aliasPlausibleForModule reports whether `alias` could reasonably be the Go-side package name of `modulePath`. A name is plausible if it appears verbatim as a `/`-segment of the path, OR if it matches a segment with a conventional `go-` prefix stripped (`go-redis` → `redis`), OR if it matches the second-to-last segment when the last is a Go major-version tag (`v2`/`v9`/etc). The check is liberal on purpose: false positives would mean we pick a slightly wrong alias hint, which jen handles gracefully; false negatives would mean we fall back to a path-derived alias that may itself be wrong.
func aliasPlausibleForModule(alias string, modulePath string) bool {
	if alias == "" || modulePath == "" {
		return false
	}
	for _, seg := range strings.Split(modulePath, "/") {
		if seg == alias {
			return true
		}
		if strings.HasPrefix(seg, "go-") && seg[3:] == alias {
			return true
		}
		if strings.HasSuffix(seg, "-go") && seg[:len(seg)-3] == alias {
			return true
		}
	}
	return false
}

type packageCallRef struct {
	alias string
	sym   string
}

// allPackageCallRefs walks `body` and returns every `<alias>.<UpperSymbol>(` occurrence, in source order. The `(` is the discriminator: it guarantees the symbol after the dot is a function (and therefore a value), which makes it safe to reference as `var _ = alias.Symbol` to force-import the underlying package without picking up types or constants. Common-variable and Go-keyword aliases (`c`, `tx`, `if`, ...) are filtered out so method calls on locally-bound values do not get mistaken for package-qualified calls.
func allPackageCallRefs(body string) []packageCallRef {
	out := []packageCallRef{}
	if body == "" {
		return out
	}
	i := 0
	for i < len(body) {
		if !isLetterOrUnderscoreByte(body[i]) {
			i++
			continue
		}
		start := i
		for i < len(body) && isIdentRuneByte(body[i]) {
			i++
		}
		if i >= len(body) || body[i] != '.' {
			continue
		}
		j := i + 1
		if j >= len(body) {
			continue
		}
		if !(body[j] >= 'A' && body[j] <= 'Z') {
			continue
		}
		k := j + 1
		for k < len(body) && isIdentRuneByte(body[k]) {
			k++
		}
		if k >= len(body) || body[k] != '(' {
			continue
		}
		if start > 0 && body[start-1] == '.' {
			continue
		}
		alias := body[start:i]
		if isGoReservedOrCommonVar(alias) {
			continue
		}
		identStart := body[start]
		if !(identStart >= 'a' && identStart <= 'z') && identStart != '_' {
			continue
		}
		out = append(out, packageCallRef{alias: alias, sym: body[j:k]})
	}
	return out
}

func isLetterOrUnderscoreByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_'
}

func isIdentRuneByte(b byte) bool {
	return isLetterOrUnderscoreByte(b) || (b >= '0' && b <= '9')
}

// isGoReservedOrCommonVar filters out identifiers that show up as `<ident>.X` in inline bodies but are obviously not package names — Go keywords, common single-letter receivers (`c`, `e`, `v`, `r`, `o`, `m`, `n`, ...), and the placeholder receiver names used throughout this codebase's externs (`db`, `tx`, `cfg`, etc.). Keeps the alias-detection precise without requiring a full Go parser.
func isGoReservedOrCommonVar(s string) bool {
	switch s {
	case "if", "else", "for", "range", "return", "var", "func", "switch", "case", "default", "break", "continue", "go", "defer", "select", "chan", "map", "interface", "struct", "type", "const", "true", "false", "nil", "package", "import":
		return true
	case "c", "e", "v", "r", "o", "m", "n", "h", "d", "i", "j", "k", "p", "s", "t", "u", "x", "y", "z", "f", "g", "fn", "tx", "db", "cfg", "ok", "err", "ms", "ks", "chs", "pats", "ps", "msg", "out", "tmp", "ctx", "opts", "result", "strs", "head", "args", "id", "ch", "raw", "fb", "hit", "loc", "params", "ws":
		return true
	}
	return false
}

// firstExportedRefs returns the first exported function-call symbol invoked through `alias` in `body` — e.g. `firstExportedRefs("redis.ParseURL(u); redis.NewClient(o)", "redis")` returns `["ParseURL"]`. Only call-form `<alias>.<Sym>(` matches; types and constants (which are not valid in `var _ = pkg.X`) are filtered out. We deliberately return only the FIRST symbol per body — one synthetic reference per extern is enough to force the import, and emitting more would clutter the output.
func firstExportedRefs(body string, alias string) []string {
	if alias == "" || body == "" {
		return nil
	}
	for _, ref := range allPackageCallRefs(body) {
		if ref.alias == alias {
			return []string{ref.sym}
		}
	}
	return nil
}

// lastPathSegment returns the conventional Go package name implied by an import path. For most paths this is the final `/`-delimited segment ("github.com/foo/bar" → "bar"); Go's semantic-import-versioning convention (`/v2`, `/v3`, ... — see https://go.dev/ref/mod#major-version-suffixes) is detected and the version segment is skipped, so "github.com/redis/go-redis/v9" → "redis" rather than the literal "v9", matching the actual package name the module exports.
func lastPathSegment(p string) string {
	last := p
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			last = p[i+1:]
			break
		}
	}
	if isGoMajorVersionTag(last) {
		head := p[:len(p)-len(last)-1]
		for i := len(head) - 1; i >= 0; i-- {
			if head[i] == '/' {
				return head[i+1:]
			}
		}
		return head
	}
	return last
}

// isGoMajorVersionTag reports whether s matches Go's major-version-suffix shape (`v` followed by digits), e.g. "v2", "v15".
func isGoMajorVersionTag(s string) bool {
	if len(s) < 2 || s[0] != 'v' {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// isDottedIdentGo reports whether s is a chain of identifiers separated by dots (e.g. `strings.Contains`). Anything else is treated as a raw Go expression by the extern emitter, so stdlib mappings can use inline `func(...) { ... }` literals without compiler runtime helpers.
func isDottedIdentGo(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '.':
			if i == 0 {
				return false
			}
		case r == '_':
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// opOverloadName returns the method name corresponding to a binary operator (e.g. + -> op+), with ok=false for operators that cannot be overloaded.
func opOverloadName(op ir.Op) (string, bool) {
	switch op {
	case ir.OpAdd, ir.OpSub, ir.OpMul, ir.OpDiv, ir.OpMod, ir.OpEq:
		return "op" + string(op), true
	}
	return "", false
}

// emitWireStateDecl writes the WireState enum, the Session struct and its methods, and the cookie-based load/save helpers consumed by every wired handler. When the build's NeedsSessionManager flag is true the cookie format flips to `<uuid>.<hmac>` and an in-memory registry holds the full session record server-side; otherwise the legacy base64-JSON cookie continues to carry the full payload.
func (e *CodeEmitter) emitWireStateDecl(ctx *codegen.EmitContext, block *jen.Group) {
	block.Add(jen.Type().Id("WireState").Int64())
	block.Add(jen.Const().DefsFunc(func(g *jen.Group) {
		g.Id("WireStateOk").Id("WireState").Op("=").Lit(0)
		g.Id("WireStateUnauthorized").Id("WireState").Op("=").Lit(1)
		g.Id("WireStateForbidden").Id("WireState").Op("=").Lit(2)
		g.Id("WireStateNotFound").Id("WireState").Op("=").Lit(3)
		g.Id("WireStateError").Id("WireState").Op("=").Lit(4)
	}))

	// Centralised 400 responder shared by every HTTP wire handler. Keeps the
	// shape consistent so the JS-side stub can map status → state without
	// special-casing per handler. The body still carries the `{value, state}`
	// envelope the frontend already understands; `error` is added as a
	// human-readable hint that the runtime / a custom UI can surface.
	block.Add(jen.Func().Id("__sovaRespondBadRequest").Params(
		jen.Id("w").Qual("net/http", "ResponseWriter"),
		jen.Id("msg").String(),
	).Block(
		jen.Id("w").Dot("Header").Call().Dot("Set").Call(jen.Lit("Content-Type"), jen.Lit("application/json")),
		jen.Id("w").Dot("WriteHeader").Call(jen.Qual("net/http", "StatusBadRequest")),
		jen.Id("_").Op("=").Qual("encoding/json", "NewEncoder").Call(jen.Id("w")).Dot("Encode").Call(jen.Map(jen.String()).Any().Values(jen.Dict{
			jen.Lit("value"): jen.Nil(),
			jen.Lit("state"): jen.Int64().Call(jen.Id("WireStateError")),
			jen.Lit("error"): jen.Id("msg"),
		})),
	))

	emitSessionStructAndMethods(block)

	needsManager := false
	if ctx.Cache != nil {
		if v, ok := ctx.Cache["needs_session_manager"].(bool); ok {
			needsManager = v
		}
	}

	if needsManager {
		emitSessionManagerHelpers(ctx, block)
		emitFrontendWireMethods(ctx, block)
		emitBroadcastStruct(ctx, block)
		emitSessionsFreeFuncs(ctx, block)
		emitTestHarness(ctx, block)
	} else {
		emitLegacyCookieHelpers(block)
	}
}

// emitBroadcastStruct emits the `fn____Broadcast` Go struct whose value is returned by sessions.broadcast(). It carries a chain of filter predicates that narrow which connected sessions receive a fan-out call; .to(room) appends a room-membership predicate, .filter(pred) takes a user predicate. Each frontend wire func declared in the build receives a matching method on Broadcast that iterates `__sovaSessionAll()`, applies the narrowing filters, and forwards through `__sovaWSSendTo` to every matching session.
func emitBroadcastStruct(ctx *codegen.EmitContext, block *jen.Group) {
	rawWires, _ := ctx.Cache["frontend_wire_funcs"]
	wires, _ := rawWires.([]*ir.FuncDeclStmt)

	block.Add(jen.Type().Id("fn____Broadcast").Struct(
		jen.Id("predicate").Func().Params(jen.Op("*").Id("fn____Session")).Bool(),
	))

	block.Add(jen.Func().Params(jen.Id("b").Op("*").Id("fn____Broadcast")).Id("to").Params(jen.Id("room").String()).Op("*").Id("fn____Broadcast").Block(
		jen.Id("prev").Op(":=").Id("b").Dot("predicate"),
		jen.Return(jen.Op("&").Id("fn____Broadcast").Values(jen.Dict{
			jen.Id("predicate"): jen.Func().Params(jen.Id("s").Op("*").Id("fn____Session")).Bool().Block(
				jen.If(jen.Id("prev").Op("!=").Nil().Op("&&").Op("!").Id("prev").Call(jen.Id("s"))).Block(jen.Return(jen.False())),
				jen.Return(jen.Id("s").Dot("inRoom").Call(jen.Id("room"))),
			),
		})),
	))

	block.Add(jen.Func().Params(jen.Id("b").Op("*").Id("fn____Broadcast")).Id("filter").Params(jen.Id("pred").Func().Params(jen.Op("*").Id("fn____Session")).Bool()).Op("*").Id("fn____Broadcast").Block(
		jen.Id("prev").Op(":=").Id("b").Dot("predicate"),
		jen.Return(jen.Op("&").Id("fn____Broadcast").Values(jen.Dict{
			jen.Id("predicate"): jen.Func().Params(jen.Id("s").Op("*").Id("fn____Session")).Bool().Block(
				jen.If(jen.Id("prev").Op("!=").Nil().Op("&&").Op("!").Id("prev").Call(jen.Id("s"))).Block(jen.Return(jen.False())),
				jen.Return(jen.Id("pred").Call(jen.Id("s"))),
			),
		})),
	))

	for _, fn := range wires {
		fnRef := fn
		wireName := fnRef.Name.Name
		// Method name has to match what Sova call sites lower to - the
		// mangled symbol name, since attachFrontendWireMethods now records
		// the wire-func Sym on StructMethodInfo. `wireName` stays the raw
		// name because it's the WS protocol field the frontend dispatches
		// on, not a Go identifier.
		methodGoName := wireName
		if fnRef.Name.Sym != 0 {
			methodGoName = symName(ctx, fnRef.Name.Sym)
		}
		paramDecls := make([]jen.Code, 0, len(fnRef.Params))
		paramNames := make([]jen.Code, 0, len(fnRef.Params))
		for i, prm := range fnRef.Params {
			paramName := fmt.Sprintf("__arg%d", i)
			paramDecls = append(paramDecls, jen.Id(paramName).Add(typeToGoWithContext(ctx, nil, ctx.Types, prm.Type.Typ)))
			paramNames = append(paramNames, jen.Id(paramName))
		}
		block.Add(jen.Func().Params(jen.Id("b").Op("*").Id("fn____Broadcast")).Id(methodGoName).Params(paramDecls...).Block(
			jen.List(jen.Id("__args"), jen.Id("_")).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Index().Any().Values(paramNames...)),
			jen.For(jen.Id("_").Op(",").Id("s").Op(":=").Range().Id("__sovaSessionAll").Call()).Block(
				jen.If(jen.Op("!").Id("s").Dot("IsConnected")).Block(jen.Continue()),
				jen.If(jen.Id("b").Dot("predicate").Op("!=").Nil().Op("&&").Op("!").Id("b").Dot("predicate").Call(jen.Id("s"))).Block(jen.Continue()),
				jen.Id("__sovaWSSendTo").Call(
					jen.Id("s").Dot("Id"),
					jen.Op("&").Id("fn____WSEnvelope").Values(jen.Dict{
						jen.Id("Op"):   jen.Lit("call"),
						jen.Id("Fn"):   jen.Lit(wireName),
						jen.Id("Args"): jen.Id("__args"),
					}),
				),
			),
		))
	}
}

// emitSessionsFreeFuncs emits Go function bodies for every sessions package free function (all, byId, firstByUser, allByUser, current, broadcast, onConnect, onDisconnect) under the exact mangled name each symbol received from the mangle pass. The bodies forward to the runtime registry helpers (__sovaSessionAll / __sovaSessionGet / __sovaBroadcast / etc.) emitted by emitSessionManagerHelpers. `current()` reads the per-request `__session` via context - Phase 5B keeps the simple "lookup or zero session" form; a cleaner per-goroutine context is in TODO.md.
func emitSessionsFreeFuncs(ctx *codegen.EmitContext, block *jen.Group) {
	rawPkg, _ := ctx.Cache["sessions_package"]
	pkg, _ := rawPkg.(*ir.PackageContext)
	if pkg == nil {
		return
	}
	lookupName := func(name string) string {
		if symID, ok := pkg.Scopes.LookupOnlyCurrent(pkg.Root, name); ok {
			if mangled, ok := ctx.Names.GetMangledName(symID); ok {
				return mangled
			}
		}
		return ""
	}

	if name := lookupName("all"); name != "" {
		block.Add(jen.Func().Id(name).Params().Index().Op("*").Id("fn____Session").Block(
			jen.Return(jen.Id("__sovaSessionAll").Call()),
		))
	}
	if name := lookupName("byId"); name != "" {
		block.Add(jen.Func().Id(name).Params(jen.Id("id").String()).Op("**").Id("fn____Session").Block(
			jen.Id("s").Op(":=").Id("__sovaSessionGet").Call(jen.Id("id")),
			jen.If(jen.Id("s").Op("==").Nil()).Block(jen.Return(jen.Nil())),
			jen.Return(jen.Op("&").Id("s")),
		))
	}
	if name := lookupName("firstByUser"); name != "" {
		block.Add(jen.Func().Id(name).Params(jen.Id("user").Any()).Op("**").Id("fn____Session").Block(
			jen.For(jen.Id("_").Op(",").Id("s").Op(":=").Range().Id("__sovaSessionAll").Call()).Block(
				jen.If(jen.Qual("reflect", "DeepEqual").Call(jen.Id("s").Dot("User"), jen.Id("user"))).Block(
					jen.Return(jen.Op("&").Id("s")),
				),
			),
			jen.Return(jen.Nil()),
		))
	}
	if name := lookupName("allByUser"); name != "" {
		block.Add(jen.Func().Id(name).Params(jen.Id("user").Any()).Index().Op("*").Id("fn____Session").Block(
			jen.Id("out").Op(":=").Index().Op("*").Id("fn____Session").Values(),
			jen.For(jen.Id("_").Op(",").Id("s").Op(":=").Range().Id("__sovaSessionAll").Call()).Block(
				jen.If(jen.Qual("reflect", "DeepEqual").Call(jen.Id("s").Dot("User"), jen.Id("user"))).Block(
					jen.Id("out").Op("=").Append(jen.Id("out"), jen.Id("s")),
				),
			),
			jen.Return(jen.Id("out")),
		))
	}
	if name := lookupName("current"); name != "" {
		block.Add(jen.Func().Id(name).Params().Op("*").Id("fn____Session").Block(
			jen.If(jen.Id("s").Op(":=").Id("__sovaCurrentSession").Call(), jen.Id("s").Op("!=").Nil()).Block(
				jen.Return(jen.Id("s")),
			),
			jen.Return(jen.Op("&").Id("fn____Session").Values()),
		))
	}
	if name := lookupName("broadcast"); name != "" {
		block.Add(jen.Func().Id(name).Params().Op("*").Id("fn____Broadcast").Block(
			jen.Return(jen.Op("&").Id("fn____Broadcast").Values()),
		))
	}
	if name := lookupName("onConnect"); name != "" {
		block.Add(jen.Func().Id(name).Params(jen.Id("handler").Func().Params(jen.Op("*").Id("fn____Session"))).Block(
			jen.Id("__sovaOnConnectHandlers").Op("=").Append(jen.Id("__sovaOnConnectHandlers"), jen.Id("handler")),
		))
	}
	if name := lookupName("onDisconnect"); name != "" {
		block.Add(jen.Func().Id(name).Params(jen.Id("handler").Func().Params(jen.Op("*").Id("fn____Session"))).Block(
			jen.Id("__sovaOnDisconnectHandlers").Op("=").Append(jen.Id("__sovaOnDisconnectHandlers"), jen.Id("handler")),
		))
	}
	if name := lookupName("onRoomJoin"); name != "" {
		block.Add(jen.Func().Id(name).Params(jen.Id("handler").Func().Params(jen.Op("*").Id("fn____Session"), jen.String())).Block(
			jen.Id("__sovaOnRoomJoinHandlers").Op("=").Append(jen.Id("__sovaOnRoomJoinHandlers"), jen.Id("handler")),
		))
	}
	if name := lookupName("onRoomLeave"); name != "" {
		block.Add(jen.Func().Id(name).Params(jen.Id("handler").Func().Params(jen.Op("*").Id("fn____Session"), jen.String())).Block(
			jen.Id("__sovaOnRoomLeaveHandlers").Op("=").Append(jen.Id("__sovaOnRoomLeaveHandlers"), jen.Id("handler")),
		))
	}

	block.Add(jen.Var().Id("__sovaOnConnectHandlers").Index().Func().Params(jen.Op("*").Id("fn____Session")))
	block.Add(jen.Var().Id("__sovaOnDisconnectHandlers").Index().Func().Params(jen.Op("*").Id("fn____Session")))
	block.Add(jen.Var().Id("__sovaOnRoomJoinHandlers").Index().Func().Params(jen.Op("*").Id("fn____Session"), jen.String()))
	block.Add(jen.Var().Id("__sovaOnRoomLeaveHandlers").Index().Func().Params(jen.Op("*").Id("fn____Session"), jen.String()))

	block.Add(jen.Func().Id("__sovaFireOnConnect").Params(jen.Id("s").Op("*").Id("fn____Session")).Block(
		jen.For(jen.Id("_").Op(",").Id("h").Op(":=").Range().Id("__sovaOnConnectHandlers")).Block(
			jen.Id("h").Call(jen.Id("s")),
		),
	))
	block.Add(jen.Func().Id("__sovaFireOnDisconnect").Params(jen.Id("s").Op("*").Id("fn____Session")).Block(
		jen.For(jen.Id("_").Op(",").Id("h").Op(":=").Range().Id("__sovaOnDisconnectHandlers")).Block(
			jen.Id("h").Call(jen.Id("s")),
		),
	))
	block.Add(jen.Func().Id("__sovaFireOnRoomJoin").Params(jen.Id("s").Op("*").Id("fn____Session"), jen.Id("room").String()).Block(
		jen.For(jen.Id("_").Op(",").Id("h").Op(":=").Range().Id("__sovaOnRoomJoinHandlers")).Block(
			jen.Id("h").Call(jen.Id("s"), jen.Id("room")),
		),
	))
	block.Add(jen.Func().Id("__sovaFireOnRoomLeave").Params(jen.Id("s").Op("*").Id("fn____Session"), jen.Id("room").String()).Block(
		jen.For(jen.Id("_").Op(",").Id("h").Op(":=").Range().Id("__sovaOnRoomLeaveHandlers")).Block(
			jen.Id("h").Call(jen.Id("s"), jen.Id("room")),
		),
	))

	block.Add(jen.Var().Id("__sovaCurrentSessionStore").Qual("sync", "Map"))

	block.Add(jen.Func().Id("__sovaGoid").Params().Int64().Block(
		jen.Var().Id("b").Index(jen.Lit(64)).Byte(),
		jen.Id("n").Op(":=").Qual("runtime", "Stack").Call(jen.Id("b").Index(jen.Empty(), jen.Empty()), jen.False()),
		jen.Id("fields").Op(":=").Qual("bytes", "Fields").Call(jen.Id("b").Index(jen.Empty(), jen.Id("n"))),
		jen.If(jen.Qual("", "len").Call(jen.Id("fields")).Op("<").Lit(2)).Block(jen.Return(jen.Lit(0))),
		jen.List(jen.Id("id"), jen.Id("_")).Op(":=").Qual("strconv", "ParseInt").Call(jen.String().Parens(jen.Id("fields").Index(jen.Lit(1))), jen.Lit(10), jen.Lit(64)),
		jen.Return(jen.Id("id")),
	))

	block.Add(jen.Func().Id("__sovaSetCurrentSession").Params(jen.Id("s").Op("*").Id("fn____Session")).Block(
		jen.Id("__sovaCurrentSessionStore").Dot("Store").Call(jen.Id("__sovaGoid").Call(), jen.Id("s")),
	))

	block.Add(jen.Func().Id("__sovaClearCurrentSession").Params().Block(
		jen.Id("__sovaCurrentSessionStore").Dot("Delete").Call(jen.Id("__sovaGoid").Call()),
	))

	block.Add(jen.Func().Id("__sovaCurrentSession").Params().Op("*").Id("fn____Session").Block(
		jen.List(jen.Id("v"), jen.Id("ok")).Op(":=").Id("__sovaCurrentSessionStore").Dot("Load").Call(jen.Id("__sovaGoid").Call()),
		jen.If(jen.Op("!").Id("ok")).Block(jen.Return(jen.Nil())),
		jen.List(jen.Id("s"), jen.Id("_")).Op(":=").Id("v").Assert(jen.Op("*").Id("fn____Session")),
		jen.Return(jen.Id("s")),
	))

	block.Add(jen.Func().Id("__sovaPushWireVar").Params(jen.Id("name").String(), jen.Id("value").Any()).Block(
		jen.List(jen.Id("raw"), jen.Id("err")).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Id("value")),
		jen.If(jen.Id("err").Op("!=").Nil()).Block(jen.Return()),
		jen.For(jen.Id("_").Op(",").Id("s").Op(":=").Range().Id("__sovaSessionAll").Call()).Block(
			jen.If(jen.Op("!").Id("s").Dot("IsConnected")).Block(jen.Continue()),
			jen.Id("__sovaWSSendTo").Call(
				jen.Id("s").Dot("Id"),
				jen.Op("&").Id("fn____WSEnvelope").Values(jen.Dict{
					jen.Id("Op"):    jen.Lit("var"),
					jen.Id("Fn"):    jen.Id("name"),
					jen.Id("Value"): jen.Id("raw"),
				}),
			),
		),
	))
}

// emitFrontendWireMethods walks every `wire func` declared on the frontend and emits a matching Go method on `*fn____Session` whose body marshals the args, builds a WS envelope, and delivers it to the session through the outbox. For wire funcs with non-none return types the method also registers a per-call reply channel via __sovaWSRegisterReply, awaits the reply, and decodes the JSON value into the Go return type. Phase 7 adds the Broadcast fan-out path that reuses the same envelope shape.
func emitFrontendWireMethods(ctx *codegen.EmitContext, block *jen.Group) {
	rawWires, _ := ctx.Cache["frontend_wire_funcs"]
	wires, _ := rawWires.([]*ir.FuncDeclStmt)
	if len(wires) == 0 {
		return
	}
	noneTyp := ctx.Types.TypNone()
	for _, fn := range wires {
		fnRef := fn
		wireName := fnRef.Name.Name
		// The Go method emitted on `*fn____Session` has to be reachable
		// from Sova call sites. After analyze_wire began propagating the
		// wire-func's symbol onto StructMethodInfo.Sym (so hover/sig-help
		// see the docstring), `@.printConsole(...)` in Sova now lowers to
		// `__session.<mangled>(...)` instead of `__session.printConsole(...)`.
		// Use the mangled name here so the dispatch lines up.
		methodGoName := wireName
		if fnRef.Name.Sym != 0 {
			methodGoName = symName(ctx, fnRef.Name.Sym)
		}
		hasReturn := fnRef.ReturnType != nil && fnRef.ReturnType.Typ != 0 && fnRef.ReturnType.Typ != noneTyp

		paramDecls := make([]jen.Code, 0, len(fnRef.Params))
		paramNames := make([]jen.Code, 0, len(fnRef.Params))
		for i, prm := range fnRef.Params {
			paramName := fmt.Sprintf("__arg%d", i)
			_ = prm.Name.Name
			paramType := typeToGoWithContext(ctx, nil, ctx.Types, prm.Type.Typ)
			paramDecls = append(paramDecls, jen.Id(paramName).Add(paramType))
			paramNames = append(paramNames, jen.Id(paramName))
		}

		methodDecl := jen.Func().Params(jen.Id("s").Op("*").Id("fn____Session")).Id(methodGoName).Params(paramDecls...)
		if hasReturn {
			methodDecl = methodDecl.Add(typeToGoWithContext(ctx, nil, ctx.Types, fnRef.ReturnType.Typ))
		}

		bodyStmts := []jen.Code{}
		argsLit := jen.Index().Any().Values(paramNames...)
		bodyStmts = append(bodyStmts,
			jen.List(jen.Id("__args"), jen.Id("_")).Op(":=").Qual("encoding/json", "Marshal").Call(argsLit),
		)

		if hasReturn {
			retType := typeToGoWithContext(ctx, nil, ctx.Types, fnRef.ReturnType.Typ)
			bodyStmts = append(bodyStmts,
				jen.Id("__cid").Op(":=").Id("__sovaNewSessionId").Call(),
				jen.Id("__ch").Op(":=").Id("__sovaWSRegisterReply").Call(jen.Id("__cid")),
				jen.Id("__ok").Op(":=").Id("__sovaWSSendTo").Call(
					jen.Id("s").Dot("Id"),
					jen.Op("&").Id("fn____WSEnvelope").Values(jen.Dict{
						jen.Id("Op"):   jen.Lit("call"),
						jen.Id("Id"):   jen.Id("__cid"),
						jen.Id("Fn"):   jen.Lit(wireName),
						jen.Id("Args"): jen.Id("__args"),
					}),
				),
				jen.Var().Id("__zero").Add(retType),
				jen.If(jen.Op("!").Id("__ok")).Block(jen.Return(jen.Id("__zero"))),
				jen.Id("__env").Op(":=").Op("<-").Id("__ch"),
				jen.If(jen.Id("__env").Op("==").Nil()).Block(jen.Return(jen.Id("__zero"))),
				jen.Var().Id("__result").Add(retType),
				jen.Id("_").Op("=").Qual("encoding/json", "Unmarshal").Call(jen.Id("__env").Dot("Value"), jen.Op("&").Id("__result")),
				jen.Return(jen.Id("__result")),
			)
		} else {
			bufLimit, buffered := bufferLimitForWire(fnRef)
			if buffered {
				bodyStmts = append(bodyStmts,
					jen.Id("__env").Op(":=").Op("&").Id("fn____WSEnvelope").Values(jen.Dict{
						jen.Id("Op"):   jen.Lit("call"),
						jen.Id("Fn"):   jen.Lit(wireName),
						jen.Id("Args"): jen.Id("__args"),
					}),
					jen.If(jen.Id("s").Dot("IsConnected")).Block(
						jen.Id("__sovaWSSendTo").Call(jen.Id("s").Dot("Id"), jen.Id("__env")),
					).Else().Block(
						jen.Id("__sovaSessionEnqueue").Call(jen.Id("s"), jen.Lit(wireName), jen.Id("__env"), jen.Lit(bufLimit)),
					),
				)
			} else {
				bodyStmts = append(bodyStmts,
					jen.Id("__sovaWSSendTo").Call(
						jen.Id("s").Dot("Id"),
						jen.Op("&").Id("fn____WSEnvelope").Values(jen.Dict{
							jen.Id("Op"):   jen.Lit("call"),
							jen.Id("Fn"):   jen.Lit(wireName),
							jen.Id("Args"): jen.Id("__args"),
						}),
					),
				)
			}
		}

		block.Add(methodDecl.Block(bodyStmts...))
	}
}

// emitSessionStructAndMethods writes the fn____Session struct mirroring the Sova-side sessions.Session shape plus the methods bound to it (authenticate, logout, role helpers, rooms helpers, isAuthenticated). These are emitted unconditionally because the Sova-side type surface is the same regardless of whether the session manager is active.
func emitSessionStructAndMethods(block *jen.Group) {
	block.Add(jen.Type().Id("fn____Session").Struct(
		jen.Id("Id").String().Tag(map[string]string{"json": "id,omitempty"}),
		jen.Id("User").Any().Tag(map[string]string{"json": "user,omitempty"}),
		jen.Id("Roles").Index().String().Tag(map[string]string{"json": "roles,omitempty"}),
		jen.Id("Claims").Map(jen.String()).Any().Tag(map[string]string{"json": "claims,omitempty"}),
		jen.Id("Rooms").Index().String().Tag(map[string]string{"json": "rooms,omitempty"}),
		jen.Id("ConnectedAt").Int64().Tag(map[string]string{"json": "connectedAt,omitempty"}),
		jen.Id("IsConnected").Bool().Tag(map[string]string{"json": "isConnected,omitempty"}),
		jen.Id("Auth").Bool().Tag(map[string]string{"json": "auth,omitempty"}),
		jen.Id("Pending").Map(jen.String()).Index().Index().Byte().Tag(map[string]string{"json": "-"}),
	))

	recv := func() *jen.Statement { return jen.Params(jen.Id("s").Op("*").Id("fn____Session")) }

	block.Add(jen.Func().Add(recv()).Id("authenticate").Params(jen.Id("u").Any(), jen.Id("claims").Map(jen.String()).Any()).Block(
		jen.Id("s").Dot("User").Op("=").Id("u"),
		jen.Id("s").Dot("Auth").Op("=").True(),
		jen.If(jen.Id("claims").Op("!=").Nil()).Block(
			jen.Id("s").Dot("Claims").Op("=").Id("claims"),
		),
	))
	block.Add(jen.Func().Add(recv()).Id("logout").Params().Block(
		jen.Id("s").Dot("User").Op("=").Nil(),
		jen.Id("s").Dot("Auth").Op("=").False(),
		jen.Id("s").Dot("Roles").Op("=").Nil(),
		jen.Id("s").Dot("Claims").Op("=").Nil(),
		jen.Id("s").Dot("Rooms").Op("=").Nil(),
	))
	block.Add(jen.Func().Add(recv()).Id("addRoles").Params(jen.Id("rs").Index().String()).Block(
		jen.Id("s").Dot("Roles").Op("=").Append(jen.Id("s").Dot("Roles"), jen.Id("rs").Op("...")),
	))
	block.Add(jen.Func().Add(recv()).Id("removeRoles").Params(jen.Id("rs").Index().String()).Block(
		jen.Id("skip").Op(":=").Map(jen.String()).Bool().Values(),
		jen.For(jen.Id("_").Op(",").Id("r").Op(":=").Range().Id("rs")).Block(
			jen.Id("skip").Index(jen.Id("r")).Op("=").True(),
		),
		jen.Id("out").Op(":=").Id("s").Dot("Roles").Index(jen.Empty(), jen.Lit(0)),
		jen.For(jen.Id("_").Op(",").Id("r").Op(":=").Range().Id("s").Dot("Roles")).Block(
			jen.If(jen.Op("!").Id("skip").Index(jen.Id("r"))).Block(
				jen.Id("out").Op("=").Append(jen.Id("out"), jen.Id("r")),
			),
		),
		jen.Id("s").Dot("Roles").Op("=").Id("out"),
	))
	block.Add(jen.Func().Add(recv()).Id("setRoles").Params(jen.Id("rs").Index().String()).Block(
		jen.Id("s").Dot("Roles").Op("=").Id("rs"),
	))
	block.Add(jen.Func().Add(recv()).Id("clearRoles").Params().Block(
		jen.Id("s").Dot("Roles").Op("=").Nil(),
	))
	block.Add(jen.Func().Add(recv()).Id("hasRole").Params(jen.Id("role").String()).Bool().Block(
		jen.For(jen.Id("_").Op(",").Id("r").Op(":=").Range().Id("s").Dot("Roles")).Block(
			jen.If(jen.Id("r").Op("==").Id("role")).Block(jen.Return(jen.True())),
		),
		jen.Return(jen.False()),
	))
	block.Add(jen.Func().Add(recv()).Id("isAuthenticated").Params().Bool().Block(
		jen.Return(jen.Id("s").Dot("Auth")),
	))

	block.Add(jen.Func().Add(recv()).Id("join").Params(jen.Id("room").String()).Block(
		jen.For(jen.Id("_").Op(",").Id("r").Op(":=").Range().Id("s").Dot("Rooms")).Block(
			jen.If(jen.Id("r").Op("==").Id("room")).Block(jen.Return()),
		),
		jen.Id("s").Dot("Rooms").Op("=").Append(jen.Id("s").Dot("Rooms"), jen.Id("room")),
		jen.Id("__sovaFireOnRoomJoin").Call(jen.Id("s"), jen.Id("room")),
	))
	block.Add(jen.Func().Add(recv()).Id("leave").Params(jen.Id("room").String()).Block(
		jen.Id("found").Op(":=").False(),
		jen.Id("out").Op(":=").Id("s").Dot("Rooms").Index(jen.Empty(), jen.Lit(0)),
		jen.For(jen.Id("_").Op(",").Id("r").Op(":=").Range().Id("s").Dot("Rooms")).Block(
			jen.If(jen.Id("r").Op("!=").Id("room")).Block(
				jen.Id("out").Op("=").Append(jen.Id("out"), jen.Id("r")),
			).Else().Block(
				jen.Id("found").Op("=").True(),
			),
		),
		jen.Id("s").Dot("Rooms").Op("=").Id("out"),
		jen.If(jen.Id("found")).Block(
			jen.Id("__sovaFireOnRoomLeave").Call(jen.Id("s"), jen.Id("room")),
		),
	))
	block.Add(jen.Func().Add(recv()).Id("inRoom").Params(jen.Id("room").String()).Bool().Block(
		jen.For(jen.Id("_").Op(",").Id("r").Op(":=").Range().Id("s").Dot("Rooms")).Block(
			jen.If(jen.Id("r").Op("==").Id("room")).Block(jen.Return(jen.True())),
		),
		jen.Return(jen.False()),
	))
}

// emitLegacyCookieHelpers writes the cookie-only load/save path used by builds that do not need the server-side session manager (no frontend wires, no mutable wire vars). The whole session marshals to base64-JSON in the cookie, no HMAC, no registry.
func emitLegacyCookieHelpers(block *jen.Group) {
	block.Add(jen.Func().Id("__sovaTestBypassAuth").Params().Bool().Block(jen.Return(jen.False())))
	block.Add(jen.Func().Id("__sovaFireOnRoomJoin").Params(jen.Id("s").Op("*").Id("fn____Session"), jen.Id("room").String()).Block())
	block.Add(jen.Func().Id("__sovaFireOnRoomLeave").Params(jen.Id("s").Op("*").Id("fn____Session"), jen.Id("room").String()).Block())
	block.Add(jen.Func().Id("__sovaLoadSession").Params(jen.Id("r").Op("*").Qual("net/http", "Request")).Op("*").Id("fn____Session").Block(
		jen.List(jen.Id("c"), jen.Id("err")).Op(":=").Id("r").Dot("Cookie").Call(jen.Lit("sova_session")),
		jen.If(jen.Id("err").Op("!=").Nil()).Block(jen.Return(jen.Op("&").Id("fn____Session").Values())),
		jen.List(jen.Id("raw"), jen.Id("derr")).Op(":=").Qual("encoding/base64", "StdEncoding").Dot("DecodeString").Call(jen.Id("c").Dot("Value")),
		jen.If(jen.Id("derr").Op("!=").Nil()).Block(jen.Return(jen.Op("&").Id("fn____Session").Values())),
		jen.Var().Id("s").Id("fn____Session"),
		jen.Id("_").Op("=").Qual("encoding/json", "Unmarshal").Call(jen.Id("raw"), jen.Op("&").Id("s")),
		jen.Return(jen.Op("&").Id("s")),
	))

	block.Add(jen.Func().Id("__sovaSaveSession").Params(
		jen.Id("w").Qual("net/http", "ResponseWriter"),
		jen.Id("s").Op("*").Id("fn____Session"),
	).Block(
		jen.List(jen.Id("raw"), jen.Id("_")).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Id("s")),
		jen.Qual("net/http", "SetCookie").Call(jen.Id("w"), jen.Op("&").Qual("net/http", "Cookie").Values(jen.Dict{
			jen.Id("Name"):     jen.Lit("sova_session"),
			jen.Id("Value"):    jen.Qual("encoding/base64", "StdEncoding").Dot("EncodeToString").Call(jen.Id("raw")),
			jen.Id("Path"):     jen.Lit("/"),
			jen.Id("HttpOnly"): jen.True(),
			jen.Id("SameSite"): jen.Qual("net/http", "SameSiteLaxMode"),
		})),
	))
}

// emitSessionManagerHelpers writes the WebSocket-era cookie + registry path: the cookie holds only `<sessionId>.<hmac>`, the actual session record lives in a process-local registry guarded by a sync.RWMutex, and ids are signed with HMAC-SHA256 using a secret resolved from WIRE_SESSION_SECRET env > manifest > stable dev fallback. Reconnects within the grace window re-attach to the same record; expired records are purged by a grace timer (Phase 10 wires the timer itself; Phase 3 lays the storage and signing primitives).
func emitSessionManagerHelpers(ctx *codegen.EmitContext, block *jen.Group) {
	manifestSecret := manifestSessionSecret(ctx)

	block.Add(jen.Var().Id("__sovaSessionRegistry").Op("=").Struct(
		jen.Id("mu").Qual("sync", "RWMutex"),
		jen.Id("m").Map(jen.String()).Op("*").Id("fn____Session"),
	).Values(jen.Dict{
		jen.Id("m"): jen.Map(jen.String()).Op("*").Id("fn____Session").Values(),
	}))

	block.Add(jen.Var().Id("__sovaSessionSecretCache").Index().Byte())

	block.Add(jen.Func().Id("__sovaSessionSecret").Params().Index().Byte().Block(
		jen.If(jen.Id("__sovaSessionSecretCache").Op("!=").Nil()).Block(
			jen.Return(jen.Id("__sovaSessionSecretCache")),
		),
		jen.If(jen.Id("v").Op(":=").Qual("os", "Getenv").Call(jen.Lit("WIRE_SESSION_SECRET")), jen.Id("v").Op("!=").Lit("")).Block(
			jen.Id("__sovaSessionSecretCache").Op("=").Index().Byte().Parens(jen.Id("v")),
			jen.Return(jen.Id("__sovaSessionSecretCache")),
		),
		jen.If(jen.Lit(manifestSecret).Op("!=").Lit("")).Block(
			jen.Id("__sovaSessionSecretCache").Op("=").Index().Byte().Parens(jen.Lit(manifestSecret)),
			jen.Return(jen.Id("__sovaSessionSecretCache")),
		),
		jen.Qual("log", "Println").Call(jen.Lit("[sova] warning: WIRE_SESSION_SECRET not set and no manifest secret; using insecure dev fallback - DO NOT USE IN PRODUCTION")),
		jen.Id("__sovaSessionSecretCache").Op("=").Index().Byte().Parens(jen.Lit("sova-dev-session-secret-DO-NOT-USE-IN-PRODUCTION")),
		jen.Return(jen.Id("__sovaSessionSecretCache")),
	))

	block.Add(jen.Func().Id("__sovaSignSessionId").Params(jen.Id("id").String()).String().Block(
		jen.Id("mac").Op(":=").Qual("crypto/hmac", "New").Call(jen.Qual("crypto/sha256", "New"), jen.Id("__sovaSessionSecret").Call()),
		jen.Id("mac").Dot("Write").Call(jen.Index().Byte().Parens(jen.Id("id"))),
		jen.Id("sig").Op(":=").Qual("encoding/base64", "RawURLEncoding").Dot("EncodeToString").Call(jen.Id("mac").Dot("Sum").Call(jen.Nil())),
		jen.Return(jen.Id("id").Op("+").Lit(".").Op("+").Id("sig")),
	))

	block.Add(jen.Func().Id("__sovaVerifySessionId").Params(jen.Id("token").String()).Params(jen.String(), jen.Bool()).Block(
		jen.Id("dot").Op(":=").Qual("strings", "LastIndexByte").Call(jen.Id("token"), jen.LitRune('.')),
		jen.If(jen.Id("dot").Op("<=").Lit(0).Op("||").Id("dot").Op("==").Qual("", "len").Call(jen.Id("token")).Op("-").Lit(1)).Block(
			jen.Return(jen.Lit(""), jen.False()),
		),
		jen.Id("id").Op(":=").Id("token").Index(jen.Empty(), jen.Id("dot")),
		jen.Id("sigPart").Op(":=").Id("token").Index(jen.Id("dot").Op("+").Lit(1), jen.Empty()),
		jen.List(jen.Id("sig"), jen.Id("derr")).Op(":=").Qual("encoding/base64", "RawURLEncoding").Dot("DecodeString").Call(jen.Id("sigPart")),
		jen.If(jen.Id("derr").Op("!=").Nil()).Block(jen.Return(jen.Lit(""), jen.False())),
		jen.Id("mac").Op(":=").Qual("crypto/hmac", "New").Call(jen.Qual("crypto/sha256", "New"), jen.Id("__sovaSessionSecret").Call()),
		jen.Id("mac").Dot("Write").Call(jen.Index().Byte().Parens(jen.Id("id"))),
		jen.Id("expected").Op(":=").Id("mac").Dot("Sum").Call(jen.Nil()),
		jen.If(jen.Op("!").Qual("crypto/hmac", "Equal").Call(jen.Id("sig"), jen.Id("expected"))).Block(
			jen.Return(jen.Lit(""), jen.False()),
		),
		jen.Return(jen.Id("id"), jen.True()),
	))

	block.Add(jen.Func().Id("__sovaNewSessionId").Params().String().Block(
		jen.Var().Id("buf").Index(jen.Lit(16)).Byte(),
		jen.List(jen.Id("_"), jen.Id("err")).Op(":=").Qual("crypto/rand", "Read").Call(jen.Id("buf").Index(jen.Empty(), jen.Empty())),
		jen.If(jen.Id("err").Op("!=").Nil()).Block(
			jen.Qual("log", "Fatalf").Call(jen.Lit("sova: crypto/rand failed: %v"), jen.Id("err")),
		),
		jen.Return(jen.Qual("encoding/hex", "EncodeToString").Call(jen.Id("buf").Index(jen.Empty(), jen.Empty()))),
	))

	block.Add(jen.Func().Id("__sovaSessionGet").Params(jen.Id("id").String()).Op("*").Id("fn____Session").Block(
		jen.Id("__sovaSessionRegistry").Dot("mu").Dot("RLock").Call(),
		jen.Defer().Id("__sovaSessionRegistry").Dot("mu").Dot("RUnlock").Call(),
		jen.Return(jen.Id("__sovaSessionRegistry").Dot("m").Index(jen.Id("id"))),
	))

	block.Add(jen.Func().Id("__sovaSessionPut").Params(jen.Id("s").Op("*").Id("fn____Session")).Block(
		jen.If(jen.Id("s").Op("==").Nil().Op("||").Id("s").Dot("Id").Op("==").Lit("")).Block(jen.Return()),
		jen.Id("__sovaSessionRegistry").Dot("mu").Dot("Lock").Call(),
		jen.Defer().Id("__sovaSessionRegistry").Dot("mu").Dot("Unlock").Call(),
		jen.Id("__sovaSessionRegistry").Dot("m").Index(jen.Id("s").Dot("Id")).Op("=").Id("s"),
	))

	block.Add(jen.Func().Id("__sovaSessionDelete").Params(jen.Id("id").String()).Block(
		jen.Id("__sovaSessionRegistry").Dot("mu").Dot("Lock").Call(),
		jen.Defer().Id("__sovaSessionRegistry").Dot("mu").Dot("Unlock").Call(),
		jen.Qual("", "delete").Call(jen.Id("__sovaSessionRegistry").Dot("m"), jen.Id("id")),
	))

	block.Add(jen.Func().Id("__sovaSessionAll").Params().Index().Op("*").Id("fn____Session").Block(
		jen.Id("__sovaSessionRegistry").Dot("mu").Dot("RLock").Call(),
		jen.Defer().Id("__sovaSessionRegistry").Dot("mu").Dot("RUnlock").Call(),
		jen.Id("out").Op(":=").Make(jen.Index().Op("*").Id("fn____Session"), jen.Lit(0), jen.Qual("", "len").Call(jen.Id("__sovaSessionRegistry").Dot("m"))),
		jen.For(jen.Id("_").Op(",").Id("s").Op(":=").Range().Id("__sovaSessionRegistry").Dot("m")).Block(
			jen.Id("out").Op("=").Append(jen.Id("out"), jen.Id("s")),
		),
		jen.Return(jen.Id("out")),
	))

	block.Add(jen.Func().Id("__sovaLoadSession").Params(jen.Id("r").Op("*").Qual("net/http", "Request")).Op("*").Id("fn____Session").Block(
		jen.If(jen.Id("__sovaTestBypassAuth").Call()).Block(
			jen.Return(jen.Id("__sovaTestEnsureBypassSession").Call()),
		),
		jen.List(jen.Id("c"), jen.Id("err")).Op(":=").Id("r").Dot("Cookie").Call(jen.Lit("sova_session")),
		jen.If(jen.Id("err").Op("==").Nil()).Block(
			jen.List(jen.Id("id"), jen.Id("ok")).Op(":=").Id("__sovaVerifySessionId").Call(jen.Id("c").Dot("Value")),
			jen.If(jen.Id("ok")).Block(
				jen.If(jen.Id("existing").Op(":=").Id("__sovaSessionGet").Call(jen.Id("id")), jen.Id("existing").Op("!=").Nil()).Block(
					jen.Return(jen.Id("existing")),
				),
				jen.Return(jen.Op("&").Id("fn____Session").Values(jen.Dict{jen.Id("Id"): jen.Id("id")})),
			),
		),
		jen.Return(jen.Op("&").Id("fn____Session").Values()),
	))

	block.Add(jen.Var().Id("__sovaTestBypassSession").Op("*").Id("fn____Session"))
	block.Add(jen.Var().Id("__sovaTestBypassSessionMu").Qual("sync", "Mutex"))

	block.Add(jen.Func().Id("__sovaTestBypassAuth").Params().Bool().Block(
		jen.Return(jen.Qual("os", "Getenv").Call(jen.Lit("SOVA_TEST_BYPASS_AUTH")).Op("!=").Lit("")),
	))

	block.Add(jen.Func().Id("__sovaTestEnsureBypassSession").Params().Op("*").Id("fn____Session").Block(
		jen.Id("__sovaTestBypassSessionMu").Dot("Lock").Call(),
		jen.Defer().Id("__sovaTestBypassSessionMu").Dot("Unlock").Call(),
		jen.If(jen.Id("__sovaTestBypassSession").Op("!=").Nil()).Block(jen.Return(jen.Id("__sovaTestBypassSession"))),
		jen.Id("s").Op(":=").Op("&").Id("fn____Session").Values(jen.Dict{
			jen.Id("Id"):          jen.Lit("test-bypass"),
			jen.Id("IsConnected"): jen.True(),
			jen.Id("Auth"):        jen.True(),
			jen.Id("ConnectedAt"): jen.Qual("time", "Now").Call().Dot("Unix").Call(),
		}),
		jen.Id("__sovaSessionPut").Call(jen.Id("s")),
		jen.Id("__sovaTestBypassSession").Op("=").Id("s"),
		jen.Return(jen.Id("s")),
	))

	block.Add(jen.Func().Id("__sovaSaveSession").Params(
		jen.Id("w").Qual("net/http", "ResponseWriter"),
		jen.Id("s").Op("*").Id("fn____Session"),
	).Block(
		jen.If(jen.Id("s").Op("==").Nil()).Block(jen.Return()),
		jen.If(jen.Id("s").Dot("Auth").Op("&&").Id("s").Dot("Id").Op("==").Lit("")).Block(
			jen.Id("s").Dot("Id").Op("=").Id("__sovaNewSessionId").Call(),
		),
		jen.If(jen.Id("s").Dot("Id").Op("!=").Lit("")).Block(
			jen.If(jen.Id("s").Dot("ConnectedAt").Op("==").Lit(0)).Block(
				jen.Id("s").Dot("ConnectedAt").Op("=").Qual("time", "Now").Call().Dot("Unix").Call(),
			),
			jen.Id("__sovaSessionPut").Call(jen.Id("s")),
			jen.Qual("net/http", "SetCookie").Call(jen.Id("w"), jen.Op("&").Qual("net/http", "Cookie").Values(jen.Dict{
				jen.Id("Name"):     jen.Lit("sova_session"),
				jen.Id("Value"):    jen.Id("__sovaSignSessionId").Call(jen.Id("s").Dot("Id")),
				jen.Id("Path"):     jen.Lit("/"),
				jen.Id("HttpOnly"): jen.True(),
				jen.Id("SameSite"): jen.Qual("net/http", "SameSiteLaxMode"),
			})),
			jen.Return(),
		),
		jen.If(jen.Op("!").Id("s").Dot("Auth")).Block(
			jen.Qual("net/http", "SetCookie").Call(jen.Id("w"), jen.Op("&").Qual("net/http", "Cookie").Values(jen.Dict{
				jen.Id("Name"):   jen.Lit("sova_session"),
				jen.Id("Value"):  jen.Lit(""),
				jen.Id("Path"):   jen.Lit("/"),
				jen.Id("MaxAge"): jen.Lit(-1),
			})),
		),
	))

	emitWSTransport(ctx, block)
}

// emitWSTransport emits the WebSocket transport piece used when the build needs the session manager: a per-connection wrapper around gorilla/websocket.Conn, an outbox channel for server-pushed messages, read and write loops, and the `/__sova/ws` HTTP handler that upgrades incoming HTTP requests after validating the HMAC-signed session cookie. The frontend client (Phase 6) opens this endpoint after the page boots; backend-pushed RPC messages (Phase 5) are written through the per-connection outbox.
func emitWSTransport(ctx *codegen.EmitContext, block *jen.Group) {
	block.Add(jen.Type().Id("fn____WSConn").Struct(
		jen.Id("conn").Op("*").Qual("github.com/gorilla/websocket", "Conn"),
		jen.Id("session").Op("*").Id("fn____Session"),
		jen.Id("outbox").Chan().Index().Byte(),
		jen.Id("done").Chan().Struct(),
		jen.Id("closeOnce").Qual("sync", "Once"),
	))

	block.Add(jen.Var().Id("__sovaWSConns").Op("=").Struct(
		jen.Id("mu").Qual("sync", "RWMutex"),
		jen.Id("bySession").Map(jen.String()).Index().Op("*").Id("fn____WSConn"),
	).Values(jen.Dict{
		jen.Id("bySession"): jen.Map(jen.String()).Index().Op("*").Id("fn____WSConn").Values(),
	}))

	block.Add(jen.Var().Id("__sovaUpgrader").Op("=").Qual("github.com/gorilla/websocket", "Upgrader").Values(jen.Dict{
		jen.Id("ReadBufferSize"):  jen.Lit(1024),
		jen.Id("WriteBufferSize"): jen.Lit(1024),
		jen.Id("CheckOrigin"): jen.Func().Params(jen.Id("r").Op("*").Qual("net/http", "Request")).Bool().Block(
			jen.Return(jen.True()),
		),
	}))

	block.Add(jen.Type().Id("fn____WSEnvelope").Struct(
		jen.Id("Op").String().Tag(map[string]string{"json": "op"}),
		jen.Id("Id").String().Tag(map[string]string{"json": "id,omitempty"}),
		jen.Id("Fn").String().Tag(map[string]string{"json": "fn,omitempty"}),
		jen.Id("Args").Qual("encoding/json", "RawMessage").Tag(map[string]string{"json": "args,omitempty"}),
		jen.Id("Value").Qual("encoding/json", "RawMessage").Tag(map[string]string{"json": "value,omitempty"}),
		jen.Id("Error").String().Tag(map[string]string{"json": "error,omitempty"}),
	))

	block.Add(jen.Func().Params(jen.Id("c").Op("*").Id("fn____WSConn")).Id("close").Params().Block(
		jen.Id("c").Dot("closeOnce").Dot("Do").Call(jen.Func().Params().Block(
			jen.Qual("", "close").Call(jen.Id("c").Dot("done")),
			jen.Qual("", "close").Call(jen.Id("c").Dot("outbox")),
			jen.Id("_").Op("=").Id("c").Dot("conn").Dot("Close").Call(),
			jen.If(jen.Id("c").Dot("session").Op("!=").Nil()).Block(
				jen.Id("__sovaWSConnsRemove").Call(jen.Id("c").Dot("session").Dot("Id"), jen.Id("c")),
				jen.Id("remaining").Op(":=").Id("__sovaWSConnsFor").Call(jen.Id("c").Dot("session").Dot("Id")),
				jen.If(jen.Qual("", "len").Call(jen.Id("remaining")).Op("==").Lit(0)).Block(
					jen.Id("c").Dot("session").Dot("IsConnected").Op("=").False(),
					jen.Go().Id("__sovaScheduleGracePurge").Call(jen.Id("c").Dot("session").Dot("Id")),
				),
			),
		)),
	))

	block.Add(jen.Func().Id("__sovaWSConnsAdd").Params(jen.Id("id").String(), jen.Id("c").Op("*").Id("fn____WSConn")).Block(
		jen.Id("__sovaWSConns").Dot("mu").Dot("Lock").Call(),
		jen.Defer().Id("__sovaWSConns").Dot("mu").Dot("Unlock").Call(),
		jen.Id("__sovaWSConns").Dot("bySession").Index(jen.Id("id")).Op("=").Append(
			jen.Id("__sovaWSConns").Dot("bySession").Index(jen.Id("id")),
			jen.Id("c"),
		),
	))

	block.Add(jen.Func().Id("__sovaWSConnsRemove").Params(jen.Id("id").String(), jen.Id("c").Op("*").Id("fn____WSConn")).Block(
		jen.Id("__sovaWSConns").Dot("mu").Dot("Lock").Call(),
		jen.Defer().Id("__sovaWSConns").Dot("mu").Dot("Unlock").Call(),
		jen.Id("existing").Op(":=").Id("__sovaWSConns").Dot("bySession").Index(jen.Id("id")),
		jen.Id("out").Op(":=").Id("existing").Index(jen.Empty(), jen.Lit(0)),
		jen.For(jen.Id("_").Op(",").Id("x").Op(":=").Range().Id("existing")).Block(
			jen.If(jen.Id("x").Op("!=").Id("c")).Block(
				jen.Id("out").Op("=").Append(jen.Id("out"), jen.Id("x")),
			),
		),
		jen.If(jen.Qual("", "len").Call(jen.Id("out")).Op("==").Lit(0)).Block(
			jen.Qual("", "delete").Call(jen.Id("__sovaWSConns").Dot("bySession"), jen.Id("id")),
		).Else().Block(
			jen.Id("__sovaWSConns").Dot("bySession").Index(jen.Id("id")).Op("=").Id("out"),
		),
	))

	block.Add(jen.Func().Id("__sovaWSConnsFor").Params(jen.Id("id").String()).Index().Op("*").Id("fn____WSConn").Block(
		jen.Id("__sovaWSConns").Dot("mu").Dot("RLock").Call(),
		jen.Defer().Id("__sovaWSConns").Dot("mu").Dot("RUnlock").Call(),
		jen.Id("src").Op(":=").Id("__sovaWSConns").Dot("bySession").Index(jen.Id("id")),
		jen.Id("out").Op(":=").Make(jen.Index().Op("*").Id("fn____WSConn"), jen.Qual("", "len").Call(jen.Id("src"))),
		jen.Qual("", "copy").Call(jen.Id("out"), jen.Id("src")),
		jen.Return(jen.Id("out")),
	))

	block.Add(jen.Var().Id("__sovaWSBackendHandlers").Op("=").Map(jen.String()).Func().Params(jen.Op("*").Id("fn____Session"), jen.Qual("encoding/json", "RawMessage")).Params(jen.Any(), jen.Id("WireState")).Values())

	block.Add(jen.Func().Id("__sovaWSRegisterBackendHandler").Params(
		jen.Id("name").String(),
		jen.Id("h").Func().Params(jen.Op("*").Id("fn____Session"), jen.Qual("encoding/json", "RawMessage")).Params(jen.Any(), jen.Id("WireState")),
	).Block(
		jen.Id("__sovaWSBackendHandlers").Index(jen.Id("name")).Op("=").Id("h"),
	))

	block.Add(jen.Func().Id("__sovaWSDispatch").Params(jen.Id("c").Op("*").Id("fn____WSConn"), jen.Id("env").Op("*").Id("fn____WSEnvelope")).Block(
		jen.Switch(jen.Id("env").Dot("Op")).Block(
			jen.Case(jen.Lit("reply")).Block(
				jen.Id("__sovaWSDeliverReply").Call(jen.Id("env")),
			),
			jen.Case(jen.Lit("call")).Block(
				jen.List(jen.Id("h"), jen.Id("ok")).Op(":=").Id("__sovaWSBackendHandlers").Index(jen.Id("env").Dot("Fn")),
				jen.If(jen.Op("!").Id("ok")).Block(jen.Return()),
				jen.Go().Func().Params().Block(
					jen.List(jen.Id("val"), jen.Id("state")).Op(":=").Id("h").Call(jen.Id("c").Dot("session"), jen.Id("env").Dot("Args")),
					jen.List(jen.Id("raw"), jen.Id("_")).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Map(jen.String()).Any().Values(jen.Dict{
						jen.Lit("value"): jen.Id("val"),
						jen.Lit("state"): jen.Int64().Call(jen.Id("state")),
					})),
					jen.List(jen.Id("payload"), jen.Id("_")).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Op("&").Id("fn____WSEnvelope").Values(jen.Dict{
						jen.Id("Op"):    jen.Lit("reply"),
						jen.Id("Id"):    jen.Id("env").Dot("Id"),
						jen.Id("Value"): jen.Qual("encoding/json", "RawMessage").Parens(jen.Id("raw")),
					})),
					jen.Select().Block(
						jen.Case(jen.Id("c").Dot("outbox").Op("<-").Id("payload")).Block(),
						jen.Default().Block(),
					),
				).Call(),
			),
			jen.Default().Block(),
		),
	))

	block.Add(jen.Var().Id("__sovaWSPending").Op("=").Struct(
		jen.Id("mu").Qual("sync", "Mutex"),
		jen.Id("m").Map(jen.String()).Chan().Op("*").Id("fn____WSEnvelope"),
	).Values(jen.Dict{
		jen.Id("m"): jen.Map(jen.String()).Chan().Op("*").Id("fn____WSEnvelope").Values(),
	}))

	block.Add(jen.Func().Id("__sovaWSRegisterReply").Params(jen.Id("id").String()).Chan().Op("*").Id("fn____WSEnvelope").Block(
		jen.Id("ch").Op(":=").Make(jen.Chan().Op("*").Id("fn____WSEnvelope"), jen.Lit(1)),
		jen.Id("__sovaWSPending").Dot("mu").Dot("Lock").Call(),
		jen.Id("__sovaWSPending").Dot("m").Index(jen.Id("id")).Op("=").Id("ch"),
		jen.Id("__sovaWSPending").Dot("mu").Dot("Unlock").Call(),
		jen.Return(jen.Id("ch")),
	))

	block.Add(jen.Func().Id("__sovaWSDeliverReply").Params(jen.Id("env").Op("*").Id("fn____WSEnvelope")).Block(
		jen.Id("__sovaWSPending").Dot("mu").Dot("Lock").Call(),
		jen.Id("ch").Op(":=").Id("__sovaWSPending").Dot("m").Index(jen.Id("env").Dot("Id")),
		jen.Qual("", "delete").Call(jen.Id("__sovaWSPending").Dot("m"), jen.Id("env").Dot("Id")),
		jen.Id("__sovaWSPending").Dot("mu").Dot("Unlock").Call(),
		jen.If(jen.Id("ch").Op("!=").Nil()).Block(
			jen.Id("ch").Op("<-").Id("env"),
			jen.Qual("", "close").Call(jen.Id("ch")),
		),
	))

	block.Add(jen.Func().Id("__sovaWSReadLoop").Params(jen.Id("c").Op("*").Id("fn____WSConn")).Block(
		jen.Defer().Id("c").Dot("close").Call(),
		jen.For().Block(
			jen.List(jen.Id("_"), jen.Id("data"), jen.Id("err")).Op(":=").Id("c").Dot("conn").Dot("ReadMessage").Call(),
			jen.If(jen.Id("err").Op("!=").Nil()).Block(jen.Return()),
			jen.Var().Id("env").Id("fn____WSEnvelope"),
			jen.If(jen.Qual("encoding/json", "Unmarshal").Call(jen.Id("data"), jen.Op("&").Id("env")).Op("!=").Nil()).Block(
				jen.Continue(),
			),
			jen.Id("__sovaWSDispatch").Call(jen.Id("c"), jen.Op("&").Id("env")),
		),
	))

	block.Add(jen.Func().Id("__sovaWSWriteLoop").Params(jen.Id("c").Op("*").Id("fn____WSConn")).Block(
		jen.For().Block(
			jen.Select().Block(
				jen.Case(jen.Op("<-").Id("c").Dot("done")).Block(jen.Return()),
				jen.Case(jen.List(jen.Id("msg"), jen.Id("ok")).Op(":=").Op("<-").Id("c").Dot("outbox")).Block(
					jen.If(jen.Op("!").Id("ok")).Block(jen.Return()),
					jen.If(jen.Id("err").Op(":=").Id("c").Dot("conn").Dot("WriteMessage").Call(
						jen.Qual("github.com/gorilla/websocket", "TextMessage"),
						jen.Id("msg"),
					), jen.Id("err").Op("!=").Nil()).Block(jen.Return()),
				),
			),
		),
	))

	block.Add(jen.Func().Id("__sovaWSHandler").Params(
		jen.Id("w").Qual("net/http", "ResponseWriter"),
		jen.Id("r").Op("*").Qual("net/http", "Request"),
	).Block(
		jen.Var().Id("sess").Op("*").Id("fn____Session"),
		jen.Var().Id("sid").String(),
		// Hoisted out of the bypass / cookie branches so the gorilla
		// Upgrade call can see whatever Set-Cookie we staged. Stays nil
		// when no cookie has to be issued (existing session is reused).
		jen.Var().Id("__wsRespHdr").Qual("net/http", "Header"),
		jen.If(jen.Id("__sovaTestBypassAuth").Call()).Block(
			jen.Id("sess").Op("=").Id("__sovaTestEnsureBypassSession").Call(),
			jen.Id("sid").Op("=").Id("sess").Dot("Id"),
		).Else().Block(
			// Best-effort: read an existing `sova_session` cookie. We only
			// trust the cookie when its HMAC verifies; anything else is
			// treated as "no session yet" and a fresh one is bootstrapped.
			// This is how a brand-new visitor lands on the page: we can't
			// reject them with 401 because the browser then surfaces a
			// useless "WebSocket failed" with no further detail (the
			// HTTP/101 path eats the body).
			jen.List(jen.Id("cookie"), jen.Id("err")).Op(":=").Id("r").Dot("Cookie").Call(jen.Lit("sova_session")),
			jen.If(jen.Id("err").Op("==").Nil()).Block(
				jen.List(jen.Id("verifiedSid"), jen.Id("ok")).Op(":=").Id("__sovaVerifySessionId").Call(jen.Id("cookie").Dot("Value")),
				jen.If(jen.Id("ok")).Block(
					jen.Id("sid").Op("=").Id("verifiedSid"),
					jen.Id("sess").Op("=").Id("__sovaSessionGet").Call(jen.Id("sid")),
				),
			),
			// First-time visitor or expired record: synthesise an
			// anonymous session, register it, and stage a Set-Cookie for
			// the 101 response. gorilla/websocket's Upgrade builds the
			// switching-protocols response by hand and ignores anything
			// already written via `w.Header()`, so we have to thread the
			// cookie through its `responseHeader` parameter instead.
			jen.If(jen.Id("sess").Op("==").Nil()).Block(
				jen.Id("sid").Op("=").Id("__sovaNewSessionId").Call(),
				jen.Id("sess").Op("=").Op("&").Id("fn____Session").Values(jen.Dict{
					jen.Id("Id"):          jen.Id("sid"),
					jen.Id("ConnectedAt"): jen.Qual("time", "Now").Call().Dot("Unix").Call(),
				}),
				jen.Id("__sovaSessionPut").Call(jen.Id("sess")),
				jen.Id("__wsRespHdr").Op("=").Qual("net/http", "Header").Values(),
				jen.Id("__wsRespHdr").Dot("Add").Call(jen.Lit("Set-Cookie"), jen.Parens(jen.Op("&").Qual("net/http", "Cookie").Values(jen.Dict{
					jen.Id("Name"):     jen.Lit("sova_session"),
					jen.Id("Value"):    jen.Id("__sovaSignSessionId").Call(jen.Id("sid")),
					jen.Id("Path"):     jen.Lit("/"),
					jen.Id("HttpOnly"): jen.True(),
					jen.Id("SameSite"): jen.Qual("net/http", "SameSiteLaxMode"),
				})).Dot("String").Call()),
			),
		),
		jen.List(jen.Id("conn"), jen.Id("upgErr")).Op(":=").Id("__sovaUpgrader").Dot("Upgrade").Call(jen.Id("w"), jen.Id("r"), jen.Id("__wsRespHdr")),
		jen.If(jen.Id("upgErr").Op("!=").Nil()).Block(jen.Return()),
		jen.Id("wsc").Op(":=").Op("&").Id("fn____WSConn").Values(jen.Dict{
			jen.Id("conn"):    jen.Id("conn"),
			jen.Id("session"): jen.Id("sess"),
			jen.Id("outbox"):  jen.Make(jen.Chan().Index().Byte(), jen.Lit(32)),
			jen.Id("done"):    jen.Make(jen.Chan().Struct()),
		}),
		jen.Id("wasConnected").Op(":=").Id("sess").Dot("IsConnected"),
		jen.Id("sess").Dot("IsConnected").Op("=").True(),
		jen.If(jen.Id("sess").Dot("ConnectedAt").Op("==").Lit(0)).Block(
			jen.Id("sess").Dot("ConnectedAt").Op("=").Qual("time", "Now").Call().Dot("Unix").Call(),
		),
		jen.Id("__sovaWSConnsAdd").Call(jen.Id("sid"), jen.Id("wsc")),
		jen.If(jen.Op("!").Id("wasConnected")).Block(
			jen.Id("__sovaFireOnConnect").Call(jen.Id("sess")),
			jen.Go().Id("__sovaSessionFlush").Call(jen.Id("sess")),
		),
		jen.Go().Id("__sovaWSWriteLoop").Call(jen.Id("wsc")),
		jen.Id("__sovaWSReadLoop").Call(jen.Id("wsc")),
	))

	block.Add(jen.Func().Id("__sovaScheduleGracePurge").Params(jen.Id("sid").String()).Block(
		jen.Id("grace").Op(":=").Id("__sovaSessionGraceSeconds").Call(),
		jen.If(jen.Id("__sovaTestHarnessActive").Call()).Block(
			jen.Id("__sovaTestRegisterGracePurge").Call(jen.Id("sid"), jen.Id("grace")),
			jen.Return(),
		),
		jen.Qual("time", "Sleep").Call(jen.Qual("time", "Duration").Call(jen.Id("grace")).Op("*").Qual("time", "Second")),
		jen.Id("__sovaRunGracePurge").Call(jen.Id("sid")),
	))

	block.Add(jen.Func().Id("__sovaRunGracePurge").Params(jen.Id("sid").String()).Block(
		jen.Id("sess").Op(":=").Id("__sovaSessionGet").Call(jen.Id("sid")),
		jen.If(jen.Id("sess").Op("==").Nil().Op("||").Id("sess").Dot("IsConnected")).Block(jen.Return()),
		jen.Id("__sovaSessionDelete").Call(jen.Id("sid")),
		jen.Id("__sovaFireOnDisconnect").Call(jen.Id("sess")),
	))

	graceSeconds := manifestGraceSeconds(ctx)
	if graceSeconds <= 0 {
		graceSeconds = 5
	}
	block.Add(jen.Func().Id("__sovaSessionGraceSeconds").Params().Int().Block(
		jen.If(jen.Id("v").Op(":=").Qual("os", "Getenv").Call(jen.Lit("WIRE_SESSION_GRACE")), jen.Id("v").Op("!=").Lit("")).Block(
			jen.If(jen.List(jen.Id("n"), jen.Id("err")).Op(":=").Qual("strconv", "Atoi").Call(jen.Id("v")), jen.Id("err").Op("==").Nil().Op("&&").Id("n").Op(">").Lit(0)).Block(
				jen.Return(jen.Id("n")),
			),
		),
		jen.Return(jen.Lit(graceSeconds)),
	))

	block.Add(jen.Func().Id("__sovaSessionEnqueue").Params(
		jen.Id("s").Op("*").Id("fn____Session"),
		jen.Id("wireName").String(),
		jen.Id("env").Op("*").Id("fn____WSEnvelope"),
		jen.Id("limit").Int(),
	).Block(
		jen.If(jen.Id("s").Op("==").Nil().Op("||").Id("limit").Op("<=").Lit(0)).Block(jen.Return()),
		jen.List(jen.Id("payload"), jen.Id("err")).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Id("env")),
		jen.If(jen.Id("err").Op("!=").Nil()).Block(jen.Return()),
		jen.Id("__sovaSessionRegistry").Dot("mu").Dot("Lock").Call(),
		jen.Defer().Id("__sovaSessionRegistry").Dot("mu").Dot("Unlock").Call(),
		jen.If(jen.Id("s").Dot("Pending").Op("==").Nil()).Block(
			jen.Id("s").Dot("Pending").Op("=").Map(jen.String()).Index().Index().Byte().Values(),
		),
		jen.Id("q").Op(":=").Append(jen.Id("s").Dot("Pending").Index(jen.Id("wireName")), jen.Id("payload")),
		jen.If(jen.Qual("", "len").Call(jen.Id("q")).Op(">").Id("limit")).Block(
			jen.Id("q").Op("=").Id("q").Index(jen.Qual("", "len").Call(jen.Id("q")).Op("-").Id("limit"), jen.Empty()),
		),
		jen.Id("s").Dot("Pending").Index(jen.Id("wireName")).Op("=").Id("q"),
	))

	block.Add(jen.Func().Id("__sovaSessionFlush").Params(jen.Id("s").Op("*").Id("fn____Session")).Block(
		jen.If(jen.Id("s").Op("==").Nil()).Block(jen.Return()),
		jen.Id("__sovaSessionRegistry").Dot("mu").Dot("Lock").Call(),
		jen.Id("pending").Op(":=").Id("s").Dot("Pending"),
		jen.Id("s").Dot("Pending").Op("=").Nil(),
		jen.Id("__sovaSessionRegistry").Dot("mu").Dot("Unlock").Call(),
		jen.For(jen.Id("_").Op(",").Id("queue").Op(":=").Range().Id("pending")).Block(
			jen.For(jen.Id("_").Op(",").Id("msg").Op(":=").Range().Id("queue")).Block(
				jen.Id("conns").Op(":=").Id("__sovaWSConnsFor").Call(jen.Id("s").Dot("Id")),
				jen.For(jen.Id("_").Op(",").Id("c").Op(":=").Range().Id("conns")).Block(
					jen.Select().Block(
						jen.Case(jen.Id("c").Dot("outbox").Op("<-").Id("msg")).Block(),
						jen.Default().Block(),
					),
				),
			),
		),
	))

	block.Add(jen.Func().Id("__sovaWSSendTo").Params(jen.Id("sessionId").String(), jen.Id("env").Op("*").Id("fn____WSEnvelope")).Bool().Block(
		jen.List(jen.Id("payload"), jen.Id("err")).Op(":=").Qual("encoding/json", "Marshal").Call(jen.Id("env")),
		jen.If(jen.Id("err").Op("!=").Nil()).Block(jen.Return(jen.False())),
		jen.Id("conns").Op(":=").Id("__sovaWSConnsFor").Call(jen.Id("sessionId")),
		jen.If(jen.Qual("", "len").Call(jen.Id("conns")).Op("==").Lit(0)).Block(jen.Return(jen.False())),
		jen.For(jen.Id("_").Op(",").Id("c").Op(":=").Range().Id("conns")).Block(
			jen.Select().Block(
				jen.Case(jen.Id("c").Dot("outbox").Op("<-").Id("payload")).Block(),
				jen.Default().Block(),
			),
		),
		jen.Return(jen.True()),
	))
}

// bufferLimitForWire reads `wire(buffer: ...)` off a frontend wire-func declaration. Returns (limit, enabled). `wire(buffer: true)` defaults to 100; `wire(buffer: N)` uses N; unset or `wire(buffer: false)` disables buffering.
func bufferLimitForWire(fn *ir.FuncDeclStmt) (int, bool) {
	if fn == nil || fn.Wire == nil || fn.Wire.Options == nil {
		return 0, false
	}
	v, ok := fn.Wire.Options["buffer"]
	if !ok {
		return 0, false
	}
	switch v.Kind {
	case ir.WireOptBool:
		if v.Bool {
			return 100, true
		}
		return 0, false
	case ir.WireOptInt:
		if v.Int <= 0 {
			return 0, false
		}
		return int(v.Int), true
	}
	return 0, false
}

// reactiveWireVarOriginalName returns the original (pre-mangle) name of the reactive wire variable identified by `sym`, or "" if the symbol does not refer to a `@reactive wire let` declaration. Used by assignment codegen to gate the broadcast-on-mutate plumbing on a per-symbol basis.
func reactiveWireVarOriginalName(ctx *codegen.EmitContext, sym ir.SymID) string {
	if ctx == nil || ctx.Cache == nil || sym == 0 {
		return ""
	}
	raw, ok := ctx.Cache["reactive_wire_vars"]
	if !ok {
		return ""
	}
	vars, ok := raw.([]*ir.VarDeclStmt)
	if !ok {
		return ""
	}
	for _, vd := range vars {
		if len(vd.Targets) == 0 || vd.Targets[0].Name == nil {
			continue
		}
		if vd.Targets[0].Name.Sym == sym {
			return vd.Targets[0].Name.Name
		}
	}
	return ""
}

// manifestGraceSeconds returns the configured reconnect grace window in seconds (default 5 when unset).
func manifestGraceSeconds(ctx *codegen.EmitContext) int {
	if ctx.Cache == nil {
		return 0
	}
	raw, ok := ctx.Cache["build_config"]
	if !ok {
		return 0
	}
	if cfg, ok := raw.(interface{ WireSessionGraceSecondsValue() int }); ok {
		return cfg.WireSessionGraceSecondsValue()
	}
	return 0
}

// manifestSessionSecret extracts the manifest-configured HMAC secret out of the BuildConfig cache. Empty when the user has not set [wire].session_secret; codegen then falls back to env or a dev placeholder.
func manifestSessionSecret(ctx *codegen.EmitContext) string {
	if ctx.Cache == nil {
		return ""
	}
	raw, ok := ctx.Cache["build_config"]
	if !ok {
		return ""
	}
	if cfg, ok := raw.(interface{ WireSessionSecretValue() string }); ok {
		return cfg.WireSessionSecretValue()
	}
	return ""
}

// emitDevOnlyBoot generates a minimal HTTP server boot that only fires when SOVA_DEV=1. Used when the project has no wired endpoints but the developer still wants `sova dev` to serve the frontend with live reload.
func (e *CodeEmitter) emitDevOnlyBoot(ctx *codegen.EmitContext, g *jen.Group) {
	g.If(jen.Qual("os", "Getenv").Call(jen.Lit("SOVA_DEV")).Op("!=").Lit("1")).Block(
		jen.Return(),
	)
	g.Id("__mux").Op(":=").Qual("net/http", "NewServeMux").Call()
	defaultPort := wireConfiguredPort(ctx)
	defaultHost := wireConfiguredHost(ctx)
	g.Id("__port").Op(":=").Qual("os", "Getenv").Call(jen.Lit("WIRE_PORT"))
	g.If(jen.Id("__port").Op("==").Lit("")).Block(
		jen.Id("__port").Op("=").Lit(defaultPort),
	)
	g.Id("__host").Op(":=").Qual("os", "Getenv").Call(jen.Lit("WIRE_HOST"))
	g.If(jen.Id("__host").Op("==").Lit("")).Block(
		jen.Id("__host").Op("=").Lit(defaultHost),
	)
	g.Id("__addr").Op(":=").Id("__host").Op("+").Lit(":").Op("+").Id("__port")
	g.Id("__sovaDevServeMaybe").Call(jen.Id("__mux"))
	g.Qual("log", "Printf").Call(jen.Lit("sova dev server listening on %s"), jen.Id("__addr"))
	g.Qual("log", "Fatal").Call(jen.Qual("net/http", "ListenAndServe").Call(jen.Id("__addr"), jen.Id("__mux")))
}

// emitWireServerBoot generates the HTTP server boot sequence for wired endpoints: handler registration, listen address resolution (env > manifest > default), and ListenAndServe blocking call.
func (e *CodeEmitter) emitWireServerBoot(ctx *codegen.EmitContext, g *jen.Group) {
	g.Id("__mux").Op(":=").Qual("net/http", "NewServeMux").Call()
	for _, fn := range e.wiredFuncs {
		fnRef := fn
		handlerName := symName(ctx, fnRef.Name.Sym) + "__wireHandler"
		pattern := fnRef.Wire.Method + " " + pathWithBraces(fnRef.Wire.Path)
		g.Id("__mux").Dot("HandleFunc").Call(jen.Lit(pattern), jen.Id(handlerName))
		if fnRef.Wire.Transport == "ws" && needsSessionManagerFromCache(ctx) {
			g.Id("__sovaWSRegisterBackendHandler").Call(jen.Lit(fnRef.Name.Name), jen.Id(handlerName+"__ws"))
		}
	}
	for _, vd := range e.wiredVars {
		if len(vd.Targets) == 0 || vd.Targets[0].Name == nil {
			continue
		}
		handlerName := symName(ctx, vd.Targets[0].Name.Sym) + "__wireHandler"
		pattern := vd.Wire.Method + " " + pathWithBraces(vd.Wire.Path)
		g.Id("__mux").Dot("HandleFunc").Call(jen.Lit(pattern), jen.Id(handlerName))
	}
	if needsSessionManagerFromCache(ctx) {
		g.Id("__mux").Dot("HandleFunc").Call(jen.Lit("/__sova/ws"), jen.Id("__sovaWSHandler"))
	}
	defaultPort := wireConfiguredPort(ctx)
	defaultHost := wireConfiguredHost(ctx)
	g.Id("__port").Op(":=").Qual("os", "Getenv").Call(jen.Lit("WIRE_PORT"))
	g.If(jen.Id("__port").Op("==").Lit("")).Block(
		jen.Id("__port").Op("=").Lit(defaultPort),
	)
	g.Id("__host").Op(":=").Qual("os", "Getenv").Call(jen.Lit("WIRE_HOST"))
	g.If(jen.Id("__host").Op("==").Lit("")).Block(
		jen.Id("__host").Op("=").Lit(defaultHost),
	)
	g.Id("__addr").Op(":=").Id("__host").Op("+").Lit(":").Op("+").Id("__port")
	g.Id("__sovaDevServeMaybe").Call(jen.Id("__mux"))
	g.Qual("log", "Printf").Call(jen.Lit("sova wire server listening on %s"), jen.Id("__addr"))
	g.Qual("log", "Fatal").Call(jen.Qual("net/http", "ListenAndServe").Call(jen.Id("__addr"), jen.Id("__mux")))
}

// wireConfiguredPort fishes the manifest-configured wire port out of the BuildConfig cache, falling back to "8080".
func wireConfiguredPort(ctx *codegen.EmitContext) string {
	if ctx.Cache == nil {
		return "8080"
	}
	raw, ok := ctx.Cache["build_config"]
	if !ok {
		return "8080"
	}
	if cfg, ok := raw.(interface{ WirePortValue() int }); ok {
		port := cfg.WirePortValue()
		if port > 0 {
			return strconv.Itoa(port)
		}
	}
	return "8080"
}

// wireConfiguredHost returns the manifest-configured wire host, or empty string (listen on all interfaces).
func wireConfiguredHost(ctx *codegen.EmitContext) string {
	if ctx.Cache == nil {
		return ""
	}
	raw, ok := ctx.Cache["build_config"]
	if !ok {
		return ""
	}
	if cfg, ok := raw.(interface{ WireHostValue() string }); ok {
		return cfg.WireHostValue()
	}
	return ""
}

// sessionFieldNameToGo maps a Sova-side session field name (lowercase) to the matching Go-side struct field name (Pascal-case + JSON-tag-compatible). Unrecognised fields fall back to capitalizing the first letter.
func sessionFieldNameToGo(name string) string {
	switch name {
	case "user":
		return "User"
	case "roles":
		return "Roles"
	}
	if name == "" {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// pathWithBraces converts Sova-style `:name` path placeholders to Go's net/http 1.22+ `{name}` placeholder syntax.
func pathWithBraces(p string) string {
	out := strings.Builder{}
	i := 0
	for i < len(p) {
		if p[i] == ':' {
			out.WriteByte('{')
			j := i + 1
			for j < len(p) && (p[j] == '_' || (p[j] >= 'a' && p[j] <= 'z') || (p[j] >= 'A' && p[j] <= 'Z') || (p[j] >= '0' && p[j] <= '9')) {
				j++
			}
			out.WriteString(p[i+1 : j])
			out.WriteByte('}')
			i = j
			continue
		}
		out.WriteByte(p[i])
		i++
	}
	return out.String()
}

// emitWiredVarGetter writes a synthetic getter `func <name>() T { return <init> }` for a wired top-level var/const. The init expression is embedded inline; the getter is reachable both from the HTTP handler and from internal backend code. For `@reactive wire let` the emitter switches to a real package-level variable instead so backend mutations update the canonical storage and broadcast naturally.
func (e *CodeEmitter) emitWiredVarGetter(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, block *jen.Group, vd *ir.VarDeclStmt) {
	if len(vd.Targets) == 0 || vd.Targets[0].Name == nil {
		return
	}
	target := vd.Targets[0]
	name := symName(ctx, target.Name.Sym)
	innerTyp := ir.TypID(0)
	if target.TypeAnn != nil {
		innerTyp = target.TypeAnn.Typ
	}
	innerType := typeToGoWithContext(ctx, pkg, ctx.Types, innerTyp)
	if reactiveWireVarOriginalName(ctx, target.Name.Sym) != "" {
		block.Add(jen.Var().Id(name).Add(innerType).Op("=").Add(e.buildExpr(ctx, pkg, f, vd.Init)))
		return
	}
	block.Add(jen.Func().Id(name).Params().Add(innerType).BlockFunc(func(g *jen.Group) {
		g.Return(e.buildExpr(ctx, pkg, f, vd.Init))
	}))
}

// emitWiredVarHandler writes an HTTP handler that returns the current value of a wired top-level var. The handler enforces authn/authz like a wired function, then encodes `{value, state}` JSON.
func (e *CodeEmitter) emitWiredVarHandler(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, block *jen.Group, vd *ir.VarDeclStmt) {
	if len(vd.Targets) == 0 || vd.Targets[0].Name == nil {
		return
	}
	target := vd.Targets[0]
	getterName := symName(ctx, target.Name.Sym)
	handlerName := getterName + "__wireHandler"

	block.Add(jen.Func().Id(handlerName).Params(
		jen.Id("w").Qual("net/http", "ResponseWriter"),
		jen.Id("r").Op("*").Qual("net/http", "Request"),
	).BlockFunc(func(g *jen.Group) {
		g.Id("__session").Op(":=").Id("__sovaLoadSession").Call(jen.Id("r"))
		g.Id("_").Op("=").Id("__session")
		if needsSessionManagerFromCache(ctx) {
			g.Id("__sovaSetCurrentSession").Call(jen.Id("__session"))
			g.Defer().Id("__sovaClearCurrentSession").Call()
		}
		if vd.Wire.RequireAuthN {
			g.If(jen.Op("!").Id("__session").Dot("Auth").Op("&&").Op("!").Id("__sovaTestBypassAuth").Call()).Block(
				jen.Id("w").Dot("Header").Call().Dot("Set").Call(jen.Lit("Content-Type"), jen.Lit("application/json")),
				jen.Id("w").Dot("WriteHeader").Call(jen.Qual("net/http", "StatusUnauthorized")),
				jen.Id("_").Op("=").Qual("encoding/json", "NewEncoder").Call(jen.Id("w")).Dot("Encode").Call(jen.Map(jen.String()).Any().Values(jen.Dict{
					jen.Lit("value"): jen.Nil(),
					jen.Lit("state"): jen.Int64().Call(jen.Id("WireStateUnauthorized")),
				})),
				jen.Return(),
			)
		}
		for _, role := range vd.Wire.RequiredRoles {
			roleLit := role
			g.If(jen.Op("!").Id("__session").Dot("hasRole").Call(jen.Lit(roleLit))).Block(
				jen.Id("w").Dot("Header").Call().Dot("Set").Call(jen.Lit("Content-Type"), jen.Lit("application/json")),
				jen.Id("w").Dot("WriteHeader").Call(jen.Qual("net/http", "StatusForbidden")),
				jen.Id("_").Op("=").Qual("encoding/json", "NewEncoder").Call(jen.Id("w")).Dot("Encode").Call(jen.Map(jen.String()).Any().Values(jen.Dict{
					jen.Lit("value"): jen.Nil(),
					jen.Lit("state"): jen.Int64().Call(jen.Id("WireStateForbidden")),
				})),
				jen.Return(),
			)
		}
		var valueExpr *jen.Statement
		if reactiveWireVarOriginalName(ctx, target.Name.Sym) != "" {
			valueExpr = jen.Id(getterName)
		} else {
			valueExpr = jen.Id(getterName).Call()
		}
		g.Id("__val").Op(":=").Add(valueExpr)
		g.Id("__sovaSaveSession").Call(jen.Id("w"), jen.Id("__session"))
		g.Id("w").Dot("Header").Call().Dot("Set").Call(jen.Lit("Content-Type"), jen.Lit("application/json"))
		g.Id("_").Op("=").Qual("encoding/json", "NewEncoder").Call(jen.Id("w")).Dot("Encode").Call(jen.Map(jen.String()).Any().Values(jen.Dict{
			jen.Lit("value"): jen.Id("__val"),
			jen.Lit("state"): jen.Int64().Call(jen.Id("WireStateOk")),
		}))
	}))
}

// resolveWireMaxBody returns the request-body byte cap for a wired function.
// Reads `wire(maxBody: <bytes>)` when provided; falls back to 1 MiB which
// keeps the default safe for typical JSON payloads without surprising users
// who don't think about request-body limits. A literal `0` opts out of the
// cap entirely - handy for endpoints that accept large uploads where the
// caller wants to validate size in user code.
func resolveWireMaxBody(spec *ir.WireSpec) int64 {
	if spec != nil && spec.Options != nil {
		if opt, ok := spec.Options["maxBody"]; ok && opt.Kind == ir.WireOptInt {
			if opt.Int < 0 {
				return 1 << 20
			}
			return opt.Int
		}
	}
	return 1 << 20
}

// emitWiredHandler writes a Go HTTP handler for a wired function: it decodes path/query/body arguments, calls the implementation, and JSON-encodes the {value, state} response. The implementation function itself is emitted as a regular Go function via the standard FuncDeclStmt path.
func (e *CodeEmitter) emitWiredHandler(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, block *jen.Group, fn *ir.FuncDeclStmt) {
	handlerName := symName(ctx, fn.Name.Sym) + "__wireHandler"
	implName := symName(ctx, fn.Name.Sym)

	pathArgSet := map[string]bool{}
	for _, a := range fn.Wire.PathArgs {
		pathArgSet[a] = true
	}

	method := fn.Wire.Method
	hasBody := method != "GET" && method != "DELETE"

	block.Add(jen.Func().Id(handlerName).Params(
		jen.Id("w").Qual("net/http", "ResponseWriter"),
		jen.Id("r").Op("*").Qual("net/http", "Request"),
	).BlockFunc(func(g *jen.Group) {
		g.Id("__session").Op(":=").Id("__sovaLoadSession").Call(jen.Id("r"))
		g.Id("_").Op("=").Id("__session")
		if needsSessionManagerFromCache(ctx) {
			g.Id("__sovaSetCurrentSession").Call(jen.Id("__session"))
			g.Defer().Id("__sovaClearCurrentSession").Call()
		}

		if fn.Wire.RequireAuthN {
			g.If(jen.Op("!").Id("__session").Dot("Auth").Op("&&").Op("!").Id("__sovaTestBypassAuth").Call()).Block(
				jen.Id("w").Dot("Header").Call().Dot("Set").Call(jen.Lit("Content-Type"), jen.Lit("application/json")),
				jen.Id("w").Dot("WriteHeader").Call(jen.Qual("net/http", "StatusUnauthorized")),
				jen.Id("_").Op("=").Qual("encoding/json", "NewEncoder").Call(jen.Id("w")).Dot("Encode").Call(jen.Map(jen.String()).Any().Values(jen.Dict{
					jen.Lit("value"): jen.Nil(),
					jen.Lit("state"): jen.Int64().Call(jen.Id("WireStateUnauthorized")),
				})),
				jen.Return(),
			)
		}
		for _, role := range fn.Wire.RequiredRoles {
			roleLit := role
			g.If(jen.Op("!").Id("__session").Dot("hasRole").Call(jen.Lit(roleLit))).Block(
				jen.Id("w").Dot("Header").Call().Dot("Set").Call(jen.Lit("Content-Type"), jen.Lit("application/json")),
				jen.Id("w").Dot("WriteHeader").Call(jen.Qual("net/http", "StatusForbidden")),
				jen.Id("_").Op("=").Qual("encoding/json", "NewEncoder").Call(jen.Id("w")).Dot("Encode").Call(jen.Map(jen.String()).Any().Values(jen.Dict{
					jen.Lit("value"): jen.Nil(),
					jen.Lit("state"): jen.Int64().Call(jen.Id("WireStateForbidden")),
				})),
				jen.Return(),
			)
		}

		if hasBody {
			var nonPathParams []*ir.FuncParam
			for _, param := range fn.Params {
				if !pathArgSet[param.Name.Name] {
					nonPathParams = append(nonPathParams, param)
				}
			}
			if len(nonPathParams) > 0 {
				// Cap the body so a giant payload can't OOM the server, and
				// surface any JSON parse error as a clean 400 instead of
				// letting the handler run with silently-zeroed params (the
				// previous behaviour, which made invalid requests
				// indistinguishable from "user forgot a field"). The limit
				// is `wire(maxBody: <bytes>)`-configurable so wires that
				// accept large uploads can raise the ceiling; `0` disables
				// the cap entirely.
				limit := resolveWireMaxBody(fn.Wire)
				if limit > 0 {
					g.If(jen.Id("r").Dot("Body").Op("!=").Nil()).Block(
						jen.Id("r").Dot("Body").Op("=").Qual("net/http", "MaxBytesReader").Call(jen.Id("w"), jen.Id("r").Dot("Body"), jen.Lit(limit)),
					)
				}
				g.Id("__body").Op(":=").Map(jen.String()).Qual("encoding/json", "RawMessage").Values()
				g.If(jen.Id("r").Dot("Body").Op("!=").Nil()).Block(
					jen.If(jen.Id("__decErr").Op(":=").Qual("encoding/json", "NewDecoder").Call(jen.Id("r").Dot("Body")).Dot("Decode").Call(jen.Op("&").Id("__body")).Op(";").Id("__decErr").Op("!=").Nil()).Block(
						jen.Id("__sovaRespondBadRequest").Call(jen.Id("w"), jen.Lit("invalid request body: ").Op("+").Id("__decErr").Dot("Error").Call()),
						jen.Return(),
					),
				)
			}
		}

		var callArgs []jen.Code
		for _, param := range fn.Params {
			paramName := symNameWithUnused(ctx, pkg, param.Name.Sym)
			paramTypeID := ir.TypID(0)
			if param.Type != nil {
				paramTypeID = param.Type.Typ
			}
			if pathArgSet[param.Name.Name] {
				g.Id(paramName).Op(":=").Add(decodeStringToGo(ctx.Types, paramTypeID, jen.Id("r").Dot("PathValue").Call(jen.Lit(param.Name.Name))))
				callArgs = append(callArgs, jen.Id(paramName))
				continue
			}
			if hasBody {
				g.Var().Id(paramName).Add(typeToGoWithContext(ctx, pkg, ctx.Types, paramTypeID))
				g.If(jen.Id("raw").Op(",").Id("ok").Op(":=").Id("__body").Index(jen.Lit(param.Name.Name)).Op(";").Id("ok")).Block(
					jen.If(jen.Id("__pErr").Op(":=").Qual("encoding/json", "Unmarshal").Call(jen.Id("raw"), jen.Op("&").Id(paramName)).Op(";").Id("__pErr").Op("!=").Nil()).Block(
						jen.Id("__sovaRespondBadRequest").Call(jen.Id("w"), jen.Lit("invalid value for '"+param.Name.Name+"': ").Op("+").Id("__pErr").Dot("Error").Call()),
						jen.Return(),
					),
				)
				callArgs = append(callArgs, jen.Id(paramName))
				continue
			}
			g.Id(paramName).Op(":=").Add(decodeStringToGo(ctx.Types, paramTypeID, jen.Id("r").Dot("URL").Dot("Query").Call().Dot("Get").Call(jen.Lit(param.Name.Name))))
			callArgs = append(callArgs, jen.Id(paramName))
		}

		callWithSession := append([]jen.Code{jen.Id("__session")}, callArgs...)
		hasReturn := fn.ReturnType != nil && fn.ReturnType.Typ != 0 && fn.ReturnType.Typ != ctx.Types.TypNone()
		if hasReturn {
			g.Id("__val").Op(":=").Id(implName).Call(callWithSession...)
		} else {
			g.Id(implName).Call(callWithSession...)
		}
		g.Id("__sovaSaveSession").Call(jen.Id("w"), jen.Id("__session"))
		g.Id("w").Dot("Header").Call().Dot("Set").Call(jen.Lit("Content-Type"), jen.Lit("application/json"))
		if hasReturn {
			g.Id("_").Op("=").Qual("encoding/json", "NewEncoder").Call(jen.Id("w")).Dot("Encode").Call(jen.Map(jen.String()).Any().Values(jen.Dict{
				jen.Lit("value"): jen.Id("__val"),
				jen.Lit("state"): jen.Int64().Call(jen.Id("WireStateOk")),
			}))
		} else {
			g.Id("_").Op("=").Qual("encoding/json", "NewEncoder").Call(jen.Id("w")).Dot("Encode").Call(jen.Map(jen.String()).Any().Values(jen.Dict{
				jen.Lit("value"): jen.Nil(),
				jen.Lit("state"): jen.Int64().Call(jen.Id("WireStateOk")),
			}))
		}
	}))
}

// emitWiredWSAdapter emits the WS-call adapter `<handlerName>__ws(*fn____Session, json.RawMessage) (any, WireState)` for a backend wire func with `wire(transport: "ws")`. The adapter mirrors the HTTP handler's authn/authz checks, unpacks the JSON args array, calls the impl, and returns `(value, state)` for the WS dispatcher to wrap into the reply envelope.
func (e *CodeEmitter) emitWiredWSAdapter(ctx *codegen.EmitContext, pkg *ir.PackageContext, f *ir.File, block *jen.Group, fn *ir.FuncDeclStmt) {
	adapterName := symName(ctx, fn.Name.Sym) + "__wireHandler__ws"
	implName := symName(ctx, fn.Name.Sym)
	block.Add(jen.Func().Id(adapterName).Params(
		jen.Id("__session").Op("*").Id("fn____Session"),
		jen.Id("__rawArgs").Qual("encoding/json", "RawMessage"),
	).Params(jen.Any(), jen.Id("WireState")).BlockFunc(func(g *jen.Group) {
		g.Id("__sovaSetCurrentSession").Call(jen.Id("__session"))
		g.Defer().Id("__sovaClearCurrentSession").Call()

		if fn.Wire.RequireAuthN {
			g.If(jen.Op("!").Id("__session").Dot("Auth").Op("&&").Op("!").Id("__sovaTestBypassAuth").Call()).Block(
				jen.Return(jen.Nil(), jen.Id("WireStateUnauthorized")),
			)
		}
		for _, role := range fn.Wire.RequiredRoles {
			roleLit := role
			g.If(jen.Op("!").Id("__session").Dot("hasRole").Call(jen.Lit(roleLit))).Block(
				jen.Return(jen.Nil(), jen.Id("WireStateForbidden")),
			)
		}

		g.Var().Id("__args").Index().Qual("encoding/json", "RawMessage")
		g.If(jen.Qual("", "len").Call(jen.Id("__rawArgs")).Op(">").Lit(0)).Block(
			// A WS-call envelope with garbage `args` is a bad-request shape;
			// answer with WireStateError instead of running the wire body
			// against zero-valued params. The dispatcher relays the state
			// back to the caller as an inbound `op:"reply"` carrying the
			// error sentinel.
			jen.If(jen.Id("__argsErr").Op(":=").Qual("encoding/json", "Unmarshal").Call(jen.Id("__rawArgs"), jen.Op("&").Id("__args")).Op(";").Id("__argsErr").Op("!=").Nil()).Block(
				jen.Return(jen.Nil(), jen.Id("WireStateError")),
			),
		)

		var callArgs []jen.Code
		for i, param := range fn.Params {
			paramName := symNameWithUnused(ctx, pkg, param.Name.Sym)
			paramTypeID := ir.TypID(0)
			if param.Type != nil {
				paramTypeID = param.Type.Typ
			}
			g.Var().Id(paramName).Add(typeToGoWithContext(ctx, pkg, ctx.Types, paramTypeID))
			g.If(jen.Qual("", "len").Call(jen.Id("__args")).Op(">").Lit(i)).Block(
				jen.If(jen.Id("__pErr").Op(":=").Qual("encoding/json", "Unmarshal").Call(jen.Id("__args").Index(jen.Lit(i)), jen.Op("&").Id(paramName)).Op(";").Id("__pErr").Op("!=").Nil()).Block(
					jen.Return(jen.Nil(), jen.Id("WireStateError")),
				),
			)
			callArgs = append(callArgs, jen.Id(paramName))
		}

		hasReturn := fn.ReturnType != nil && fn.ReturnType.Typ != 0 && fn.ReturnType.Typ != ctx.Types.TypNone()
		if hasReturn {
			g.Id("__val").Op(":=").Id(implName).Call(append([]jen.Code{jen.Id("__session")}, callArgs...)...)
			g.Return(jen.Id("__val"), jen.Id("WireStateOk"))
		} else {
			g.Id(implName).Call(append([]jen.Code{jen.Id("__session")}, callArgs...)...)
			g.Return(jen.Nil(), jen.Id("WireStateOk"))
		}
	}))
}

// decodeStringToGo parses an HTTP-supplied string (path segment or query parameter) into the Go representation of the given Sova type.
func decodeStringToGo(tt *ir.TypeTable, typID ir.TypID, expr *jen.Statement) *jen.Statement {
	if typID == 0 {
		return expr
	}
	ty, ok := tt.GetByID(typID)
	if !ok {
		return expr
	}
	switch ty.Kind {
	case ir.TK_PrimitiveInt:
		return jen.Func().Params().Int64().BlockFunc(func(g *jen.Group) {
			g.List(jen.Id("n"), jen.Id("_")).Op(":=").Qual("strconv", "ParseInt").Call(expr, jen.Lit(10), jen.Lit(64))
			g.Return(jen.Id("n"))
		}).Call()
	case ir.TK_PrimitiveFloat:
		return jen.Func().Params().Float64().BlockFunc(func(g *jen.Group) {
			g.List(jen.Id("n"), jen.Id("_")).Op(":=").Qual("strconv", "ParseFloat").Call(expr, jen.Lit(64))
			g.Return(jen.Id("n"))
		}).Call()
	case ir.TK_PrimitiveBool:
		return jen.Func().Params().Bool().BlockFunc(func(g *jen.Group) {
			g.List(jen.Id("b"), jen.Id("_")).Op(":=").Qual("strconv", "ParseBool").Call(expr)
			g.Return(jen.Id("b"))
		}).Call()
	}
	return expr
}
