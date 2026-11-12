package version

import (
	"testing"
	"time"
)

// 断言只用标准库（core#140）：本包测试此前为一条断言引入
// github.com/cloudwego/hertz/pkg/common/test/assert——一个测试便利库把
// hertz 拉进 core 的模块图，全矩阵下游在 MVS 下被迫接受重依赖。
// 生产代码对 hertz 的唯一合法消费面是 transport/hertzbridge（单源适配层），
// 由 CI 的 guard-hertz 任务守卫；version 测试面保持零框架依赖。
func TestMetadata(t *testing.T) {
	Version = "v0.8.0"
	BuildCommit = "abc1234"
	BuildBranch = "main"
	BuildTime = "2026-01-01"

	m := Metadata()
	want := map[string]string{
		"version":      "v0.8.0",
		"build_commit": "abc1234",
		"build_branch": "main",
		"build_time":   "2026-01-01",
	}
	for k, v := range want {
		if got := m[k]; got != v {
			t.Errorf("m[%q] = %q, want %q", k, got, v)
		}
	}
	if len(m) != len(want) {
		t.Errorf("Metadata() len = %d, want %d", len(m), len(want))
	}
}

func TestStartTime(t *testing.T) {
	if StartTime.IsZero() {
		t.Error("expected non-zero StartTime")
	}
	if time.Since(StartTime) > time.Minute {
		t.Error("StartTime should be recent")
	}
}
