package encoding

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// Fnv1a64 返回数据的 64 位 FNV-1a 哈希值（非加密哈希，用于缓存分片/内容粗校验）。
// 跨服务持久化过哈希值的场景请勿更改算法假设（历史名 Hash，2026-09 改为如实命名）。
func Fnv1a64(data []byte) uint64 {
	// FNV-1a 64 位参数：offset basis 14695981039346656037，prime 1099511628211
	var h uint64 = 14695981039346656037
	for _, b := range data {
		h ^= uint64(b)
		h *= 1099511628211
	}
	return h
}

// Sha256Hex 返回数据的 SHA-256 十六进制字符串。
// 适用于内容校验和、缓存键等非加密签名场景（2026-09-15 治理轮：
// 移除 Md5/Md5Hex/Md5Bytes/Sha1 包装——共享 SDK 提供弱哈希入口容易被
// 误用到凭据/签名场景；确有 MD5/SHA-1 合规需求的业务自行引 crypto 标准库）。
func Sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Sha256 返回字符串的 SHA-256 十六进制字符串。
func Sha256(str string) string {
	return Sha256Hex([]byte(str))
}

// Base64Encode 返回字符串的 Base64 编码。
func Base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// Base64Decode 解码 Base64 编码的字符串。
func Base64Decode(s string) (string, error) {
	rs, err := base64.StdEncoding.DecodeString(s)
	return string(rs), err
}

// Base64URLEncode 返回字符串的 Base64 URL 安全编码。
func Base64URLEncode(s string) string {
	return base64.URLEncoding.EncodeToString([]byte(s))
}

// Base64URLDecode 解码 Base64 URL 安全编码的字符串。
func Base64URLDecode(s string) (string, error) {
	rs, err := base64.URLEncoding.DecodeString(s)
	return string(rs), err
}
