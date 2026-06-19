package pkgmgr

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const DefaultIndex = "https://github.com/sova-lang/index"

type Config struct {
	Indexes  []string `toml:"indexes,omitempty"`
	CacheDir string   `toml:"cache_dir,omitempty"`

	homeDir string
}

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

func (c *Config) HomeDir() string { return c.homeDir }

func (c *Config) IndexURLs() []string {
	if len(c.Indexes) > 0 {
		out := make([]string, len(c.Indexes))
		copy(out, c.Indexes)
		return out
	}

	return []string{DefaultIndex}
}

func (c *Config) CacheRoot() string {
	if c.CacheDir != "" {
		return c.CacheDir
	}

	return filepath.Join(c.homeDir, "cache")
}

func (c *Config) IndexRoot() string {
	return filepath.Join(c.homeDir, "index")
}
