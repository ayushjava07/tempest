package api

import (
	"context"
	"net/http"
	"time"

	"github.com/tempest-io/tempest/internal/auth"
	"github.com/tempest-io/tempest/pkg/errors"
)

type responseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += n
	return n, err
}

func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		_ = time.Since(start)
	})
}

func RecoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				writeProblem(w, r, http.StatusInternalServerError, "Internal Server Error", "panic recovered")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func RequireAuth(resolver *auth.Resolver, action auth.Action) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := auth.BearerToken(r.Header)
			if !ok {
				mapError(w, r, errors.UnauthenticatedError(errors.ErrUnauthenticated))
				return
			}
			principal, err := resolver.Resolve(r.Context(), auth.HashToken(token))
			if err != nil {
				mapError(w, r, err)
				return
			}
			if !principal.Role.Allow(action) {
				mapError(w, r, errors.PermissionDeniedError(errors.ErrPermissionDenied))
				return
			}
			ctx := auth.WithPrincipal(r.Context(), principal)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func ContextNamespace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p, ok := auth.PrincipalFrom(r.Context()); ok {
			ns := string(p.Namespace)
			if ns != "" {
				ctx := context.WithValue(r.Context(), namespaceKey{}, ns)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

type namespaceKey struct{}

func NamespaceFrom(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(namespaceKey{}).(string)
	return v, ok
}
