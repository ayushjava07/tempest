# Tempest Architecture & Subsystems Reference

Tempest is a high-throughput, fault-tolerant distributed workflow orchestration engine designed for high-concurrency Directed Acyclic Graph (DAG) task execution, transactional saga compensations, and deterministically replayable state machines.

---

## 1. High-Level System Architecture

Tempest employs a distributed, shared-nothing coordinator/worker architecture built around monotonic lease fencing, memory-mapped write-ahead logging, and work-stealing task queues.

```
                         +-----------------------------------+
                         |           Tempest CLI             |
                         |     (tempest workflow submit)     |
                         +-----------------+-----------------+
                                           | HTTP / gRPC
                                           v
+-----------------------------------------------------------------------------------+
|                                 Tempest Cluster                                   |
|                                                                                   |
|  +-------------------------------------+  +------------------------------------+  |
|  |             Leader Node             |  |            Follower Node           |  |
|  |  +-------------------------------+  |  |  +-------------------------------+ |  |
|  |  | Distributed Leader Election   |  |  |  | Distributed Leader Election   | |  |
|  |  | (internal/leader, Lease/Heart)|  |  |  | (Standby / Heartbeat Watcher) | |  |
|  |  +---------------+---------------+  |  +--+---------------+---------------+--+  |
|  |                  |                  |                     |                    |
|  |  +---------------v---------------+  |                     |                    |
|  |  | DAG Compiler & Cycle Detector |  |                     |                    |
|  |  | (internal/dag, Kahn's Algo)   |  |                     |                    |
|  |  +---------------+---------------+  |                     |                    |
|  |                  |                  |                     |                    |
|  |  +---------------v---------------+  |                     |                    |
|  |  | Priority Lane Task Scheduler  |  |                     |                    |
|  |  | (CRITICAL, HIGH, DEFAULT, LOW)|  |                     |                    |
|  |  +---------------+---------------+  |                     |                    |
|  |                  |                  |                     |                    |
|  |  +---------------v---------------+  |     +---------------v---------------+    |
|  |  | Distributed Lock Fencing      |=======>| Distributed Lock Fencing      |    |
|  |  | (internal/fencing, Monotonic) |  |     | (Storage & Worker Lease Guard)|    |
|  |  +---------------+---------------+  |     +---------------+---------------+    |
|  +------------------|------------------+                     |                    |
|                     |                                        |                    |
|  +------------------v----------------------------------------v-----------------+  |
|  |                         Worker Task Execution Pool                          |  |
|  |  +----------------------------------+  +----------------------------------+ |  |
|  |  | Chase-Lev Work-Stealing Pool     |  | Distributed Step Rate Throttler  | |  |
|  |  | (internal/stealer, Deque Lockless|  | (internal/throttler, Sliding-Win)| |  |
|  |  +----------------------------------+  +----------------------------------+ |  |
|  |  +----------------------------------+  +----------------------------------+ |  |
|  |  | Pluggable Secret Vault Provider  |  | Step Output Delta Diff Engine    | |  |
|  |  | (KMS, HashiCorp Vault, Env, File)|  | (internal/deltadiff, RFC 6902)   | |  |
|  |  +----------------------------------+  +----------------------------------+ |  |
|  +----------------------------------+------------------------------------------+  |
|                                     |                                             |
|  +----------------------------------v------------------------------------------+  |
|  |                          Durable Storage Layer                             |  |
|  |  +----------------------------------+  +----------------------------------+ |  |
|  |  | Mmap Fast Circular Ring Buffer   |  | Segmented Write-Ahead Log (WAL)  | |  |
|  |  | (internal/mmapring, Zero-Copy)   |  | (internal/wal, CRC32 Checksums)  | |  |
|  |  +----------------------------------+  +----------------------------------+ |  |
|  |  +----------------------------------+  +----------------------------------+ |  |
|  |  | Workflow State Archiver (Gzip/Z) |  | State Machine Replay Engine      | |  |
|  |  | (internal/archiver, Cold Tier)   |  | (internal/replay, Determinism)   | |  |
|  |  +----------------------------------+  +----------------------------------+ |  |
|  +-----------------------------------------------------------------------------+  |
+-----------------------------------------------------------------------------------+
```

---

## 2. Core Subsystems Reference

