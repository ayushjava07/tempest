package ptrutil

func Of[T any](v T) *T {
	return &v
}

func Deref[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

func Equal[T comparable](a, b *T) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func Copy[T any](p *T) *T {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func IsNil(p any) bool {
	if p == nil {
		return true
	}
	switch v := p.(type) {
	case interface{ IsNil() bool }:
		return v.IsNil()
	case *any:
		return v == nil
	default:
		return false
	}
}
