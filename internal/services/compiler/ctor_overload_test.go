package compiler

import "testing"

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
