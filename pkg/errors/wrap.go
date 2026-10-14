package errors

import (
	stderrors "errors"
	"fmt"
)

// Wrap 用 msg 包装 err 并返回新 err。err 为 nil 时返回 nil(不创建无意义包装)。
// kv 列表作为附加上下文附加到包装消息(用于日志关联,例如 operation 名)。
//
// 跟 fmt.Errorf("...: %w", err) 的区别: Wrap 显式 nil-safe(避免
// "%!w(<nil>)" 之类输出),并支持附加 kv 对。
//
// 业务场景:
//
//	if err := db.Query(...); err != nil {
//	    return errors.Wrap(err, "query user", "user_id", uid)
//	}
//
// 输出: "query user: <原始 err> user_id=42"(kv 用 = 分隔拼接)
// 解包: errors.Is / errors.As 仍能识别原始 err(走 Unwrap)。
func Wrap(err error, msg string, kv ...any) error {
	if err == nil {
		return nil
	}
	// 拼接 msg + ": " + err.Error()
	// 加上 kv 后缀(每对 key+value 用 = 拼接)
	if len(kv) == 0 {
		return &wrappedError{msg: msg, err: err}
	}
	return &wrappedError{msg: msg, err: err, kv: kv}
}

// wrappedError 是 Wrap 内部用的结构,实现 Unwrap + 携带 kv。
type wrappedError struct {
	msg string
	err error
	kv  []any
}

func (w *wrappedError) Error() string {
	if len(w.kv) == 0 {
		return w.msg + ": " + w.err.Error()
	}
	return w.msg + ": " + w.err.Error() + " " + formatKV(w.kv)
}

// Unwrap 让 errors.Is / errors.As 能识别原始 err。
func (w *wrappedError) Unwrap() error {
	return w.err
}

// formatKV 把 k/v 对格式化成 "k1=v1 k2=v2" 形式。偶数长度,否则丢弃最后一个。
func formatKV(kv []any) string {
	if len(kv)%2 != 0 {
		kv = kv[:len(kv)-1]
	}
	var sb []byte
	for i := 0; i < len(kv); i += 2 {
		if i > 0 {
			sb = append(sb, ' ')
		}
		sb = append(sb, fmt.Sprintf("%v=%v", kv[i], kv[i+1])...)
	}
	return string(sb)
}

// Cause 是 errors.Cause 的便捷版:返回 wrap 链最底层的 error。
// 等价 errors.Unwrap 递归到 nil。
func Cause(err error) error {
	for {
		u := stderrors.Unwrap(err)
		if u == nil {
			return err
		}
		err = u
	}
}
