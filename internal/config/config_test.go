package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_FromXDGConfigPath(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("WNWN_CONFIG_FILE", "")

	configPath := filepath.Join(xdg, "wnwn", Filename)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("[ui]\ndefault_view='projects'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UI.DefaultView != "projects" {
		t.Fatalf("default_view = %q, want projects", cfg.UI.DefaultView)
	}
}

func TestLoad_FallsBackToLegacyDataDirConfig(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("WNWN_CONFIG_FILE", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	legacyPath := filepath.Join(dataDir, Filename)
	if err := os.WriteFile(legacyPath, []byte("[archive]\nauto_archive_done=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Archive.AutoArchiveDone {
		t.Fatal("expected auto_archive_done=true from legacy config")
	}
}

func TestLoad_OverridePathMissingReturnsError(t *testing.T) {
	t.Setenv("WNWN_CONFIG_FILE", filepath.Join(t.TempDir(), "missing.toml"))

	_, err := Load(t.TempDir())
	if err == nil {
		t.Fatal("expected error when WNWN_CONFIG_FILE points to missing file")
	}
}

func TestLoad_KeysDisableNormalized(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("WNWN_CONFIG_FILE", "")

	configPath := filepath.Join(xdg, "wnwn", Filename)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "[keys.disable]\nlist=[' done ', 'Done', '']\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Keys.Disable.List) != 1 || cfg.Keys.Disable.List[0] != "done" {
		t.Fatalf("unexpected normalized disable list: %#v", cfg.Keys.Disable.List)
	}
}
