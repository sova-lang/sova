package pkgmgr

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// DefaultIndex is the canonical Sova package index, used when `~/.sova/config.toml` is absent or has no `indexes` list. Adding more indexes is opt-in via the config file.
const DefaultIndex = "https://github.com/sova-lang/index"

// Config is the per-user package-manager configuration, loaded from `~/.sova/config.toml`. The file is optional; everything has sensible defaults.
type Config struct {
	Indexes  []string `toml:"indexes,omitempty"`
	CacheDir string   `toml:"cache_dir,omitempty"`

	homeDir string
}

// LoadConfig reads `~/.sova/config.toml` (or the override via SOVA_HOME). Missing file → zero Config with defaults.
func LoadConfig() (*Config, error) {
	home, err := SovaHome()
	if err != nil {
		return nil, err
	}
	cfg := &Config{homeDir: home}
	data, err := os.ReadFile(filepath.Join(home, "config.toml"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return nil, err
	}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse sova config: %w", err)
	}
	return cfg, nil
}

// SovaHome resolves the per-user Sova state directory. Honors `$SOVA_HOME` when set; otherwise `$HOME/.sova`. The directory is created if it doesn't yet exist.
func SovaHome() (string, error) {
	if env := os.Getenv("SOVA_HOME"); env != "" {
		if err := os.MkdirAll(env, 0o755); err != nil {
			return "", err
		}
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".sova")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// HomeDir returns the resolved Sova home directory.
func (c *Config) HomeDir() string { return c.homeDir }

// IndexURLs returns the effective list of index URLs to merge in order. Empty config → just the default. Explicit `indexes = [...]` replaces the default entirely; users who want both must list both.
func (c *Config) IndexURLs() []string {
	if len(c.Indexes) > 0 {
		out := make([]string, len(c.Indexes))
		copy(out, c.Indexes)
		return out
	}
	return []string{DefaultIndex}
}

// CacheRoot returns the directory where bare clones and materialised commit trees live. Configurable via `cache_dir` in config.toml; defaults to `<home>/cache`.
func (c *Config) CacheRoot() string {
	if c.CacheDir != "" {
		return c.CacheDir
	}
	return filepath.Join(c.homeDir, "cache")
}

// IndexRoot returns the directory where the per-user copies of the index repos live, one subdir per index keyed by URL hash.
func (c *Config) IndexRoot() string {
	return filepath.Join(c.homeDir, "index")
}
