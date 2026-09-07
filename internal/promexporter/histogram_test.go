package promexporter

import (
	"math"
	"sync"
	"testing"
)

func TestHistogramBucketDistribution(t *testing.T) {
	buckets := []float64{0.1, 0.5, 1.0, 5.0}
	h := NewHistogram("test_duration", "duration", map[string]string{"service": "api"}, buckets)

	observations := []float64{0.05, 0.2, 0.4, 0.9, 2.0, 7.0}
	for _, obs := range observations {
		h.Observe(obs)
	}

	samples := h.Samples()

	bucketCounts := make(map[string]float64)
	var sumVal, countVal float64

	for _, s := range samples {
		if s.Name == "test_duration_bucket" {
			le := s.Labels["le"]
			bucketCounts[le] = s.Value
		} else if s.Name == "test_duration_sum" {
			sumVal = s.Value
		} else if s.Name == "test_duration_count" {
			countVal = s.Value
		}
	}

	if bucketCounts["0.1"] != 1 {
		t.Fatalf("expected 1 in le=0.1, got %f", bucketCounts["0.1"])
	}
	if bucketCounts["0.5"] != 3 {
		t.Fatalf("expected 3 in le=0.5, got %f", bucketCounts["0.5"])
	}
	if bucketCounts["1"] != 4 {
		t.Fatalf("expected 4 in le=1, got %f", bucketCounts["1"])
	}
	if bucketCounts["5"] != 5 {
		t.Fatalf("expected 5 in le=5, got %f", bucketCounts["5"])
	}
	if bucketCounts["+Inf"] != 6 {
		t.Fatalf("expected 6 in le=+Inf, got %f", bucketCounts["+Inf"])
	}

	if countVal != 6 {
		t.Fatalf("expected count 6, got %f", countVal)
	}
	if math.Abs(sumVal-10.55) > 0.0001 {
		t.Fatalf("expected sum ~10.55, got %f", sumVal)
	}
}

func TestConcurrentHistogramObservations(t *testing.T) {
	h := NewHistogram("concurrent_obs", "help", nil, DefaultBuckets)

	const goroutines = 20
	const perRoutine = 100
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(val float64) {
			defer wg.Done()
			for j := 0; j < perRoutine; j++ {
				h.Observe(val)
			}
		}(float64(i) * 0.05)
	}

	wg.Wait()

	samples := h.Samples()
	var totalCount float64
	for _, s := range samples {
		if s.Name == "concurrent_obs_count" {
			totalCount = s.Value
		}
	}

	expectedCount := float64(goroutines * perRoutine)
	if totalCount != expectedCount {
		t.Fatalf("expected total count %f, got %f", expectedCount, totalCount)
	}
}
