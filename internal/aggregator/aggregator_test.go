package aggregator

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestReservoir_QuantilesAndStats(t *testing.T) {
	res := NewReservoir(100)

	// Add integers 1 through 100
	for i := 1; i <= 100; i++ {
		res.Add(float64(i))
	}

	count, min, max, mean := res.Stats()
	if count != 100 {
		t.Fatalf("expected count 100, got %d", count)
	}
	if min != 1.0 || max != 100.0 {
		t.Fatalf("expected min 1.0 and max 100.0, got min=%f max=%f", min, max)
	}
	if math.Abs(mean-50.5) > 0.001 {
		t.Fatalf("expected mean 50.5, got %f", mean)
	}

	// Test quantiles
	p50, err := res.Quantile(0.50)
	if err != nil {
		t.Fatalf("Quantile(0.50) error: %v", err)
	}
	if math.Abs(p50-50.5) > 1.0 {
		t.Fatalf("expected P50 around 50.5, got %f", p50)
	}

	p99, err := res.Quantile(0.99)
	if err != nil {
		t.Fatalf("Quantile(0.99) error: %v", err)
	}
	if math.Abs(p99-99.0) > 1.0 {
		t.Fatalf("expected P99 around 99.0, got %f", p99)
	}
}

func TestRollingSeries_Pruning(t *testing.T) {
	cfg := SeriesConfig{
		BucketDuration: 10 * time.Millisecond,
		WindowDuration: 50 * time.Millisecond,
		ReservoirSize:  64,
	}
	series := NewRollingSeries(cfg)

	base := time.Now()
	// Record at t=0
	series.Record(10.0, base)
	// Record at t=20ms
	series.Record(20.0, base.Add(20*time.Millisecond))
	// Record at t=65ms (window is 50ms, cutoff is 15ms -> t=0 is pruned, t=20ms and t=65ms kept)
	series.Record(30.0, base.Add(65*time.Millisecond))

	_, count, sum := series.Snapshot()
	// Bucket at t=0 is outside [100ms - 50ms, 100ms], so it was pruned
	if count != 2 {
		t.Fatalf("expected 2 active data points after pruning, got %d", count)
	}
	if sum != 50.0 {
		t.Fatalf("expected sum 50.0, got %f", sum)
	}
}

func TestAggregator_NamedMetricsWithLabels(t *testing.T) {
	cfg := SeriesConfig{
		BucketDuration: 1 * time.Second,
		WindowDuration: 10 * time.Second,
		ReservoirSize:  128,
	}
	agg := New(cfg)

	labels := map[string]string{"workflow": "etl", "node": "worker-1"}

	for i := 1; i <= 50; i++ {
		agg.Record("task_duration_ms", labels, float64(i*10))
	}

	quantiles, err := agg.GetQuantiles("task_duration_ms", labels, 0.50, 0.90)
	if err != nil {
		t.Fatalf("GetQuantiles failed: %v", err)
	}

	if quantiles[0.50] < 200 || quantiles[0.50] > 300 {
		t.Fatalf("expected P50 around 250, got %f", quantiles[0.50])
	}
	if quantiles[0.90] < 400 || quantiles[0.90] > 500 {
		t.Fatalf("expected P90 around 450, got %f", quantiles[0.90])
	}

	rate, totalCount, err := agg.Rate("task_duration_ms", labels)
	if err != nil {
		t.Fatalf("Rate failed: %v", err)
	}
	if totalCount != 50 {
		t.Fatalf("expected count 50, got %d", totalCount)
	}
	if rate != 5.0 { // 50 / 10s
		t.Fatalf("expected rate 5.0, got %f", rate)
	}
}

func TestAggregator_SweeperLifecycle(t *testing.T) {
	cfg := SeriesConfig{
		BucketDuration: 10 * time.Millisecond,
		WindowDuration: 50 * time.Millisecond,
		ReservoirSize:  64,
	}
	agg := New(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	agg.StartSweeper(ctx, 20*time.Millisecond)
	agg.Record("metric_a", nil, 42.0)

	time.Sleep(50 * time.Millisecond)
	agg.Stop()
}

func TestAggregator_ConcurrentAccess(t *testing.T) {
	cfg := SeriesConfig{
		BucketDuration: 100 * time.Millisecond,
		WindowDuration: 1 * time.Second,
		ReservoirSize:  256,
	}
	agg := New(cfg)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			lbls := map[string]string{"worker": "pool"}
			for j := 0; j < 50; j++ {
				agg.Record("concurrent_metric", lbls, float64(id*10+j))
			}
			_, _ = agg.GetQuantiles("concurrent_metric", lbls, 0.50, 0.95)
		}(i)
	}

	wg.Wait()
}
