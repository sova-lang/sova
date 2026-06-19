package compiler

import "testing"

func TestCyclicPackageImportTypeResolution(t *testing.T) {
	c := New()
	c.AddSource("src/main.sova", `package cy on backend
import "cy/a"
import "cy/b"
func main() {
    let ab = a.Make()
    let ba = ab.toB()
    let back = ba.toA()
    let _ = back
}
`)
	c.AddSource("src/a/a.sova", `package cy/a on backend
import "cy/b"
type A {
    name: string = ""
    new(name: string) { this.name = name }
    func toB(): b.B {
        return new b.B(this.name)
    }
}
func Make(): A {
    return new A("hello")
}
`)
	c.AddSource("src/b/b.sova", `package cy/b on backend
import "cy/a"
type B {
    name: string = ""
    new(name: string) { this.name = name }
    func toA(): a.A {
        return new a.A(this.name)
    }
}
`)

	_ = c.Check()
	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("cyclic-import build produced errors")
	}
}
