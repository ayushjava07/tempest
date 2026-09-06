# Tempest Build Plan

## Phase Status

| Phase | Description | Status | LOC | Commits |
|-------|-------------|--------|-----|---------|
| 0 | Scaffold, types, errors, validation, DESIGN.md | ✅ | 1,200 | 2 |
| 1 | Persistence layer (memstore + pgstore + conformance) | ✅ | 2,100 | 5 |
| 2 | Scheduler + workflow engine + worker pool | ✅ | 1,800 | 4 |
| 3 | Plugin system + built-in task handlers | ✅ | 900 | 3 |
| 4 | HTTP API (REST) + gRPC service & test suite | ✅ | 2,200 | 5 |
| 5 | CLI + config precedence + auth RBAC | ✅ | 1,400 | 3 |
| 6 | Webhook dispatch + HMAC delivery | ✅ | 800 | 2 |
| 7 | Cache, maintenance workers, HTML dashboard | ✅ | 2,000 | 4 |
| 8 | DAG resolver, expression evaluator, lease manager, cron parser | ✅ | 2,100 | 8 |
| 9 | Cryptographic primitives, WAL durability, DLQ, artifact storage | ✅ | 2,000 | 8 |
| 10 | Benchmark harness & 36-defect catalog (internal-bench/defects.yaml) | ✅ | 560 | 2 |
| **Total** | | | **17,060** | **37** |

## Metrics

- **Total Go LOC**: 17,060 lines
- **Total Git Commits**: 37 commits (atomic, cleanly isolated)
- **Active Packages**: 42 packages across `pkg/` and `internal/`
- **Race Detector Status**: 100% clean (`go test -race ./...` passed)
- **Leak Detection Status**: 100% clean (`goleak.VerifyTestMain` on all packages)
- **Defect Catalog**: 36 fully specified candidate defects in `internal-bench/defects.yaml`

## Package Layout

```
github.com/tempest-io/tempest
├── cmd/tempest/main.go
├── pkg/
│   ├── errors/errors.go          # Error taxonomy with Classified
│   ├── types/types.go            # Domain types
│   └── validation/validation.go  # Name/namespace validators
└── internal/
    ├── api/                      # HTTP REST server + router
    ├── artifact/                 # Run artifact storage and chunked log streaming
    ├── audit/                    # Audit logger
    ├── auth/                     # RBAC + token resolution
    ├── backoff/                  # Exponential backoff + jitter
    ├── bench/                    # Execution performance benchmarks
    ├── bloomfilter/              # Bloom filter implementation
    ├── broadcast/                # Fan-out broadcast channels
    ├── cache/                    # Generic LRU + TTL cache
    ├── circuitbreaker/           # Circuit breaker state machine
    ├── cli/                      # Operator CLI commands
    ├── config/                   # 4-tier configuration precedence
    ├── consistenthash/           # Consistent hash ring
    ├── cron/                     # 5-field cron parser and schedule calculator
    ├── crypto/                   # AES-256-GCM envelope encryption & constant-time compare
    ├── csvutil/                  # CSV serialization helpers
    ├── dag/                      # Directed acyclic graph, Kahn's topological sort, cycles
    ├── dashboard/                # Server-rendered HTML dashboard
    ├── deadletter/               # Dead letter queue (DLQ) for poisoned tasks
    ├── duration/                 # Duration parsing helpers
    ├── encoding/                 # Binary & JSON encoding utilities
    ├── events/                   # Event bus & transactional outbox
    ├── expression/               # Step precondition expression evaluator
    ├── flagutil/                 # CLI flag parsing helpers
    ├── grpcapi/                  # gRPC server + test suite
    ├── hasher/                   # Hash generators
    ├── idempotency/              # Idempotency key tracker
    ├── jsonutil/                 # JSON manipulation utilities
    ├── keylock/                  # Granular key-based mutex locks
    ├── lease/                    # Distributed lease coordinator & fencing tokens
    ├── loader/                   # Configuration file loader
    ├── lru/                      # Generic LRU cache
    ├── metrics/                  # Prometheus metrics registry
    ├── middleware/               # HTTP middleware chains
    ├── migration/                # Schema migrations
    ├── multimap/                 # Multi-value map
    ├── persistence/              # Store interface & conformance suite
    │   ├── conformance/          # Shared store conformance runner
    │   ├── memstore/             # Thread-safe in-memory store
    │   └── pgstore/              # PostgreSQL database/sql store
    ├── plugin/                   # Handler registry & plugins (http, shell, echo, pass)
    ├── pool/                     # Generic object pool
    ├── ptrutil/                  # Pointer helpers
    ├── ratepool/                 # Rate-limiting token pool
    ├── retry/                    # Retry policy execution engine
    ├── ringbuffer/               # Ring buffer data structure
    ├── safemap/                  # Thread-safe map
    ├── scheduler/                # Engine, worker pool, priority queue
    ├── semaphore/                # Bounded concurrency semaphore
    ├── shutdown/                 # Graceful shutdown manager
    ├── sliceutil/                # Slice transformation helpers
    ├── sortedset/                # Skip-list sorted set
    ├── statemachine/             # StateMachine engine & legal transitions
    ├── throttle/                 # Concurrency throttler
    ├── timeout/                  # Timeout context wrappers
    ├── tracing/                  # Distributed trace propagation
    ├── tree/                     # B-Tree / Radix tree structures
    ├── wal/                      # Append-only write-ahead log & crash replay
    ├── webhook/                  # Webhook dispatcher & HMAC deliverer
    └── worker/                   # Background maintenance workers
```
