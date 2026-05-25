---
sidebar_position: 3
title: Configuration
description: Configure Gantry via environment variables, config file, or CLI flags.
---

# Configuration

Gantry is configured via **environment variables**, a **YAML config file**, or **CLI flags**. These are evaluated in priority order:

```
CLI flags  >  Environment variables  >  gantry.yaml  >  Built-in defaults
```

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `GANTRY_PORT` | `8080` | HTTP listen port |
| `GANTRY_DB` | `./data/gantry.db` | Database connection string |
| `GANTRY_DEV` | `false` | Enable development mode |
| `GANTRY_ADMIN_PASSWORD` | `changeme` | Initial admin user password |
| `GANTRY_JWT_SECRET` | *(auto-generated)* | JWT signing secret (HMAC-SHA256) |
| `GANTRY_DATA_DIR` | `./data` | Directory for SQLite DB and encryption key |
| `GANTRY_ENCRYPTION_KEY` | *(auto-generated)* | AES-256-GCM key for encrypting plugin configs |
| `GANTRY_INSECURE_ALLOW` | `false` | Skip production security validation (not for real deployments) |
| `GANTRY_METRICS_TOKEN` | *(unset)* | Bearer token required to scrape `/metrics` when set |
| `GANTRY_DEV_ORIGINS` | `http://localhost:5173,http://127.0.0.1:5173` | Allowed CORS origins when `GANTRY_DEV=true` |

### Example

```bash
export GANTRY_PORT=9000
export GANTRY_DB=postgres://user:pass@localhost:5432/gantry
export GANTRY_ADMIN_PASSWORD=my-secure-password
export GANTRY_JWT_SECRET=a-long-random-string-here

gantry serve
```

## Config File (gantry.yaml)

Place `gantry.yaml` in the current directory or pass `--config /path/to/gantry.yaml`:

```yaml
# gantry.yaml
port: 8080
db: ./data/gantry.db        # or postgres:// for PostgreSQL
dev: false
adminPassword: changeme
jwtSecret: ""               # auto-generated if empty
dataDir: ./data
encryptionKey: ""           # auto-generated if empty
```

```bash
gantry serve --config ./gantry.yaml
```

## CLI Flags (`gantry serve`)

| Flag | Description |
|---|---|
| `--port, -p` | HTTP listen port |
| `--db` | Database DSN |
| `--dev` | Enable development mode |
| `--admin-password` | Initial admin password |
| `--config` | Path to gantry.yaml |
| `--tls-cert` | TLS certificate file (enables HTTPS) |
| `--tls-key` | TLS private key file |

## Database Configuration

### SQLite (Default)

SQLite is the default and requires zero configuration. The database file is created automatically.

```bash
# Default location
GANTRY_DB=./data/gantry.db

# Custom location
GANTRY_DB=/var/lib/gantry/gantry.db
```

Gantry enables WAL mode and foreign keys automatically.

### PostgreSQL

For production multi-instance deployments:

```bash
GANTRY_DB=postgres://gantry:password@postgres.example.com:5432/gantry
# or
GANTRY_DB=postgresql://gantry:password@postgres.example.com:5432/gantry?sslmode=require
```

:::info PostgreSQL Support
PostgreSQL support is currently in progress. SQLite is recommended for most deployments.
:::

## Security Configuration

### JWT Secret

The JWT secret signs all authentication tokens. It should be a long, random string that stays stable across restarts.

```bash
# Generate a good secret
openssl rand -hex 32

GANTRY_JWT_SECRET=a64characterrandomhexstringhere
```

If `GANTRY_JWT_SECRET` is empty, Gantry auto-generates a random secret on startup. **This means all existing sessions are invalidated on every restart.** Always set this in production.

### Admin Password

The default admin password is `changeme`. **Change it before exposing Gantry to a network.**

```bash
GANTRY_ADMIN_PASSWORD=my-secure-password
```

You can also change it after startup via **Settings → Users** in the UI, or via the API:

```bash
curl -X PUT http://localhost:8080/api/v1/auth/me/password \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"currentPassword": "changeme", "newPassword": "my-secure-password"}'
```

### Encryption Key

Gantry encrypts sensitive plugin configuration (API tokens, credentials) with AES-256-GCM.

```bash
# Auto-generated and persisted to $GANTRY_DATA_DIR/encryption.key
# Or set explicitly:
GANTRY_ENCRYPTION_KEY=your-32-byte-hex-key-here
```

:::warning Key Management
Do not lose your encryption key. If it changes, existing encrypted plugin configs become unreadable. Always back up `$GANTRY_DATA_DIR/encryption.key` or set `GANTRY_ENCRYPTION_KEY` explicitly in production.
:::

## Development Mode

`--dev` / `GANTRY_DEV=true` enables:

- Verbose structured JSON logging to stdout
- Open CORS headers (all origins allowed) for local frontend development
- More detailed error messages in API responses

Never run development mode in production.

## Data Directory

`GANTRY_DATA_DIR` controls where Gantry stores persistent data:

```
$GANTRY_DATA_DIR/
├── gantry.db          # SQLite database (if using SQLite)
└── encryption.key     # Auto-generated AES-256-GCM key
```

In Docker deployments, mount a volume to this directory:

```bash
docker run -v /path/to/data:/data -e GANTRY_DATA_DIR=/data ...
```

## CLI Auth Configuration

The `gantry` CLI commands (`get`, `apply`, `export`, `describe`, `run`) connect to a running server. Configure them via:

| Environment Variable | Description |
|---|---|
| `GANTRY_SERVER` | Server URL (default: `http://localhost:8080`) |
| `GANTRY_TOKEN` | Bearer token or API key for authentication |

```bash
export GANTRY_SERVER=https://gantry.your-org.com
export GANTRY_TOKEN=gantry_yourapikey

gantry get service
```

Or pass flags:

```bash
gantry get service --server https://gantry.your-org.com --token gantry_yourapikey
```
