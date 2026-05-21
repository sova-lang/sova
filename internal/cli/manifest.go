package cli

import (
	"errors"
	"io/fs"
	"os"

	"github.com/BurntSushi/toml"
)

// ManifestFilename is the conventional name of the project manifest in the working directory.
const ManifestFilename = "sova.toml"

type manifest struct {
	Project manifestProject `toml:"project"`
	Build   manifestBuild   `toml:"build"`
	Wire    manifestWire    `toml:"wire"`
	Serve   manifestServe   `toml:"serve"`
}

type manifestProject struct {
	Entry     string `toml:"entry"`
	SourceDir string `toml:"source_dir"`
}

type manifestBuild struct {
	OutputDir  string `toml:"output_dir"`
	OutputName string `toml:"output_name"`
}

type manifestWire struct {
	Port                 int    `toml:"port"`
	Host                 string `toml:"host"`
	SessionSecret        string `toml:"session_secret"`
	SessionGraceSeconds  int    `toml:"session_grace_seconds"`
}

type manifestServe struct {
	Port       int    `toml:"port"`
	StrictPort bool   `toml:"strict_port"`
	Host       string `toml:"host"`
	Frontend   *bool  `toml:"frontend"`
	WebDir     string `toml:"web_dir"`
}

// LoadManifest reads sova.toml from path. Returns the zero manifest and ok=false if the file does not exist; any other I/O or parse error is returned.
func LoadManifest(path string) (manifest, bool, error) {
	var m manifest
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return m, false, nil
		}
		return m, false, err
	}
	if err := toml.Unmarshal(data, &m); err != nil {
		return m, false, err
	}
	return m, true, nil
}

// applyManifest overlays manifest values onto cfg, leaving fields already set by higher-priority sources untouched.
func applyManifest(cfg *BuildConfig, m manifest) {
	if cfg.Entry == "" && m.Project.Entry != "" {
		cfg.Entry = m.Project.Entry
	}
	if m.Project.SourceDir != "" {
		cfg.SourceDir = m.Project.SourceDir
	}
	if m.Build.OutputDir != "" {
		cfg.OutputDir = m.Build.OutputDir
	}
	if m.Build.OutputName != "" {
		cfg.OutputName = m.Build.OutputName
	}
	if m.Wire.Port != 0 {
		cfg.WirePort = m.Wire.Port
	}
	if m.Wire.Host != "" {
		cfg.WireHost = m.Wire.Host
	}
	if m.Wire.SessionSecret != "" {
		cfg.WireSessionSecret = m.Wire.SessionSecret
	}
	if m.Wire.SessionGraceSeconds > 0 {
		cfg.WireSessionGraceSeconds = m.Wire.SessionGraceSeconds
	}
	if m.Serve.Port != 0 {
		cfg.ServePort = m.Serve.Port
	}
	if m.Serve.Host != "" {
		cfg.ServeHost = m.Serve.Host
	}
	if m.Serve.StrictPort {
		cfg.ServeStrictPort = true
	}
	if m.Serve.Frontend != nil {
		cfg.ServeFrontend = *m.Serve.Frontend
	}
	if m.Serve.WebDir != "" {
		cfg.ServeWebDir = m.Serve.WebDir
	}
}
