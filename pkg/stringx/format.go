package stringx

import (
	"fmt"
	"strings"
)

// FormatTemplate 用 vars 替换 template 里的 {key} 占位符。
//
// 模板语法: `{key}` 形式,key 在 vars 里查找。
//   - 未找到的 key 保留原 `{key}` 不替换(避免静默替换空字符串导致歧义)
//   - 嵌套 key 不支持(只 1 层)
//   - 业务场景: 错误信息模板("user {user_id} not found in tenant {tenant_id}")
//     SQL 模板("SELECT * FROM users WHERE id = {id}"),log 模板
//
// 用法:
//
//	msg := stringx.FormatTemplate("user {user_id} not found", map[string]any{
//	    "user_id": 42,
//	})
//	// → "user 42 not found"
//
// 注意: 这个函数不转义;如果业务需要把用户输入嵌入 SQL/HTML,先做
// 业务自己的转义/参数化(否则是注入漏洞)。
func FormatTemplate(template string, vars map[string]any) string {
	if len(vars) == 0 {
		return template
	}
	var sb strings.Builder
	sb.Grow(len(template))
	i := 0
	for i < len(template) {
		if i+1 < len(template) && template[i] == '{' {
			// 找匹配的 '}'
			end := strings.IndexByte(template[i+1:], '}')
			if end < 0 {
				// 没找到闭合,原样 append 剩余
				sb.WriteString(template[i:])
				break
			}
			key := template[i+1 : i+1+end]
			if val, ok := vars[key]; ok {
				fmt.Fprintf(&sb, "%v", val)
			} else {
				// key 不存在,保留原 {key} 形式
				sb.WriteString(template[i : i+1+end+1])
			}
			i += 1 + end + 1
		} else {
			sb.WriteByte(template[i])
			i++
		}
	}
	return sb.String()
}
