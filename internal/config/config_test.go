package config

import (
	"path/filepath"
	"testing"
)

func TestLoadUsesLocalAppData(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)

	cfg := Load(false)
	want := filepath.Join(base, "RouteManager")
	if cfg.DataDir != want {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, want)
	}
}

func TestLoadKeepsDevDataSeparate(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)

	cfg := Load(true)
	want := filepath.Join(base, "RouteManager-dev")
	if cfg.DataDir != want {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, want)
	}
}
