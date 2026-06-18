package compiler

import "testing"

func TestMethodOverloadResolution(t *testing.T) {
	c := New()
	c.AddSource("src/main.sova", `package overload on backend
type Box {
    name: string = ""
    new(name: string) { this.name = name }
    func describe(): string { return "no arg: " + this.name }
    func describe(prefix: string): string { return prefix + ": " + this.name }
    func describe(n: int): string { return "n=" + n + " " + this.name }
}
func main() {
    let b = new Box("hello")
    let _ = b.describe()
    let _ = b.describe("p")
    let _ = b.describe(7)
}
`)

	_ = c.Check()
	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("method overload resolution produced errors")
	}
}

func TestTopLevelFuncOverloadResolution(t *testing.T) {
	c := New()
	c.AddSource("src/main.sova", `package overload on backend
func greet(x: int): string { return "int:" + x }
func greet(s: string): string { return "str:" + s }
func main() {
    let _ = greet(42)
    let _ = greet("hi")
}
`)

	_ = c.Check()
	if c.Diag.Errored() {
		c.Diag.Print()
		t.Fatalf("top-level overload resolution produced errors")
	}
}
