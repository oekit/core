package config

import (
	"os"
	"path/filepath"
	"testing"
)

type loaderCfg struct {
	Name  string `yaml:"name"`
	Port  int    `yaml:"port"`
	Token string `yaml:"token"`
}

func writeConfigFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestLoadBootstrap_DefaultFile covers the "etc/config.yaml" path.
func TestLoadBootstrap_DefaultFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeConfigFile(t, "etc/config.yaml", "name: app\nport: 8080\n")

	cfg, err := LoadBootstrap[loaderCfg]("dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Name != "app" || cfg.Port != 8080 {
		t.Errorf("expected name=app port=8080, got %+v", cfg)
	}
}

// TestLoadBootstrap_EnvFlagFallback covers the "etc/config-{env}.yaml" fallback.
func TestLoadBootstrap_EnvFlagFallback(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeConfigFile(t, "etc/config-prod.yaml", "name: prod\nport: 9090\n")

	cfg, err := LoadBootstrap[loaderCfg]("prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Name != "prod" || cfg.Port != 9090 {
		t.Errorf("expected name=prod port=9090, got %+v", cfg)
	}
}

// TestLoadBootstrap_EnvExpansion covers ${VAR} expansion inside the file.
func TestLoadBootstrap_EnvExpansion(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("LOADER_TEST_TOKEN", "secret123")
	writeConfigFile(t, "etc/config.yaml", "name: app\ntoken: ${LOADER_TEST_TOKEN}\n")

	cfg, err := LoadBootstrap[loaderCfg]("dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Token != "secret123" {
		t.Errorf("expected token=secret123, got %q", cfg.Token)
	}
}

// TestLoadBootstrap_NotFound covers the error path when no config file exists.
func TestLoadBootstrap_NotFound(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	_, err := LoadBootstrap[loaderCfg]("dev")
	if err == nil {
		t.Error("expected error for missing config file")
	}
}

// TestLoadBootstrap_InvalidYAML covers the YAML parse error path.
func TestLoadBootstrap_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeConfigFile(t, "etc/config.yaml", "name: : :")

	_, err := LoadBootstrap[loaderCfg]("dev")
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}
