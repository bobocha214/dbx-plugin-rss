package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsurePluginDataDir(t *testing.T) {
	t.Setenv("DBX_PLUGIN_DATA_DIR", "")
	if _, err := ensurePluginDataDir(); err == nil {
		t.Fatal("missing host directory must fail")
	}
	dir := filepath.Join(t.TempDir(), "plugin", "data")
	t.Setenv("DBX_PLUGIN_DATA_DIR", dir)
	got, err := ensurePluginDataDir()
	if err != nil || got != dir {
		t.Fatalf("got %q, %v", got, err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("directory not created: %v", err)
	}
	if _, err := ensurePluginDataDir(); err != nil {
		t.Fatalf("existing directory must work: %v", err)
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DBX_PLUGIN_DATA_DIR", file)
	if _, err := ensurePluginDataDir(); err == nil {
		t.Fatal("file cannot be a data directory")
	}
}
