package compiler

import (
	"strings"
	"testing"

	"sova/internal/ir"
)

func TestDocCommentBlankLineSeparation(t *testing.T) {
	src := `package smoke on backend

/// First doc block - belongs to Apple only.
type Apple {}

/// Second doc block - belongs to Banana only.
type Banana {}

/// Doc A.
/// Doc B (same chain).
type Cherry {}

/// First chunk.

/// Real doc (after blank line).
type Date {}
`
	c := New()
	c.AddSource("main.sova", src)
	_ = c.Check()

	pkg, ok := c.Packages["smoke"]
	if !ok {
		t.Fatalf("package smoke not loaded")
	}

	docs := map[string]string{}

	for _, f := range pkg.Files {
		for _, st := range f.Hir.Statements {
			td, ok := st.(*ir.TypeDeclStmt)
			if !ok {
				continue
			}

			docs[td.Name.Name] = td.GetDoc()
		}
	}

	cases := []struct {
		name        string
		mustHave    []string
		mustNotHave []string
	}{
		{
			name:        "Apple",
			mustHave:    []string{"First doc block - belongs to Apple only."},
			mustNotHave: []string{"Banana", "Cherry", "Date"},
		},
		{
			name:        "Banana",
			mustHave:    []string{"Second doc block - belongs to Banana only."},
			mustNotHave: []string{"Apple", "Cherry", "Date"},
		},
		{
			name:        "Cherry",
			mustHave:    []string{"Doc A.", "Doc B (same chain)."},
			mustNotHave: []string{"Apple", "Banana", "First chunk", "Date"},
		},
		{
			name:        "Date",
			mustHave:    []string{"Real doc (after blank line)."},
			mustNotHave: []string{"First chunk"},
		},
	}

	for _, tc := range cases {
		doc := docs[tc.name]
		for _, want := range tc.mustHave {
			if !strings.Contains(doc, want) {
				t.Errorf("type %s: doc missing %q\n  doc=%q", tc.name, want, doc)
			}
		}

		for _, unwanted := range tc.mustNotHave {
			if strings.Contains(doc, unwanted) {
				t.Errorf("type %s: doc must NOT contain %q (leaked from another decl)\n  doc=%q", tc.name, unwanted, doc)
			}
		}
	}
}
