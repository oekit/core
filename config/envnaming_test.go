package config

import (
	"strings"
	"testing"
)

// ---- env 命名策略：全局前缀 ----

func TestEnvPrefix_AppliesToDerivedPathName(t *testing.T) {
	// 前缀 + 路径派生名：yaml 键 upload.local_import.enabled
	// → PB_UPLOAD_LOCAL_IMPORT_ENABLED
	t.Setenv("PB_UPLOAD_LOCAL_IMPORT_ENABLED", "true")
	var cfg struct {
		Upload struct {
			LocalImport struct {
				Enabled bool `yaml:"enabled"`
			} `yaml:"local_import"`
		} `yaml:"upload"`
	}
	if err := ApplyEnv(&cfg, WithEnvNaming(EnvNaming{Prefix: "PB_"})); err != nil {
		t.Fatalf("ApplyEnv: %v", err)
	}
	if !cfg.Upload.LocalImport.Enabled {
		t.Error("Enabled should be read from PB_UPLOAD_LOCAL_IMPORT_ENABLED")
	}
}

func TestEnvPrefix_AppliesToEnvTagName(t *testing.T) {
	// 前缀也加在显式 env tag 名之前：tag PORT → PB_PORT
	t.Setenv("PB_PORT", "8080")
	var cfg struct {
		Port int `yaml:"port" env:"PORT"`
	}
	if err := ApplyEnv(&cfg, WithEnvNaming(EnvNaming{Prefix: "PB_"})); err != nil {
		t.Fatalf("ApplyEnv: %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
}

func TestEnvPrefix_UnprefixedNameNotRead(t *testing.T) {
	// 加了前缀就只读带前缀的名字：没加前缀的同名变量不生效
	t.Setenv("PORT", "8080")
	var cfg struct {
		Port int `yaml:"port" env:"PORT"`
	}
	if err := ApplyEnv(&cfg, WithEnvNaming(EnvNaming{Prefix: "PB_"})); err != nil {
		t.Fatalf("ApplyEnv: %v", err)
	}
	if cfg.Port != 0 {
		t.Errorf("Port = %d, want 0 (prefixed name only)", cfg.Port)
	}
}

func TestEnvPrefix_NoStrategy_KeepsLegacyBehavior(t *testing.T) {
	// 无前缀时：没打 env tag 的字段**不读** env（历史行为逐字不变）
	t.Setenv("ENABLED", "true")
	var cfg struct {
		Enabled bool `yaml:"enabled"`
	}
	if err := ApplyEnv(&cfg); err != nil {
		t.Fatalf("ApplyEnv: %v", err)
	}
	if cfg.Enabled {
		t.Error("untagged field must not read env without a naming strategy")
	}
}

func TestEnvPrefix_InvalidPrefixFailsClosed(t *testing.T) {
	// 前缀含非法字符 → ValidateTags fail-closed（否则变量名永远 miss，静默失效）
	var cfg struct {
		Port int `yaml:"port"`
	}
	err := ValidateTags(&cfg, WithEnvNaming(EnvNaming{Prefix: "PB-"}))
	if err == nil || !strings.Contains(err.Error(), "invalid characters") {
		t.Errorf("expected invalid prefix error, got %v", err)
	}
}

// ---- env 命名策略：逐字段覆盖 ----

func TestEnvOverrides_OverridesEnvName(t *testing.T) {
	// 覆盖表命中 → 读表里的名字（与 yaml 键路径不对应也没关系）
	t.Setenv("PB_LOCAL_IMPORT_ENABLED", "true")
	var cfg struct {
		Upload struct {
			LocalImport struct {
				Enabled bool `yaml:"enabled"`
			} `yaml:"local_import"`
		} `yaml:"upload"`
	}
	overrides := map[string]string{"upload.local_import.enabled": "PB_LOCAL_IMPORT_ENABLED"}
	if err := ApplyEnv(&cfg, WithEnvNaming(EnvNaming{Overrides: overrides})); err != nil {
		t.Fatalf("ApplyEnv: %v", err)
	}
	if !cfg.Upload.LocalImport.Enabled {
		t.Error("Enabled should be read from the override name")
	}
}

func TestEnvOverrides_EmptyValueDisablesEnv(t *testing.T) {
	// 覆盖值为空串 → 该字段不走 env（显式豁免，压过 env tag）
	t.Setenv("LEGACY_RETRIES", "5")
	var cfg struct {
		Retries int `yaml:"retries" env:"LEGACY_RETRIES"`
	}
	overrides := map[string]string{"retries": ""}
	if err := ApplyEnv(&cfg, WithEnvNaming(EnvNaming{Overrides: overrides})); err != nil {
		t.Fatalf("ApplyEnv: %v", err)
	}
	if cfg.Retries != 0 {
		t.Errorf("Retries = %d, want 0 (overridden to not read env)", cfg.Retries)
	}
}

func TestEnvOverrides_BeatsEnvTag(t *testing.T) {
	// 覆盖优先于 env tag：表里写了就是表里说了算
	t.Setenv("PB_REAL", "9")
	t.Setenv("OLD_NAME", "1")
	var cfg struct {
		Port int `yaml:"port" env:"OLD_NAME"`
	}
	overrides := map[string]string{"port": "PB_REAL"}
	if err := ApplyEnv(&cfg, WithEnvNaming(EnvNaming{Overrides: overrides})); err != nil {
		t.Fatalf("ApplyEnv: %v", err)
	}
	if cfg.Port != 9 {
		t.Errorf("Port = %d, want 9 (override wins over env tag)", cfg.Port)
	}
}

func TestEnvOverrides_UnspecifiedFieldsUnchanged(t *testing.T) {
	// 覆盖表只影响命中的字段：未命中的字段行为不变（仍按 env tag 读 env）
	t.Setenv("OTHER_FIELD", "kept")
	var cfg struct {
		Other string `yaml:"other" env:"OTHER_FIELD"`
	}
	overrides := map[string]string{"some.other.key": "WHATEVER"}
	if err := ApplyEnv(&cfg, WithEnvNaming(EnvNaming{Overrides: overrides})); err != nil {
		t.Fatalf("ApplyEnv: %v", err)
	}
	if cfg.Other != "kept" {
		t.Errorf("Other = %q, want %q", cfg.Other, "kept")
	}
}

func TestEnvOverrides_InvalidNameFailsClosed(t *testing.T) {
	// 覆盖名非法 → fail-closed（名字非法 = 永远 miss = 静默失效）
	var cfg struct {
		Port int `yaml:"port"`
	}
	overrides := map[string]string{"port": "has space"}
	err := ValidateTags(&cfg, WithEnvNaming(EnvNaming{Overrides: overrides}))
	if err == nil || !strings.Contains(err.Error(), "not a valid variable name") {
		t.Errorf("expected invalid override name error, got %v", err)
	}
}

func TestEnvOverrides_UnknownPathIsNoOp(t *testing.T) {
	// 覆盖表里不存在的键路径不报错（同一张表可跨多个结构体共用）
	var cfg struct {
		Port int `yaml:"port"`
	}
	overrides := map[string]string{"no.such.key": "SOME_NAME"}
	if err := ValidateTags(&cfg, WithEnvNaming(EnvNaming{Overrides: overrides})); err != nil {
		t.Errorf("unknown override path must not fail validation, got %v", err)
	}
}

func TestEnvOverrides_DashedFieldNotAddressable(t *testing.T) {
	// yaml:"-" 的字段不出现在 yaml 里 → 没有键路径 → 覆盖表按 yaml 键路径
	// 寻址不到它（连它的子树也不可寻址）。
	t.Setenv("DASHED_NAME", "9")
	var cfg struct {
		Secret string `yaml:"-"`
	}
	overrides := map[string]string{"secret": "DASHED_NAME"}
	if err := ApplyEnv(&cfg, WithEnvNaming(EnvNaming{Overrides: overrides})); err != nil {
		t.Fatalf("ApplyEnv: %v", err)
	}
	if cfg.Secret != "" {
		t.Errorf("Secret = %q, want empty (yaml:\"-\" is not addressable)", cfg.Secret)
	}
}

// ---- 覆盖表键路径口径（与严格模式诊断一致）----

func TestEnvOverrides_YamlPathConvention(t *testing.T) {
	// 键路径口径：嵌套用 . 连接、切片元素不编号、inline 拍平到父级
	cases := []struct {
		name     string
		yamlPath string
		envName  string
		check    func(t *testing.T, cfg *pathCfg)
	}{
		{
			name:     "nested",
			yamlPath: "upload.local_import.enabled",
			envName:  "NESTED_NAME",
			check: func(t *testing.T, cfg *pathCfg) {
				if cfg.Upload.LocalImport.Enabled != 7 {
					t.Errorf("nested override not applied: %+v", cfg)
				}
			},
		},
		{
			name:     "slice element not numbered",
			yamlPath: "items.qty",
			envName:  "SLICE_NAME",
			check: func(t *testing.T, cfg *pathCfg) {
				if len(cfg.Items) == 0 || cfg.Items[0].Qty != 7 {
					t.Errorf("slice override not applied: %+v", cfg)
				}
			},
		},
		{
			name:     "inline flattened to parent",
			yamlPath: "flat",
			envName:  "INLINE_NAME",
			check: func(t *testing.T, cfg *pathCfg) {
				if cfg.Flat != 7 {
					t.Errorf("inline override not applied: %+v", cfg)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(c.envName, "7")
			// 切片需预置元素：env 只填**已存在**的切片元素（与既有遍历口径一致）。
			cfg := pathCfg{Items: []pathItem{{}}}
			overrides := map[string]string{c.yamlPath: c.envName}
			if err := ApplyEnv(&cfg, WithEnvNaming(EnvNaming{Overrides: overrides})); err != nil {
				t.Fatalf("ApplyEnv: %v", err)
			}
			c.check(t, &cfg)
		})
	}
}

type pathItem struct {
	Qty int `yaml:"qty"`
}

// PathFlat 以 inline 嵌入：其字段在 yaml 里拍平到父级（键为 "flat"）。
type PathFlat struct {
	Flat int `yaml:"flat"`
}

type pathCfg struct {
	Upload struct {
		LocalImport struct {
			Enabled int `yaml:"enabled"`
		} `yaml:"local_import"`
	} `yaml:"upload"`
	Items    []pathItem `yaml:"items"`
	PathFlat `yaml:",inline"`
}

// ---- 命名一致性检查（lint）----

func TestEnvNamingLint_OverrideFieldExempt(t *testing.T) {
	// 覆盖表命中的字段：最终名由表决定，豁免派生名一致性检查
	var cfg struct {
		Path string `yaml:"path" env:"LOG_PATH"`
	}
	overrides := map[string]string{"path": "HOST_SPECIFIC_PATH"}
	err := ValidateTags(&cfg, WithEnvNaming(EnvNaming{Overrides: overrides}), WithEnvNamingLint())
	if err != nil {
		t.Errorf("overridden field must be exempt from naming lint, got %v", err)
	}
}

func TestEnvNamingLint_PrefixUsesPathDerivedName(t *testing.T) {
	// 设了前缀：约定名是**路径级**派生名，tag 与之不一致才报错
	var ok struct {
		Upload struct {
			LocalImport struct {
				Enabled bool `yaml:"enabled" env:"UPLOAD_LOCAL_IMPORT_ENABLED"`
			} `yaml:"local_import"`
		} `yaml:"upload"`
	}
	if err := ValidateTags(&ok, WithEnvNaming(EnvNaming{Prefix: "PB_"}), WithEnvNamingLint()); err != nil {
		t.Errorf("path-derived tag should pass naming lint, got %v", err)
	}
	var bad struct {
		Upload struct {
			LocalImport struct {
				Enabled bool `yaml:"enabled" env:"ENABLED"`
			} `yaml:"local_import"`
		} `yaml:"upload"`
	}
	err := ValidateTags(&bad, WithEnvNaming(EnvNaming{Prefix: "PB_"}), WithEnvNamingLint())
	if err == nil || !strings.Contains(err.Error(), "UPLOAD_LOCAL_IMPORT_ENABLED") {
		t.Errorf("expected naming lint to flag leaf-only name, got %v", err)
	}
}

func TestLintEnvNaming_WithStrategy(t *testing.T) {
	// LintEnvNaming 也接受命名策略：覆盖命中的字段豁免
	var cfg struct {
		Path string `yaml:"path" env:"LOG_PATH"`
	}
	overrides := map[string]string{"path": "HOST_PATH"}
	issues, err := LintEnvNaming(&cfg, WithEnvNaming(EnvNaming{Overrides: overrides}))
	if err != nil {
		t.Fatalf("LintEnvNaming: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("overridden field must be exempt, got %v", issues)
	}
}

// ---- 重名判定：按最终名 + yaml 路径去重 ----

func TestEnvField_DuplicateNameAcrossPathsFails(t *testing.T) {
	// 不同 yaml 路径用同一个 env 名 → 歧义（必有一个静默拿不到值）→ 报错
	var cfg struct {
		A string `yaml:"a" env:"SAME_NAME"`
		B string `yaml:"b" env:"SAME_NAME"`
	}
	err := ValidateTags(&cfg)
	if err == nil || !strings.Contains(err.Error(), "already used") {
		t.Errorf("expected duplicate env name error, got %v", err)
	}
}

func TestEnvField_DuplicateNameSamePathSliceOK(t *testing.T) {
	// 同一 yaml 路径的多个实例（切片元素）→ 同一逻辑字段，不算重名
	var cfg struct {
		Items []struct {
			Qty int `yaml:"qty" env:"SLICE_QTY"`
		} `yaml:"items"`
	}
	err := ValidateTags(&cfg)
	if err != nil {
		t.Errorf("slice elements of the same field must not be reported as duplicates, got %v", err)
	}
}

func TestEnvField_PrefixDerivedDuplicateFails(t *testing.T) {
	// 前缀策略下：不同路径派生名撞车（a.b 与 a_b 都推出 A_B）→ 报错
	var cfg struct {
		A struct {
			B int `yaml:"b"`
		} `yaml:"a"`
		AB int `yaml:"a_b"`
	}
	err := ValidateTags(&cfg, WithEnvNaming(EnvNaming{Prefix: "PB_"}))
	if err == nil || !strings.Contains(err.Error(), "already used") {
		t.Errorf("expected derived-name collision error, got %v", err)
	}
}

// ---- require 字段的提示用最终名 ----

func TestRequire_ErrorMentionsFinalEnvName(t *testing.T) {
	// require 缺失时的提示要指向**真正生效**的变量名（加前缀后）
	var cfg struct {
		Secret string `yaml:"secret" require:"true" env:"APP_SECRET"`
	}
	err := ValidateTags(&cfg, WithFieldSet(FieldSet{}), WithEnvNaming(EnvNaming{Prefix: "PB_"}))
	if err == nil || !strings.Contains(err.Error(), "PB_APP_SECRET") {
		t.Errorf("expected error to mention PB_APP_SECRET, got %v", err)
	}
}
