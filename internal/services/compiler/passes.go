package compiler

import "sova/internal/passes"

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
	pm.Register(&passes.PassResolveAssets{})
	pm.Register(&passes.PassPropagateAsync{})
	pm.Register(&passes.PassPopulateBuiltins{})
	pm.Register(&passes.PassInitOrder{})
	pm.Register(&passes.PassMangle{})
	pm.Register(&passes.PassDetectUnused{})
	pm.Register(&passes.PassComputeReachability{})
	pm.Register(&passes.PassEmitGo{})
	pm.Register(&passes.PassEmitJS{})
	pm.Register(&passes.PassTestDiscovery{})
	return pm
}

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
		"resolve_assets",
		"propagate_async",
		"populate_builtins",
		"test_discovery",
		"init_order",
		"mangle",
		"compute_reachability",
		"emit_go",
		"emit_js",
	}
}

func (c *CompilerContext) RunTestPipeline() error {
	if err := c.resolveImports(); err != nil {
		return err
	}

	return c.runPipeline(TestPipeline())
}

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
		"resolve_assets",
		"propagate_async",
		"populate_builtins",
		"init_order",
		"mangle",
		"detect_unused",
		"compute_reachability",
		"emit_go",
		"emit_js",
	}
}

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
		"resolve_assets",
		"propagate_async",
	}
}
