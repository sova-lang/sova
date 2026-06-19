package compiler

import "testing"

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
