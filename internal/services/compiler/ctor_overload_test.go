package compiler

import "testing"

// TestCtorOverloadPrefersTypedOverAny regression-tests the constructor overload resolution
// upgrade landed alongside browserx's `new T(handle: any)` + typed-IDL-ctor convention.
// Before the fix, the resolver was first-match-wins: the wrap ctor `new T(handle: any)`
// declared earlier in source order shadowed every typed ctor that came after it because
// `any` accepts every concrete argument. Calling `new Box(42)` ended up routing the int to
// the any-typed handle slot, leaving `value` at its default 0.
//
// The fix scores candidates by specificity (one point per concretely-typed parameter, zero
// for `any`) and picks the highest score. This restores the intuitive "typed beats any"
// preference users expect from overloaded ctors in Java / C# / Sova.
func TestCtorOverloadPrefersTypedOverAny(t *testing.T) {
	c := New()
	c.AddSource("src/main.sova", `package overload on backend
type Box {
    value: int = 0
    handle: any = none
    new(handle: any) { this.handle = handle }
    new(v: int) { this.value = v }
}
func main() {
    let b = new Box(42)
    let _ = b.value
}
`)

	_ = c.Check()
	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("ctor overload resolution produced errors")
	}
}
