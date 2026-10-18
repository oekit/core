package config

import (
	"strings"
	"testing"
)

type fsInner struct {
	DSN     string `yaml:"dsn" default:"root@tcp(localhost)/app"`
	MaxOpen int    `yaml:"max_open" default:"10"`
}

type fsConfig struct {
	Host   string    `yaml:"host" default:"localhost"`
	Port   int       `yaml:"port" default:"8080"`
	Name   string    `yaml:"name" default:"app"`
	Inner  fsInner   `yaml:"inner"`
	Items  []fsInner `yaml:"items"`
	Secret string    `yaml:"secret" require:"true" env:"FS_TEST_SECRET"`
}

// ---- FieldSetFromYAML ----

func TestFieldSetFromYAML_MarksPresentKeys(t *testing.T) {
	var cfg fsConfig
	data := []byte("port: 0\nname: \"\"\ninner:\n  max_open: 0\n")
	set, err := FieldSetFromYAML(&cfg, data)
	if err != nil {
		t.Fatalf("FieldSetFromYAML: %v", err)
	}
	for _, path := range []string{"Port", "Name", "Inner", "Inner.MaxOpen"} {
		if !set.Has(path) {
			t.Errorf("expected %q marked set", path)
		}
	}
	for _, path := range []string{"Host", "Inner.DSN", "Secret"} {
		if set.Has(path) {
			t.Errorf("did not expect %q marked set", path)
		}
	}
}

func TestFieldSetFromYAML_SliceElements(t *testing.T) {
	var cfg fsConfig
	data := []byte("items:\n  - dsn: \"\"\n  - max_open: 0\n")
	set, err := FieldSetFromYAML(&cfg, data)
	if err != nil {
		t.Fatalf("FieldSetFromYAML: %v", err)
	}
	if !set.Has("Items") || !set.Has("Items[0]") || !set.Has("Items[0].DSN") {
		t.Errorf("Items[0].DSN should be set-marked, set=%v", set)
	}
	if !set.Has("Items[1].MaxOpen") {
		t.Errorf("Items[1].MaxOpen should be set-marked, set=%v", set)
	}
	if set.Has("Items[1].DSN") {
		t.Error("Items[1].DSN should not be set-marked")
	}
}

func TestFieldSetFromYAML_EmptyData(t *testing.T) {
	var cfg fsConfig
	set, err := FieldSetFromYAML(&cfg, []byte("  # only comment\n"))
	if err != nil {
		t.Fatalf("FieldSetFromYAML: %v", err)
	}
	if len(set) != 0 {
		t.Errorf("expected empty set, got %v", set)
	}
}

// ---- FillDefaults + FieldSet：显式零值不被 default 覆盖 ----

func TestFillDefaults_WithFieldSet_KeepsExplicitZero(t *testing.T) {
	cfg := fsConfig{Port: 0, Name: ""}
	set := FieldSet{"Port": true, "Name": true}
	if err := FillDefaults(&cfg, WithFieldSet(set)); err != nil {
		t.Fatalf("FillDefaults: %v", err)
	}
	if cfg.Port != 0 {
		t.Errorf("explicit Port 0 overwritten by default, got %d", cfg.Port)
	}
	if cfg.Name != "" {
		t.Errorf("explicit empty Name overwritten by default, got %q", cfg.Name)
	}
	if cfg.Host != "localhost" {
		t.Errorf("Host default not filled, got %q", cfg.Host)
	}
}

func TestApplyTagPasses_YamlExplicitZeroWins(t *testing.T) {
	t.Setenv("FS_TEST_SECRET", "s3cret")
	cfg := fsConfig{}
	data := []byte("port: 0\nname: \"\"\ninner:\n  max_open: 0\n")
	if err := ApplyTagPasses(&cfg, data); err != nil {
		t.Fatalf("ApplyTagPasses: %v", err)
	}
	if cfg.Port != 0 {
		t.Errorf("yaml explicit port:0 should stick, got %d", cfg.Port)
	}
	if cfg.Name != "" {
		t.Errorf("yaml explicit name:\"\" should stick, got %q", cfg.Name)
	}
	if cfg.Inner.MaxOpen != 0 {
		t.Errorf("yaml explicit max_open:0 should stick, got %d", cfg.Inner.MaxOpen)
	}
	if cfg.Host != "localhost" {
		t.Errorf("Host default not filled, got %q", cfg.Host)
	}
	if cfg.Inner.DSN != "root@tcp(localhost)/app" {
		t.Errorf("Inner.DSN default not filled, got %q", cfg.Inner.DSN)
	}
	if cfg.Secret != "s3cret" {
		t.Errorf("Secret from env, got %q", cfg.Secret)
	}
}

