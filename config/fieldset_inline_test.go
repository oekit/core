package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

type fsInline struct {
	Region string `yaml:"region" default:"cn"`
	Shards int    `yaml:"shards" default:"4"`
}

type fsInlinePtr struct {
	Name string `yaml:"name"`
}

type fsInlineCfg struct {
	Port   int          `yaml:"port" default:"8080"`
	Inline fsInline     `yaml:",inline"`
	Ptr    *fsInlinePtr `yaml:",inline"`
}

// loadInline 镜像 LoadBootstrap 的两步：yaml.Unmarshal + ApplyTagPasses（后者只做
// 标记与填充，不反序列化）。
func loadInline(t *testing.T, doc string) *fsInlineCfg {
	t.Helper()
	var cfg fsInlineCfg
	if err := yaml.Unmarshal([]byte(doc), &cfg); err != nil {
		t.Fatal(err)
	}
	if err := ApplyTagPasses(&cfg, []byte(doc)); err != nil {
		t.Fatal(err)
	}
	return &cfg
}

// TestFieldSetInline_ExplicitZeroSurvives 钉住 v0.9.1：`,inline` 子字段在通用 map
// 解码下被拍平到父级映射，按字段名取 key 永远 miss——inline 字段的显式零值此前
// 仍会被 default 覆盖。修复后以同一映射继续下推，叶子路径正常标记。
func TestFieldSetInline_ExplicitZeroSurvives(t *testing.T) {
	doc := "region: \"\"\nshards: 0\nport: 0\n"
	var cfg fsInlineCfg
	set, err := FieldSetFromYAML(&cfg, []byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"Inline.Region", "Inline.Shards", "Port"} {
		if !set.Has(p) {
			t.Errorf("inline/顶层显式零值应被标记 %q，set=%v", p, set)
		}
	}
	if got := loadInline(t, doc); got.Inline.Region != "" || got.Inline.Shards != 0 || got.Port != 0 {
		t.Errorf("显式零值不应被 default 覆盖：%+v", got)
	}
}

// TestFieldSetInline_PartialPresent：inline 子树里只写了部分键，其余仍吃 default。
func TestFieldSetInline_PartialPresent(t *testing.T) {
	var cfg fsInlineCfg
	set, err := FieldSetFromYAML(&cfg, []byte("region: us\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !set.Has("Inline.Region") {
		t.Errorf("Inline.Region 应被标记，set=%v", set)
	}
	if set.Has("Inline.Shards") || set.Has("Port") {
		t.Errorf("未出现的键不应被标记，set=%v", set)
	}
	got := loadInline(t, "region: us\n")
	if got.Inline.Region != "us" || got.Inline.Shards != 4 || got.Port != 8080 {
		t.Errorf("部分提供的 inline 子树应保留其余 default：%+v", got)
	}
}

// TestFieldSetInline_ProvidedViaPointer：yaml 提供了指针 inline 的叶子键。
func TestFieldSetInline_ProvidedViaPointer(t *testing.T) {
	got := loadInline(t, "name: svc\n")
	if got.Ptr == nil || got.Ptr.Name != "svc" {
		t.Fatalf("指针 inline 应按类型下推并保留显式值：%+v", got)
	}
	if got.Inline.Region != "cn" {
		t.Errorf("未提供的 inline 叶子仍应吃 default，got %q", got.Inline.Region)
	}
}
