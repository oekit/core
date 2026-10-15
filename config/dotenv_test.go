package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := "# comment line\nDOTENV_TEST_A=alpha\nDOTENV_TEST_B = \"quoted value\"\nbroken line no equals\nDOTENV_TEST_A=ignored\n"
	if err := os.WriteFile(envPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOTENV_TEST_A", "preset") // 已设置不被覆盖

	LoadDotEnv(envPath)

	if got := os.Getenv("DOTENV_TEST_A"); got != "preset" {
		t.Fatalf("已设置变量不应被覆盖: %q", got)
	}
	if got := os.Getenv("DOTENV_TEST_B"); got != "quoted value" {
		t.Fatalf("引号应剥除: %q", got)
	}
}

func TestLoadDotEnvFirstFoundWins(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.env")
	second := filepath.Join(dir, "b.env")
	if err := os.WriteFile(first, []byte("DOTENV_TEST_C=first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("DOTENV_TEST_C=second\nDOTENV_TEST_D=second\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	LoadDotEnv(first, second) // 第一个存在即加载并返回
	if got := os.Getenv("DOTENV_TEST_C"); got != "first" {
		t.Fatalf("应加载第一个文件: %q", got)
	}
	if got := os.Getenv("DOTENV_TEST_D"); got != "" {
		t.Fatalf("不应叠加第二个文件: %q", got)
	}
}

func TestLoadDotEnvMissingSilent(t *testing.T) {
	LoadDotEnv(filepath.Join(t.TempDir(), "nonexistent.env")) // 不存在静默返回不 panic
}
