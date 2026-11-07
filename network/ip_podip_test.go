package network

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

// TestPodIPFallback_LogsErrorAndKeepsLoopback 回归（oekit#141 core 侧）：
// GetPodIP 取不到本机 IP 时的回退必须**可观测**——ERROR 级日志携带原因；
// 返回值保持历史行为（127.0.0.1）不变——注册侧的回环拒绝防线（registry#64）
// 依赖这里不改签名、不改值，只加可见性。
func TestPodIPFallback_LogsErrorAndKeepsLoopback(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	got := podIPFallback(errors.New("network: no suitable local IP found"))
	if got != "127.0.0.1" {
		t.Fatalf("回退值应保持 127.0.0.1（兼容承诺），得到 %q", got)
	}
	out := buf.String()
	if !strings.Contains(out, "ERROR") {
		t.Errorf("回退必须打 ERROR 级日志，实际输出：\n%s", out)
	}
	for _, want := range []string{"network.podip", "no suitable local IP", "oekit#141"} {
		if !strings.Contains(out, want) {
			t.Errorf("回退日志应包含 %q（定位「注册为什么被拒」的第一现场），实际输出：\n%s", want, out)
		}
	}
}
