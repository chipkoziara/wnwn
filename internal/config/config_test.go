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

func TestLoad_UndoGraceDefaultsAndNormalize(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("WNWN_CONFIG_FILE", "")

	configPath := filepath.Join(xdg, "wnwn", Filename)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "[ui]\nundo_grace_seconds=0\nundo_key='  U '\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UI.UndoGraceSeconds != 30 {
		t.Fatalf("undo_grace_seconds = %d, want 30", cfg.UI.UndoGraceSeconds)
	}
	if cfg.UI.UndoKey != "u" {
		t.Fatalf("undo_key = %q, want %q", cfg.UI.UndoKey, "u")
	}
}

func TestLoad_TabsNormalize(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("WNWN_CONFIG_FILE", "")

	configPath := filepath.Join(xdg, "wnwn", Filename)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "[ui]\ntabs=[' Views ', 'inbox', 'invalid', 'views', 'projects']\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.UI.Tabs
	if len(got) != 3 || got[0] != "views" || got[1] != "inbox" || got[2] != "projects" {
		t.Fatalf("tabs = %#v", got)
	}
}

func TestLoad_ViewsSavedTrimmed(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("WNWN_CONFIG_FILE", "")

	configPath := filepath.Join(xdg, "wnwn", Filename)
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "[views]\nuse_defaults=false\n[[views.saved]]\nname='  Home  '\nquery=' tag:@home '\ninclude_archived=true\n[[views.saved]]\nname='  '\nquery='state:next-action'\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Views.UseDefaults {
		t.Fatal("expected use_defaults=false")
	}
	if len(cfg.Views.Saved) != 1 {
		t.Fatalf("saved views = %#v", cfg.Views.Saved)
	}
	if cfg.Views.Saved[0].Name != "Home" || cfg.Views.Saved[0].Query != "tag:@home" || !cfg.Views.Saved[0].IncludeArchived {
		t.Fatalf("saved view = %#v", cfg.Views.Saved[0])
	}
}
