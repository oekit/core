package config

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// core#139 的护栏：config 包三类失败必须能用导出哨兵 errors.Is 分流，
// 且所有历史错误文案逐字不变（entrypoints_test 钉住 LoadBootstrap 面，
// 本文件钉 Unmarshal/LoadFile 面与哨兵可达性）。

// TestSentinel_ParseError YAML 语法损坏 → ErrParse。
func TestSentinel_ParseError(t *testing.T) {
	var cfg struct {
		Port int `yaml:"port"`
	}
	err := Unmarshal([]byte("{"), &cfg)
	if err == nil {
		t.Fatal("损坏 YAML 应拒载")
	}
	if !errors.Is(err, ErrParse) {
		t.Fatalf("解析失败应可 errors.Is(err, ErrParse)，实际：%v", err)
	}
	if !strings.HasPrefix(err.Error(), "parse config: ") {
		t.Fatalf("Unmarshal 解析失败文案应保持「parse config: ...」逐字不变，实际：%q", err.Error())
	}
}

// TestSentinel_TagPassError tag 接线失败（range 越界）→ ErrTagPass。
func TestSentinel_TagPassError(t *testing.T) {
	var cfg struct {
		Port int `yaml:"port" range:"[1,65535]"`
	}
	err := Unmarshal([]byte("port: 0\n"), &cfg)
	if err == nil {
		t.Fatal("range 越界应拒载")
	}
	if !errors.Is(err, ErrTagPass) {
		t.Fatalf("tag 接线失败应可 errors.Is(err, ErrTagPass)，实际：%v", err)
	}
	if !strings.HasPrefix(err.Error(), "apply struct tags: ") {
		t.Fatalf("Unmarshal tag 失败文案应保持「apply struct tags: ...」逐字不变，实际：%q", err.Error())
	}
}

// TestSentinel_TagPassErrorViaLoadFile LoadFile 的 tag 失败：文案带路径前缀
// 「apply struct tags for config file X: ...」（与 LoadBootstrap 历史文案同形）。
func TestSentinel_TagPassErrorViaLoadFile(t *testing.T) {
	path := t.TempDir() + "/sentinel-tag.yaml"
	if err := os.WriteFile(path, []byte("port: 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Port int `yaml:"port" range:"[1,65535]"`
	}
	err := LoadFile(path, &cfg)
	if err == nil {
		t.Fatal("range 越界应拒载")
	}
	if !errors.Is(err, ErrTagPass) {
		t.Fatalf("LoadFile tag 失败应可 errors.Is(err, ErrTagPass)，实际：%v", err)
	}
	if !strings.HasPrefix(err.Error(), "apply struct tags for config file "+path+": ") {
		t.Fatalf("LoadFile tag 失败文案应保持「apply struct tags for config file X: ...」逐字不变，实际：%q", err.Error())
	}
}

// TestSentinel_EnvExpandError env 注入安全拒载 → ErrEnvExpand（fail-closed
// 拒载静默损坏值，core#59/#83）。
func TestSentinel_EnvExpandError(t *testing.T) {
	t.Setenv("SENTINEL_EXPAND_VAR", `C:\temp`) // 反斜杠：双引号标量里被重新解释，一律拒载
	var cfg struct {
		Token string `yaml:"token"`
	}
	err := Unmarshal([]byte(`token: "${SENTINEL_EXPAND_VAR}"`), &cfg)
	if err == nil {
		t.Fatal("env 值含反斜杠应拒载")
	}
	if !errors.Is(err, ErrEnvExpand) {
		t.Fatalf("env 注入拒载应可 errors.Is(err, ErrEnvExpand)，实际：%v", err)
	}
	// 拒载文案逐字不变（含变量名与改道提示）
	if !strings.Contains(err.Error(), "expand ${SENTINEL_EXPAND_VAR}: env value contains a backslash") ||
		!strings.Contains(err.Error(), "inject it via ApplyEnvOverlay instead") {
		t.Fatalf("env 拒载文案应逐字保持，实际：%q", err.Error())
	}
}

// TestSentinel_UnknownKeyStillAs 哨兵挂链不得破坏既有可编程分流面：严格模式
// 的 *UnknownKeyError 经 parseErr 通道，errors.As 必须依旧直达，且同时可
// errors.Is(err, ErrParse)。
func TestSentinel_UnknownKeyStillAs(t *testing.T) {
	var cfg struct {
		Port int `yaml:"port"`
	}
	err := Unmarshal([]byte("port: 8080\nunknwon: 1\n"), &cfg, WithStrict())
	if err == nil {
		t.Fatal("严格模式未知键应拒载")
	}
	var uke *UnknownKeyError
	if !errors.As(err, &uke) {
		t.Fatalf("严格模式错误应可 errors.As(*UnknownKeyError)，实际：%v", err)
	}
	if !errors.Is(err, ErrParse) {
		t.Fatalf("严格模式拒载属解析线，应可 errors.Is(err, ErrParse)，实际：%v", err)
	}
}

// TestSentinel_ThreeCategoriesExclusive 三类哨兵互斥：一次失败只挂一个，
// 防止「永远命中第一类」的假分流。
func TestSentinel_ThreeCategoriesExclusive(t *testing.T) {
	var cfg struct {
		Port int `yaml:"port" range:"[1,65535]"`
	}
	tagErr := Unmarshal([]byte("port: 0\n"), &cfg)
	if errors.Is(tagErr, ErrParse) || errors.Is(tagErr, ErrEnvExpand) {
		t.Fatalf("tag 失败不应同时命中其他类：%v", tagErr)
	}

	var cfg2 struct {
		Token string `yaml:"token"`
	}
	t.Setenv("SENTINEL_EXPAND_VAR2", `C:\temp`)
	expandErr := Unmarshal([]byte(`token: "${SENTINEL_EXPAND_VAR2}"`), &cfg2)
	if errors.Is(expandErr, ErrParse) || errors.Is(expandErr, ErrTagPass) {
		t.Fatalf("env 拒载不应同时命中其他类：%v", expandErr)
	}

	var cfg3 struct {
		Port int `yaml:"port"`
	}
	parseErr := Unmarshal([]byte("{"), &cfg3)
	if errors.Is(parseErr, ErrTagPass) || errors.Is(parseErr, ErrEnvExpand) {
		t.Fatalf("解析失败不应同时命中其他类：%v", parseErr)
	}
}
