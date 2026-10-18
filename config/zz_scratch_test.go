package config

import (
	"testing"
)

type igInline struct {
	Region string `yaml:"region" default:"cn"`
}

type igCfg struct {
	Inline igInline `yaml:",inline"`
	Port   int      `yaml:"port" default:"8080"`
}

// 期望：yaml 显式 region: ""（空串零值）不应被 default "cn" 覆盖。
func TestScratch_InlineGap(t *testing.T) {
	var cfg igCfg
	set, err := FieldSetFromYAML(&cfg, []byte("region: \"\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("FieldSet=%v", set)
	if !set.Has("Inline.Region") {
		t.Errorf("inline 字段未被标记为显式提供（缺口证实）：set=%v", set)
	}
	if err := ApplyTagPasses(&cfg, []byte("region: \"\"\n")); err != nil {
		t.Fatal(err)
	}
	if cfg.Inline.Region != "" {
		t.Errorf("yaml 显式 region 空串被 default %q 覆盖（缺口证实）", cfg.Inline.Region)
	}
	if cfg.Port != 8080 {
		t.Errorf("未提供字段应吃 default，got %d", cfg.Port)
	}
}
