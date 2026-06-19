package scss

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type Config struct {
	Command  string
	Disabled bool
}

type Preprocessor struct {
	command string
}

func New(cfg Config) Preprocessor {
	if cfg.Disabled {
		return Preprocessor{}
	}

	if cfg.Command != "" {
		if path, err := exec.LookPath(cfg.Command); err == nil {
			return Preprocessor{command: path}
		}

		return Preprocessor{command: cfg.Command}
	}

	for _, candidate := range []string{"sass", "dart-sass"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return Preprocessor{command: path}
		}
	}

	return Preprocessor{}
}

func (p Preprocessor) Available() bool {
	return p.command != ""
}

func (p Preprocessor) Command() string {
	return p.command
}

func (p Preprocessor) Compile(path string) ([]byte, error) {
	if !p.Available() {
		return nil, fmt.Errorf("no SCSS preprocessor configured (`sass` not on PATH; set [build.scss] command in sova.toml or disable @embed on .scss files)")
	}

	args := []string{path}

	if strings.EqualFold(filepath.Ext(path), ".sass") {
		args = append([]string{"--indented"}, args...)
	}

	cmd := exec.Command(p.command, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		trimmed := strings.TrimSpace(stderr.String())
		if trimmed == "" {
			trimmed = err.Error()
		}

		return nil, fmt.Errorf("%s: %s", filepath.Base(p.command), trimmed)
	}

	return stdout.Bytes(), nil
}

func IsSassPath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".scss" || ext == ".sass"
}
