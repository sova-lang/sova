package compiler

import "testing"

// TestCrossFileLetInNestedPackage regression-tests Faithbook BUG #10.
// Before the fix in pass_infer_types.preComputeTopLevelVarSignatures,
// a `let` declared in one file of a nested package wasn't visible to
// a *type checker* lookup from a file processed earlier in
// alphabetical order, because the let's symbol type stayed at 0 until
// resolveStmts visited its declaring file. Functions had a
// preCompute pass that stamped their signatures up front; vars
// didn't. The repro below has oauth.sova (alphabetically first) read
// `myFlag` declared in state.sova; before the fix the read came back
// `<unresolved>`.
func TestCrossFileLetInNestedPackage(t *testing.T) {
	c := New()
	c.AddSource("src/main.sova", `package bug10 on backend
import "bug10/sub"
func main() { let _ = sub.readOne() }
`)
	c.AddSource("src/sub/oauth.sova", `package bug10/sub on backend
func readOne(): bool { return myFlag }
`)
	c.AddSource("src/sub/state.sova", `package bug10/sub on backend
let myFlag: bool = false
`)

	if err := c.Check(); err != nil {
		t.Fatalf("check: %v", err)
	}
	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("compile produced errors")
	}
}
