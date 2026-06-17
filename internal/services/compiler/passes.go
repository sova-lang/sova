package compiler

import "sova/internal/passes"

// buildPassManager creates a new PassManager and registers all passes.
func buildPassManager() *passes.PassManager {
	pm := passes.NewPassManager()
	pm.Register(&passes.PassResolveLibs{})
	pm.Register(&passes.PassCheckImports{})
	pm.Register(&passes.PassExpandSynths{})
	pm.Register(&passes.PassInlineMixins{})
	pm.Register(&passes.PassBindDeclare{})
	pm.Register(&passes.PassResolveNames{})
	pm.Register(&passes.PassResolveTypeRefs{})
	pm.Register(&passes.PassPrecomputeSignatures{})
	pm.Register(&passes.PassInferTypes{})
	pm.Register(&passes.PassAnalyzeWire{})
	pm.Register(&passes.PassAnalyzeExterns{})
	pm.Register(&passes.PassAggregateExternModules{})
	pm.Register(&passes.PassAnalyzeSharedMembers{})
	pm.Register(&passes.PassAnalyzeComposables{})
	pm.Register(&passes.PassFoldAnnotations{})
	pm.Register(&passes.PassResolveEmbeds{})
	pm.Register(&passes.PassPropagateAsync{})
	pm.Register(&passes.PassPopulateBuiltins{})
	pm.Register(&passes.PassInitOrder{})
	pm.Register(&passes.PassMangle{})
	pm.Register(&passes.PassDetectUnused{})
	pm.Register(&passes.PassEmitGo{})
	pm.Register(&passes.PassEmitJS{})
	pm.Register(&passes.PassTestDiscovery{})
	return pm
}

// TestPipeline runs the regular type-checking passes, test_discovery, and BOTH the Go and JS emitters (in test mode). The Go emitter produces the test driver `main()` that iterates the registry; the JS emitter produces a parallel bundle exposing `__sovaJSTestRun(name)` so the Go driver can invoke the same tests in an embedded goja runtime for frontend-flavoured tests.
func TestPipeline() []string {
	return []string{
		"resolve_libs",
		"check_imports",
		"expand_synths",
		"inline_mixins",
		"bind_declare",
		"resolve_names",
		"resolve_typerefs",
		"precompute_signatures",
		"analyze_wire",
		"infer_types",
		"analyze_externs",
		"aggregate_extern_modules",
		"analyze_shared_members",
		"analyze_composables",
		"fold_annotations",
		"resolve_embeds",
		"propagate_async",
		"populate_builtins",
		"test_discovery",
		"init_order",
		"mangle",
		"emit_go",
		"emit_js",
	}
}

// RunTestPipeline runs the test-mode pipeline (no codegen) and returns the resulting registry via the shared cache key for downstream tooling. Currently called only by the future `sova test` CLI; exported so test harnesses can invoke it directly.
func (c *CompilerContext) RunTestPipeline() error {
	if err := c.resolveImports(); err != nil {
		return err
	}
	return c.runPipeline(TestPipeline())
}

// compilerPipeline returns the full pipeline of compiler passes.
func compilerPipeline() []string {
	return []string{
		"resolve_libs",
		"check_imports",
		"expand_synths",
		"inline_mixins",
		"bind_declare",
		"resolve_names",
		"resolve_typerefs",
		"precompute_signatures",
		"analyze_wire",
		"infer_types",
		"analyze_externs",
		"aggregate_extern_modules",
		"analyze_shared_members",
		"analyze_composables",
		"fold_annotations",
		"resolve_embeds",
		"propagate_async",
		"populate_builtins",
		"init_order",
		"mangle",
		"detect_unused",
		"emit_go",
		"emit_js",
	}
}

// checkPipeline is used for LSPs and REPLs. Just the minimum of passes.
func checkPipeline() []string {
	return []string{
		"resolve_libs",
		"check_imports",
		"expand_synths",
		"inline_mixins",
		"bind_declare",
		"resolve_names",
		"resolve_typerefs",
		"precompute_signatures",
		"analyze_wire",
		"infer_types",
		"analyze_externs",
		"aggregate_extern_modules",
		"analyze_shared_members",
		"analyze_composables",
		"fold_annotations",
		"resolve_embeds",
		"propagate_async",
	}
}
