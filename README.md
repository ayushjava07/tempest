# Tempest

A distributed workflow-orchestration platform for internal platform teams.

## Quick Start

```bash
# Build
go build -o tempest ./cmd/tempest

# Run server
./tempest server :8080

# Submit a workflow
curl -X POST http://localhost:8080/v1/runs \
  -H "Content-Type: application/json" \
  -d '{"workflow": "my-workflow", "version": 1, "input": {"key": "value"}}'
```

## Configuration

Tempest supports three configuration sources with defined precedence:

1. **CLI flags** (highest priority): `--server-addr :8080`
2. **Environment variables**: `TEMPEST_SERVER_ADDR=:8080`
3. **Config file** (JSON): `{"server.addr": ":8080"}`
4. **Defaults** (lowest priority)

## API

### REST API (v1)
- `POST /v1/runs` — Submit a new run
- `GET /v1/runs/{id}` — Get run details
- `GET /v1/runs` — List runs
- `POST /v1/runs/{id}/cancel` — Cancel a run
- `POST /v1/registry/definitions` — Create workflow definition
- `GET /v1/registry/definitions` — List definitions
- `GET /v1/registry/definitions/{name}` — Get definition
- `POST /v1/webhooks` — Register webhook
- `GET /v1/webhooks` — List webhooks
- `DELETE /v1/webhooks/{id}` — Delete webhook
- `GET /v1/readyz` — Health check
- `GET /v1/livez` — Liveness check

### Authentication
All API requests require a Bearer token:
```
Authorization: Bearer <token>
```

Roles: `reader`, `operator`, `admin`

## Plugins
Tempest supports custom task handlers via the plugin interface:
- `pass` — Complete immediately
- `echo` — Echo input as output
- `shell` — Run shell commands
- `http` — Make HTTP requests
- `fail` — Always fail with an error

## License
MIT License
