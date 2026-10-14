package jsonrepair

import (
	"regexp"
	"strings"
)

// fenceRe markdown 代码围栏（```json/``` 均可；取第一个围栏块内容）。
var fenceRe = regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)\\s*```")

// stripCodeFence 剥离 markdown 代码围栏：取第一个 ``` 围栏块；无栅栏原样
// TrimSpace 返回。多围栏块输出（模型补多段代码）语义为「取第一块」——与
// 宽容解析链一致（语义与 agentkit/textutil.StripCodeFence 同源，下沉时内联：
// 本包 stdlib-only，不引上游 SDK 依赖）。
func stripCodeFence(s string) string {
	trimmed := strings.TrimSpace(s)
	if m := fenceRe.FindStringSubmatch(trimmed); m != nil {
		return strings.TrimSpace(m[1])
	}
	return trimmed
}
