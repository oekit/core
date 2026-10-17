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
//	_ = ApplyEnv(overlay)        // 空结构体上解 env → env 非零值进入 overlay
//	OverlayMerge(dst, overlay)   // overlay 的非零值压过 dst（文件值）
//
// env 未设置的字段不影响 dst（overlay 对应字段为零值，OverlayMerge 跳过）。
// 注意：OverlayMerge 是「非零值才覆盖」，env 显式配置零值（如 APP_PORT=0）
// 无法把文件里的非零值清成零——零值歧义与 ApplyEnv 一致，见 tags.go 文档。
func ApplyEnvOverlay[T any](dst *T) error {
	var overlay T
	if err := ApplyEnv(&overlay); err != nil {
		return err
	}
	OverlayMerge(dst, &overlay)
	return nil
}
