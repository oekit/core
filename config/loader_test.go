package config

import (
	"os"
	"strings"
	"testing"
)

// mustExpandEnv 断言展开成功并返回结果文本（fail-closed 用例见下方单独测试）。
func mustExpandEnv(t *testing.T, input []byte) string {
	t.Helper()
	out, err := expandEnv(input)
	if err != nil {
		t.Fatalf("expandEnv unexpected error: %v", err)
	}
	return string(out)
}

func TestExpandEnv_Basic(t *testing.T) {
	t.Setenv("TEST_USER", "alice")
	input := []byte(`user: ${TEST_USER}`)
	got := mustExpandEnv(t, input)
	if got != "user: alice" {
		t.Errorf("expected 'user: alice', got %q", got)
	}
}

func TestExpandEnv_Default(t *testing.T) {
	os.Unsetenv("TEST_MISSING")
	input := []byte(`pass: ${TEST_MISSING:default123}`)
	got := mustExpandEnv(t, input)
	if got != "pass: default123" {
		t.Errorf("expected 'pass: default123', got %q", got)
	}
}

func TestExpandEnv_EnvOverridesDefault(t *testing.T) {
	t.Setenv("TEST_OVERRIDE", "real")
	input := []byte(`val: ${TEST_OVERRIDE:fallback}`)
	got := mustExpandEnv(t, input)
	if got != "val: real" {
		t.Errorf("expected 'val: real', got %q", got)
	}
}

func TestExpandEnv_Multiple(t *testing.T) {
	t.Setenv("A", "1")
	t.Setenv("B", "2")
	input := []byte(`a: ${A}, b: ${B}`)
	got := mustExpandEnv(t, input)
	if got != "a: 1, b: 2" {
		t.Errorf("expected 'a: 1, b: 2', got %q", got)
	}
}

func TestExpandEnv_NoPlaceholder(t *testing.T) {
	input := []byte(`plain: text`)
	got := mustExpandEnv(t, input)
	if got != "plain: text" {
		t.Errorf("expected unchanged, got %q", got)
	}
}

func TestExpandEnv_EmptyDefault(t *testing.T) {
	os.Unsetenv("TEST_EMPTY_DEF")
	input := []byte(`x: ${TEST_EMPTY_DEF:}`)
	got := mustExpandEnv(t, input)
	if got != "x: " {
		t.Errorf("expected 'x: ', got %q", got)
	}
}

func TestExpandEnv_UndefinedNoDefault(t *testing.T) {
	os.Unsetenv("TEST_NODEF")
	input := []byte(`x: ${TEST_NODEF}`)
	got := mustExpandEnv(t, input)
	if got != "x: ${TEST_NODEF}" {
		t.Errorf("expected placeholder preserved, got %q", got)
	}
}

// ---- core#59：替换值含 YAML 特殊字符时 fail-closed，不再静默损坏 ----

type envExpandCfg struct {
	Password string `yaml:"password"`
	Token    string `yaml:"token"`
	DSN      string `yaml:"dsn"`
}

func TestUnmarshal_ExpandEnvFailsClosedOnLineBreak(t *testing.T) {
	// 复现自 issue：PROBE_PW="secret\nline2" + password: "${PROBE_PW}"
	// 旧行为：解析成功、换行被折叠为空格（Password="secret line2"，静默损坏）。
	t.Setenv("PROBE_PW", "secret\nline2")
	var cfg envExpandCfg
	err := Unmarshal([]byte(`password: "${PROBE_PW}"`+"\n"), &cfg)
	if err == nil {
		t.Fatal("expected fail-closed error for env value with line break")
	}
	if !strings.Contains(err.Error(), "PROBE_PW") {
		t.Errorf("error should name the offending env var, got %v", err)
	}
	if cfg.Password != "" {
		t.Errorf("corrupted value must not land in config, got %q", cfg.Password)
	}
}

func TestUnmarshal_ExpandEnvFailsClosedOnCommentSequence(t *testing.T) {
	// 复现自 issue：PROBE_TOKEN="abc # trailing" + token: ${PROBE_TOKEN}
	// 旧行为：解析成功、" # trailing" 被当注释截断（Token="abc"，静默损坏）。
	t.Setenv("PROBE_TOKEN", "abc # trailing")
	var cfg envExpandCfg
	err := Unmarshal([]byte("token: ${PROBE_TOKEN}\n"), &cfg)
	if err == nil {
		t.Fatal("expected fail-closed error for env value containing ' #'")
	}
	if !strings.Contains(err.Error(), "PROBE_TOKEN") {
		t.Errorf("error should name the offending env var, got %v", err)
	}
}

