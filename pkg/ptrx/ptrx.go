package ptrx

// Of 返回 v 的指针副本。
func Of[T any](v T) *T {
	return &v
}

// Deref 解引用 p，当 p 为 nil 时返回 fallback。
func Deref[T any](p *T, fallback T) T {
	if p == nil {
		return fallback
	}
	return *p
}
