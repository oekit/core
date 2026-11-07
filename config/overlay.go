package config

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// OverlaySource 把多个 ConfigSource 组合为一个：
//
//   - Read：按参数顺序逐个读取并深合并（后者覆盖前者），返回合并后的 blob。
//     任一子源读取失败即返回该错误
//   - Watch：任一子源变化即触发 onChange（扇入），ctx 取消传播到全部子源
//
// 合并语义为 JSON blob 级深合并（对象递归、标量/数组整体覆盖），与
// OverlayMerge 的结构体级合并互补——多来源场景推荐
// OverlaySource + ConfigWatcher[T] 组合实现多源热更新。
// 所有子源须产出同一格式（JSON）内容。
func OverlaySource(srcs ...ConfigSource) ConfigSource {
	return &overlaySource{srcs: srcs}
}

type overlaySource struct {
	srcs []ConfigSource
}

func (o *overlaySource) Read() ([]byte, error) {
	var merged map[string]any
	for i, src := range o.srcs {
		data, err := src.Read()
		if err != nil {
			return nil, fmt.Errorf("overlay source[%d]: %w", i, err)
		}
		if len(data) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("overlay source[%d]: decode json: %w", i, err)
		}
		merged = deepMergeMaps(merged, m)
	}
	if merged == nil {
		return []byte("{}"), nil
	}
	out, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("overlay: encode merged json: %w", err)
	}
	return out, nil
}

func (o *overlaySource) Watch(ctx context.Context, onChange func()) {
	var wg sync.WaitGroup
	for _, src := range o.srcs {
		wg.Add(1)
		s := src
		go func() {
			defer wg.Done()
			s.Watch(ctx, onChange)
		}()
	}
	go func() {
		wg.Wait() // ctx 取消后各子源 Watch 返回，扇入 goroutine 随之退出
	}()
}

// deepMergeMaps 深合并 b 到 a（b 覆盖 a）：map 递归合并，其余类型整体覆盖。
func deepMergeMaps(a, b map[string]any) map[string]any {
	if a == nil {
		return b
	}
	for k, bv := range b {
		if bm, ok := bv.(map[string]any); ok {
			if am, ok := a[k].(map[string]any); ok {
				a[k] = deepMergeMaps(am, bm)
				continue
			}
		}
		a[k] = bv
	}
	return a
}

// ApplyEnvOverlay 把环境变量解析到独立的 overlay 并覆盖 dst（#2）：
// 与 ApplyEnv（只填零值字段，「yaml 显式值优先」）语义相反——本函数实现
// 「env 优先于配置文件」的 12-factor 场景（Helm/docker-compose 靠 env 传配置）。
//
// 等价手写：
//
//	overlay := &T{}
//	set := FieldSet{}
//	_ = ApplyEnv(overlay, WithFieldSet(set)) // 空结构体上解 env → 命中补记进 set
//	OverlayMergeExplicit(dst, overlay, set)  // env 命中即覆盖（含显式零值）
//
// **env 命中即覆盖，含显式零值**（core#50）：`KUBOARD_MCP_ENABLED=false` 能把
// 文件里的 true 关掉、`APP_PORT=0` 能把非零端口清零。此前走 OverlayMerge 的
// 「非零值才覆盖」模型，env 显式设置的 false/0 落进 overlay 后被当零值跳过，
// 永远压不过文件值——12-factor 宿主最常见的布尔开关表达法（`ENV=false 关功能`）
// 因此静默失效，本函数现改走 OverlayMergeExplicit（显式集合逃生门，见 merge.go）。
//
// env 未设置（或显式空串/纯空白）的字段不影响 dst（不进命中集合）。
// env 值无法按字段类型解析时 fail-closed 报错；可传 WithLenientEnv 转为
// WARN + 跳过——被跳过的字段不进命中集合，不会用零值清掉 dst 的文件值
// （见 fieldset.go）。
//
// 可传 WithEnvNaming 启用 env 命名策略（前缀 / 逐字段覆盖）——见 envnaming.go。
// 注意：本函数内部自带 FieldSet 记录 env 命中路径，传入 WithFieldSet 会被
// 覆盖（overlay 侧没有 yaml 显式值需要保护）。
func ApplyEnvOverlay[T any](dst *T, opts ...PassOption) error {
	var overlay T
	set := FieldSet{}
	overlayOpts := make([]PassOption, 0, len(opts)+1)
	overlayOpts = append(overlayOpts, opts...)
	overlayOpts = append(overlayOpts, WithFieldSet(set))
	if err := ApplyEnv(&overlay, overlayOpts...); err != nil {
		return err
	}
	OverlayMergeExplicit(dst, &overlay, set)
	return nil
}
