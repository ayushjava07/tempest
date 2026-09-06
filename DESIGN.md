# Tempest — Design Document

## Product Name
**Tempest** — a distributed workflow-orchestration platform for internal platform teams.

## Pitch
Tempest lets teams define, schedule, and monitor multi-step workflows as directed acyclic graphs. It provides durable state, automatic retries, pluggable task handlers, and both HTTP and gRPC APIs.

## Module Layout
```
github.com/tempest-io/tempest
├── cmd/tempest/              # CLI entry point
├── pkg/
│   ├── types/                # Domain types (Run, Event, Delivery, etc.)
│   ├── errors/               # Error taxonomy
│   └── validation/           # Input validators
└── internal/
    ├── api/                  # HTTP REST API (v1)
    ├── auth/                 # RBAC + token resolution
    ├── backoff/              # Exponential backoff
    ├── cache/                # Generic in-memory cache with TTL
    ├── cli/                  # CLI commands
    ├── config/               # Configuration (flag > env > file > default)
    ├── dashboard/            # Server-rendered HTML status dashboard
    ├── grpcapi/              # gRPC API surface
    ├── metrics/              # Counters, gauges, histograms
    ├── migration/            # PostgreSQL schema migrations
    ├── persistence/          # Store interface
    │   ├── memstore/         # In-memory implementation
    │   ├── pgstore/          # PostgreSQL implementation
    │   └── conformance/      # Store conformance tests
    ├── plugin/               # Handler interface + registry
    │   └── handlers/         # pass, echo, shell, http, fail handlers
    ├── scheduler/            # Engine, lease management, worker pool
    ├── webhook/              # Dispatcher + Deliverer + HMAC signatures
    ├── worker/               # Background maintenance (expiry, compaction, lease reaper)
    └── workflow/             # NewRun + ReadySteps
```

## Data Model
- **WorkflowDefinition**: ID (namespace, name, version), steps, tags, timeouts
- **StepDefinition**: ID, handler, depends_on, retry policy, timeout
- **Run**: ID, workflow ref, state (pending→queued→running→succeeded/failed/cancelled/timed_out), input, steps, error
- **StepRun**: step_id, state, attempt, output, error
- **QueueItem**: run_id, step_id, visible_at, attempt
- **Event**: ID, type, namespace, run_id, step_id, payload
- **WebhookEndpoint**: ID, URL, secret, active, types
- **Delivery**: ID, event_id, endpoint_id, status, attempts
- **APIToken**: ID, namespace, role, label, token_hash

## State Machine
```
                    ┌─────────┐
                    │ PENDING │
                    └────┬────┘
                         │ enqueue
                    ┌────▼────┐
                    │  QUEUED │
                    └────┬────┘
                         │ dequeue
                    ┌────▼────┐
                    │ RUNNING │
                    └──┬───┬──┘
                       │   │
          ┌────────────┘   └────────────┐
     ┌────▼─────┐              ┌───────▼────────┐
     │ SUCCEEDED│              │  FAILED        │
     └──────────┘              └────────────────┘
          │                          │
          │               ┌──────────▼────────┐
          │               │    TIMED_OUT      │
          │               └───────────────────┘
          │                          │
     ┌────▼───────┐                  │
     │ CANCELLED  │ ◄────────────────┘
     └────────────┘
```

## Subsystem Boundaries
1. **Workflow Engine**: DAG resolution, step readiness detection
2. **Scheduler**: Lease management, worker pool, retry/backoff
3. **Persistence**: Store interface with Postgres and in-memory implementations
4. **Cache**: TTL-based in-memory cache with invalidation callbacks
5. **API**: REST + gRPC with auth middleware
6. **CLI**: Operator tooling
7. **Plugins**: Extensible handler interface
8. **Webhooks**: Event dispatch with HMAC signatures
9. **Workers**: Background maintenance (expiry, compaction, lease reaping)
10. **Metrics**: Counters, gauges, histograms, /debug endpoint
11. **Dashboard**: Server-rendered HTML status page
12. **Config**: Three-source configuration merging
13. **Auth**: Token-based RBAC
14. **Migration**: Schema versioning

## Concurrency Model
- Worker pool with configurable goroutine count
- Per-lease mutex for step execution
- Background goroutines for maintenance, each with context cancellation
- Clock injection for deterministic testing