func TestExpandEnv_FailClosedOnLeadingHashAndCR(t *testing.T) {
	t.Setenv("PROBE_HASH", "#notavalue")
	if _, err := expandEnv([]byte("token: ${PROBE_HASH}\n")); err == nil {
		t.Error("expected error for leading '#' (whole value becomes a comment)")
	}
	t.Setenv("PROBE_CR", "abc\rdef")
	if _, err := expandEnv([]byte("token: ${PROBE_CR}\n")); err == nil {
		t.Error("expected error for carriage return (line folding)")
	}
}

func TestExpandEnv_SafeValuesStillExpand(t *testing.T) {
	// 常规值（含 URL 的冒号、井号不前导空格、空格分隔多词）不受拦截影响。
	t.Setenv("PROBE_DSN", "postgres://user:pass@host:5432/db")
	t.Setenv("PROBE_HASH_IN_WORD", "a#b")
	t.Setenv("PROBE_WORDS", "abc def ghi")
	var cfg envExpandCfg
	err := Unmarshal([]byte("dsn: ${PROBE_DSN}\ntoken: \"${PROBE_HASH_IN_WORD} ${PROBE_WORDS}\"\n"), &cfg)
	if err != nil {
		t.Fatalf("safe values must keep expanding, got %v", err)
	}
	if cfg.DSN != "postgres://user:pass@host:5432/db" {
		t.Errorf("DSN = %q", cfg.DSN)
	}
	if cfg.Token != "a#b abc def ghi" {
		t.Errorf("Token = %q", cfg.Token)
	}
}

func TestLoadFile_ExpandEnvFailClosedCarriesPath(t *testing.T) {
	t.Setenv("PROBE_PW", "secret\nline2")
	dir := t.TempDir()
	path := dir + "/config.yaml"
	if err := os.WriteFile(path, []byte("password: \"${PROBE_PW}\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var cfg envExpandCfg
	err := LoadFile(path, &cfg)
	if err == nil {
		t.Fatal("expected fail-closed error via LoadFile")
	}
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "PROBE_PW") {
		t.Errorf("error should carry file path and env var name, got %v", err)
	}
}

// TestExpandEnvBackslashIntercepted 回归 core#83：反斜杠此前不在损坏拦截清单
// 里——双引号标量里的 env 值 `\n` 被 YAML 重新解释成真换行，值被静默改写
// （"a\nb" len4 变 len3）；Windows 路径 C:\temp 触发 unknown escape 的报错也
// 不带变量名。现含反斜杠一律 fail-closed，报错带变量名。
func TestExpandEnvBackslashIntercepted(t *testing.T) {
	type cfg struct {
		Secret string `yaml:"secret"`
	}
	t.Setenv("TOKEN_X", `a\nb`)
	var c cfg
	err := Unmarshal([]byte(`secret: "${TOKEN_X}"`), &c)
	if err == nil {
		t.Fatalf("env value with backslash must fail closed, got decoded=%q", c.Secret)
	}
	if !strings.Contains(err.Error(), "TOKEN_X") {
		t.Errorf("error must name the variable, got: %v", err)
	}
	if !strings.Contains(err.Error(), "backslash") {
		t.Errorf("error must mention backslash, got: %v", err)
	}
	// 静默改写面收口：绝不能把被重释后的值写进字段
	if c.Secret != "" {
		t.Errorf("silently rewritten value leaked into field: %q", c.Secret)
	}

	// Windows 路径形态同样拦截（曾报 unknown escape 且不带变量名）
	t.Setenv("WIN_PATH", `C:\temp\data`)
	var c2 cfg
	err = Unmarshal([]byte(`secret: "${WIN_PATH}"`), &c2)
	if err == nil || !strings.Contains(err.Error(), "WIN_PATH") {
		t.Errorf("windows-style path must fail closed naming WIN_PATH, got: %v", err)
	}
}

// TestEnvParseErrorNamesVariable 回归 core#83 第二点：env 注入的值破坏 YAML
// 语法（如双引号标量里的 `"` 破坏配对）时，解析错误必须点名变量——裸的
// yaml 语法错误指不到现场。
func TestEnvParseErrorNamesVariable(t *testing.T) {
	type cfg struct {
		Secret string `yaml:"secret"`
	}
	t.Setenv("TOKEN_Y", `abc"def`) // 注入引号破坏双引号标量配对
	var c cfg
	err := Unmarshal([]byte(`secret: "${TOKEN_Y}"`), &c)
	if err == nil {
		t.Fatal("quote-bearing env value must break the double-quoted scalar")
	}
	if !strings.Contains(err.Error(), "TOKEN_Y") {
		t.Errorf("parse error must name the env variable, got: %v", err)
	}
	// 对照：没发生替换时错误保持原样，不掺 env 提示
	err2 := Unmarshal([]byte(`secret: "abc"def"`), &c)
	if err2 == nil || strings.Contains(err2.Error(), "env placeholder") {
		t.Errorf("error without expansion must stay raw, got: %v", err2)
	}
}
