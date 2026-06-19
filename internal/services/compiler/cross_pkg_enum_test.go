package compiler

import "testing"

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
