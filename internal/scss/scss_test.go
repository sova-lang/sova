package scss

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsSassPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"Button.css", false},
		{"Button.scss", true},
		{"Button.sass", true},
		{"Button.SCSS", true},
		{"sub/dir/foo.scss", true},
		{"foo.txt", false},
		{"foo", false},
	}
	for _, tc := range cases {
		if got := IsSassPath(tc.path); got != tc.want {
			t.Errorf("IsSassPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestNewDisabled(t *testing.T) {
	p := New(Config{Disabled: true})
	if p.Available() {
		t.Errorf("Disabled config should produce unavailable preprocessor")
	}
}

func TestNewAutoDiscoveryAbsentBinary(t *testing.T) {
	if _, err := exec.LookPath("sass"); err == nil {
		t.Skip("sass is on PATH; this test only covers the absent case")
	}
	if _, err := exec.LookPath("dart-sass"); err == nil {
		t.Skip("dart-sass is on PATH; this test only covers the absent case")
	}
	p := New(Config{})
	if p.Available() {
		t.Errorf("auto-discovery should yield unavailable when no binary is on PATH")
	}
}

func TestCompileWithoutBinaryErrors(t *testing.T) {
	p := Preprocessor{}
	_, err := p.Compile("anything.scss")
	if err == nil {
		t.Fatalf("Compile on zero-value Preprocessor must error")
	}
	if !strings.Contains(err.Error(), "no SCSS preprocessor configured") {
		t.Errorf("error should explain the missing preprocessor, got: %v", err)
	}
}

func TestCompileRealSassWhenAvailable(t *testing.T) {
	sassBin, err := exec.LookPath("sass")
	if err != nil {
		t.Skip("sass not on PATH; install dart-sass to enable this test")
	}
	dir := t.TempDir()
	scssPath := filepath.Join(dir, "in.scss")
	scssSrc := `$accent: rebeccapurple;
.btn {
  background: $accent;
  &:hover { background: lighten($accent, 5%); }
}`
	if err := os.WriteFile(scssPath, []byte(scssSrc), 0o644); err != nil {
		t.Fatalf("write scss: %v", err)
	}
	p := Preprocessor{}
	p = New(Config{Command: sassBin})
	if !p.Available() {
		t.Fatalf("New should locate the explicitly-given binary")
	}
	out, err := p.Compile(scssPath)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	css := string(out)
	if !strings.Contains(css, "rebeccapurple") {
		t.Errorf("compiled CSS should contain the resolved $accent value: %s", css)
	}
	if strings.Contains(css, "$accent") {
		t.Errorf("compiled CSS should not contain raw SCSS variable: %s", css)
	}
}

