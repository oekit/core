package config

import (
	"errors"
	"strings"
	"testing"
)

// ---- 严格模式（#20）：未知键拒载 + 合法键清单 + 嵌套路径 ----

type strictInner struct {
	BaseURL   string `yaml:"base_url"`
	TimeoutMS int    `yaml:"timeout_ms"`
}

type strictOuter struct {
	LLM struct {
		APIKey string `yaml:"api_key"`
	} `yaml:"llm"`
	DB strictInner `yaml:"db"`
	// 内嵌结构体（无 yaml 名）→ yaml.v3 语义：拍平到本层
	strictInner `yaml:",inline"`
}

func TestStrictRejectsUnknownKeyWithPathAndLegalKeys(t *testing.T) {
	data := []byte(`
llm:
  api_key: k
db:
  base_url: http://x
  base_ur: http://typo
`)
	var cfg strictOuter
	err := unmarshalConfig(data, &cfg, newLoadOptions([]LoadOption{WithStrict()}))
	if err == nil {
		t.Fatalf("期望未知键被拒载")
	}
	var uerr *UnknownKeyError
	if !errors.As(err, &uerr) {
		t.Fatalf("期望 *UnknownKeyError，实际 %T: %v", err, err)
	}
	if uerr.Path != "db.base_ur" {
		t.Fatalf("期望嵌套路径 db.base_ur，实际 %q", uerr.Path)
	}
	if uerr.Key != "base_ur" {
		t.Fatalf("期望键名 base_ur，实际 %q", uerr.Key)
	}
	// 合法键清单必须含本层全部合法键
	legal := strings.Join(uerr.Legal, ",")
	if !strings.Contains(legal, "base_url") || !strings.Contains(legal, "timeout_ms") {
		t.Fatalf("合法键清单缺 base_url/timeout_ms：%v", uerr.Legal)
	}
	// 近似拼写建议
	if uerr.Near != "base_url" {
		t.Fatalf("期望拼写建议 base_url，实际 %q", uerr.Near)
	}
	msg := err.Error()
	if !strings.Contains(msg, "db.base_ur") {
		t.Fatalf("错误信息应含路径：%s", msg)
	}
	if !strings.Contains(msg, "base_url") {
		t.Fatalf("错误信息应含合法键：%s", msg)
	}
}

func TestStrictAcceptsLegalKeys(t *testing.T) {
	data := []byte(`
llm:
  api_key: k
db:
  base_url: http://x
  timeout_ms: 100
base_url: http://inline
timeout_ms: 200
`)
	var cfg strictOuter
	if err := unmarshalConfig(data, &cfg, newLoadOptions([]LoadOption{WithStrict()})); err != nil {
		t.Fatalf("合法键不应报错：%v", err)
	}
	if cfg.DB.BaseURL != "http://x" {
		t.Fatalf("嵌套键未正确解码：%+v", cfg.DB)
	}
	// inline 拍平的键也要落到内嵌结构体
	if cfg.BaseURL != "http://inline" {
		t.Fatalf("inline 键未正确解码：%+v", cfg.strictInner)
	}
}

// 非严格模式必须保持历史行为：未知键静默忽略（不报错）。
func TestNonStrictIgnoresUnknownKey(t *testing.T) {
	data := []byte(`
db:
  base_url: http://x
  totally_unknown: 1
`)
	var cfg strictOuter
	if err := unmarshalConfig(data, &cfg, newLoadOptions(nil)); err != nil {
		t.Fatalf("非严格模式不应因未知键报错：%v", err)
	}
}

func TestStrictTopLevelUnknownKey(t *testing.T) {
	data := []byte("llm_typo:\n  api_key: k\n")
	var cfg strictOuter
	err := unmarshalConfig(data, &cfg, newLoadOptions([]LoadOption{WithStrict()}))
	if err == nil {
		t.Fatalf("期望顶层未知键被拒载")
	}
	var uerr *UnknownKeyError
	if !errors.As(err, &uerr) {
		t.Fatalf("期望 *UnknownKeyError，实际 %T", err)
	}
	if uerr.Path != "llm_typo" {
		t.Fatalf("期望路径 llm_typo，实际 %q", uerr.Path)
	}
}

