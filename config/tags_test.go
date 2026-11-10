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

// ---- WithLenientEnv（core#51 宽容档） ----

// TestApplyEnv_LenientSkipsUnparsableValue 钉住 core#51：SERVER_PORT=abc 这类
// env 笔误不再阻断启动——WARN + 跳过（字段保持零值、FieldSet 不补记），
// 后续 FillDefaults 可用 default 兜底（Spring `${ENV:default}` 式契约）。
func TestApplyEnv_LenientSkipsUnparsableValue(t *testing.T) {
	t.Setenv("TAGS_TEST_PORT", "abc")
	cfg := struct {
		Port int `yaml:"port" env:"TAGS_TEST_PORT" default:"8080"`
	}{}
	set := FieldSet{}
	if err := ApplyEnv(&cfg, WithLenientEnv(), WithFieldSet(set)); err != nil {
		t.Fatalf("lenient mode should skip instead of failing, got %v", err)
	}
	if cfg.Port != 0 {
		t.Errorf("skipped field must stay zero, got %d", cfg.Port)
	}
	if set.Has("Port") {
		t.Error("lenient skip must not mark the field as provided in FieldSet")
	}
	// 跳过 ≠ 提供零值：default 仍应兜底。
	if err := FillDefaults(&cfg, WithFieldSet(set)); err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 8080 {
		t.Errorf("default should fill after lenient skip, got %d", cfg.Port)
	}
}

// TestApplyEnv_LenientStillAppliesParsableValues 宽容档只放宽「解析失败」，
// 可解析的 env 值照常写入并补记 FieldSet。
func TestApplyEnv_LenientStillAppliesParsableValues(t *testing.T) {
	t.Setenv("TAGS_TEST_PORT", "7000")
	cfg := struct {
		Port int `yaml:"port" env:"TAGS_TEST_PORT"`
	}{}
	set := FieldSet{}
	if err := ApplyEnv(&cfg, WithLenientEnv(), WithFieldSet(set)); err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 7000 {
		t.Errorf("parsable env must still apply, got %d", cfg.Port)
	}
	if !set.Has("Port") {
		t.Error("successful env write must be recorded in FieldSet")
	}
}