func TestApplyEnv_WithFieldSet_YamlZeroNotOverwritten(t *testing.T) {
	t.Setenv("FS_TEST_PORT_ZERO", "9999")
	cfg := struct {
		Port int `yaml:"port" env:"FS_TEST_PORT_ZERO" default:"8080"`
	}{Port: 0}
	set := FieldSet{"Port": true}
	if err := ApplyEnv(&cfg, WithFieldSet(set)); err != nil {
		t.Fatalf("ApplyEnv: %v", err)
	}
	if cfg.Port != 0 {
		t.Errorf("yaml explicit 0 overwritten by env, got %d", cfg.Port)
	}
	if err := FillDefaults(&cfg, WithFieldSet(set)); err != nil {
		t.Fatalf("FillDefaults: %v", err)
	}
	if cfg.Port != 0 {
		t.Errorf("yaml explicit 0 overwritten by default, got %d", cfg.Port)
	}
}

func TestApplyEnv_MarksSet_SoDefaultSkips(t *testing.T) {
	t.Setenv("FS_TEST_PORT_ENV", "9090")
	cfg := struct {
		Port int `yaml:"port" env:"FS_TEST_PORT_ENV" default:"8080"`
	}{}
	set := FieldSet{}
	if err := ApplyEnv(&cfg, WithFieldSet(set)); err != nil {
		t.Fatalf("ApplyEnv: %v", err)
	}
	if cfg.Port != 9090 {
		t.Fatalf("env not applied, got %d", cfg.Port)
	}
	if !set.Has("Port") {
		t.Fatal("ApplyEnv should mark Port in FieldSet")
	}
	if err := FillDefaults(&cfg, WithFieldSet(set)); err != nil {
		t.Fatalf("FillDefaults: %v", err)
	}
	if cfg.Port != 9090 {
		t.Errorf("env value overwritten by default, got %d", cfg.Port)
	}
}

// ---- require ----

func TestRequire_NotFilledByDefault(t *testing.T) {
	cfg := struct {
		Secret string `default:"fallback" require:"true"`
	}{}
	err := FillDefaults(&cfg)
	if err != nil {
		t.Fatalf("FillDefaults: %v", err)
	}
	if cfg.Secret != "" {
		t.Errorf("require field must not get default, got %q", cfg.Secret)
	}
}

func TestRequire_MissingWithFieldSet_Fails(t *testing.T) {
	cfg := struct {
		Secret string `require:"true" env:"FS_REQ_NOT_SET_VAR"`
	}{}
	set := FieldSet{}
	err := ValidateTags(&cfg, WithFieldSet(set))
	if err == nil {
		t.Fatal("expected require error")
	}
	if !strings.Contains(err.Error(), "required") || !strings.Contains(err.Error(), "FS_REQ_NOT_SET_VAR") {
		t.Errorf("error should mention required + env name, got %v", err)
	}
}

func TestRequire_YamlExplicitZeroPasses(t *testing.T) {
	cfg := struct {
		Port int `require:"true"`
	}{Port: 0}
	set := FieldSet{"Port": true}
	if err := ValidateTags(&cfg, WithFieldSet(set)); err != nil {
		t.Errorf("explicit zero should satisfy require, got %v", err)
	}
}

func TestRequire_EnvSatisfies(t *testing.T) {
	t.Setenv("FS_REQ_SECRET", "from-env")
	cfg := struct {
		Secret string `require:"true" env:"FS_REQ_SECRET" default:"nope"`
	}{}
	data := []byte("{}\n")
	if err := ApplyTagPasses(&cfg, data); err != nil {
		t.Fatalf("ApplyTagPasses: %v", err)
	}
	if cfg.Secret != "from-env" {
		t.Errorf("got %q", cfg.Secret)
	}
}

func TestRequire_StandaloneZeroFails(t *testing.T) {
	cfg := struct {
		Secret string `require:"true"`
	}{}
	err := ValidateTags(&cfg)
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Errorf("standalone require should fail on zero, got %v", err)
	}
}

