package webhookdelivery

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"
)

// PooledClientConfig configures high-throughput connection reuse parameters.
type PooledClientConfig struct {
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	MaxConnsPerHost     int
	IdleConnTimeout     time.Duration
	RequestTimeout      time.Duration
}

// DefaultPooledConfig returns production tuned transport parameters.
func DefaultPooledConfig() PooledClientConfig {
	return PooledClientConfig{
		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 100,
		MaxConnsPerHost:     250,
		IdleConnTimeout:     90 * time.Second,
		RequestTimeout:      15 * time.Second,
	}
}

// NewPooledHTTPClient constructs an http.Client with connection pooling and keep-alives.
func NewPooledHTTPClient(cfg PooledClientConfig) *http.Client {
	if cfg.MaxIdleConns <= 0 {
		cfg = DefaultPooledConfig()
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   cfg.MaxIdleConnsPerHost,
		MaxConnsPerHost:       cfg.MaxConnsPerHost,
		IdleConnTimeout:       cfg.IdleConnTimeout,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   cfg.RequestTimeout,
	}
}

// BatchDispatcher orchestrates concurrent parallel dispatch of webhook batches using worker pools.
type BatchDispatcher struct {
	worker         *DeliveryWorker
	maxConcurrency int
}

// NewBatchDispatcher creates a batch delivery dispatcher.
func NewBatchDispatcher(worker *DeliveryWorker, maxConcurrency int) *BatchDispatcher {
	if maxConcurrency <= 0 {
		maxConcurrency = 20
	}
	return &BatchDispatcher{
		worker:         worker,
		maxConcurrency: maxConcurrency,
	}
}

// DispatchBatch sends a collection of delivery records across endpoints concurrently.
func (b *BatchDispatcher) DispatchBatch(ctx context.Context, records []*DeliveryRecord, eps map[string]EndpointConfig) error {
	sem := make(chan struct{}, b.maxConcurrency)
	var wg sync.WaitGroup

	for _, rec := range records {
		ep, ok := eps[rec.EndpointID]
		if !ok || !ep.Active {
			continue
		}

		wg.Add(1)
		go func(r *DeliveryRecord, endpoint EndpointConfig) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			_ = b.worker.Deliver(ctx, r, endpoint, SignPayload)
		}(rec, ep)
	}

	wg.Wait()
	return ctx.Err()
}
