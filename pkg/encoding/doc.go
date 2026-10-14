// Package encoding 提供常用的编码和哈希工具函数。
//
// 当前能力：JSON 编解码包装（MarshalJSON/UnmarshalJSON 等）、非加密哈希
// （Fnv1a64）、加密级摘要（Sha256/Sha256Hex）、Base64（标准/URL 安全）。
//
// 历史说明：曾提供 Md5/Md5Hex/Md5Bytes/Sha1 弱哈希包装，2026-09-15 治理轮
// 移除（共享 SDK 提供弱哈希入口容易被误用到凭据/签名场景）；确有 MD5/SHA-1
// 合规需求的业务自行使用 crypto 标准库。
package encoding
