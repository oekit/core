package config

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
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
// 等价手写（合并一步走本包内部专用的 env 合并变体，见下）：
//
//	overlay := &T{}
//	set := FieldSet{}
//	_ = ApplyEnv(overlay, WithFieldSet(set)) // 空结构体上解 env → 命中补记进 set
//	overlayMergeEnv(dst, overlay, set)       // env 命中即覆盖（含显式零值）
//
// **env 命中即覆盖，含显式零值**（core#50）：`KUBOARD_MCP_ENABLED=false` 能把
// 文件里的 true 关掉、`APP_PORT=0` 能把非零端口清零。此前走 OverlayMerge 的
// 「非零值才覆盖」模型，env 显式设置的 false/0 落进 overlay 后被当零值跳过，
// 永远压不过文件值——12-factor 宿主最常见的布尔开关表达法（`ENV=false 关功能`）
// 因此静默失效，本函数现改走显式集合逃生门（core#50）+ 专用 env 合并变体
// （overlayMergeEnv，见 merge.go）。
//
// env 未设置（或显式空串/纯空白）的字段不影响 dst（不进命中集合）——**对
// merge:"overwrite"/"zeroable" 字段同样成立**（core#101）：env overlay 里
// 未命中字段恒为零值工件，若按 overwrite/zeroable 语义无条件写入，结构体
// 同时用于 OverlayMerge 热更（overwrite）与本函数（12-factor）时，后者每次
// 执行都会把 overwrite/zeroable 字段静默清掉。env 变体下未命中字段的零值
// 一律跳过，非零值仍按 merge tag 语义覆盖。
// env 值无法按字段类型解析时 fail-closed 报错；可传 WithLenientEnv 转为
// WARN + 跳过——被跳过的字段不进命中集合，不会用零值清掉 dst 的文件值
// （见 fieldset.go）。
//
// 可传 WithEnvNaming 启用 env 命名策略（前缀 / 逐字段覆盖）——见 envnaming.go。
// 注意：本函数内部自带 FieldSet 记录 env 命中路径，传入 WithFieldSet 会被
// 覆盖（overlay 侧没有 yaml 显式值需要保护）。
//
// 切片元素字段同样支持（core#85）：此前 overlay 是空结构体，切片全 nil，
// ApplyEnv 无法到达元素字段（walkTaggedStruct 遍历 0 个元素），元素上的
// env tag / 覆盖表条目静默失效——与 ApplyEnv（yaml 优先路径下元素存在、
// env 生效）行为相反。现按 dst 的切片长度给 overlay 构造**零值**元素
// （不拷 dst 值：env 命中即覆盖的语义要求 env 值不因 dst 已有值而缺席），
// 合并侧对带元素级命中路径的切片逐元素下推——dst 非 env 命中的元素字段
// 保持 yaml 值，命中的字段取 env 值。
func ApplyEnvOverlay[T any](dst *T, opts ...PassOption) error {
	if dst == nil {
		return nil
	}
	var overlay T
	var grown []grownSliceRef
	growOverlaySlices(reflect.ValueOf(&overlay).Elem(), reflect.ValueOf(dst).Elem(), "", &grown)
	set := FieldSet{}
	overlayOpts := make([]PassOption, 0, len(opts)+1)
	overlayOpts = append(overlayOpts, opts...)
	overlayOpts = append(overlayOpts, WithFieldSet(set))
	if err := ApplyEnv(&overlay, overlayOpts...); err != nil {
		return err
	}
	// 没有任何 env 落点的构造元素回缩为 nil：构造只服务于「env 能不能到达
	// 元素字段」，env 未命中时零值元素若留在 overlay 里，会触发合并侧的
	// 整体替换把 dst 清零——回缩后该切片回到旧判定（nil ⇒ 不覆盖 dst）。
	for _, g := range grown {
		if !hasElementExplicit(set, g.path) {
			g.val.Set(reflect.Zero(g.val.Type()))
		}
	}
	overlayMergeEnv(dst, &overlay, set)
	return nil
}

// grownSliceRef 记录一个被构造出来的 overlay 切片：path 是 Go 字段路径
// （与 FieldSet 键同口径，前缀匹配元素级命中用），val 是切片字段本身
// （overlay 为局部变量，字段值地址稳定，ApplyEnv 不会移动它）。
type grownSliceRef struct {
	path string
	val  reflect.Value
}

// growOverlaySlices 把 dst 中 []struct / []*struct 切片的长度同步到 overlay
// （同类型两棵结构体树同步下降）：元素为**零值**（[]struct）或指向零值的
// 新指针（[]*struct），让 ApplyEnv 的元素遍历有落点。嵌套 struct / *struct
// 继续下探；map 不遍历（与 tag pass 口径一致）。构造过的切片记入 grown
// （路径 + 字段值），供 ApplyEnv 之后按「有没有元素级 env 命中」决定回缩。
func growOverlaySlices(overlay, dst reflect.Value, path string, grown *[]grownSliceRef) {
	if overlay.Kind() != reflect.Struct || dst.Kind() != reflect.Struct {
		return
	}
	t := overlay.Type()
	for i := 0; i < t.NumField(); i++ {
		if !t.Field(i).IsExported() {
			continue
		}
		of, df := overlay.Field(i), dst.Field(i)
		fpath := t.Field(i).Name
		if path != "" {
			fpath = path + "." + fpath
		}
		switch of.Kind() {
		case reflect.Slice:
			elem := of.Type().Elem()
			isStruct := elem.Kind() == reflect.Struct
			isStructPtr := elem.Kind() == reflect.Pointer && elem.Elem().Kind() == reflect.Struct
			if df.Len() == 0 || of.Len() > 0 || (!isStruct && !isStructPtr) {
				continue
			}
			g := reflect.MakeSlice(of.Type(), df.Len(), df.Len())
			if isStructPtr {
				for j := 0; j < df.Len(); j++ {
					g.Index(j).Set(reflect.New(elem.Elem())) // 指向零值，元素字段可被 env 命中
				}
			}
			of.Set(g)
			*grown = append(*grown, grownSliceRef{path: fpath, val: of})
		case reflect.Struct:
			growOverlaySlices(of, df, fpath, grown)
		case reflect.Pointer:
			if of.Type().Elem().Kind() == reflect.Struct && !of.IsNil() && !df.IsNil() {
				growOverlaySlices(of.Elem(), df.Elem(), fpath, grown)
			}
		}
	}
}
