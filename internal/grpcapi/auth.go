package grpcapi

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/tempest-io/tempest/internal/auth"
)

func UnaryInterceptor(resolver *auth.Resolver) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		p, err := resolvePrincipal(ctx, resolver)
		if err != nil {
			return nil, err
		}
		return handler(auth.WithPrincipal(ctx, p), req)
	}
}

func resolvePrincipal(ctx context.Context, resolver *auth.Resolver) (*auth.Principal, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}
	vals := md.Get("authorization")
	if len(vals) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing authorization")
	}
	token := strings.TrimPrefix(vals[0], "Bearer ")
	principal, err := resolver.Resolve(ctx, auth.HashToken(token))
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid token")
	}
	return principal, nil
}
