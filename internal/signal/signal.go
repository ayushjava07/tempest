package signal

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

type Signal struct {
	sig os.Signal
}

func Catch(signals ...os.Signal) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, signals...)
	go func() {
		<-sigCh
		cancel()
	}()
	return ctx
}

func Notify(signals ...os.Signal) <-chan os.Signal {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, signals...)
	return sigCh
}

func Ignore(signals ...os.Signal) {
	signal.Ignore(signals...)
}

func Reset(signals ...os.Signal) {
	signal.Reset(signals...)
}

func WaitForShutdown(signals ...os.Signal) os.Signal {
	sigCh := make(chan os.Signal, 1)
	if len(signals) == 0 {
		signals = []os.Signal{syscall.SIGINT, syscall.SIGTERM}
	}
	signal.Notify(sigCh, signals...)
	return <-sigCh
}