### 2.1 DAG Compiler & Execution Graph (`internal/dag`)
- **Topological Sorting**: Implements Kahn's algorithm for linear step ordering and parallel fan-out extraction.
- **Cycle Detection**: Validates graphs at ingestion time, preventing deadlocks or infinite loops.
- **Dynamic Mutation**: Supports runtime graph mutations via `GraphMutator`, enabling dynamic DAG expansion based on upstream task outputs.

### 2.2 Work-Stealing Task Pool (`internal/stealer`)
- **Chase-Lev Deque**: Cache-conscious double-ended queue where worker threads push and pop from the bottom (LIFO for cache locality), while idle worker threads steal tasks from the top (FIFO).
- **Zero Lock Contention**: Atomic load/store instructions govern steal operations, yielding sub-microsecond task dispatch latencies.

### 2.3 Resilient Distributed Consensus & Fencing (`internal/leader`, `internal/fencing`)
- **Heartbeat & Lease-Based Leader Election**: Raft-lite lease mechanism providing deterministic master-worker topologies with automatic failover upon missed heartbeat deadlines.
- **Monotonic Fencing Tokens**: Guards storage and state operations against split-brain scenarios and zombie leader writes. Every lease renewal increments a 64-bit fencing token verified at write time.

### 2.4 Durable High-Performance Storage (`internal/mmapring`, `internal/wal`, `internal/archiver`)
- **Zero-Copy Memory-Mapped Ring Buffer**: Direct memory-mapped disk buffer with ring-index wrapping, atomic sequence head/tail pointers, and zero-allocation log entry ingestion.
- **Segmented Write-Ahead Log**: Crash-consistent append-only log with per-record CRC32 verification and automatic file rotation.
- **Cold State Archiver**: Automated compression (Zstandard and gzip) and retention tiering for completed workflow executions.

### 2.5 Priority Lane Scheduling (`internal/lane`)
- **Multi-Queue Priority Bands**: Four discrete priority lanes (`CRITICAL`, `HIGH`, `DEFAULT`, `LOW`).
- **Starvation-Free Round-Robin**: Weighted fair dispatch ensuring low-priority background maintenance tasks never starve while high-priority SLA tasks receive immediate servicing.

### 2.6 Transactional Sagas & Rollback Engine (`internal/saga`)
- **Orchestration Saga Pattern**: Step-by-step transaction coordination where each forward task defines an inverse compensation hook.
- **Guaranteed Rollback Ordering**: Strict LIFO rollback execution guaranteeing atomicity across distributed microservices.

### 2.7 Zero-Allocation High-Speed JSON Serializer (`internal/fastjson`)
- **Direct Buffer Encoder**: Specialized buffer builder that writes JSON payloads without heap allocations or reflection overhead.
- **High-Throughput Event Streaming**: Designed for streaming millions of workflow state transitions per second to disk and network sockets.

### 2.8 Observability & Metrics Export (`internal/promexporter`)
- **Prometheus Standard Metric Types**: Counters, Gauges, and Histograms with configurable exponential bucketing.
- **Thread-Safe Label Resolution**: High-performance metric scraping endpoint (`/metrics`) compatible with Prometheus and Grafana.

---

## 3. Workflow Execution Lifecycle

```
[Workflow Submitted]
         |
         v
[Static Analyzer & Schema Validation]  ---> (Syntax / Dependency Check)
         |
         v
[DAG Ingestion & WAL Append]           ---> (Durable Transaction Record)
         |
         v
[Priority Lane Scheduler Dispatch]    ---> (CRITICAL / HIGH / DEFAULT / LOW)
         |
         v
[Worker Pool Execution (Work-Stealing)]
    |                     |
    v (Success)           v (Failure / Abort)
[Delta Diff Generation]  [Saga LIFO Compensation Rollback]
    |                     |
    v                     v
[Approval Gate / Finish] [WAL Commit: FAILED]
    |
    v
[Archiver Cold Compression]
```

---

## 4. Concurrency & Memory Safety Guarantees

1. **Zero Goroutine Leaking**: Verified using Uber's `goleak` across all subsystem test mains.
2. **Race-Condition Free**: Tested under `-race` flag across high-concurrency benchmarks.
3. **Graceful Termination**: All background loops subscribe to cancellation contexts with deterministic shutdown barriers.
