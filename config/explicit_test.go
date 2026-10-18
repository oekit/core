package config

import (
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// ---- 测试用结构体 ----

type explInline struct {
	Region string `yaml:"region" default:"cn"`
}

type explSub struct {
	Port int    `yaml:"port" default:"5432" env:"EXPL_PG_PORT"`
	Mode string `yaml:"mode" default:"rw"`
}

type explItem struct {
	Qty int `yaml:"qty" default:"1"`
}

type explCfg struct {
	Host     string        `yaml:"host" default:"localhost"`
	Port     int           `yaml:"port" default:"8080" env:"EXPL_PORT"`
	Debug    bool          `yaml:"debug" default:"true"`
	Timeout  time.Duration `yaml:"timeout" default:"5s"`
	Hosts    []string      `yaml:"hosts" default:"a,b"`
	Lower    string        `default:"x"` // 无 yaml tag：键为字段名全小写 "lower"
	Skip     string        `yaml:"-" default:"skipped"`
	Inline   explInline    `yaml:",inline"`
	Sub      explSub       `yaml:"sub"`
	Ptr      *explSub      `yaml:"ptr"`
	Items    []explItem    `yaml:"items"`
	NotInDoc string        `yaml:"not_in_doc" default:"d"`
}

func mustExplicitYAML(t *testing.T, doc string, v any) ExplicitFields {
	t.Helper()
	e, err := ExplicitFromYAML([]byte(doc), v)
	if err != nil {
		t.Fatalf("ExplicitFromYAML: %v", err)
	}
	return e
}

// ---- ExplicitFromYAML ----

func TestExplicitFromYAML_MarksPresentKeysByYAMLRules(t *testing.T) {
	var cfg explCfg
	e := mustExplicitYAML(t, `
host: h
port: 0          # 显式零值
debug: false     # 显式 false
timeout: ~       # null = 未提供
hosts: []        # 显式空切片
lower: L
region: us       # inline 字段键在父映射
sub:
  port: 0
ptr:
  mode: ro
items:
  - qty: 0
  - {}
unknown_key: 1   # 结构体没有的键忽略
`, &cfg)

	want := []string{
		"Host", "Hosts", "Debug", "Inline.Region", "Items", "Items[0]", "Items[0].Qty", "Items[1]",
		"Lower", "Port", "Ptr", "Ptr.Mode", "Sub", "Sub.Port",
	}
	sort.Strings(want)
	if got := e.Paths(); !reflect.DeepEqual(got, want) {
		t.Fatalf("paths =\n  %v\nwant\n  %v", got, want)
	}
	for _, absent := range []string{"Timeout", "Skip", "Sub.Mode", "Ptr.Port", "NotInDoc"} {
		if e.Has(absent) {
			t.Errorf("%s 不应被标记（null/跳过/未出现）", absent)
		}
	}
}

func TestExplicitFromYAML_AliasAndMergeKey(t *testing.T) {
	var cfg explCfg
	e := mustExplicitYAML(t, `
base: &base
  port: 0
sub:
  <<: *base
  mode: rw
ptr: *base
`, &cfg)
	for _, p := range []string{"Sub", "Sub.Port", "Sub.Mode", "Ptr", "Ptr.Port"} {
		if !e.Has(p) {
			t.Errorf("%s 应经别名/合并键被标记，got %v", p, e.Paths())
		}
	}
}

func TestExplicitFromYAML_EdgeInputs(t *testing.T) {
	var cfg explCfg
	if e := mustExplicitYAML(t, "", &cfg); len(e) != 0 {
		t.Errorf("空文档应为空集合，got %v", e.Paths())
	}
	if e := mustExplicitYAML(t, "just-a-scalar", &cfg); len(e) != 0 {
		t.Errorf("顶层标量应为空集合，got %v", e.Paths())
	}
	if _, err := ExplicitFromYAML([]byte("a: [unclosed"), &cfg); err == nil {
		t.Error("非法 yaml 应报错")
	}
	if _, err := ExplicitFromYAML([]byte("a: 1"), cfg); err == nil {
		t.Error("非指针应报错")
	}
	var m map[string]int
	if _, err := ExplicitFromYAML([]byte("a: 1"), &m); err == nil {
		t.Error("非结构体应报错")
	}
}

// ---- ExplicitFromEnv ----

func TestExplicitFromEnv_MarksSetEnvIncludingNilSubtree(t *testing.T) {
	t.Setenv("EXPL_PORT", "0")
	t.Setenv("EXPL_PG_PORT", "   ") // 空白 = 未设置
	var cfg explCfg                 // Ptr 为 nil，仍按类型下探
	e, err := ExplicitFromEnv(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !e.Has("Port") {
		t.Errorf("EXPL_PORT=0 应被记为显式，got %v", e.Paths())
	}
	if e.Has("Sub.Port") || e.Has("Ptr.Port") {
		t.Errorf("空白 env 不应记为显式，got %v", e.Paths())
	}
	t.Setenv("EXPL_PG_PORT", "1")
	e, _ = ExplicitFromEnv(&cfg)
	if !e.Has("Sub.Port") || !e.Has("Ptr.Port") {
		t.Errorf("nil *struct 内的 env 字段也应按类型下探标记，got %v", e.Paths())
	}
}

// ---- FillDefaults / ApplyEnv + WithExplicit ----

func TestFillDefaults_WithExplicitKeepsExplicitZero(t *testing.T) {
	cfg := explCfg{} // Port=0, Debug=false, Hosts=nil
	explicit := ExplicitFields{}
	explicit.Mark("Port")
	explicit.Mark("Debug")
	explicit.Mark("Hosts")
	if err := FillDefaults(&cfg, WithExplicit(explicit)); err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 0 || cfg.Debug || len(cfg.Hosts) != 0 {
		t.Errorf("显式零值不应被 default 覆盖：port=%d debug=%v hosts=%v", cfg.Port, cfg.Debug, cfg.Hosts)
	}
	if cfg.Host != "localhost" || cfg.Timeout != 5*time.Second || cfg.Sub.Port != 5432 {
		t.Errorf("未显式提供的字段仍应填 default：%+v", cfg)
	}
}

func TestApplyEnv_WithExplicitYAMLZeroBeatsEnv(t *testing.T) {
	t.Setenv("EXPL_PORT", "9000")
	cfg := explCfg{}
	explicit := ExplicitFields{}
	explicit.Mark("Port") // yaml 显式 port: 0
	if err := ApplyEnv(&cfg, WithExplicit(explicit)); err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 0 {
		t.Errorf("yaml 显式零值应优先于 env，got %d", cfg.Port)
	}
	// 不传 explicit：退回既有判定，env 填零值字段
	cfg2 := explCfg{}
	if err := ApplyEnv(&cfg2); err != nil {
		t.Fatal(err)
	}
	if cfg2.Port != 9000 {
		t.Errorf("无来源信息时 env 应填零值字段，got %d", cfg2.Port)
	}
}

// ---- LoadBootstrap 端到端 ----

type explBoot struct {
	Port    int    `yaml:"port" default:"8080" env:"EXPL_BOOT_PORT"`
	RedisDB int    `yaml:"redis_db" default:"1" env:"EXPL_BOOT_REDIS_DB"`
	Null    int    `yaml:"nullable" default:"7"`
	Name    string `yaml:"name" default:"svc"`
}

func TestLoadBootstrap_ExplicitZeroSurvives(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("EXPL_BOOT_REDIS_DB", "0") // env 显式给零值
	t.Setenv("EXPL_BOOT_PORT", "9000")  // yaml 显式 0 应压过它
	writeConfigFile(t, "etc/config.yaml", `
port: 0
nullable: ~
`)
	cfg, err := LoadBootstrap[explBoot]("dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != 0 {
		t.Errorf("yaml 显式 port: 0 不应被 env/default 覆盖，got %d", cfg.Port)
	}
	if cfg.RedisDB != 0 {
		t.Errorf("env REDIS_DB=0 不应被 default 1 覆盖，got %d", cfg.RedisDB)
	}
	if cfg.Null != 7 {
		t.Errorf("yaml null 应视为未提供、吃 default，got %d", cfg.Null)
	}
	if cfg.Name != "svc" {
		t.Errorf("未出现的键应填 default，got %q", cfg.Name)
	}
}

// ---- required ----

type explReq struct {
	DSN string `yaml:"dsn" env:"EXPL_REQ_DSN" required:"true"`
	Opt string `yaml:"opt" required:"false"`
	Sub *struct {
		Token string `yaml:"token" required:"true"`
	} `yaml:"sub"`
}

func TestLoadBootstrap_RequiredFailsFastWithEnvHint(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeConfigFile(t, "etc/config.yaml", "opt: x\n")
	_, err := LoadBootstrap[explReq]("dev")
	if err == nil {
		t.Fatal("required 字段缺失应报错")
	}
	msg := err.Error()
	for _, want := range []string{"DSN (env EXPL_REQ_DSN)", "Sub.Token"} {
		if !strings.Contains(msg, want) {
			t.Errorf("错误应一次性列出全部缺失并附 env 提示，缺 %q：%v", want, msg)
		}
	}

	t.Setenv("EXPL_REQ_DSN", "pg://x")
	writeConfigFile(t, "etc/config.yaml", "sub:\n  token: t\n")
	cfg, err := LoadBootstrap[explReq]("dev")
	if err != nil {
		t.Fatalf("yaml+env 提供后应通过：%v", err)
	}
	if cfg.DSN != "pg://x" || cfg.Sub == nil || cfg.Sub.Token != "t" {
		t.Errorf("值不匹配：%+v", cfg)
	}
}

func TestCheckRequired_TagMisuse(t *testing.T) {
	type bad1 struct {
		X int `required:"true" default:"1"`
	}
	if err := CheckRequired(&bad1{}, ExplicitFields{}); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("required+default 应报互斥错误，got %v", err)
	}
	type bad2 struct {
		X int `required:"yes"`
	}
	if err := CheckRequired(&bad2{}, ExplicitFields{}); err == nil || !strings.Contains(err.Error(), "invalid required tag") {
		t.Errorf("非法 required 值应报错，got %v", err)
	}
	type rec struct {
		Next *rec `yaml:"next"`
		V    int  `yaml:"v" required:"true"`
	}
	err := CheckRequired(&rec{}, ExplicitFields{}) // 递归类型按类型下探不得死循环
	if err == nil || !strings.Contains(err.Error(), "V") {
		t.Errorf("递归类型应正常报出缺失，got %v", err)
	}
}

// ---- CheckEnvOrphans ----

type explOrphan struct {
	Fmt string `env:"EXPL_ORPHAN_ARCHIVE_ARCHIVE_FORMAT"` // 拼错：多了一个 ARCHIVE
	OK  string `env:"EXPL_ORPHAN_LEVEL"`
	Sub *struct {
		Deep string `env:"EXPL_ORPHAN_DEEP"`
	}
}

func TestCheckEnvOrphans_CatchesTypo(t *testing.T) {
	t.Setenv("EXPL_ORPHAN_ARCHIVE_FORMAT", "zip") // 部署时设的是正确的名字
	t.Setenv("EXPL_ORPHAN_LEVEL", "info")
	t.Setenv("EXPL_ORPHAN_DEEP", "d")   // nil *struct 内的 tag 也算消费方
	t.Setenv("EXPL_ORPHANX_OTHER", "o") // 前缀 "EXPL_ORPHAN_" 带下划线不误报
	var cfg explOrphan
	orphans, err := CheckEnvOrphans(&cfg, "EXPL_ORPHAN_")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(orphans, []string{"EXPL_ORPHAN_ARCHIVE_FORMAT"}) {
		t.Errorf("应只报出无人消费的孤儿变量，got %v", orphans)
	}
	if _, err := CheckEnvOrphans(&cfg); err == nil {
		t.Error("无前缀应报错")
	}
	if _, err := CheckEnvOrphans(&cfg, " "); err == nil {
		t.Error("空白前缀应报错")
	}
}

// ---- ValidateTags env 文法 ----

func TestValidateTags_EnvNameSyntax(t *testing.T) {
	type bad struct {
		X string `env:"BAD NAME"`
	}
	if err := ValidateTags(&bad{}); err == nil || !strings.Contains(err.Error(), "invalid env tag") {
		t.Errorf("含空白的 env 名应报错，got %v", err)
	}
	type bad2 struct {
		X string `env:"A=B"`
	}
	if err := ValidateTags(&bad2{}); err == nil {
		t.Error("含 '=' 的 env 名应报错")
	}
	type ok struct {
		X string `env:"GOOD_NAME"`
	}
	if err := ValidateTags(&ok{}); err != nil {
		t.Errorf("合法 env 名不应报错：%v", err)
	}
}
