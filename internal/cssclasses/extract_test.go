package cssclasses

import (
	"reflect"
	"testing"
)

func TestExtractFlatCSS(t *testing.T) {
	in := `.btn { color: red; }
.btn-large { padding: 16px; }
.alert.error { background: pink; }`
	got := Names(in)
	want := []string{"btn", "btn-large", "alert", "error"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestExtractSkipsValues(t *testing.T) {
	in := `.box {
  padding: .5rem;
  margin: 1.5em;
  content: ".not-a-class";
  background: url("foo.png");
}`
	got := Names(in)
	want := []string{"box"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestExtractSkipsComments(t *testing.T) {
	in := `// .commented-line-class
/* .commented-block-class */
.real { color: red; }
/* .deeply { .nested-not-real { color: red; } } */`
	got := Names(in)
	want := []string{"real"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestExtractNestedSCSS(t *testing.T) {
	in := `.card {
  .header { font-weight: bold; }
  .body { padding: 8px; }
  &.featured { border: 2px solid gold; }
}`
	got := Names(in)
	want := []string{"card", "header", "body", "featured"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestExtractMultipleSelectorsInOne(t *testing.T) {
	in := `.a, .b, .c { color: red; }`
	got := Names(in)
	want := []string{"a", "b", "c"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestExtractDedupes(t *testing.T) {
	in := `.btn { } .btn:hover { } .btn.large { }`
	got := Names(in)
	want := []string{"btn", "large"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestExtractIgnoresInterpolation(t *testing.T) {
	in := `.btn { }
.#{$prefix}-variant { } // SCSS interpolation - skip
.real-#{$mod} { }       // partial interpolation - skip too`
	got := Names(in)
	want := []string{"btn", "real-"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestExtractEmpty(t *testing.T) {
	got := Names("")
	if len(got) != 0 {
		t.Errorf("got %v want empty", got)
	}
}

func TestExtractEntriesTrackPositions(t *testing.T) {
	in := ".primary { color: red; }\n.secondary { color: blue; }\n"
	entries := Extract(in)
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}

	if entries[0].Name != "primary" || entries[0].Line != 0 || entries[0].Char != 0 {
		t.Errorf("entry[0]: got %+v want {primary, 0, 0}", entries[0])
	}

	if entries[1].Name != "secondary" || entries[1].Line != 1 || entries[1].Char != 0 {
		t.Errorf("entry[1]: got %+v want {secondary, 1, 0}", entries[1])
	}
}

func TestRuleAtFlatRule(t *testing.T) {
	in := ".primary { color: red; padding: 8px; }"
	entries := Extract(in)
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}

	rule := RuleAt(in, entries[0].Offset)
	want := ".primary { color: red; padding: 8px; }"
	if rule != want {
		t.Errorf("rule mismatch:\n got %q\nwant %q", rule, want)
	}
}

func TestRuleAtCommaSelector(t *testing.T) {
	in := ".a, .b, .c {\n  color: red;\n}"
	entries := Extract(in)
	if len(entries) != 3 {
		t.Fatalf("want 3 entries, got %d", len(entries))
	}

	rule := RuleAt(in, entries[1].Offset)
	if rule == "" {
		t.Fatal("expected non-empty rule for .b")
	}

	if rule[len(rule)-1] != '}' {
		t.Errorf("rule should end at closing brace; got %q", rule)
	}
}

func TestRuleAtNoBrace(t *testing.T) {
	in := "@extend .other; .real { color: red; }"
	entries := Extract(in)
	for _, e := range entries {
		if e.Name == "other" {
			if rule := RuleAt(in, e.Offset); rule != "" {
				t.Errorf(".other in @extend should have no rule; got %q", rule)
			}
		}
	}
}

func TestImportsModernSass(t *testing.T) {
	in := `@use "variables";
@use "mixins" as m;
.foo { color: red; }`
	got := Imports(in)
	want := []string{"variables", "mixins"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestImportsWithConfig(t *testing.T) {
	in := `@use "theme" with ($primary: rebeccapurple);
@import "reset.css";`
	got := Imports(in)
	want := []string{"theme", "reset.css"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestImportsMultiplePaths(t *testing.T) {
	in := `@import "a", "b", "c";`
	got := Imports(in)
	want := []string{"a", "b", "c"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestImportsSkipsAtRulesInsideBlocks(t *testing.T) {
	in := `.btn {
  @use "should-not-match";
}
@use "top-level";`
	got := Imports(in)
	want := []string{"should-not-match", "top-level"}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestRuleAtNestedScss(t *testing.T) {
	in := ".card {\n  color: red;\n  .header { font-weight: bold; }\n}"
	entries := Extract(in)
	if len(entries) < 2 {
		t.Fatalf("want at least 2 entries, got %d", len(entries))
	}

	cardRule := RuleAt(in, entries[0].Offset)
	if cardRule == "" {
		t.Fatal(".card should have a rule")
	}

	if cardRule[len(cardRule)-1] != '}' {
		t.Errorf(".card rule should end at outer brace; got %q", cardRule)
	}
}
