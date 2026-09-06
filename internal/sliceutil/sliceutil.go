package sliceutil

func Contains[T comparable](s []T, v T) bool {
	for _, item := range s {
		if item == v {
			return true
		}
	}
	return false
}

func Index[T comparable](s []T, v T) int {
	for i, item := range s {
		if item == v {
			return i
		}
	}
	return -1
}

func Unique[T comparable](s []T) []T {
	seen := make(map[T]bool)
	var result []T
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}

func Filter[T any](s []T, fn func(T) bool) []T {
	var result []T
	for _, v := range s {
		if fn(v) {
			result = append(result, v)
		}
	}
	return result
}

func Map[T, U any](s []T, fn func(T) U) []U {
	result := make([]U, len(s))
	for i, v := range s {
		result[i] = fn(v)
	}
	return result
}

func Reduce[T, U any](s []T, init U, fn func(U, T) U) U {
	result := init
	for _, v := range s {
		result = fn(result, v)
	}
	return result
}

func Any[T any](s []T, fn func(T) bool) bool {
	for _, v := range s {
		if fn(v) {
			return true
		}
	}
	return false
}

func All[T any](s []T, fn func(T) bool) bool {
	for _, v := range s {
		if !fn(v) {
			return false
		}
	}
	return true
}

func Chunk[T any](s []T, size int) [][]T {
	if size <= 0 {
		return nil
	}
	var result [][]T
	for i := 0; i < len(s); i += size {
		end := i + size
		if end > len(s) {
			end = len(s)
		}
		result = append(result, s[i:end])
	}
	return result
}

func Flatten[T any](s [][]T) []T {
	var result []T
	for _, sub := range s {
		result = append(result, sub...)
	}
	return result
}

func Reverse[T any](s []T) []T {
	result := make([]T, len(s))
	for i, v := range s {
		result[len(s)-1-i] = v
	}
	return result
}

func Drop[T any](s []T, n int) []T {
	if n >= len(s) {
		return nil
	}
	return s[n:]
}

func Take[T any](s []T, n int) []T {
	if n > len(s) {
		n = len(s)
	}
	return s[:n]
}

func GroupBy[T any, K comparable](s []T, fn func(T) K) map[K][]T {
	result := make(map[K][]T)
	for _, v := range s {
		key := fn(v)
		result[key] = append(result[key], v)
	}
	return result
}
