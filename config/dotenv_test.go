package config

import (
	"os"
	"path/filepath"
	"strings"
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

// 这条是 issue #60 的主判据：`os.Getenv(k) != ""` 无法区分「未设置」与
// 「设置但为空」，运维显式清空的值会被文件反向覆盖。
func TestLoadDotEnv_ExplicitEmptyNotOverridden(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envPath, []byte("DOTENV_EMPTY=fromfile\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOTENV_EMPTY", "") // 显式清空——是「已设置」，不是「未设置」

	LoadDotEnv(envPath)

	if v, ok := os.LookupEnv("DOTENV_EMPTY"); !ok || v != "" {
		t.Fatalf("显式清空的变量不得被文件覆盖: %q ok=%v", v, ok)
	}
}

// `export K=V` 是 shell 风格拷贝里最常见的形态；不剥前缀会把整行
// `export K` 当成键名写进环境（Go 允许含空格的键名），真实变量永远读不到。
func TestLoadDotEnv_ExportPrefix(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env")
	data := "export DOTENV_EXP=exp\nexport\tDOTENV_EXP_TAB=tab\nexport NOT_EQUALS\n"
	if err := os.WriteFile(envPath, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	LoadDotEnv(envPath)

	if got := os.Getenv("DOTENV_EXP"); got != "exp" {
		t.Fatalf("export 前缀应剥除: %q", got)
	}
	if got := os.Getenv("DOTENV_EXP_TAB"); got != "tab" {
		t.Fatalf("export + tab 也应剥除: %q", got)
	}
	if _, ok := os.LookupEnv("export DOTENV_EXP"); ok {
		t.Fatal("不得把 `export K` 当成键名写进环境（垃圾变量）")
	}
}

func TestLoadDotEnv_ValueForms(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env")
	data := strings.Join([]string{
		`DOTENV_V1=a # 行内注释`,
		`DOTENV_V2="a # b"`,
		`DOTENV_V3=abc#123`,
		`DOTENV_V4=#整行是注释`,
		`DOTENV_V5="quoted" # 引号后是注释`,
		`DOTENV_V6='single'`,
		`DOTENV_V7="unclosed`,
		`DOTENV_V8=`,
		`DOTENV_V9="a" "b"`,
	}, "\n") + "\n"
	if err := os.WriteFile(envPath, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	LoadDotEnv(envPath)

	for _, tc := range []struct{ key, want string }{
		{"DOTENV_V1", "a"},         // 行内注释被切掉
		{"DOTENV_V2", "a # b"},     // 引号内的 # 不是注释
		{"DOTENV_V3", "abc#123"},   // 不带空白的 # 属于值本身（口令里常见）
		{"DOTENV_V4", ""},          // 等号后直接是注释 ⇒ 空值
		{"DOTENV_V5", "quoted"},    // 引号后才算注释（旧实现剥成 `quoted" # 注释`）
		{"DOTENV_V6", "single"},    // 单引号同样剥
		{"DOTENV_V7", `"unclosed`}, // 未闭合按字面，不吞内容
		{"DOTENV_V8", ""},          // 空值就是空值
		{"DOTENV_V9", "a"},         // 引号后内容忽略
	} {
		if got := os.Getenv(tc.key); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestLoadDotEnv_CRLF(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envPath, []byte("DOTENV_CRLF=win\r\nDOTENV_CRLF_Q=\"q\"\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	LoadDotEnv(envPath)

	if got := os.Getenv("DOTENV_CRLF"); got != "win" {
		t.Fatalf("CRLF 文件不应把 \\r 带进值: %q", got)
	}
	if got := os.Getenv("DOTENV_CRLF_Q"); got != "q" {
		t.Fatalf("CRLF + 引号应正常剥除: %q", got)
	}
}

func TestLoadDotEnv_InvalidKeySkipped(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envPath, []byte("bad key=v\n=novalue\nDOTENV_OK=ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	LoadDotEnv(envPath)

	if _, ok := os.LookupEnv("bad key"); ok {
		t.Fatal("键里带空白是非法行，不得写进环境")
	}
	if got := os.Getenv("DOTENV_OK"); got != "ok" {
		t.Fatalf("合法行仍应生效: %q", got)
	}
}
