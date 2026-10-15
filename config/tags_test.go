package config

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// ---- 测试用配置结构体 ----

type tagsInner struct {
	DSN     string `yaml:"dsn" default:"root@tcp(localhost)/app"`
	MaxOpen int    `yaml:"max_open" default:"10" range:"(0,100]"`
}

type tagsLeaf struct {
	Mode string `yaml:"mode" default:"dev" options:"dev,prod"`
}

type tagsItem struct {
	Qty int `yaml:"qty" range:"[1,10]"`
}

type tagsFull struct {
	Host    string        `yaml:"host" default:"localhost"`
	Port    int           `yaml:"port" default:"8080" env:"TAGS_TEST_PORT" range:"(0,65535]"`
	Debug   bool          `yaml:"debug" default:"true"`
	Timeout time.Duration `yaml:"timeout" default:"5s"`
	Hosts   []string      `yaml:"hosts" default:"a.example.com,b.example.com"`
	Inner   tagsInner     `yaml:"inner"`
	Ptr     *tagsInner    `yaml:"ptr"`
	Leaf    tagsLeaf      `yaml:"leaf"`
	Items   []tagsItem    `yaml:"items"`
	hidden  string        `default:"nope"` // 非导出字段应被跳过
}

// ---- FillDefaults ----

func TestFillDefaults_FillsZeroFieldsOnly(t *testing.T) {
	cfg := tagsFull{Port: 9090, Hosts: []string{"x.example.com"}}
	if err := FillDefaults(&cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Host != "localhost" {
		t.Errorf("Host default not filled, got %q", cfg.Host)
	}
	if cfg.Port != 9090 {
		t.Errorf("explicit Port 9090 overwritten, got %d", cfg.Port)
	}
	if !cfg.Debug {
		t.Error("Debug default true not filled")
	}
	if cfg.Timeout != 5*time.Second {
		t.Errorf("Timeout default not filled, got %v", cfg.Timeout)
	}
	if len(cfg.Hosts) != 1 || cfg.Hosts[0] != "x.example.com" {
		t.Errorf("explicit Hosts overwritten, got %v", cfg.Hosts)
	}
	if cfg.Inner.DSN != "root@tcp(localhost)/app" || cfg.Inner.MaxOpen != 10 {
		t.Errorf("nested Inner defaults not filled, got %+v", cfg.Inner)
	}
	if cfg.Ptr == nil || cfg.Ptr.DSN != "root@tcp(localhost)/app" {
		t.Errorf("nil Ptr not allocated/filled, got %+v", cfg.Ptr)
	}
	if cfg.Leaf.Mode != "dev" {
		t.Errorf("Leaf.Mode default not filled, got %q", cfg.Leaf.Mode)
	}
	if cfg.hidden != "" {
		t.Errorf("unexported field should be skipped, got %q", cfg.hidden)
	}
}

// TestFillDefaults_YamlSetZeroOverwritten 锁定设计决策：零值歧义——
// yaml 显式写的 0 与未写字段反射不可区分，会被 default 覆盖（见 godoc）。
func TestFillDefaults_YamlSetZeroOverwritten(t *testing.T) {
	cfg := struct {
		Port int `default:"8080"`
	}{Port: 0}
	if err := FillDefaults(&cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("expected documented zero-value overwrite to 8080, got %d", cfg.Port)
	}
}

func TestFillDefaults_MalformedDefaultTag(t *testing.T) {
	cfg := struct {
		N int `default:"abc"`
	}{}
	err := FillDefaults(&cfg)
	if err == nil {
		t.Fatal("expected error for malformed default tag")
	}
	if !strings.Contains(err.Error(), `field "N"`) {
		t.Errorf("error should carry field path, got %v", err)
	}
}

// ---- ApplyEnv ----

func TestApplyEnv_FillsZeroAndRespectsPrecedence(t *testing.T) {
	t.Setenv("TAGS_TEST_PORT", "7000")

	// 字段已有显式值 → env 不覆盖（yaml > env）。
	cfg := tagsFull{Port: 9090}
	if err := ApplyEnv(&cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != 9090 {
		t.Errorf("explicit Port overwritten by env, got %d", cfg.Port)
	}

	// 字段为零值 → env 填充；随后 default 不得覆盖（env > default）。
	var cfg2 tagsFull
	if err := ApplyEnv(&cfg2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg2.Port != 7000 {
		t.Fatalf("env value not applied, got %d", cfg2.Port)
	}
	if err := FillDefaults(&cfg2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg2.Port != 7000 {
		t.Errorf("default overwrote env value, got %d", cfg2.Port)
	}
}

func TestApplyEnv_UnsetOrEmptyEnvSkipped(t *testing.T) {
	t.Setenv("TAGS_TEST_EMPTY", "   ")
	cfg := struct {
		A int    `env:"TAGS_TEST_MISSING"`
		B string `env:"TAGS_TEST_EMPTY"`
	}{}
	if err := ApplyEnv(&cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.A != 0 || cfg.B != "" {
		t.Errorf("unset/empty env should be skipped, got %+v", cfg)
	}
}

func TestApplyEnv_TypedValues(t *testing.T) {
	t.Setenv("TAGS_TEST_TIMEOUT", "3s")
	t.Setenv("TAGS_TEST_HOSTS", "x.com, y.com")
	t.Setenv("TAGS_TEST_ENABLED", "1")
	cfg := struct {
		Timeout time.Duration `env:"TAGS_TEST_TIMEOUT"`
		Hosts   []string      `env:"TAGS_TEST_HOSTS"`
		Enabled bool          `env:"TAGS_TEST_ENABLED"`
	}{}
	if err := ApplyEnv(&cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Timeout != 3*time.Second {
		t.Errorf("expected 3s, got %v", cfg.Timeout)
	}
	if len(cfg.Hosts) != 2 || cfg.Hosts[0] != "x.com" || cfg.Hosts[1] != "y.com" {
		t.Errorf("expected [x.com y.com], got %v", cfg.Hosts)
	}
	if !cfg.Enabled {
		t.Error("expected Enabled=true")
	}
}

func TestApplyEnv_InvalidValueFailsClosed(t *testing.T) {
	t.Setenv("TAGS_TEST_PORT", "abc")
	cfg := struct {
		Port int `env:"TAGS_TEST_PORT"`
	}{}
	err := ApplyEnv(&cfg)
	if err == nil {
		t.Fatal("expected error for invalid env value")
	}
	if !strings.Contains(err.Error(), `field "Port"`) || !strings.Contains(err.Error(), "TAGS_TEST_PORT") {
		t.Errorf("error should carry field path and env name, got %v", err)
	}
}

// ---- ValidateTags: range ----

func TestValidateTags_RangeMatrix(t *testing.T) {
	cases := []struct {
		name    string
		tag     string
		val     float64
		wantErr bool
	}{
		{"closed-in", "[0,100]", 50, false},
		{"closed-low-edge", "[0,100]", 0, false},
		{"closed-high-edge", "[0,100]", 100, false},
		{"closed-above", "[0,100]", 100.5, true},
		{"closed-below", "[0,100]", -0.5, true},
		{"open-in", "(0,100)", 50, false},
		{"open-excludes-low", "(0,100)", 0, true},
		{"open-excludes-high", "(0,100)", 100, true},
		{"half-open-left-includes-low", "[0,100)", 0, false},
		{"half-open-left-excludes-high", "[0,100)", 100, true},
		{"half-open-right-excludes-low", "(0,100]", 0, true},
		{"half-open-right-includes-high", "(0,100]", 100, false},
		{"low-open-at-bound", "[10,)", 10, false},
		{"low-open-below", "[10,)", 9.9, true},
		{"high-open-at-bound", "(,10]", 10, false},
		{"high-open-above", "(,10]", 10.1, true},
		{"float-bounds-in", "(0.5,1.5)", 1.0, false},
		{"float-bounds-below", "(0.5,1.5)", 0.2, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			typ := reflect.StructOf([]reflect.StructField{{
				Name: "V",
				Type: reflect.TypeOf(float64(0)),
				Tag:  reflect.StructTag(`range:"` + tc.tag + `"`),
			}})
			inst := reflect.New(typ)
			inst.Elem().Field(0).SetFloat(tc.val)
			err := ValidateTags(inst.Interface())
			if tc.wantErr && err == nil {
				t.Errorf("tag %q value %v: expected error", tc.tag, tc.val)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("tag %q value %v: unexpected error: %v", tc.tag, tc.val, err)
			}
			if tc.wantErr && err != nil && !strings.Contains(err.Error(), `"V"`) {
				t.Errorf("violation error should carry field path, got %v", err)
			}
		})
	}
}

func TestValidateTags_RangeMalformedTags(t *testing.T) {
	for _, tag := range []string{"abc", "[1,2,3]", "[5,1]", "[,]", "[0,100"} {
		// 动态构造带非法 tag 的结构体（编译期 tag 无法循环注入）。
		typ := reflect.StructOf([]reflect.StructField{{
			Name: "V",
			Type: reflect.TypeOf(float64(0)),
			Tag:  reflect.StructTag(`range:"` + tag + `"`),
		}})
		inst := reflect.New(typ)
		inst.Elem().Field(0).SetFloat(1)
		if err := ValidateTags(inst.Interface()); err == nil {
			t.Errorf("malformed range tag %q: expected error", tag)
		}
	}
}

func TestValidateTags_RangeDuration(t *testing.T) {
	type durCfg struct {
		D time.Duration `range:"(0,30s]"`
	}

	ok := durCfg{D: 5 * time.Second}
	if err := ValidateTags(&ok); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	edge := durCfg{D: 30 * time.Second}
	if err := ValidateTags(&edge); err != nil {
		t.Errorf("upper bound 30s should be inclusive: %v", err)
	}

	violation := durCfg{D: 31 * time.Second}
	err := ValidateTags(&violation)
	if err == nil {
		t.Fatal("expected violation error for 31s")
	}
	if !strings.Contains(err.Error(), `"D"`) || !strings.Contains(err.Error(), "30s") {
		t.Errorf("error should carry field path and range, got %v", err)
	}

	// Duration 边界必须是 duration 文法，纯数字 fail-closed。
	badTag := struct {
		D time.Duration `range:"(0,30]"`
	}{D: 5 * time.Second}
	if err := ValidateTags(&badTag); err == nil {
		t.Error("expected error for plain-number bound on duration field")
	}
}

func TestValidateTags_RangeWrongType(t *testing.T) {
	cfg := struct {
		S string `range:"(0,100]"`
	}{S: "x"}
	err := ValidateTags(&cfg)
	if err == nil {
		t.Fatal("expected error for range on string field")
	}
	if !strings.Contains(err.Error(), `field "S"`) {
		t.Errorf("error should carry field path, got %v", err)
	}
}

// ---- ValidateTags: options ----

func TestValidateTags_Options(t *testing.T) {
	valid := struct {
		Env string `options:"dev,prod"`
	}{Env: "dev"}
	if err := ValidateTags(&valid); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	invalid := struct {
		Env string `options:"dev,prod"`
	}{Env: "staging"}
	err := ValidateTags(&invalid)
	if err == nil {
		t.Fatal("expected error for value outside options")
	}
	if !strings.Contains(err.Error(), `field "Env"`) || !strings.Contains(err.Error(), "dev,prod") {
		t.Errorf("error should carry field path and allowed values, got %v", err)
	}

	// 空串同样必须命中枚举（可选枚举请配 default tag）。
	empty := struct {
		Env string `options:"dev,prod"`
	}{}
	if err := ValidateTags(&empty); err == nil {
		t.Error("expected error for empty value against options")
	}

	// options 仅限 string 字段。
	wrongType := struct {
		N int `options:"1,2"`
	}{N: 1}
	if err := ValidateTags(&wrongType); err == nil {
		t.Error("expected error for options on int field")
	}
}

// ---- 遍历：嵌套 / 切片 / 指针 ----

func TestValidateTags_NestedAndSlicePaths(t *testing.T) {
	nested := struct {
		Server struct {
			Port int `range:"(0,65535]"`
		}
	}{}
	nested.Server.Port = 70000
	err := ValidateTags(&nested)
	if err == nil {
		t.Fatal("expected violation error for nested field")
	}
	if !strings.Contains(err.Error(), `"Server.Port"`) {
		t.Errorf("error should carry nested path Server.Port, got %v", err)
	}

	slice := struct {
		Items []tagsItem
	}{Items: []tagsItem{{Qty: 5}, {Qty: 0}}}
	err = ValidateTags(&slice)
	if err == nil {
		t.Fatal("expected violation error for slice element")
	}
	if !strings.Contains(err.Error(), `"Items[1].Qty"`) {
		t.Errorf("error should carry slice path Items[1].Qty, got %v", err)
	}

	ptrSlice := struct {
		Ptrs []*tagsItem
	}{Ptrs: []*tagsItem{nil, {Qty: 0}}}
	err = ValidateTags(&ptrSlice)
	if err == nil {
		t.Fatal("expected violation error for pointer slice element")
	}
	if !strings.Contains(err.Error(), `"Ptrs[1].Qty"`) {
		t.Errorf("error should carry path Ptrs[1].Qty, got %v", err)
	}
}

func TestTags_NonStructInputsError(t *testing.T) {
	if err := FillDefaults(nil); err == nil {
		t.Error("FillDefaults(nil): expected error")
	}
	if err := ApplyEnv(42); err == nil {
		t.Error("ApplyEnv(42): expected error")
	}
	if err := ValidateTags("x"); err == nil {
		t.Error(`ValidateTags("x"): expected error`)
	}
	slice := []int{1}
	if err := FillDefaults(&slice); err == nil {
		t.Error("FillDefaults(*[]int): expected error")
	}
	m := map[string]int{}
	if err := ValidateTags(&m); err == nil {
		t.Error("ValidateTags(*map): expected error")
	}
	// 空结构体是合法输入（无字段 → 无操作）。
	if err := FillDefaults(&struct{}{}); err != nil {
		t.Errorf("FillDefaults(&struct{}{}): unexpected error: %v", err)
	}
}

// ---- LoadBootstrap 接线 ----

func TestLoadBootstrap_TagPipeline(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("TAGS_TEST_PORT", "7000")
	writeConfigFile(t, "etc/config.yaml", `
host: yaml-host
port: 9090
inner:
  max_open: 50
`)
	cfg, err := LoadBootstrap[tagsFull]("dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Host != "yaml-host" {
		t.Errorf("yaml explicit value should win over default, got %q", cfg.Host)
	}
	if cfg.Port != 9090 {
		t.Errorf("yaml explicit value should win over env tag, got %d", cfg.Port)
	}
	if cfg.Timeout != 5*time.Second {
		t.Errorf("default not filled, got %v", cfg.Timeout)
	}
	if len(cfg.Hosts) != 2 {
		t.Errorf("[]string default not filled, got %v", cfg.Hosts)
	}
	if cfg.Inner.DSN != "root@tcp(localhost)/app" || cfg.Inner.MaxOpen != 50 {
		t.Errorf("nested fill/validate wrong, got %+v", cfg.Inner)
	}
	if cfg.Ptr == nil {
		t.Error("nil Ptr should be allocated by FillDefaults")
	}
	if !cfg.Debug {
		t.Errorf("bool default not filled, got %v", cfg.Debug)
	}
}

func TestLoadBootstrap_TagPipelineEnvAndDefaultFallback(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeConfigFile(t, "etc/config.yaml", "host: h\n")

	// yaml 未写 port：env 有值 → env 优先于 default。
	t.Setenv("TAGS_TEST_PORT", "7000")
	cfg, err := LoadBootstrap[tagsFull]("dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != 7000 {
		t.Errorf("env tag should fill missing yaml value, got %d", cfg.Port)
	}

	// env 缺省 → default 兜底。
	os.Unsetenv("TAGS_TEST_PORT")
	cfg2, err := LoadBootstrap[tagsFull]("dev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg2.Port != 8080 {
		t.Errorf("default tag should fill when env missing, got %d", cfg2.Port)
	}
}

func TestLoadBootstrap_TagViolationFails(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeConfigFile(t, "etc/config.yaml", "port: 70000\n")
	_, err := LoadBootstrap[tagsFull]("dev")
	if err == nil {
		t.Fatal("expected fail-closed error for range violation")
	}
	if !strings.Contains(err.Error(), `"Port"`) || !strings.Contains(err.Error(), "(0,65535]") {
		t.Errorf("error should carry field path and range, got %v", err)
	}
}

func TestLoadBootstrap_NonStructSkipsTagPasses(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeConfigFile(t, "etc/config.yaml", "a: 1\n")
	cfg, err := LoadBootstrap[map[string]int]("dev")
	if err != nil {
		t.Fatalf("non-struct config should keep working, got error: %v", err)
	}
	if (*cfg)["a"] != 1 {
		t.Errorf("expected a=1, got %v", cfg)
	}
}
