package fencing

import (
	"testing"
	"time"
)

func BenchmarkCASTokenGeneratorParallel(b *testing.B) {
	gen := NewCASTokenGenerator()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = gen.AcquireFastToken("bench-resource", "worker", 10*time.Second)
		}
	})
}

func BenchmarkMutexTokenGeneratorParallel(b *testing.B) {
	gen := NewTokenGenerator()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = gen.AcquireToken("bench-resource", "worker", 10*time.Second)
		}
	})
}
