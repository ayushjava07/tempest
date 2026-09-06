package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/tempest-io/tempest/internal/api"
	"github.com/tempest-io/tempest/internal/auth"
	grpcapi "github.com/tempest-io/tempest/internal/grpcapi"
	"github.com/tempest-io/tempest/internal/persistence/memstore"
	"github.com/tempest-io/tempest/internal/scheduler"
)

func serverCmd() *Command {
	return &Command{
		Name:    "server",
		Summary: "start the Tempest server",
		Run: func(ctx *Context) error {
			addr := ":8080"
			if len(ctx.Args) > 0 {
				addr = ctx.Args[0]
			}
			store := memstore.New()
			resolver := auth.NewResolver(store)
			engine := scheduler.NewEngine(store, nil, scheduler.Options{})
			httpServer := api.NewServer(api.ServerOptions{
				Store:    store,
				Resolver: resolver,
				Engine:   engine,
				Addr:     addr,
			})
			grpcAddr := ":9090"
			grpcServer, err := grpcapi.NewGRPCServer(grpcapi.GRPCServerOptions{
				Store:    store,
				Engine:   engine,
				Resolver: resolver,
				Addr:     grpcAddr,
			})
			if err != nil {
				return fmt.Errorf("grpc server: %w", err)
			}
			go func() { _ = grpcServer.Serve(grpcAddr) }()
			quit := make(chan os.Signal, 1)
			signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
			stopCtx, stopCancel := context.WithCancel(context.Background())
			defer stopCancel()
			go func() {
				<-quit
				stopCancel()
				grpcServer.GracefulStop()
				_ = httpServer.Shutdown(context.Background())
			}()
			ctx.Printf("tempest server listening on %s (gRPC %s)\n", addr, grpcAddr)
			if err := httpServer.ListenAndServe(); err != nil && err != context.Canceled {
				return err
			}
			_ = stopCtx
			return nil
		},
	}
}
