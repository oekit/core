package config

import "testing"

type overwriteNilInner struct {
	Host string
	Port int
}

type overwriteNilCfg struct {
	Scalar *string            `merge:"overwrite"`
	Block  *overwriteNilInner `merge:"overwrite"`
	Plain  *overwriteNilInner // 无 tag：nil 仍表示"未设置"，不得清 dst
	Keep   *overwriteNilInner `merge:"zeroable"`
}

// TestOverlayMerge_OverwritePointerNilClearsDst 回归 core#231：src 的
// overwrite 指针字段为 nil 时（远端配置删除该块）必须清空 dst，与
// zeroable 同权。此前 overwrite 走 continue 静默保留旧值——与 struct/map
// 分支的 overwrite 置零语义不对称，远端删除配置块的热更静默失效。
func TestOverlayMerge_OverwritePointerNilClearsDst(t *testing.T) {
	scalar := "old"
	dst := &overwriteNilCfg{
		Scalar: &scalar,
		Block:  &overwriteNilInner{Host: "old-host", Port: 8080},
		Plain:  &overwriteNilInner{Host: "plain"},
		Keep:   &overwriteNilInner{Host: "keep"},
	}
	src := &overwriteNilCfg{} // 所有指针 nil：模拟远端删除全部配置块

	OverlayMerge(dst, src)

	if dst.Scalar != nil {
		t.Errorf("overwrite pointer must be cleared on nil src, got %q", *dst.Scalar)
	}
	if dst.Block != nil {
		t.Errorf("overwrite *struct must be cleared on nil src, got %+v", dst.Block)
	}
	if dst.Keep != nil {
		t.Errorf("zeroable *struct must be cleared on nil src, got %+v", dst.Keep)
	}
	if dst.Plain == nil || dst.Plain.Host != "plain" {
		t.Errorf("untagged pointer nil must keep dst (未设置语义), got %+v", dst.Plain)
	}
}

// TestOverlayMerge_OverwritePointerNonNilStillReplaces 对照：src 非 nil 时
// overwrite 仍整体替换（含其内部零值字段），nil 清空修复不改变正常路径。
func TestOverlayMerge_OverwritePointerNonNilStillReplaces(t *testing.T) {
	dst := &overwriteNilCfg{
		Block: &overwriteNilInner{Host: "old", Port: 8080},
	}
	src := &overwriteNilCfg{
		Block: &overwriteNilInner{}, // 零值块也是有效替换
	}
	OverlayMerge(dst, src)

	if dst.Block == nil || dst.Block.Host != "" || dst.Block.Port != 0 {
		t.Errorf("overwrite must replace pointer target as a whole, got %+v", dst.Block)
	}
}

// TestOverlayMergeEnv_OverwritePointerNilKeepsDst 钉住 core#101 的 env 变体
// 不被 core#231 修复波及：env overlay 里 overwrite 指针为 nil = env 未命中，
// 不得清掉 dst 的文件值。
func TestOverlayMergeEnv_OverwritePointerNilKeepsDst(t *testing.T) {
	dst := &overwriteNilCfg{
		Block: &overwriteNilInner{Host: "from-file", Port: 8080},
	}
	src := &overwriteNilCfg{} // env overlay 未命中任何字段

	overlayMergeEnv(dst, src, nil)

	if dst.Block == nil || dst.Block.Host != "from-file" || dst.Block.Port != 8080 {
		t.Errorf("env overlay must not clear overwrite pointer on nil src, got %+v", dst.Block)
	}
}
