package codegen

import "sova/internal/ir"

type TestRegistryEntryView struct {
	Pkg               *ir.PackageContext
	File              *ir.PreparsedFile
	Decl              *ir.TestDeclStmt
	GroupPath         []string
	SetupBodies       [][]ir.Stmt
	TeardownBodies    [][]ir.Stmt
	SetupAlls         []*ir.SetupStmt
	SetupAllOwners    []string
	TeardownAlls      []*ir.TeardownStmt
	TeardownAllOwners []string
	Parallel          bool
}

const TestRegistryViewCacheKey = "test_registry_view"