// TestApplyEnv_LenientDoesNotAffectDefaults 宽容档仅作用于 env 值解析：
// default tag 解析失败仍是硬错误（文件/代码面变更走评审，保持 fail-closed）。
func TestApplyEnv_LenientDoesNotAffectDefaults(t *testing.T) {
	cfg := struct {
		N int `default:"abc"`
	}{}
	if err := ApplyEnv(&cfg, WithLenientEnv()); err != nil {
		t.Fatal(err) // 无 env 字段，ApplyEnv 本身不报错
	}
	if err := FillDefaults(&cfg, WithLenientEnv()); err == nil {
		t.Error("malformed default tag must stay fail-closed regardless of lenient env")
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
	// "]0,100[" / "0,100]" 钉住文档口径：区间必须以 [/( 开头、]/) 结尾，
	// 反向括号写法不支持（core#58，doc 与实现二选一后取「改文档」）。
	for _, tag := range []string{"abc", "[1,2,3]", "[5,1]", "[,]", "[0,100", "]0,100[", "0,100]"} {
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

// TestApplyEnvPreservesWhitespace 回归 core#88：字符串字段此前被静默裁剪
// （"  pa ss  " → "pa ss"）——口令/token 两端空格是合法内容，裁剪让认证
// 失败现场与配置现场不一致；${VAR} 文本替换路径不裁剪，两条路径对同一
// env 值给出不同结果。现在字符串字段写入原始值，判定「是否未设置」仍用
// 裁剪值；非字符串类型解析前裁剪。
func TestApplyEnvPreservesWhitespace(t *testing.T) {
	type cfg struct {
		Token string        `yaml:"token" env:"WS_TOKEN"`
		Port  int           `yaml:"port" env:"WS_PORT"`
		Dur   time.Duration `yaml:"dur" env:"WS_DUR"`
		Tags  []string      `yaml:"tags" env:"WS_TAGS"`
	}
	var c cfg
	t.Setenv("WS_TOKEN", "  pa ss  ")
	t.Setenv("WS_PORT", " 8080 ")
	t.Setenv("WS_DUR", " 5s ")
	t.Setenv("WS_TAGS", " a , b ")
	if err := ApplyEnv(&c); err != nil {
		t.Fatalf("ApplyEnv: %v", err)
	}
	if c.Token != "  pa ss  " {
		t.Errorf("string field whitespace was stripped: %q, want %q", c.Token, "  pa ss  ")
	}
	if c.Port != 8080 {
		t.Errorf("int field must parse trimmed value: %d, want 8080", c.Port)
	}
	if c.Dur != 5*time.Second {
		t.Errorf("duration field must parse trimmed value: %v, want 5s", c.Dur)
	}
	if len(c.Tags) != 2 || c.Tags[0] != "a" || c.Tags[1] != "b" {
		t.Errorf("[]string field: %v, want [a b]", c.Tags)
	}

	// 纯空白仍视同未设置（判定口径不变）
	var c2 cfg
	t.Setenv("WS_TOKEN", "   ")
	if err := ApplyEnv(&c2); err != nil {
		t.Fatalf("ApplyEnv blank: %v", err)
	}
	if c2.Token != "" {
		t.Errorf("blank env must be treated as unset, got %q", c2.Token)
	}

	// 与 ${VAR} 文本替换路径同值：同一 env 值两条路径结果一致
	t.Setenv("WS_TOKEN", "  pa ss  ")
	var viaText struct {
		Token string `yaml:"token"`
	}
	if err := Unmarshal([]byte(`token: "${WS_TOKEN}"`), &viaText); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if viaText.Token != "  pa ss  " {
		t.Errorf("text path mangled the value: %q", viaText.Token)
	}
	if viaText.Token != "  pa ss  " || c.Token != "  pa ss  " {
		t.Errorf("two env paths diverge: tag path %q vs text path %q", c.Token, viaText.Token)
	}
}

// ---- ApplyEnv 裁剪档（#104 WithTrimStrings）----

func TestApplyEnv_WithTrimStringsTrimsStringFields(t *testing.T) {
	t.Setenv("TRIM_TEST_TOKEN", "  abc  ")
	t.Setenv("TRIM_TEST_MODE", " v4 ")
	t.Setenv("TRIM_TEST_NUM", " 42 ")
	t.Setenv("TRIM_TEST_HOSTS", " a.com , b.com ")

	cfg := struct {
		Token string   `env:"TRIM_TEST_TOKEN"`
		Mode  string   `env:"TRIM_TEST_MODE" options:"v4,v5,auto"`
		Num   int      `env:"TRIM_TEST_NUM"`
		Hosts []string `env:"TRIM_TEST_HOSTS"`
	}{}
	if err := ApplyEnv(&cfg, WithTrimStrings()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Token != "abc" {
		t.Errorf("Token = %q, 期望裁剪后 %q（#104）", cfg.Token, "abc")
	}
	if cfg.Mode != "v4" {
		t.Errorf("Mode = %q, 期望裁剪后 v4", cfg.Mode)
	}
	if cfg.Num != 42 {
		t.Errorf("Num = %d, 期望 42（非字符串类型解析前本就裁剪）", cfg.Num)
	}
	if got := strings.Join(cfg.Hosts, ","); got != "a.com,b.com" {
		t.Errorf("Hosts = %v, 期望 [a.com b.com]", cfg.Hosts)
	}
	// 边界声明：枚举校验发生在裁剪之后——裁剪后的值必须过 options 校验
	if err := ValidateTags(&cfg); err != nil {
		t.Errorf("裁剪后 options 校验应通过: %v", err)
	}
}

// TestApplyEnv_WithoutTrimStringsKeepsRawValue 缺省档与 v0.13.5 现状逐字节一致：
// 字符串字段原样写入（core#88），带空白的枚举值被 options 校验拒载（升级消费方
// 的第二类故障形态——服务起不来）。
func TestApplyEnv_WithoutTrimStringsKeepsRawValue(t *testing.T) {
	t.Setenv("TRIM_TEST_TOKEN", "  abc  ")
	t.Setenv("TRIM_TEST_MODE", " v4 ")

	cfg := struct {
		Token string `env:"TRIM_TEST_TOKEN"`
		Mode  string `env:"TRIM_TEST_MODE" options:"v4,v5,auto"`
	}{}
	if err := ApplyEnv(&cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Token != "  abc  " {
		t.Errorf("Token = %q, 期望原样 %q（core#88 语义为缺省）", cfg.Token, "  abc  ")
	}
	if err := ValidateTags(&cfg); err == nil {
		t.Error("带空白的枚举值应被 options 校验拒载（不开档的现状）")
	}
}

func TestApplyEnv_WithTrimStringsWhitespaceOnlyStillUnset(t *testing.T) {
	t.Setenv("TRIM_TEST_WS", "   ")
	cfg := struct {
		A string `env:"TRIM_TEST_WS"`
	}{}
	if err := ApplyEnv(&cfg, WithTrimStrings()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.A != "" {
		t.Errorf("纯空白 env 应视同未设置, got %q", cfg.A)
	}
}

func TestApplyTagPasses_WithTrimStrings(t *testing.T) {
	t.Setenv("TRIM_TEST_PASS_MODE", " v4 ")
	cfg := struct {
		Name string `yaml:"name"`
		Mode string `yaml:"mode" env:"TRIM_TEST_PASS_MODE" options:"v4,v5,auto"`
	}{}
	if err := ApplyTagPasses(&cfg, []byte("name: svc\n"), WithTrimStrings()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Mode != "v4" {
		t.Errorf("Mode = %q, 期望裁剪后 v4", cfg.Mode)
	}
}

// TestApplyTagPasses_WithoutTrimStringsRejectsBlankPaddedEnum 不开档的接线现状：
// 带空白的枚举值在 ValidateTags 拒载（升级消费方实证的「服务起不来」形态）。
func TestApplyTagPasses_WithoutTrimStringsRejectsBlankPaddedEnum(t *testing.T) {
	t.Setenv("TRIM_TEST_PASS_MODE", " v4 ")
	cfg := struct {
		Mode string `yaml:"mode" env:"TRIM_TEST_PASS_MODE" options:"v4,v5,auto"`
	}{}
	err := ApplyTagPasses(&cfg, nil)
	if err == nil {
		t.Fatal("带空白的枚举值应拒载（不开档现状）")
	}
	if !strings.Contains(err.Error(), "options") {
		t.Errorf("错误应指向 options 校验, got %v", err)
	}
}

// TestUnmarshal_WithEnvTrimStrings LoadOption 接线：WithEnvTrimStrings 挂在
// Unmarshal / LoadFile / LoadBootstrap 上使 ApplyEnv 环节裁剪；不开档同输入拒载。
func TestUnmarshal_WithEnvTrimStrings(t *testing.T) {
	t.Setenv("TRIM_TEST_LOPT_MODE", " auto ")

	var cfg struct {
		Mode string `yaml:"mode" env:"TRIM_TEST_LOPT_MODE" options:"v4,v5,auto"`
	}
	if err := Unmarshal(nil, &cfg, WithEnvTrimStrings()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Mode != "auto" {
		t.Errorf("Mode = %q, 期望裁剪后 auto", cfg.Mode)
	}

	var cfg2 struct {
		Mode string `yaml:"mode" env:"TRIM_TEST_LOPT_MODE" options:"v4,v5,auto"`
	}
	if err := Unmarshal(nil, &cfg2); err == nil {
		t.Fatal("不开档时带空白的枚举值应拒载（现状）")
	}
}

// TestApplyEnvOverlay_WithTrimStrings 裁剪档透传 ApplyEnvOverlay（12-factor
// env 优先路径同样生效）。
func TestApplyEnvOverlay_WithTrimStrings(t *testing.T) {
	t.Setenv("TRIM_TEST_OVL_TOKEN", "  tok  ")
	dst := struct {
		Token string `yaml:"token" env:"TRIM_TEST_OVL_TOKEN"`
	}{Token: "file-value"}
	if err := ApplyEnvOverlay(&dst, WithTrimStrings()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dst.Token != "tok" {
		t.Errorf("Token = %q, 期望 env 优先覆盖为裁剪后 tok", dst.Token)
	}
}
