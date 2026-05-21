package codegen

import "sova/internal/ir"

// TestRegistryEntryView is the codegen-facing shape of one discovered test, deliberately defined in the shared `codegen` package so neither `passes` nor `codegen/golang` need to import the other (which would form a cycle through pass_codegen.go). The `passes.PassTestDiscovery` pass produces matching `passes.TestEntry` values; the wrapper adapter in `pass_codegen.go` converts them into `[]TestRegistryEntryView` and stores the slice under `TestRegistryViewCacheKey`. The SetupAlls / TeardownAlls slices carry the once-per-group hook statements inherited at this entry's scope, paired with their unique node IDs (from the IR allocator) and joined-group-path owner keys so the codegen can sync.Once-gate them across siblings.
type TestRegistryEntryView struct {
	Pkg                *ir.PackageContext
	File               *ir.PreparsedFile
	Decl               *ir.TestDeclStmt
	GroupPath          []string
	SetupBodies        [][]ir.Stmt
	TeardownBodies     [][]ir.Stmt
	SetupAlls          []*ir.SetupStmt
	SetupAllOwners     []string
	TeardownAlls       []*ir.TeardownStmt
	TeardownAllOwners  []string
	Parallel           bool
}

// TestRegistryViewCacheKey publishes the test-mode codegen view of the test registry to the Go/JS emitters. Always matches the same content as `passes.TestRegistryCacheKey`, just re-packaged so the emitters can consume it without importing the `passes` package.
const TestRegistryViewCacheKey = "test_registry_view"
