# Tempest Build Plan

## Phase Status

| Phase | Description | Status | LOC | Commits |
|-------|-------------|--------|-----|---------|
| 0 | Scaffold, types, errors, validation | ✅ | 1,200 | 1 |
| 1 | Persistence layer (memstore + pgstore) | ✅ | 1,400 | 1 |
| 2 | Scheduler + workflow engine | ✅ | 800 | 1 |
| 3 | Plugin system + handlers | ✅ | 400 | 1 |
| 4 | HTTP API (REST) + gRPC | ✅ | 1,200 | 1 |
| 5 | CLI + config + auth | ✅ | 600 | 1 |
| 6 | Webhook dispatch + delivery | ✅ | 400 | 1 |
| 7 | Migration + integration tests | ✅ | 200 | 1 |
| **Total** | | | **6,200** | **7** |

## Metrics

- **Go LOC**: 5,976 (production), 1,180 (test)
- **Total LOC**: 7,156
- **Packages**: 21
- **Test coverage**: types, errors, validation, backoff, workflow, plugin, handlers, auth, memstore, scheduler, api, webhook, config

## Package Layout

```
github.com/tempest-io/tempest
├── cmd/tempest/main.go
├── pkg/
│   ├── errors/errors.go          # Error taxonomy with Classified
│   ├── types/types.go            # Domain types
│   └── validation/validation.go  # Name/namespace validators
└── internal/
    ├── api/                      # HTTP REST server
    │   ├── mux.go, server.go, middleware.go, errors.go
    │   └── v1/types.go           # API DTOs
    ├── auth/auth.go              # RBAC + token resolution
    ├── backoff/backoff.go        # Exponential backoff
    ├── cli/                      # CLI commands
    │   ├── cli.go, server.go, runs.go, workflows.go
    │   └── migrate.go, registry.go, tokens.go
    ├── config/config.go          # flag/env/file precedence
    ├── grpcapi/                  # gRPC server + auth interceptor
    ├── migration/migration.go    # pgx schema migrations
    ├── persistence/
    │   ├── interface.go          # Store interface
    │   ├── memstore/             # In-memory store
    │   ├── pgstore/              # PostgreSQL store
    │   └── conformance/          # Store conformance tests
    ├── plugin/                   # Handler registry + interface
    │   └── handlers/             # pass, echo, shell, http, fail
    ├── scheduler/                # Engine, loop, worker pool
    ├── webhook/                  # Dispatcher + Deliverer + HMAC
    └── workflow/                 # NewRun + ReadySteps
```

## Contracts Implemented

- **Error taxonomy**: `Classified` wrapper with 10 error classes
- **Determinism**: Clock injected everywhere; no wall-clock in tests
- **RBAC**: reader/operator/admin roles with action matrix
- **Config**: flag > env (TEMPEST_*) > file > default precedence
- **Webhook**: HMAC-SHA256 signatures, retry with backoff
- **API**: REST + gRPC with auth interceptors, problem+json errors
- **Validation**: `^[a-z][a-z0-9\-\.]{0,63}$` for names
