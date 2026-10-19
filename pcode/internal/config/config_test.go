// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func clearRegistryEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"CODER_WORKSPACE_REFERENCE_FILE",
		"CODER_WORKSPACE_REGISTRY_FILE",
		"CODER_RUNTIME_HOME",
		"CODER_RUNTIME_DATA_DIR",
		"CODER_RUNTIME_BIN",
		"PCODE_HOME",
	} {
		t.Setenv(key, "")
	}
}

func TestLoadReferenceFileWins(t *testing.T) {
	clearRegistryEnv(t)
	home := t.TempDir()
	t.Setenv("CODER_RUNTIME_HOME", home)
	t.Setenv("PCODE_HOME", t.TempDir())
	t.Setenv("CODER_WORKSPACE_REFERENCE_FILE", "/tmp/custom-workspaces.tsv")
	cfg := Load()
	if cfg.RegistryFile != "/tmp/custom-workspaces.tsv" {
		t.Fatalf("registry=%s", cfg.RegistryFile)
	}
	if cfg.Home != home {
		t.Fatalf("home=%s want %s", cfg.Home, home)
	}
}

func TestLoadCoderRuntimeHomeWinsOverPcodeHome(t *testing.T) {
	clearRegistryEnv(t)
	home := t.TempDir()
	t.Setenv("CODER_RUNTIME_HOME", home)
	t.Setenv("PCODE_HOME", t.TempDir())
	cfg := Load()
	if cfg.Home != home {
		t.Fatalf("home=%s want %s", cfg.Home, home)
	}
	wantReg := filepath.Join(home, "data", "workspaces.tsv")
	if cfg.RegistryFile != wantReg {
		t.Fatalf("registry=%s want %s", cfg.RegistryFile, wantReg)
	}
	if cfg.DataDir != filepath.Join(home, "data") {
		t.Fatalf("data=%s", cfg.DataDir)
	}
	if cfg.BinDir != filepath.Join(home, "bin") {
		t.Fatalf("bin=%s", cfg.BinDir)
	}
}

func TestLoadPcodeHome(t *testing.T) {
	clearRegistryEnv(t)
	home := t.TempDir()
	t.Setenv("PCODE_HOME", home)
	// A present Prajvis dir must not beat PCODE_HOME.
	orig := prajvisRuntimeHome
	prajvisRuntimeHome = t.TempDir()
	t.Cleanup(func() { prajvisRuntimeHome = orig })
	cfg := Load()
	if cfg.Home != home {
		t.Fatalf("home=%s want %s", cfg.Home, home)
	}
	if cfg.RegistryFile != filepath.Join(home, "data", "workspaces.tsv") {
		t.Fatalf("registry=%s", cfg.RegistryFile)
	}
}

func TestLoadPrajvisHomeWhenPresent(t *testing.T) {
	clearRegistryEnv(t)
	dir := t.TempDir()
	orig := prajvisRuntimeHome
	prajvisRuntimeHome = dir
	t.Cleanup(func() { prajvisRuntimeHome = orig })
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := Load()
	if cfg.Home != dir {
		t.Fatalf("home=%s want %s (Prajvis path must be used when it exists)", cfg.Home, dir)
	}
	if cfg.RegistryFile != filepath.Join(dir, "data", "workspaces.tsv") {
		t.Fatalf("registry=%s", cfg.RegistryFile)
	}
}

func TestLoadPrajvisHomeWhenAbsentUsesXDG(t *testing.T) {
	clearRegistryEnv(t)
	orig := prajvisRuntimeHome
	prajvisRuntimeHome = filepath.Join(t.TempDir(), "missing")
	t.Cleanup(func() { prajvisRuntimeHome = orig })
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	cfg := Load()
	want := filepath.Join(xdg, "pcode")
	if cfg.Home != want {
		t.Fatalf("home=%s want %s", cfg.Home, want)
	}
	if cfg.RegistryFile != filepath.Join(want, "data", "workspaces.tsv") {
		t.Fatalf("registry=%s", cfg.RegistryFile)
	}
}

func TestLoadXDGFallsBackToHomeConfig(t *testing.T) {
	clearRegistryEnv(t)
	orig := prajvisRuntimeHome
	prajvisRuntimeHome = filepath.Join(t.TempDir(), "missing")
	t.Cleanup(func() { prajvisRuntimeHome = orig })
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)
	cfg := Load()
	want := filepath.Join(home, ".config", "pcode")
	if cfg.Home != want {
		t.Fatalf("home=%s want %s", cfg.Home, want)
	}
}

func TestLoadDataDirOverrideFollowsHomeRoot(t *testing.T) {
	clearRegistryEnv(t)
	home := t.TempDir()
	data := t.TempDir()
	t.Setenv("CODER_RUNTIME_HOME", home)
	t.Setenv("CODER_RUNTIME_DATA_DIR", data)
	cfg := Load()
	if cfg.Home != home {
		t.Fatalf("home=%s", cfg.Home)
	}
	if cfg.DataDir != data {
		t.Fatalf("data=%s", cfg.DataDir)
	}
	if cfg.RegistryFile != filepath.Join(data, "workspaces.tsv") {
		t.Fatalf("registry=%s", cfg.RegistryFile)
	}
}

func TestDirExists(t *testing.T) {
	dir := t.TempDir()
	if !dirExists(dir) {
		t.Fatal("expected existing dir")
	}
	if dirExists(filepath.Join(dir, "nope")) {
		t.Fatal("missing dir should be false")
	}
	f := filepath.Join(dir, "file")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if dirExists(f) {
		t.Fatal("file should not count as dir")
	}
}
