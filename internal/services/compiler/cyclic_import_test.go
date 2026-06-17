package compiler

import "testing"

// TestCyclicPackageImportTypeResolution regression-tests the cyclic-import
// support added when std/list grew a `stream(): streams.Stream<T>` method and
// std/streams gained a `toList(): list.List<T>` method - each side legitimately
// references the other's types. Before the precompute_signatures pass was
// extracted out of infer_types, type checking was order-dependent: whichever
// package the per-package infer_types loop hit first would see the other's
// struct types with nil StructFields / StructCtors / StructMethods, and every
// cross-package call would fail with `Type <unresolved> is not indexable` or
// `Function parameter type mismatch; found func constructor`. Now that
// precompute runs as its own cross-package pass before infer_types, every
// struct's signature surface is populated before any body is walked.
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
