package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const Filename = "config.toml"

type Config struct {
	Archive ArchiveConfig `toml:"archive"`
	UI      UIConfig      `toml:"ui"`
	Keys    KeysConfig    `toml:"keys"`
}

type ArchiveConfig struct {
	AutoArchiveDone     bool `toml:"auto_archive_done"`
	AutoArchiveCanceled bool `toml:"auto_archive_canceled"`
}

type UIConfig struct {
	DefaultView string `toml:"default_view"`
}

type KeysConfig struct {
	List        map[string]string `toml:"list"`
	Project     map[string]string `toml:"project"`
	ViewResults map[string]string `toml:"view_results"`
}

func Default() Config {
	return Config{
		Archive: ArchiveConfig{
			AutoArchiveDone:     false,
			AutoArchiveCanceled: false,
		},
		UI: UIConfig{DefaultView: "inbox"},
		Keys: KeysConfig{
			List:        map[string]string{},
			Project:     map[string]string{},
			ViewResults: map[string]string{},
		},
	}
}

func Path(dataDir string) string {
	return filepath.Join(dataDir, Filename)
}

func Load(dataDir string) (Config, error) {
	cfg := Default()
	raw, err := os.ReadFile(Path(dataDir))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("reading config: %w", err)
	}

	if err := toml.Unmarshal(raw, &cfg); err != nil {
		return Default(), fmt.Errorf("parsing config.toml: %w", err)
	}
	cfg.normalize()
	return cfg, nil
}

func (c *Config) normalize() {
	c.UI.DefaultView = strings.TrimSpace(strings.ToLower(c.UI.DefaultView))
	if c.UI.DefaultView == "" {
		c.UI.DefaultView = "inbox"
	}
	if c.Keys.List == nil {
		c.Keys.List = map[string]string{}
	}
	if c.Keys.Project == nil {
		c.Keys.Project = map[string]string{}
	}
	if c.Keys.ViewResults == nil {
		c.Keys.ViewResults = map[string]string{}
	}
}
