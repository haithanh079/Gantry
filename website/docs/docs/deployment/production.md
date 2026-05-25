---
sidebar_position: 3
title: Production Deployment
description: Checklist and best practices for running Gantry in production.
---

# Production Deployment

## Pre-Deployment Checklist

Before going live, complete every item on this list:

### Security

- [ ] **Change `GANTRY_ADMIN_PASSWORD`** — Never use `changeme` in production
- [ ] **Set `GANTRY_JWT_SECRET`** — Use a stable 64-character random hex string. If unset, the server refuses to start in production.
- [ ] **Set `GANTRY_ENCRYPTION_KEY`** — Or ensure `data/encryption.key` already exists from a prior run. Required for stable plugin credential encryption.
- [ ] **Enable HTTPS** — Use a reverse proxy (Nginx, Caddy, Traefik) or set `--tls-cert`/`--tls-key`
- [ ] **Disable dev mode** — Ensure `GANTRY_DEV=false` (the default). Production startup fails if dev mode is on.
- [ ] **Set `GANTRY_METRICS_TOKEN`** — Protects `/metrics` with Bearer auth (recommended)
- [ ] **Restrict admin access** — Create dedicated user accounts; don't share the `admin` account
- [ ] **Use API keys for automation** — Never use passwords in CI/CD; create a down-scoped API key
- [ ] **Review RBAC permission rules** — Deny/allow rules are enforced on the API (not UI-only)
- [ ] **Avoid `GANTRY_INSECURE_ALLOW`** — Only for intentionally insecure dev/staging; skips production validation

### Production startup validation

When `GANTRY_DEV=false` and `GANTRY_INSECURE_ALLOW` is not set, Gantry **refuses to start** unless:

| Requirement | Why |
|---|---|
| Bootstrap admin password changed from default | Prevents `changeme` on a public deployment |
| `GANTRY_JWT_SECRET` set explicitly | Sessions must survive restarts |
| `GANTRY_ENCRYPTION_KEY` or existing `encryption.key` | Plugin secrets remain decryptable after redeploy |
| `GANTRY_DEV` is false | Dev mode enables permissive CORS |

Set `GANTRY_INSECURE_ALLOW=true` only when you deliberately accept these risks (local experiments, not production).

### Built-in protections

| Feature | Behavior |
|---|---|
| SSRF guard | Webhook actions and `/api/v1/health-check` cannot target private/loopback IPs |
| Login rate limit | ~10 attempts/minute per IP on `POST /auth/login` |
| RBAC rules | Fine-grained allow/deny rules apply to entity and action APIs |
| Cookie-first SPA | Browser UI uses HttpOnly `gantry_session`; JWT is not stored in `localStorage` |

### Data

- [ ] **Back up your data directory** — Set up regular backups of `$GANTRY_DATA_DIR`
- [ ] **Back up `encryption.key`** separately — Loss of this key = loss of plugin credentials
- [ ] **Mount a persistent volume** if using Docker/Kubernetes

### Networking

- [ ] **Put a reverse proxy in front** — Gantry doesn't do TLS termination by default
- [ ] **Set CORS appropriately** — Gantry with `GANTRY_DEV=false` only serves same-origin requests
- [ ] **Configure health checks** — Use `/healthz` and `/readyz` for load balancer health probes

## Reverse Proxy Configuration

### Nginx

```nginx
server {
    listen 443 ssl http2;
    server_name gantry.your-org.com;

    ssl_certificate     /etc/nginx/certs/gantry.crt;
    ssl_certificate_key /etc/nginx/certs/gantry.key;

    location / {
        proxy_pass         http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header   Upgrade $http_upgrade;
        proxy_set_header   Connection "upgrade";
        proxy_set_header   Host $host;
        proxy_set_header   X-Real-IP $remote_addr;
        proxy_set_header   X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;

        # WebSocket support
        proxy_read_timeout 86400;
    }
}

server {
    listen 80;
    server_name gantry.your-org.com;
    return 301 https://$host$request_uri;
}
```

### Caddy

```
# Caddyfile
gantry.your-org.com {
    reverse_proxy localhost:8080
}
```

Caddy handles TLS automatically via Let's Encrypt.

### Traefik (Docker labels)

```yaml
services:
  gantry:
    image: ghcr.io/go2engle/gantry:latest
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.gantry.rule=Host(`gantry.your-org.com`)"
      - "traefik.http.routers.gantry.entrypoints=websecure"
      - "traefik.http.routers.gantry.tls.certresolver=letsencrypt"
      - "traefik.http.services.gantry.loadbalancer.server.port=8080"
```

