package compiler

import "testing"

// TestCrossFileStructFieldAccess regression-tests Faithbook BUG #18.
// Before the fix in pass_infer_types.preComputeStructFields, a
// struct type declared in one file of a nested package had its
// `StructFields` slice left nil until the main `resolveStmts` loop
// visited its declaring file. Field reads from any file processed
// earlier in alphabetical order errored with
// `type T has no field 'x' is not indexable`, regardless of whether
// the field was `@reactive` or had a non-primitive default. The
// preCompute pass now stamps StructFields from the type annotations
// before any file is walked.
func TestCrossFileStructFieldAccess(t *testing.T) {
	c := New()
	c.AddSource("src/main.sova", `package bug18 on backend
import "bug18/sub"
func main() { let _ = sub.useIt() }
`)
	c.AddSource("src/sub/use.sova", `package bug18/sub on backend
func useIt(): bool {
    let h = new Holder()
    return h.flag
}
`)
	c.AddSource("src/sub/data.sova", `package bug18/sub on backend
type Inner { val: int = 0 }
type Holder {
    @reactive a: Inner = new Inner()
    @reactive flag: bool = false
}
`)

	_ = c.Check()
	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("compile produced errors")
	}
}
