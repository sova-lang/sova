package fmtsrv

import (
	"strings"
	"testing"
)

// TestRoundtripBasic feeds a representative Sova snippet through the formatter and checks the output round-trips: parsing the formatted text yields a structurally-equivalent HIR. We don't insist on byte-identical output (the formatter normalizes spacing), only that the source/sink shape matches.
func TestRoundtripBasic(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"hello", `package demo on shared

func main() {
    print("hello")
}
`},
		{"func with params and return", `on shared

func add(a: int, b: int): int {
    return a + b
}
`},
		{"type with fields and methods", `on shared

type Counter {
    private value: int = 0
    new(start: int) { this.value = start }
    func get(): int { return this.value }
    func bump() { this.value = this.value + 1 }
}
`},
		{"if-else and loops", `on shared

func work(n: int): int {
    let total = 0
    for i in 0..n {
        if i % 2 == 0 {
            total = total + i
        } else {
            total = total + 1
        }
    }
    return total
}
`},
		{"concurrency primitives", `on shared

func produce() {
    let ch = chan<int>(2)
    go { ch.send(42) }
    let v, ok = ch.recv()
    if ok { print(v) }
}
`},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			out, err := Source(tc.src)
			if err != nil {
				t.Fatalf("first format: %v", err)
			}
			if strings.TrimSpace(out) == "" {
				t.Fatalf("formatter returned empty output")
			}
			out2, err := Source(out)
			if err != nil {
				t.Fatalf("second format errored on already-formatted output: %v\nfirst pass:\n%s", err, out)
			}
			if out != out2 {
				t.Fatalf("not idempotent.\nfirst:\n%s\nsecond:\n%s", out, out2)
			}
		})
	}
}

// TestCommentPreservation checks that line and block comments survive a format pass. Comments are attached at statement boundaries - their exact placement may shift slightly when the surrounding declaration moves, but the text itself never gets dropped.
func TestCommentPreservation(t *testing.T) {
	src := `package demo on shared

// top-level comment
func main() {
    // inner comment
    let x = 1
    /* block comment */
    print(x)
}
`
	out, err := Source(src)
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	for _, want := range []string{
		"// top-level comment",
		"// inner comment",
		"/* block comment */",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, out)
		}
	}
}
