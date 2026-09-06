package scheduler

import (
	"context"
	"sync"
)

type Pool struct {
	sem chan struct{}
	wg  sync.WaitGroup
}

func NewPool(workers int) *Pool {
	if workers <= 0 {
		workers = 4
	}
	return &Pool{sem: make(chan struct{}, workers)}
}

func (p *Pool) SubmitContext(ctx context.Context, fn func()) {
	p.sem <- struct{}{}
	p.wg.Add(1)
	go func() {
		defer func() {
			<-p.sem
			p.wg.Done()
		}()
		fn()
	}()
}

func (p *Pool) Shutdown() {
	p.wg.Wait()
}
