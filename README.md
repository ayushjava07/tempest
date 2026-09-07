# Tempest — Distributed Workflow Orchestration Engine

[![Go Report Card](https://goreportcard.com/badge/github.com/tempest-io/tempest)](https://goreportcard.com/report/github.com/tempest-io/tempest)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Tempest is a high-throughput, fault-tolerant distributed workflow orchestration engine engineered for complex Directed Acyclic Graph (DAG) task execution, transactional sagas, dynamic work-stealing, and crash-resilient state recovery.

---

## Key Features

- **Dynamic DAG Task Graph Scheduling**: Kahn-topological scheduling, automatic dependency resolution, and cycle detection.
- **Chase-Lev Work-Stealing Pool**: Lock-free task stealing deques maximizing multi-core throughput and cache locality.
- **Durable Write-Ahead Logging & Ring Buffer**: Memory-mapped circular logging (`internal/mmapring`) and crash-consistent WAL (`internal/wal`).
- **Distributed Leader Election & Fencing**: Lease-based master election with monotonic fencing tokens preventing split-brain writes.
- **Transactional Sagas with LIFO Compensation**: Multi-service distributed transaction rollback semantics.
- **Priority Lane Task Dispatch**: Starvation-free priority lanes (`CRITICAL`, `HIGH`, `DEFAULT`, `LOW`).
- **Pluggable Secret Management**: First-class support for HashiCorp Vault, AWS/GCP KMS, and file/env providers.
- **Zero-Allocation Fast JSON Engine**: Reflection-free high-speed JSON serialization for telemetry and events.
- **Prometheus Metric Exporter**: Real-time export of DAG latencies, queue depths, worker steals, and throughput.

---

## Quick Start

### 1. Build the Tempest CLI & Daemon

```bash
# Build the binary
go build -o bin/tempest ./cmd/tempest

# Verify installation
./bin/tempest version
```

### 2. Run the Workflow Orchestration Server

```bash
./bin/tempest server :8080
```

### 3. Submit and Inspect Workflows via CLI

```bash
# Submit a workflow definition
./bin/tempest workflow submit ./examples/order_pipeline.json --namespace production

# Inspect workflow status and execution timeline
./bin/tempest workflow inspect run-1234 --output json

# Pause and resume workflow runs
./bin/tempest workflow pause run-1234 --reason "maintenance window"
./bin/tempest workflow resume run-1234

# Generate shell completion
./bin/tempest completion bash > /etc/bash_completion.d/tempest
```

---

## Production Deployment & Topology

### High-Availability (HA) Configuration

For production deployment across multi-node clusters:

```
[Load Balancer (Round-Robin / Active-Passive)]
         |
         +-------------------+-------------------+
         |                   |                   |
         v                   v                   v
[Tempest Node 1]      [Tempest Node 2]     [Tempest Node 3]
 (Active Leader)       (Hot Standby)        (Hot Standby)
         |                   |                   |
         +-------------------+-------------------+
                             |
                   [Shared Storage / WAL]
              (NFS, Ceph, or S3/Blobstore Tier)
```

1. **Leader Election**: Configure cluster nodes with distinct `--node-id` flags and matching `--lease-duration` (default: 10s).
2. **Fencing Storage**: Ensure storage mounts support monotonic lock fencing tokens to prevent stale leader writes.
3. **Observability**: Scrape metrics from `/metrics` using Prometheus every 15s.

---

## CLI Command Reference

| Command | Subcommands / Flags | Description |
|---|---|---|
| `tempest server [addr]` | `--cluster`, `--node-id` | Starts the orchestration server and API daemon |
| `tempest workflow submit <file>` | `--param key=value`, `--dry-run` | Validates and submits a new DAG workflow |
| `tempest workflow inspect <run-id>` | `--output json\|table` | Fetches live execution graph status and step metrics |
| `tempest workflow pause <run-id>` | `--reason <string>` | Gracefully pauses running task graph execution |
| `tempest workflow resume <run-id>` | — | Resumes execution from paused checkpoints |
| `tempest workflow cancel <run-id>` | `--force` | Sends graceful or forceful cancellation signals |
| `tempest completion <shell>` | `bash`, `zsh` | Outputs auto-completion scripts |

---

## REST API (v1) Reference

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/v1/runs` | Submit a new workflow run |
| `GET` | `/v1/runs/{id}` | Get detailed run state and steps |
| `GET` | `/v1/runs` | List workflow runs with pagination |
| `POST` | `/v1/runs/{id}/cancel` | Cancel an in-progress workflow run |
| `POST` | `/v1/registry/definitions` | Register a reusable workflow template |
| `GET` | `/v1/registry/definitions` | List all registered templates |
| `POST` | `/v1/webhooks` | Register event notification webhook |
| `GET` | `/v1/readyz` | Readiness probe |
| `GET` | `/v1/livez` | Liveness probe |
| `GET` | `/metrics` | Prometheus metrics scrape endpoint |

---

## Testing & Verification

Tempest maintains strict quality standards:
- **Zero Race Conditions**: All code is tested under `go test -race ./...`.
- **Zero Goroutine Leaks**: Verified using `go.uber.org/goleak` across all subsystem tests.
- **Zero Deadlocks**: Verified by synthetic stress tests and deadlock detector test suites.

```bash
# Run unit and race tests
go test -v -race ./...

# Run static analysis
go vet ./...
```

For detailed architectural specifications, see [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

---

## License

MIT License. See [LICENSE](LICENSE) for details.
