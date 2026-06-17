package compiler

import "testing"

// TestCrossPackageMultiHopEnumAccess regression-tests the `pkg.EnumName.CaseName` access
// shape, which the browserx generator emits in dictionary defaults and consumers reach for
// directly when constructing enum-typed values from another package. Before the fix in
// pass_infer_types.FieldAccessExpr, the package-qualifier branch only matched single-hop
// accesses (`pkg.X`) and bailed out on `pkg.X.Y`, leaving the per-field walker to compute
// the package's type via synthesizeTypeFromExpr - which has no meaningful answer and emitted
// `Unknown base type`. The codegen side had a parallel gap: JS getEnumSymbol/getMethodSymbol
// searched only ctx.TransPkgs, missing enums declared in same-side packages, which panicked
// emit_js with `unresolved symbol: 0` once the type-check passed.
func TestCrossPackageMultiHopEnumAccess(t *testing.T) {
	c := New()
	c.AddSource("src/main.sova", `package app on backend
import "app/colors"
func describe(): string {
    let c = colors.Color.Red
    return c.value
}
`)
	c.AddSource("src/colors/colors.sova", `package app/colors on backend
enum Color(value: string) {
    Red("red"),
    Green("green"),
    Blue("blue")
}
`)

	_ = c.Check()
	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("cross-package multi-hop enum access produced errors")
	}
}