## Systemd Service (Linux)

```ini
# /etc/systemd/system/gantry.service
[Unit]
Description=Gantry IDP
After=network.target
Wants=network.target

[Service]
Type=simple
User=gantry
Group=gantry
WorkingDirectory=/opt/gantry
ExecStart=/usr/local/bin/gantry serve
Restart=on-failure
RestartSec=5

# Environment
Environment=GANTRY_DATA_DIR=/var/lib/gantry
Environment=GANTRY_PORT=8080
EnvironmentFile=/etc/gantry/gantry.env

# Security
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/var/lib/gantry

[Install]
WantedBy=multi-user.target
```

```bash
# /etc/gantry/gantry.env (chmod 600, owned by gantry:gantry)
GANTRY_ADMIN_PASSWORD=my-secure-password
GANTRY_JWT_SECRET=64charrandombytes
GANTRY_ENCRYPTION_KEY=64charrandombytes
GANTRY_METRICS_TOKEN=random-metrics-scrape-token
```

```bash
sudo systemctl enable gantry
sudo systemctl start gantry
sudo systemctl status gantry
```

## Monitoring

### Health Endpoints

| Endpoint | Use |
|---|---|
| `GET /healthz` | Process is alive. Returns `{"status":"ok"}` |
| `GET /readyz` | DB is reachable. Returns 503 if not. Use for readiness probe. |

### Metrics

`GET /metrics` returns Prometheus-format metrics:

```
# HELP gantry_entities_total Total number of entities
# TYPE gantry_entities_total gauge
gantry_entities_total 142

# HELP gantry_http_requests_total Total HTTP requests
# TYPE gantry_http_requests_total counter
gantry_http_requests_total{method="GET",status="200"} 8432
gantry_http_requests_total{method="POST",status="201"} 87

# HELP gantry_request_duration_seconds HTTP request latency
# TYPE gantry_request_duration_seconds histogram
```

### Prometheus Scrape Config

When `GANTRY_METRICS_TOKEN` is set, pass it as a Bearer token:

```yaml
# prometheus.yml
scrape_configs:
  - job_name: gantry
    static_configs:
      - targets: ['gantry.your-org.com:8080']
    metrics_path: /metrics
    authorization:
      type: Bearer
      credentials: "<GANTRY_METRICS_TOKEN>"
```

Alternatively, use `metrics_path: /metrics?token=<GANTRY_METRICS_TOKEN>` (less ideal — token may appear in logs).

## Backup Strategy

### SQLite Backup

```bash
# Live consistent backup using SQLite backup API
sqlite3 /var/lib/gantry/gantry.db ".backup /backups/gantry-$(date +%Y%m%d-%H%M).db"
```

Or use `VACUUM INTO`:

```bash
sqlite3 /var/lib/gantry/gantry.db "VACUUM INTO '/backups/gantry-$(date +%Y%m%d-%H%M).db'"
```

### Encryption Key Backup

```bash
cp /var/lib/gantry/encryption.key /secure-backup/gantry-encryption.key
```

Store this in a secure secrets manager (Vault, AWS Secrets Manager, etc.).

### Automated Daily Backup (Cron)

```cron
# /etc/cron.d/gantry-backup
0 2 * * * gantry sqlite3 /var/lib/gantry/gantry.db "VACUUM INTO '/backups/gantry-$(date +\%Y\%m\%d).db'" && find /backups -name "gantry-*.db" -mtime +30 -delete
```

## Upgrading

Gantry runs database migrations automatically on startup. Upgrades are generally safe:

```bash
# Download new binary
curl -sSL https://github.com/go2engle/gantry/releases/download/v0.2.0/gantry_linux_amd64.tar.gz | tar -xz
sudo mv gantry /usr/local/bin/gantry

# Restart
sudo systemctl restart gantry
```

For Docker:

```bash
docker compose pull
docker compose up -d
```

:::tip Backup Before Upgrading
Always back up `gantry.db` before upgrading across major versions.
:::

## High Availability

Gantry with SQLite is inherently single-instance. For HA:

1. Switch to **PostgreSQL** (`GANTRY_DB=postgres://...`)
2. Run multiple Gantry instances behind a load balancer
3. Ensure all instances share the same `GANTRY_JWT_SECRET` and `GANTRY_ENCRYPTION_KEY`
4. Use a shared `GANTRY_DATA_DIR` only for the encryption key (not for SQLite)

:::note HA Status
PostgreSQL-backed HA is currently in progress. Single-instance SQLite is recommended for most deployments.
:::
