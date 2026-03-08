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
const envConfigFile = "WNWN_CONFIG_FILE"

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
	DefaultView      string `toml:"default_view"`
	UndoGraceEnabled bool   `toml:"undo_grace_enabled"`
	UndoGraceSeconds int    `toml:"undo_grace_seconds"`
	UndoKey          string `toml:"undo_key"`
}

type KeysConfig struct {
	List        map[string]string `toml:"list"`
	Project     map[string]string `toml:"project"`
	ViewResults map[string]string `toml:"view_results"`
	Disable     KeysDisableConfig `toml:"disable"`
}

type KeysDisableConfig struct {
	List        []string `toml:"list"`
	Project     []string `toml:"project"`
	ViewResults []string `toml:"view_results"`
}

func Default() Config {
	return Config{
		Archive: ArchiveConfig{
			AutoArchiveDone:     false,
			AutoArchiveCanceled: false,
		},
		UI: UIConfig{DefaultView: "inbox", UndoGraceEnabled: true, UndoGraceSeconds: 30, UndoKey: "u"},
		Keys: KeysConfig{
			List:        map[string]string{},
			Project:     map[string]string{},
			ViewResults: map[string]string{},
		},
	}
}

func Path() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "wnwn", Filename)
	}
	xdgConfigHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if xdgConfigHome == "" {
		xdgConfigHome = filepath.Join(home, ".config")
	}
	return filepath.Join(xdgConfigHome, "wnwn", Filename)
}

func Load(dataDir string) (Config, error) {
	cfg := Default()
	paths := candidatePaths(dataDir)
	for i, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if i == 0 && strings.TrimSpace(os.Getenv(envConfigFile)) != "" {
					return cfg, fmt.Errorf("configured %s file not found: %s", envConfigFile, p)
				}
				continue
			}
			return cfg, fmt.Errorf("reading config %s: %w", p, err)
		}

		if err := toml.Unmarshal(raw, &cfg); err != nil {
			return Default(), fmt.Errorf("parsing config %s: %w", p, err)
		}
		cfg.normalize()
		return cfg, nil
	}

	return cfg, nil
}

func candidatePaths(dataDir string) []string {
	override := strings.TrimSpace(os.Getenv(envConfigFile))
	if override != "" {
		return []string{override}
	}

	paths := []string{Path()}
	if dataDir != "" {
		legacy := filepath.Join(dataDir, Filename)
		if legacy != paths[0] {
			paths = append(paths, legacy)
		}
	}
	return paths
}

func (c *Config) normalize() {
	c.UI.DefaultView = strings.TrimSpace(strings.ToLower(c.UI.DefaultView))
	if c.UI.DefaultView == "" {
		c.UI.DefaultView = "inbox"
	}
	if c.UI.UndoGraceSeconds <= 0 {
		c.UI.UndoGraceSeconds = 30
	}
	c.UI.UndoKey = strings.TrimSpace(strings.ToLower(c.UI.UndoKey))
	if c.UI.UndoKey == "" {
		c.UI.UndoKey = "u"
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
	c.Keys.Disable.List = normalizeActionList(c.Keys.Disable.List)
	c.Keys.Disable.Project = normalizeActionList(c.Keys.Disable.Project)
	c.Keys.Disable.ViewResults = normalizeActionList(c.Keys.Disable.ViewResults)
}

func normalizeActionList(actions []string) []string {
	out := make([]string, 0, len(actions))
	seen := map[string]struct{}{}
	for _, a := range actions {
		a = strings.TrimSpace(strings.ToLower(a))
		if a == "" {
			continue
		}
		if _, ok := seen[a]; ok {
			continue
		}
		seen[a] = struct{}{}
		out = append(out, a)
	}
	return out
}
