package config

import (
	"errors"
	"strings"
	"testing"
)

// TestExpandEnv_FailClosedOnLeadingAnchor 回归 core#231：env 值以 `&` 开头
// 时，替换后的 YAML 把整个标量当成锚点——`${DB_PASSWORD}` + `&foo` 解析成
// null、`&foo secret` 被截成 `secret`，解析成功但值已静默损坏。现与
// 换行/注释符同口径 fail-closed，报错点名变量。
func TestExpandEnv_FailClosedOnLeadingAnchor(t *testing.T) {
	t.Setenv("DB_PASSWORD", "&foo")
	var cfg envExpandCfg
	err := Unmarshal([]byte("password: ${DB_PASSWORD}\n"), &cfg)
	if err == nil {
		t.Fatalf("env value starting with '&' must fail closed, got decoded=%q", cfg.Password)
	}
	if !errors.Is(err, ErrEnvExpand) {
		t.Errorf("error must carry ErrEnvExpand, got %v", err)
	}
	if !strings.Contains(err.Error(), "DB_PASSWORD") || !strings.Contains(err.Error(), "leading '&'") {
		t.Errorf("error must name DB_PASSWORD and leading '&', got %v", err)
	}
	if cfg.Password != "" {
		t.Errorf("corrupted value leaked into field: %q", cfg.Password)
	}

	// `&foo secret` 形态：旧行为解析成功并截成 "secret"（锚点名之后才是值）。
	t.Setenv("DB_PASSWORD2", "&foo secret")
	var cfg2 envExpandCfg
	err = Unmarshal([]byte("password: ${DB_PASSWORD2}\n"), &cfg2)
	if err == nil {
		t.Fatalf("anchored scalar must fail closed, got decoded=%q", cfg2.Password)
	}
	if !strings.Contains(err.Error(), "DB_PASSWORD2") {
		t.Errorf("error must name DB_PASSWORD2, got %v", err)
	}
}

// TestExpandEnv_FailClosedOnLeadingAlias 回归 core#231：env 值以 `*` 开头
// 会被 YAML 当成别名引用（取值来自锚点节点，而非字面量），同样 fail-closed。
func TestExpandEnv_FailClosedOnLeadingAlias(t *testing.T) {
	t.Setenv("DB_PASSWORD", "*foo")
	var cfg envExpandCfg
	err := Unmarshal([]byte("password: ${DB_PASSWORD}\n"), &cfg)
	if err == nil {
		t.Fatalf("env value starting with '*' must fail closed, got decoded=%q", cfg.Password)
	}
	if !errors.Is(err, ErrEnvExpand) {
		t.Errorf("error must carry ErrEnvExpand, got %v", err)
	}
	if !strings.Contains(err.Error(), "DB_PASSWORD") || !strings.Contains(err.Error(), "leading '*'") {
		t.Errorf("error must name DB_PASSWORD and leading '*', got %v", err)
	}
	if cfg.Password != "" {
		t.Errorf("corrupted value leaked into field: %q", cfg.Password)
	}
}

// TestExpandEnv_NonLeadingAnchorCharsStillExpand 对照：`&`/`*` 只在前导位
// 触发 YAML 锚点/别名语义；出现在值中间时是普通字符，不得误拦。
func TestExpandEnv_NonLeadingAnchorCharsStillExpand(t *testing.T) {
	t.Setenv("PROBE_AMP_MID", "Tom & Jerry")
	t.Setenv("PROBE_STAR_MID", "5*3")
	var cfg envExpandCfg
	err := Unmarshal([]byte("token: \"${PROBE_AMP_MID}\"\ndsn: \"${PROBE_STAR_MID}\"\n"), &cfg)
	if err != nil {
		t.Fatalf("non-leading '&'/'*' must keep expanding, got %v", err)
	}
	if cfg.Token != "Tom & Jerry" || cfg.DSN != "5*3" {
		t.Errorf("Token = %q, DSN = %q", cfg.Token, cfg.DSN)
	}
}
