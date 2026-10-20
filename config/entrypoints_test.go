package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- 三个入口同源（core#25）：字节级 / 任意路径 / etc 约定 ----

type entryInner struct {
	BaseURL   string `yaml:"base_url"`
	TimeoutMS int    `yaml:"timeout_ms"`
}

type entryCfg struct {
	LLM struct {
		APIKey string `yaml:"api_key"`
	} `yaml:"llm"`
	DB   entryInner `yaml:"db"`
	Port int        `yaml:"port" env:"ENTRY_PORT" default:"8080"`
}

// TestUnmarshalStrictSameDiagnosticsAsFilePath 是「严格模式诊断三入口同源」的核心断言：
// 字节级入口在未知键上给出的错误，与文件路径版**逐字同型**（同样的
// *UnknownKeyError、同样的嵌套路径、同样的合法键清单、同样的拼写建议）。
func TestUnmarshalStrictSameDiagnosticsAsFilePath(t *testing.T) {
	data := []byte("db:\n  base_url: http://x\n  base_ur: http://typo\n")

	var byBytes entryCfg
	errBytes := Unmarshal(data, &byBytes, WithStrict())
	if errBytes == nil {
		t.Fatalf("字节级入口应拒载未知键")
	}
	var uerr *UnknownKeyError
	if !errors.As(errBytes, &uerr) {
		t.Fatalf("期望 *UnknownKeyError，实际 %T: %v", errBytes, errBytes)
	}
	if uerr.Path != "db.base_ur" {
		t.Fatalf("期望路径 db.base_ur，实际 %q", uerr.Path)
	}
	if uerr.Near != "base_url" {
		t.Fatalf("期望拼写建议 base_url，实际 %q", uerr.Near)
	}
	if len(uerr.Legal) == 0 {
		t.Fatalf("合法键清单不应为空")
	}

	// 文件路径版：同样的输入，诊断必须一致
	dir := t.TempDir()
	path := filepath.Join(dir, "plugins.yaml")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var byFile entryCfg
	errFile := LoadFile(path, &byFile, WithStrict())
	if errFile == nil {
		t.Fatalf("路径级入口应拒载未知键")
	}
	var uerr2 *UnknownKeyError
	if !errors.As(errFile, &uerr2) {
		t.Fatalf("期望 *UnknownKeyError，实际 %T", errFile)
	}
	if uerr2.Path != uerr.Path || uerr2.Near != uerr.Near || uerr2.Key != uerr.Key {
		t.Fatalf("两入口诊断不一致：\n  字节级: path=%q near=%q key=%q\n  路径级: path=%q near=%q key=%q",
			uerr.Path, uerr.Near, uerr.Key, uerr2.Path, uerr2.Near, uerr2.Key)
	}
	if strings.Join(uerr2.Legal, ",") != strings.Join(uerr.Legal, ",") {
		t.Fatalf("两入口合法键清单不一致")
	}
}

// TestLoadFileReadsArbitraryPath 覆盖「不遵循 etc/ 约定的宿主」。
func TestLoadFileReadsArbitraryPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app", "plugins.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("port: 9090\ndb:\n  base_url: http://x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var cfg entryCfg
	if err := LoadFile(path, &cfg); err != nil {
		t.Fatalf("任意路径应能加载：%v", err)
	}
	if cfg.Port != 9090 {
		t.Fatalf("yaml 显式值应生效，实际 %d", cfg.Port)
	}
	if cfg.DB.BaseURL != "http://x" {
		t.Fatalf("嵌套值未生效：%+v", cfg.DB)
	}
}

// TestUnmarshalAppliesTagPasses 字节级入口必须同样走 tag 接线
// （ApplyEnv → FillDefaults → ValidateTags），否则「同一份数据、两个入口、
// 不同默认值」是最难查的一类不一致。
func TestUnmarshalAppliesTagPasses(t *testing.T) {
	t.Setenv("ENTRY_PORT", "7777")
	var cfg entryCfg
	// yaml 未给 port → env tag 生效（env tag 优先于 default）
	if err := Unmarshal([]byte("db:\n  base_url: http://x\n"), &cfg); err != nil {
		t.Fatalf("字节级入口应成功：%v", err)
	}
	if cfg.Port != 7777 {
		t.Fatalf("env tag 应生效，实际 %d", cfg.Port)
	}
	// env 未设 → default tag 生效
	os.Unsetenv("ENTRY_PORT")
	var cfg2 entryCfg
	if err := Unmarshal([]byte("db:\n  base_url: http://x\n"), &cfg2); err != nil {
		t.Fatalf("字节级入口应成功：%v", err)
	}
	if cfg2.Port != 8080 {
		t.Fatalf("default tag 应生效，实际 %d", cfg2.Port)
	}
}

// TestUnmarshalNonStrictIgnoresUnknownKey 默认行为不变：未知键静默忽略。
func TestUnmarshalNonStrictIgnoresUnknownKey(t *testing.T) {
	var cfg entryCfg
	if err := Unmarshal([]byte("db:\n  base_url: http://x\n  totally_unknown: 1\n"), &cfg); err != nil {
		t.Fatalf("非严格模式不应因未知键报错：%v", err)
	}
}

// TestUnmarshalExpandsEnvPlaceholders 字节级入口同样展开 ${VAR}。
func TestUnmarshalExpandsEnvPlaceholders(t *testing.T) {
	t.Setenv("ENTRY_URL", "http://from-env")
	var cfg entryCfg
	if err := Unmarshal([]byte("db:\n  base_url: ${ENTRY_URL}\n"), &cfg); err != nil {
		t.Fatalf("字节级入口应成功：%v", err)
	}
	if cfg.DB.BaseURL != "http://from-env" {
		t.Fatalf("占位符未展开：%q", cfg.DB.BaseURL)
	}
}

// TestLoadBootstrapErrorTextUnchanged 既有调用方看到的错误文案必须逐字不变：
// 「parse config file X: ...」与「apply struct tags for config file X: ...」。
func TestLoadBootstrapErrorTextUnchanged(t *testing.T) {
	// 造一个 etc/config.yaml 未知键 → 应是「parse config file」而非 tag 报错
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "etc", "config.yaml"),
		[]byte("db:\n  base_ur: http://typo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	_, err := LoadBootstrap[entryCfg]("dev", WithStrict())
	if err == nil {
		t.Fatalf("未知键应被拒载")
	}
	if !strings.Contains(err.Error(), "parse config file etc/config.yaml:") {
		t.Fatalf("错误文案应保持「parse config file etc/config.yaml: ...」，实际：%v", err)
	}
}
