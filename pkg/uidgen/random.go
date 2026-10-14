// crypto/rand 随机串生成（标识符/密钥面）：与 Snowflake（可预测、仅作 ID）分居——
// 密钥、一次性凭据、高熵标识符必须走本文件。
package uidgen

import (
	"crypto/rand"
	"encoding/hex"
)

// RandomHex n 字节 crypto/rand 随机数的十六进制串（2n 字符）。n<=0 返回空串。
// 适用：对象 ID、webhook secret 等高熵标识符。
func RandomHex(nBytes int) string {
	if nBytes <= 0 {
		return ""
	}
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		panic("uidgen: crypto/rand 不可用: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// RandomString 从 alphabet 均匀采样 n 个字符（拒绝采样消模偏差，无 mod 偏置）。
// alphabet 为空时 panic。适用：带自定义字符集的短 ID/令牌。
func RandomString(alphabet string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(alphabet) == 0 {
		panic("uidgen: 空 alphabet")
	}
	if len(alphabet) == 1 {
		out := make([]byte, n)
		for i := range out {
			out[i] = alphabet[0]
		}
		return string(out)
	}
	// 拒绝采样：256 % len 会造成高位取值偏差，丢弃 >= maxMultiple 的字节。
	maxMultiple := 256 - 256%len(alphabet)
	out := make([]byte, 0, n)
	buf := make([]byte, n+8)
	for len(out) < n {
		if _, err := rand.Read(buf); err != nil {
			panic("uidgen: crypto/rand 不可用: " + err.Error())
		}
		for _, c := range buf {
			if int(c) < maxMultiple {
				out = append(out, alphabet[int(c)%len(alphabet)])
				if len(out) == n {
					break
				}
			}
		}
	}
	return string(out)
}
