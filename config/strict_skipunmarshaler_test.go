package config

import (
	"errors"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// 带 UnmarshalYAML 的宿主子树（#127 的实弹形态）：自带专门校验与专门裁决
// 错误信息——通用严格递归对它的「帮助」是降级裁决、双判定面。
type skipHost struct {
	Name  string             `yaml:"name"`
	Extra skipUnmarshalBlock `yaml:"extra"`
}

// skipUnmarshalBlock 自带 UnmarshalYAML（指针接收者——yaml.v3 的命中形态），
// 只认 legal/loaded 两个键；它的裁决错误带域语义，与 UnknownKeyError 的
// 通用文案不同（测试靠错误形态区分是哪条判定面拦的）。
type skipUnmarshalBlock struct {
	Legal  string
	Loaded bool
}

func (b *skipUnmarshalBlock) UnmarshalYAML(value *yaml.Node) error {
	type plain skipUnmarshalBlock
	var p plain
	if err := value.Decode(&p); err != nil {
		return err
	}
	if p.Legal != "" && p.Legal != "ok" {
		return &domainVerdictError{key: p.Legal}
	}
	*b = skipUnmarshalBlock(p)
	return nil
}

// domainVerdictError 是宿主专门校验的裁决错误（形态与 UnknownKeyError 无关）。
type domainVerdictError struct{ key string }

func (e *domainVerdictError) Error() string { return "extra: domain verdict rejects " + e.key }

// TestWithSkipUnmarshalerDefaultWalksIn 缺省（不开跳过）保持历史行为：
// 通用严格递归照样进入自带 UnmarshalYAML 的类型，未知键被通用判定面拦截。
func TestWithSkipUnmarshalerDefaultWalksIn(t *testing.T) {
	data := []byte("name: a\nextra:\n  unknow_key: 1\n")
	var cfg skipHost
	err := Unmarshal(data, &cfg, WithStrict())
	if err == nil {
		t.Fatal("缺省应保持历史行为：未知键拒载")
	}
	var uke *UnknownKeyError
	if !errors.As(err, &uke) {
		t.Fatalf("应报 UnknownKeyError（通用判定面），got %v", err)
	}
	if uke.Path != "extra.unknow_key" {
		t.Fatalf("嵌套路径失真: %q", uke.Path)
	}
}

// TestWithSkipUnmarshalerSkipsHostVerdict 开启跳过后：自带 UnmarshalYAML 的
// 子树整段放行通用递归——yaml 解码把该子树交给宿主的专门裁决（本例裁决
// 放行合法内容），专门裁决成为唯一判定面。
func TestWithSkipUnmarshalerSkipsHostVerdict(t *testing.T) {
	data := []byte("name: a\nextra:\n  unknow_key: 1\n  legal: ok\n  loaded: true\n")
	var cfg skipHost
	if err := Unmarshal(data, &cfg, WithStrict(), WithSkipUnmarshaler()); err != nil {
		t.Fatalf("Unmarshaler 子树应被严格校验跳过、解码交给宿主裁决: %v", err)
	}
	if !cfg.Extra.Loaded || cfg.Extra.Legal != "ok" {
		t.Fatalf("宿主 UnmarshalYAML 未生效: %+v", cfg.Extra)
	}
}

// TestWithSkipUnmarshalerHostVerdictStillFires 跳过的只是**通用键校验**：
// 宿主专门校验的错误照常拒载（严格模式不把专门裁决一起放掉）。
func TestWithSkipUnmarshalerHostVerdictStillFires(t *testing.T) {
	data := []byte("name: a\nextra:\n  legal: bad\n")
	var cfg skipHost
	err := Unmarshal(data, &cfg, WithStrict(), WithSkipUnmarshaler())
	if err == nil {
		t.Fatal("宿主专门裁决应照常拒载")
	}
	var dve *domainVerdictError
	if !errors.As(err, &dve) {
		t.Fatalf("应是宿主裁决错误而非通用文案: %v", err)
	}
	if !strings.Contains(err.Error(), "domain verdict") {
		t.Fatalf("裁决文本失真: %v", err)
	}
}

// TestWithSkipUnmarshalerPlainTypesStillChecked 跳过只针对实现 Unmarshaler
// 的类型：普通层级的未知键照拦。
func TestWithSkipUnmarshalerPlainTypesStillChecked(t *testing.T) {
	data := []byte("name: a\nunknown_top: 1\nextra:\n  legal: ok\n")
	var cfg skipHost
	err := Unmarshal(data, &cfg, WithStrict(), WithSkipUnmarshaler())
	if err == nil {
		t.Fatal("普通层级的未知键仍应拒载")
	}
	var uke *UnknownKeyError
	if !errors.As(err, &uke) || uke.Path != "unknown_top" {
		t.Fatalf("应报顶层 UnknownKeyError: %v", err)
	}
}
