package stringx

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"time"
)

// GenerateID 生成基于时间戳的唯一 ID，格式: 20060102150405-<8位hex>。
// 适用于非严格唯一性要求的场景（如日志关联、临时标识）。
// 如需全局唯一，请使用 uuid。
func GenerateID() string {
	now := time.Now()
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// 熵源不可用时降级：用纳秒时间戳的低 32 位。
		// 碰撞概率高于 crypto/rand 但仍可接受（同一秒内不同纳秒）。
		binary.LittleEndian.PutUint32(b, uint32(now.UnixNano()))
	}
	return now.Format("20060102150405") + "-" + hex.EncodeToString(b)
}

// ContainsAny 检查 text 中是否包含 keywords 中的任意一个关键词（不区分大小写）。
//
// 用法:
//
//	stringx.ContainsAny("Security vulnerability found", "security", "vuln") // true
//	stringx.ContainsAny("normal text", "critical", "major")                 // false
func ContainsAny(text string, keywords ...string) bool {
	lower := strings.ToLower(text)
	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// IndentText 为多行文本的第二行及之后每行添加缩进前缀。
// 第一行不缩进，适用于续行对齐场景。
//
// 用法:
//
//	stringx.IndentText("line1\nline2\nline3", "  ")
//	// => "line1\n  line2\n  line3"
func IndentText(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i := 1; i < len(lines); i++ {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}
