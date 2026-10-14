package errors

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"sync/atomic"
)

var _singlelineSeparator = []byte("; ")

var _bufferPool = sync.Pool{
	New: func() any {
		return &bytes.Buffer{}
	},
}

// MultiError 是聚合后的多错误类型。
// 实现 error、errors.Is（遍历检查）和 Errors()（获取子错误列表）。
type MultiError struct {
	copyNeeded atomic.Bool
	errors     []error
}

// Errors 返回所有子错误的切片。
func (merr *MultiError) Errors() []error {
	if merr == nil {
		return nil
	}
	return merr.errors
}

// Is 实现 errors.Is 接口，遍历所有子错误进行匹配。
func (merr *MultiError) Is(target error) bool {
	for _, err := range merr.Errors() {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// Unwrap 支持 errors.As / errors.Is 遍历子错误（Go 1.20 多错误解包）。
// 没有它，聚合进 MultiError 的 BizError 无法被 errors.As 识别，
// mw.Fail 会把本应 404 的错误降级为 500。
func (merr *MultiError) Unwrap() []error {
	if merr == nil {
		return nil
	}
	return merr.errors
}

// Error 返回所有错误用 "; " 连接的字符串。
func (merr *MultiError) Error() string {
	if merr == nil {
		return ""
	}

	buff := _bufferPool.Get().(*bytes.Buffer)
	buff.Reset()

	merr.writeSingleline(buff)

	result := buff.String()
	_bufferPool.Put(buff)
	return result
}

func (merr *MultiError) writeSingleline(w io.Writer) {
	first := true
	for _, item := range merr.errors {
		if first {
			first = false
		} else {
			_, _ = w.Write(_singlelineSeparator)
		}
		_, _ = io.WriteString(w, item.Error())
	}
}

type inspectResult struct {
	Count              int
	Capacity           int
	FirstErrorIdx      int
	ContainsMultiError bool
}

func inspect(errors []error) (res inspectResult) {
	first := true
	for i, err := range errors {
		if err == nil {
			continue
		}

		res.Count++
		if first {
			first = false
			res.FirstErrorIdx = i
		}

		if merr, ok := err.(*MultiError); ok {
			res.Capacity += len(merr.errors)
			res.ContainsMultiError = true
		} else {
			res.Capacity++
		}
	}
	return
}

func fromSlice(errors []error) error {
	res := inspect(errors)
	switch res.Count {
	case 0:
		return nil
	case 1:
		return errors[res.FirstErrorIdx]
	case len(errors):
		if !res.ContainsMultiError {
			return &MultiError{errors: errors}
		}
	}

	nonNilErrs := make([]error, 0, res.Capacity)
	for _, err := range errors[res.FirstErrorIdx:] {
		if err == nil {
			continue
		}

		if nested, ok := err.(*MultiError); ok {
			nonNilErrs = append(nonNilErrs, nested.errors...)
		} else {
			nonNilErrs = append(nonNilErrs, err)
		}
	}

	return &MultiError{errors: nonNilErrs}
}

// Append 将两个错误合并为一个。
// 任一为 nil 时返回另一个；都非 nil 时返回聚合后的 MultiError。
// 优化了常见的"反复 Append 到左侧"模式（copy-on-write）。
func Append(left error, right error) error {
	switch {
	case left == nil:
		return right
	case right == nil:
		return left
	}

	if _, ok := right.(*MultiError); !ok {
		if l, ok := left.(*MultiError); ok && !l.copyNeeded.Swap(true) {
			//nolint:gocritic // 有意 append 到新变量，避免修改共享的 l.errors 切片
			errs := append(l.errors, right)
			return &MultiError{errors: errs}
		} else if !ok {
			return &MultiError{errors: []error{left, right}}
		}
	}

	errors := [2]error{left, right}
	return fromSlice(errors[0:])
}
