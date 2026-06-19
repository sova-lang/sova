package compiler

import "testing"

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