// ---- env 卫生 ----

func TestEnv_InvalidName(t *testing.T) {
	cfg := struct {
		A string `env:"has space"`
	}{}
	err := ValidateTags(&cfg)
	if err == nil || !strings.Contains(err.Error(), "not a valid variable name") {
		t.Errorf("expected invalid env name error, got %v", err)
	}
}

func TestEnv_DuplicateName(t *testing.T) {
	cfg := struct {
		A string `env:"FS_DUP_VAR"`
		B string `env:"FS_DUP_VAR"`
	}{}
	err := ValidateTags(&cfg)
	if err == nil || !strings.Contains(err.Error(), "already used") {
		t.Errorf("expected duplicate env error, got %v", err)
	}
}

func TestEnv_TypoNoFalsePositive_SystemPath(t *testing.T) {
	// core#8：字段 Path / tag LOG_PATH 不得因宿主机 PATH 而误报
	t.Setenv("PATH", "/usr/bin") // 通常已存在，再设一次确保
	cfg := struct {
		Path string `yaml:"path" env:"LOG_PATH" default:""`
	}{}
	if err := ValidateTags(&cfg); err != nil {
		t.Errorf("valid LOG_PATH must not fail ValidateTags, got %v", err)
	}
	if err := ValidateTags(&cfg, WithEnvNamingLint()); err == nil {
		t.Error("naming lint should flag LOG_PATH != derived PATH")
	}
}

func TestEnv_RealTypoCaughtByNamingLint(t *testing.T) {
	// 真拼写错误 LOG_LEVELS（多 S）：静态命名检查可检出，且不依赖环境
	cfg := struct {
		Level string `yaml:"log_level" env:"LOG_LEVELS" default:"info"`
	}{}
	if err := ValidateTags(&cfg); err != nil {
		t.Errorf("default ValidateTags must stay quiet on naming, got %v", err)
	}
	err := ValidateTags(&cfg, WithEnvNamingLint())
	if err == nil || !strings.Contains(err.Error(), "LOG_LEVEL") {
		t.Fatalf("naming lint should catch LOG_LEVELS vs LOG_LEVEL, got %v", err)
	}
}

func TestEnv_IgnoreDerivedOptOut(t *testing.T) {
	cfg := struct {
		Path string `yaml:"path" env:"LOG_PATH" default:"" ignorederived:"true"`
	}{}
	if err := ValidateTags(&cfg, WithEnvNamingLint()); err != nil {
		t.Errorf("ignorederived should opt out, got %v", err)
	}
}

func TestLintEnvNaming_ListsMismatches(t *testing.T) {
	cfg := struct {
		Path  string `yaml:"path" env:"LOG_PATH"`
		Level string `yaml:"log_level" env:"LOG_LEVEL"`
		Home  string `yaml:"home" env:"MY_HOME" ignorederived:"true"`
	}{}
	issues, err := LintEnvNaming(&cfg)
	if err != nil {
		t.Fatalf("LintEnvNaming: %v", err)
	}
	if len(issues) != 1 || !strings.Contains(issues[0], "LOG_PATH") {
		t.Errorf("expected only LOG_PATH mismatch, got %v", issues)
	}
}

func TestEnv_DuplicateStillFails(t *testing.T) {
	cfg := struct {
		A string `env:"FS_DUP_VAR2"`
		B string `env:"FS_DUP_VAR2"`
	}{}
	err := ValidateTags(&cfg)
	if err == nil || !strings.Contains(err.Error(), "already used") {
		t.Errorf("duplicate env name must still fail, got %v", err)
	}
}

func TestToScreamingSnake(t *testing.T) {
	cases := map[string]string{
		"max_open":            "MAX_OPEN",
		"MaxOpen":             "MAX_OPEN",
		"BackupArchiveFormat": "BACKUP_ARCHIVE_FORMAT",
		"backupArchiveFormat": "BACKUP_ARCHIVE_FORMAT",
		"DSN":                 "DSN",
		"already_snake":       "ALREADY_SNAKE",
		"":                    "",
	}
	for in, want := range cases {
		if got := toScreamingSnake(in); got != want {
			t.Errorf("toScreamingSnake(%q)=%q, want %q", in, got, want)
		}
	}
}
