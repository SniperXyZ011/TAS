# 🎯 Tactical Armory System (TAS) — Backend

> A production-grade Go backend for managing distributed armory edge nodes. Designed for offline-first ESP32 kiosks communicating via LoRa/cellular, featuring HMAC-SHA256 signature verification, per-node API key authentication, structured logging, and Docker-based deployment.

---

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Features](#features)
- [Project Structure](#project-structure)
- [Data Models](#data-models)
- [API Reference](#api-reference)
- [Security Design](#security-design)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Local Development](#local-development)
  - [Docker Compose](#docker-compose)
- [Configuration](#configuration)
- [Database Migrations](#database-migrations)
- [Testing](#testing)
- [Tech Stack](#tech-stack)

---

## Overview

The **Tactical Armory System** is a secure, scalable backend service that serves as the central hub for a mesh of ESP32-based armory edge nodes. Each edge kiosk manages weapon check-in/check-out events and ammo consumption data locally (offline-first), then syncs batches of signed transactions to the server when connectivity is available.

The backend is responsible for:
- **Authenticating** edge nodes via per-node bcrypt-hashed API keys
- **Verifying** transaction integrity using HMAC-SHA256 signatures
- **Persisting** weapon transactions and ammo consumption logs to PostgreSQL
- **Deduplicating** events using transaction IDs as primary keys
- **Rate-limiting** node traffic with per-node token-bucket limiters
- **Providing** admin endpoints for node management and audit queries

---

## Architecture

```
┌────────────────────────────────────────────────────────────────┐
│                        TAS Backend Server                       │
│                                                                  │
│  ┌──────────┐   ┌─────────────┐   ┌──────────────────────────┐ │
│  │ Handlers │ ← │ Middleware   │   │        Services           │ │
│  │          │   │             │   │                            │ │
│  │ /health  │   │ NodeAuth    │   │ SyncService                │ │
│  │ /ready   │   │ AdminAuth   │   │   - HMAC verification      │ │
│  │ /sync    │   │ RateLimit   │   │   - Batch validation       │ │
│  │ /sync/   │   │ Recovery    │   │   - Deduplication          │ │
│  │  ammo    │   │ RequestLog  │   │                            │ │
│  │ /nodes   │   └─────────────┘   │ NodeService               │ │
│  │ /txs     │                     │   - Node registration      │ │
│  └──────────┘                     │   - Key/secret generation  │ │
│        │                          └──────────────────────────┘ │
│        ▼                                    │                    │
│  ┌────────────────────────────────────────┐ │                   │
│  │             Repositories               │◄┘                   │
│  │  NodeRepository  | TransactionRepo     │                     │
│  │  AmmoRepository                        │                     │
│  └──────────────────┬─────────────────────┘                     │
└─────────────────────┼──────────────────────────────────────────-┘
                       │
                       ▼
              ┌─────────────────┐
              │   PostgreSQL 16  │
              │                  │
              │  nodes           │
              │  transactions    │
              │  ammo_logs       │
              │  ammo_types      │
              └──────────────────┘
```

### Request Flow (Node Sync)

```
ESP32 Node
    │
    │  POST /api/v1/sync
    │  Header: X-API-Key: <plaintext-key>
    │  Body: { transactions: [...] }
    │
    ▼
Recovery Middleware
    │
ContentType Middleware
    │
NodeAuth Middleware ──── bcrypt.Compare(api_key, stored_hash) ──► 401 if invalid
    │                    Injects node_id into context
    │
RateLimit Middleware ─── token-bucket per node_id ──────────────► 429 if exceeded
    │
SyncHandler
    │
SyncService.ProcessBatch()
    ├── Validate each tx (action, timestamp window, empty ID)
    ├── HMAC-SHA256 signature check (constant-time compare)
    ├── Batch INSERT ... ON CONFLICT DO NOTHING (deduplication)
    └── Update node last_seen (async goroutine)
    │
Response: { accepted, duplicate, rejected, results: [...] }
```

---

## Features

| Feature | Details |
|---|---|
| **Offline-first sync** | Accepts batches of up to 500 transactions; timestamps tolerated up to 7 days old |
| **HMAC-SHA256 signing** | Every transaction must be signed by the node using a shared secret; server verifies with constant-time compare |
| **Per-node API keys** | Bcrypt-hashed at rest; plaintext returned only once on registration |
| **Admin authentication** | Separate `X-Admin-Key` header for node management and audit routes |
| **Per-node rate limiting** | Token-bucket limiter (configurable RPS, 2× burst); idle nodes auto-evicted from memory |
| **Natural deduplication** | `transaction_id` is the primary key; `ON CONFLICT DO NOTHING` prevents double-counting |
| **Ammo telemetry** | Load-cell events (delta grams → rounds) synced from edge nodes |
| **Structured logging** | zerolog with configurable levels and console/JSON output |
| **Graceful shutdown** | 30-second drain window on SIGINT/SIGTERM |
| **Auto-migrations** | SQL migrations applied at startup, ordered by filename |
| **Docker-ready** | Multi-stage Dockerfile; non-root user; Docker Compose with health checks |
| **Unit tested** | Tests for crypto, sync service, handlers, and config |

---

## Project Structure

```
TAS/
├── cmd/
│   └── server/
│       └── main.go             # Entrypoint — wires config, DB, repos, services, handlers, router
├── internal/
│   ├── config/
│   │   ├── config.go           # Loads env vars, validates required fields
│   │   └── config_test.go
│   ├── crypto/
│   │   ├── signature.go        # HMAC-SHA256 key generation and signature verification
│   │   └── signature_test.go
│   ├── db/
│   │   └── postgres.go         # pgx connection pool + sequential SQL migration runner
│   ├── handler/
│   │   ├── health_handler.go   # GET /health (liveness) and GET /ready (readiness + DB ping)
│   │   ├── sync_handler.go     # POST /api/v1/sync — delegates to SyncService
│   │   ├── ammo_handler.go     # POST /api/v1/sync/ammo — bulk ammo log inserts
│   │   ├── node_handler.go     # POST /api/v1/nodes, GET /api/v1/nodes/list, GET /api/v1/transactions
│   │   └── handler_test.go
│   ├── middleware/
│   │   ├── auth.go             # NodeAuthMiddleware (bcrypt) + AdminAuthMiddleware
│   │   ├── ratelimit.go        # Per-node token-bucket rate limiter with idle eviction
│   │   └── recovery.go         # Panic recovery + structured request logging
│   ├── models/
│   │   └── models.go           # Shared domain types: Node, Transaction, AmmoLog, request/response shapes
│   ├── repository/
│   │   ├── node_repo.go        # CRUD for nodes table
│   │   ├── transaction_repo.go # BatchInsert, ListAll, ListByNode
│   │   └── ammo_repo.go        # BulkInsert for ammo_logs
│   └── service/
│       ├── sync_service.go     # Transaction validation, HMAC verify, batch persistence + NodeService
│       └── sync_service_test.go
├── migrations/
│   ├── 001_create_nodes.sql          # nodes table + pgcrypto extension
│   ├── 002_create_transactions.sql   # transactions table with FK and indexes
│   └── 003_create_ammo_logs.sql      # ammo_logs + ammo_types seed data
├── Dockerfile                  # Multi-stage: go:1.22-alpine builder → alpine:3.19 runtime
├── docker-compose.yml          # postgres:16 + TAS server with health checks
├── .env.example                # Template for all required environment variables
├── go.mod
└── go.sum
```

---

## Data Models

### Node
Represents a registered ESP32 edge kiosk in the armory mesh network.

| Field | Type | Description |
|---|---|---|
| `node_id` | UUID | Auto-generated primary key |
| `name` | string | Human-readable name (e.g. `"Alpha-Vault-1"`) |
| `location` | string | Physical location of the kiosk |
| `tier` | string | `standard` \| `priority` \| `admin` |
| `is_active` | bool | Whether the node is active |
| `created_at` | timestamp | Registration time |
| `last_seen_at` | timestamp? | Last successful sync |

### Transaction
A single weapon movement event submitted by an edge node.

| Field | Type | Description |
|---|---|---|
| `transaction_id` | string | UUID generated by the edge node (deduplication key) |
| `node_id` | UUID | Authenticated node (set server-side from context) |
| `user_id` | string | Personnel identifier |
| `weapon_id` | string | Weapon identifier |
| `action` | enum | `checkout` \| `checkin` \| `audit` \| `transfer` \| `lost` \| `found` |
| `quantity` | int | Number of weapons (≥ 1) |
| `notes` | string? | Optional free-text notes |
| `timestamp` | int64 | Unix epoch (UTC) from the edge device |
| `signature` | string | Hex HMAC-SHA256 of the canonical payload |

### AmmoLog
An ammunition consumption event from a load-cell sensor.

| Field | Type | Description |
|---|---|---|
| `node_id` | UUID | Source node |
| `transaction_id` | string? | Optional linked transaction |
| `ammo_type` | string | e.g. `"5.56mm"`, `"9mm"`, `"7.62mm"` |
| `delta_grams` | int | Mass removed from bin |
| `rounds` | int | Rounds inferred from weight delta |
| `timestamp` | int64 | Unix epoch from sensor |

---

## API Reference

### Public Endpoints (no auth)

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Liveness probe — returns `{"status":"ok"}` |
| `GET` | `/ready` | Readiness probe — pings DB; returns `{"status":"ok","db":"up"}` |

---

### Node Endpoints (require `X-API-Key` header)

#### `POST /api/v1/sync`
Sync a batch of weapon transactions from an edge node.

**Request**
```json
{
  "transactions": [
    {
      "transaction_id": "550e8400-e29b-41d4-a716-446655440000",
      "user_id": "SGT-SMITH-001",
      "weapon_id": "M4A1-0042",
      "action": "checkout",
      "quantity": 1,
      "notes": "range training",
      "timestamp": 1712345678,
      "signature": "<hmac-sha256-hex>"
    }
  ]
}
```

**Response `200 OK`**
```json
{
  "accepted": 1,
  "duplicate": 0,
  "rejected": 0,
  "results": [
    {
      "transaction_id": "550e8400-e29b-41d4-a716-446655440000",
      "status": "accepted"
    }
  ]
}
```

**Transaction Status Values**

| Status | Meaning |
|---|---|
| `accepted` | Persisted successfully |
| `duplicate` | Already recorded (idempotent) |
| `invalid_signature` | HMAC-SHA256 mismatch |
| `invalid_action` | Unknown action value |
| `rejected` | Timestamp too old (> 7 days) |

---

#### `POST /api/v1/sync/ammo`
Sync ammo consumption logs from a load-cell sensor node.

**Request**
```json
{
  "logs": [
    {
      "ammo_type": "5.56mm",
      "delta_grams": -246,
      "rounds": 20,
      "timestamp": 1712345600
    }
  ]
}
```

**Response `200 OK`**
```json
{ "inserted": 1 }
```

---

### Admin Endpoints (require `X-Admin-Key` header)

#### `POST /api/v1/nodes`
Register a new edge node. Returns the plaintext API key and HMAC secret — **these are shown only once**.

**Request**
```json
{
  "name": "Alpha-Vault-1",
  "location": "Building C, Room 04",
  "tier": "standard"
}
```

**Response `201 Created`**
```json
{
  "node_id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "Alpha-Vault-1",
  "api_key": "<plaintext-key>",
  "secret": "<plaintext-hmac-secret>"
}
```

> ⚠️ **Store these values immediately.** They are never retrievable again.

---

#### `GET /api/v1/nodes/list`
List all registered nodes.

**Response `200 OK`**
```json
[
  {
    "node_id": "...",
    "name": "Alpha-Vault-1",
    "location": "Building C, Room 04",
    "tier": "standard",
    "is_active": true,
    "created_at": "2024-04-05T12:00:00Z",
    "last_seen_at": "2024-04-10T11:30:00Z"
  }
]
```

---

#### `GET /api/v1/transactions`
Paginated transaction log.

**Query Parameters**

| Param | Default | Description |
|---|---|---|
| `limit` | `50` | Max records (hard cap: 200) |
| `offset` | `0` | Pagination offset |
| `node_id` | — | Filter by specific node UUID |

**Response `200 OK`**
```json
{
  "transactions": [...],
  "limit": 50,
  "offset": 0
}
```

---

## Security Design

### Node Authentication
- Each node is issued a random 32-byte hex **API key** at registration time.
- The server stores a **bcrypt hash** of the key — the plaintext is never stored.
- On every sync request, the `X-API-Key` header is matched against all active nodes using `bcrypt.CompareHashAndPassword`.

### HMAC-SHA256 Transaction Signing
- Each node also receives a **HMAC secret** at registration.
- Before submitting a transaction, the ESP32 computes:
  ```
  payload = "<transaction_id>|<node_id>|<user_id>|<weapon_id>|<action>|<timestamp>"
  signature = HMAC-SHA256(payload, secret)
  ```
- The server verifies using **constant-time comparison** (`hmac.Equal`) to prevent timing attacks.
- `node_id` in the payload is always overridden by the server from the authenticated context — preventing impersonation even with a stolen key.

### Timestamp Replay Protection
- Transactions with timestamps older than **7 days** are rejected.
- Future timestamps (from clock skew) are tolerated.

### Rate Limiting
- Each node has an independent **token-bucket limiter** (configurable RPS, 2× burst for reconnecting nodes).
- Idle limiters are automatically evicted from memory every 5 minutes.

### Admin Key
- Admin routes (`/api/v1/nodes`, `/api/v1/transactions`) require a separate `X-Admin-Key` header.
- Generate with: `openssl rand -hex 32`

---

## Getting Started

### Prerequisites

- [Go 1.22+](https://go.dev/dl/)
- [Docker & Docker Compose](https://docs.docker.com/get-docker/)
- [PostgreSQL 16](https://www.postgresql.org/) (only required for local dev without Docker)

---

### Local Development

**1. Clone the repository**
```bash
git clone https://github.com/SniperXyZ011/TAS.git
cd TAS
```

**2. Copy and edit the environment file**
```bash
cp .env.example .env
# Edit .env — set ADMIN_API_KEY to a strong random secret:
# openssl rand -hex 32
```

**3. Start PostgreSQL** (or use Docker Compose — see below)
```bash
# Using Docker only for the DB:
docker run -d --name tas_pg \
  -e POSTGRES_USER=tas_user \
  -e POSTGRES_PASSWORD=change_me \
  -e POSTGRES_DB=tactical_armory \
  -p 5432:5432 postgres:16-alpine
```

**4. Run the server**
```bash
go run ./cmd/server
```

Migrations run automatically on startup. The server listens on `:8080` by default.

---

### Docker Compose

Run the full stack (PostgreSQL + TAS server) with a single command:

```bash
# Copy and configure your .env
cp .env.example .env

# Build and start
docker compose up --build

# Or detached:
docker compose up --build -d
```

Services:
- **`tas_postgres`** — PostgreSQL 16 on port `5432`
- **`tas_server`** — TAS backend on port `8080`

The server waits for PostgreSQL to pass its health check before starting.

**Check health:**
```bash
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

**Stop:**
```bash
docker compose down
# To also remove the DB volume:
docker compose down -v
```

---

## Configuration

All configuration is via environment variables. See [`.env.example`](.env.example):

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | — | **Required.** Full PostgreSQL DSN |
| `POSTGRES_USER` | `tas_user` | DB username (Docker Compose) |
| `POSTGRES_PASSWORD` | — | DB password |
| `POSTGRES_DB` | `tactical_armory` | Database name |
| `SERVER_PORT` | `8080` | HTTP listen port |
| `ENV` | `development` | `development` or `production` |
| `LOG_LEVEL` | `debug` | `debug` \| `info` \| `warn` \| `error` |
| `ADMIN_API_KEY` | — | **Required.** Admin route secret (use `openssl rand -hex 32`) |
| `NODE_RATE_LIMIT_RPS` | `10` | Max requests/second per node |

---

## Database Migrations

Migrations in the `migrations/` folder are applied **automatically at server startup** in filename order. Each file is tracked in a `schema_migrations` table to prevent re-runs.

| File | Description |
|---|---|
| `001_create_nodes.sql` | `nodes` table + `pgcrypto` extension |
| `002_create_transactions.sql` | `transactions` table with FK, indexes for dashboard queries |
| `003_create_ammo_logs.sql` | `ammo_logs` + `ammo_types` reference table (seeded with common calibers) |

**Seeded ammo types:**

| Type | g/round | Description |
|---|---|---|
| `5.56mm` | 12.31 g | NATO 5.56×45mm |
| `7.62mm` | 25.40 g | NATO 7.62×51mm |
| `9mm` | 12.00 g | 9×19mm Parabellum |
| `.50cal` | 114.31 g | 12.7×99mm NATO |
| `40mm` | 227.00 g | 40mm grenade |

---

## Testing

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run with race detector
go test -race ./...

# Coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

Test coverage includes:
- `internal/crypto` — HMAC generation and signature verification
- `internal/config` — Env loading and validation
- `internal/service` — Batch processing, validation, deduplication logic
- `internal/handler` — HTTP request/response handling

---

## Tech Stack

| Component | Technology |
|---|---|
| **Language** | Go 1.22 |
| **HTTP** | `net/http` (stdlib) |
| **Database** | PostgreSQL 16 via `pgx/v5` |
| **Crypto** | `crypto/hmac`, `crypto/sha256`, `golang.org/x/crypto/bcrypt` |
| **Logging** | `rs/zerolog` |
| **Rate Limiting** | `golang.org/x/time/rate` (token bucket) |
| **Containerization** | Docker (multi-stage) + Docker Compose |
| **Migrations** | Custom SQL runner (no external framework) |

---

## License

This project is for educational and demonstration purposes. Contact the repository owner for licensing inquiries.
