package uidgen

import (
	"strings"

	"github.com/google/uuid"
)

// UUID 返回一个不带连字符的 UUID 字符串。
func UUID() string {
	u := uuid.New()
	return strings.ReplaceAll(u.String(), "-", "")
}

// ShortUUID 返回一个 16 位的短 UUID。
func ShortUUID() string {
	return UUID()[:16]
}

// UUIDToUpper 返回大写的 UUID 字符串。
func UUIDToUpper() string {
	return strings.ToUpper(UUID())
}

// ShortUUIDToUpper 返回大写的 16 位短 UUID。
func ShortUUIDToUpper() string {
	return UUIDToUpper()[:16]
}

// UUIDWithHyphen 返回带连字符的 UUID 字符串。
func UUIDWithHyphen() string {
	return uuid.New().String()
}