// map 的键由调用方定义——任意键都合法，不能误报；但 map 值的结构体内部仍要查。
func TestStrictMapKeysAllowedButValuesChecked(t *testing.T) {
	type item struct {
		Name string `yaml:"name"`
	}
	type withMap struct {
		Items map[string]item `yaml:"items"`
	}
	var ok withMap
	if err := unmarshalConfig([]byte("items:\n  anything:\n    name: x\n"), &ok,
		newLoadOptions([]LoadOption{WithStrict()})); err != nil {
		t.Fatalf("map 的任意键应合法：%v", err)
	}
	var bad withMap
	err := unmarshalConfig([]byte("items:\n  anything:\n    nmae: x\n"), &bad, //nolint:misspell // 故意拼错的键名
		newLoadOptions([]LoadOption{WithStrict()}))
	if err == nil {
		t.Fatalf("map 值内部的未知键应被拒载")
	}
}

// 切片元素里的未知键也要查出来。
func TestStrictSliceElementChecked(t *testing.T) {
	type entry struct {
		Host string `yaml:"host"`
	}
	type withSlice struct {
		Entries []entry `yaml:"entries"`
	}
	var bad withSlice
	err := unmarshalConfig([]byte("entries:\n  - host: a\n    prot: b\n"), &bad,
		newLoadOptions([]LoadOption{WithStrict()}))
	if err == nil {
		t.Fatalf("切片元素内的未知键应被拒载")
	}
	var uerr *UnknownKeyError
	if !errors.As(err, &uerr) {
		t.Fatalf("期望 *UnknownKeyError，实际 %T", err)
	}
	if uerr.Key != "prot" {
		t.Fatalf("期望键名 prot，实际 %q", uerr.Key)
	}
}

// 空配置不该报错。
func TestStrictEmptyData(t *testing.T) {
	var cfg strictOuter
	if err := unmarshalConfig([]byte(""), &cfg, newLoadOptions([]LoadOption{WithStrict()})); err != nil {
		t.Fatalf("空配置不应报错：%v", err)
	}
	if err := unmarshalConfig([]byte("# only a comment\n"), &cfg,
		newLoadOptions([]LoadOption{WithStrict()})); err != nil {
		t.Fatalf("注释-only 配置不应报错：%v", err)
	}
}

func TestNearestKey(t *testing.T) {
	legal := []string{"base_url", "timeout_ms", "api_key"}
	if got := nearestKey("base_ur", legal); got != "base_url" {
		t.Fatalf("期望 base_url，实际 %q", got)
	}
	if got := nearestKey("totally_different", legal); got != "" {
		t.Fatalf("差异过大时不该给建议，实际 %q", got)
	}
}

// TestStrictMatchesYAMLv3FieldSemantics 把「严格模式的合法键判定必须与 yaml.v3
// 的字段语义一致」钉死——不一致的后果是**把合法键误报成未知键**。
//
// 实测确认的 yaml.v3 行为（2026-10-20）：
//  1. 内嵌**未导出**类型 + `yaml:",inline"` → 字段照样被拍平解码（私有字段只
//     在「非匿名」时才被跳过）；
//  2. 内嵌**无 yaml tag** → **不**拍平，被当作普通字段，键名是类型名的小写。
func TestStrictMatchesYAMLv3FieldSemantics(t *testing.T) {
	type inner struct {
		BaseURL string `yaml:"base_url"`
	}
	type withInline struct {
		inner `yaml:",inline"`
	}
	type withoutTag struct {
		inner
	}

	// ① 内嵌未导出类型 + inline：base_url 是**顶层合法键**
	var a withInline
	if err := unmarshalConfig([]byte("base_url: http://x\n"), &a,
		newLoadOptions([]LoadOption{WithStrict()})); err != nil {
		t.Fatalf("inline 内嵌未导出类型的键应合法：%v", err)
	}
	if a.BaseURL != "http://x" {
		t.Fatalf("inline 键未解码：%+v", a)
	}

	// ② 内嵌无 tag：base_url **不是**顶层合法键（yaml.v3 也不拍平）
	var b withoutTag
	err := unmarshalConfig([]byte("base_url: http://x\n"), &b,
		newLoadOptions([]LoadOption{WithStrict()}))
	if err == nil {
		t.Fatalf("无 tag 的内嵌字段应按普通字段处理（键名 = 类型名小写），base_url 不该合法")
	}
	var uerr *UnknownKeyError
	if !errors.As(err, &uerr) {
		t.Fatalf("期望 *UnknownKeyError，实际 %T", err)
	}
	if uerr.Key != "base_url" {
		t.Fatalf("期望键名 base_url，实际 %q", uerr.Key)
	}
	// 合法键应是类型名小写
	if len(uerr.Legal) != 1 || uerr.Legal[0] != "inner" {
		t.Fatalf("合法键应为 [inner]，实际 %v", uerr.Legal)
	}
}
