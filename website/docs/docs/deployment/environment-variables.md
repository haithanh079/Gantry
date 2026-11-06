---
sidebar_position: 1
title: Environment Variables
description: Full reference for all Gantry environment variables.
---

# Environment Variables

All Gantry configuration can be set via environment variables. CLI flags take precedence over env vars, which take precedence over `gantry.yaml`.

## Server Configuration

| Variable | Default | Description |
|---|---|---|
| `GANTRY_PORT` | `8080` | HTTP listen port |
| `GANTRY_DEV` | `false` | Development mode: verbose logging, open CORS, detailed errors |

## Database

| Variable | Default | Description |
|---|---|---|
| `GANTRY_DB` | `./data/gantry.db` | Database connection string. SQLite path or `postgres://` URL. |
| `GANTRY_DATA_DIR` | `./data` | Directory for SQLite DB file and encryption key |

### Database DSN Formats

```bash
# SQLite (default) — relative or absolute path
GANTRY_DB=./data/gantry.db
GANTRY_DB=/var/lib/gantry/gantry.db

# PostgreSQL
GANTRY_DB=postgres://user:password@host:5432/dbname
GANTRY_DB=postgres://user:password@host:5432/dbname?sslmode=require
GANTRY_DB=postgresql://user:password@host:5432/dbname
```

## Security

| Variable | Default | Description |
|---|---|---|
| `GANTRY_ADMIN_PASSWORD` | `changeme` | Initial admin user password (set on first run) |
| `GANTRY_JWT_SECRET` | *(auto-generated)* | HMAC-SHA256 secret for signing JWT tokens |
| `GANTRY_ENCRYPTION_KEY` | *(auto-generated)* | AES-256-GCM key for encrypting plugin configs |
| `GANTRY_PUBLIC_URL` | *(request host)* | Public http(s) origin for MCP OAuth. Example: `https://gantry.example.com`. Required behind a reverse proxy so Cursor's grant redirect and token audience stay stable. |

:::danger Change These in Production
Always set `GANTRY_ADMIN_PASSWORD`, `GANTRY_JWT_SECRET`, and `GANTRY_ENCRYPTION_KEY` to stable, random values before exposing Gantry to any network.
:::

### Generating Secure Values

```bash
# JWT secret (32 bytes = 64 hex chars)
openssl rand -hex 32

# Encryption key
openssl rand -hex 32

# Admin password
openssl rand -base64 24
```

## CLI Client Variables

These are used by `gantry get`, `apply`, `export`, `describe`, and `run` commands:

| Variable | Default | Description |
|---|---|---|
| `GANTRY_SERVER` | `http://localhost:8080` | Gantry server URL |
| `GANTRY_TOKEN` | *(none)* | Bearer token or API key for authentication |

```bash
export GANTRY_SERVER=https://gantry.your-org.com
export GANTRY_TOKEN=gantry_yourapikey
gantry get service
```

## Example: Minimal Production Config

```bash
# docker run or systemd EnvironmentFile
GANTRY_PORT=8080
GANTRY_DB=/data/gantry.db
GANTRY_DATA_DIR=/data
GANTRY_ADMIN_PASSWORD=<strong-password>
GANTRY_JWT_SECRET=<64-char-hex>
GANTRY_ENCRYPTION_KEY=<64-char-hex>
```

## Example: PostgreSQL Deployment

```bash
GANTRY_DB=postgres://gantry:password@postgres.internal:5432/gantry?sslmode=require
GANTRY_DATA_DIR=/data
GANTRY_ADMIN_PASSWORD=<strong-password>
GANTRY_JWT_SECRET=<64-char-hex>
GANTRY_ENCRYPTION_KEY=<64-char-hex>
```

## Data Directory Layout

```
$GANTRY_DATA_DIR/
├── gantry.db          # SQLite database (not present for PostgreSQL)
└── encryption.key     # Auto-generated AES-256-GCM key (32 bytes, hex-encoded)
```

Back up this entire directory. The `encryption.key` file is required to decrypt plugin configurations.
