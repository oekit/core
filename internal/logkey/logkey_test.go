package logkey

import "testing"

// TestErrValue 钉住单源常量的值：logx.ErrKey 的值是 "err"（两矩阵日志
// 落同一 Loki/ES 的检索口径），本仓常量必须与之逐字一致——常量被人改动
// 时此处立刻爆红，防止静默分裂成第三种字段名（core#149）。
func TestErrValue(t *testing.T) {
	if Err != "err" {
		t.Fatalf("logkey.Err = %q, want %q（必须与 logx.ErrKey 同值）", Err, "err")
	}
}
