# AGENTS.md — ChatGPT Codex Instructions for Gantry

Gantry is an open-source internal developer platform (IDP) — a self-hostable alternative to Backstage that deploys as a single Go binary with an embedded React frontend. Apache 2.0 licensed.

## Tech Stack

- **Backend:** Go 1.22, chi router, modernc.org/sqlite (pure Go, no CGO required)
- **Frontend:** React 18, Vite, Tailwind CSS, lucide-react icons, TypeScript
- **Auth:** bcrypt + JWT (golang-jwt/jwt/v5), API keys (`gantry_<hex>` format), GitHub OAuth SSO
- **Real-time:** gorilla/websocket
- **Encryption:** AES-256-GCM via `internal/crypto/` for plugin config secrets
- **CLI:** spf13/cobra

## Project Structure

```
cmd/gantry/          CLI entry point (serve, apply, get, describe, export, run, version)
internal/
  api/
    handlers/        REST handlers — all methods on handlers.Handlers struct
    middleware/      JWT + API key auth, request logging, metrics
    websocket/       WebSocket hub with channel subscriptions
  auth/              bcrypt password hashing, JWT lifecycle
  config/            Env var + YAML config loading
  crypto/            AES-256-GCM encryption helpers
  db/                SQLite queries (no ORM), migrations
  dispatcher/        Action dispatchers (webhook, etc.)
  entity/            Entity types, JSON Schema validation (schemas in internal/entity/schemas/)
  events/            In-process pub/sub event bus
  metrics/           Custom Prometheus text format (no external dep)
  plugins/           Plugin system: bundled registry, Kubernetes/GitHub/ArgoCD integrations
  search/            SQLite FTS5 full-text search
web/src/
  pages/             Dashboard, Catalog, EntityDetail, Actions, AuditLog, Settings, Login, Plugins, Users, StatusMonitor
  components/        Sidebar, SchemaForm, EntityGraph, GitHubTab, KubernetesTab, ArgoCDTab, etc.
  hooks/             useAuth, useTheme
  lib/
    api.ts           Typed API client (all endpoints, Bearer token auth)
    types.ts         TypeScript types for all domain objects
    plugin-runtime.ts Plugin extension point registry
  styles/globals.css Global styles + markdown rendering
website/             Docusaurus documentation site (separate from the app)
```

## Key Conventions

### Go Backend

- **No ORM.** Use `database/sql` directly. JSON fields (`tags`, `annotations`, `labels`, `spec`) stored as TEXT, marshaled with `encoding/json`.
- **Handlers** are methods on `handlers.Handlers` struct which holds DB, config, and service deps.
- **DB helpers:** `d.exec()`, `d.queryRow()`, `d.queryRows()` — receiver is `*DB`.
- **Audit logging:** always capture `IPAddress` (use `clientIP` helper) and `BeforeState`/`AfterState`.
- **Boolean in SQLite:** use `boolToInt()` helper.
- **Plugin configs** with secret fields encrypted at rest via `internal/crypto/`.
- **Metrics:** custom text format in `internal/metrics/` — do NOT add prometheus/client_golang.
- **No CGO.** Build with `CGO_ENABLED=0`. No C dependencies.

### Entity Model

Everything is an entity: `kind`, `apiVersion`, `metadata` (name, namespace, labels, annotations, tags), `spec`.

Built-in kinds: `Service`, `API`, `Infrastructure`, `Team`, `Environment`, `Documentation`, `Action`.

Each kind has a JSON Schema in `internal/entity/schemas/`. All schemas use `"additionalProperties": false`.

### Frontend

- **CSS variables:** `--gantry-bg-primary`, `--gantry-bg-secondary`, `--gantry-bg-tertiary`, `--gantry-text-primary`, `--gantry-text-secondary`, `--gantry-border`, `--gantry-accent`, `--gantry-accent-hover`, `--gantry-danger`, `--gantry-success`, `--gantry-warning`, and highlight colors `--gantry-mark-1` through `--gantry-mark-6`. Each has a light and a dark value. Do not invent other `--gantry-*` names.
- **Tailwind opacity:** `bg-[var(--gantry-accent)]/10` NOT `bg-[var(--gantry-accent)] bg-opacity-10`.
- **Dark mode contrast:** use `text-[var(--gantry-bg-primary)]` on accent backgrounds, NOT `text-white`.
- **Dark mode:** toggled via `.dark` class on `<html>`.
- **API calls:** always go through the `api` object in `web/src/lib/api.ts`.
- **SchemaForm:** auto-generates forms from JSON Schema for entity create/edit and action execution.

### API Patterns

- REST: `GET/POST /api/v1/entities`, `GET/PUT/DELETE /api/v1/entities/{kind}/{name}`
- Auth: `POST /api/v1/auth/login`, `GET /api/v1/auth/me`, `GET /api/v1/auth/github`
- Search: `GET /api/v1/search?q=`
- WebSocket: `GET /api/v1/ws`
- Metrics: `GET /metrics`
- Health: `/healthz`, `/readyz`
- Plugins: `GET/POST /api/v1/plugins`, `GET/PUT/DELETE /api/v1/plugins/{name}`, `POST .../enable|disable|sync`
- Dashboard config: `GET/PUT /api/v1/dashboard/config`
- API Keys: `GET/POST /api/v1/apikeys`, `DELETE /api/v1/apikeys/{id}`
- Graph: `GET /api/v1/graph/{kind}/{name}`
- History: `GET/POST /api/v1/history`

## Creating Plugins

Gantry plugins follow a consistent pattern across up to 11 files. Use the `status-monitor` plugin as a complete reference implementation.

### Files to Create or Modify

| # | File | Action | Purpose |
|---|------|--------|---------|
| 1 | `internal/api/handlers/<plugin>.go` | Create | Backend handler(s) — business logic, external API calls, caching |
| 2 | `internal/plugins/bundled/registry.json` | Edit | Plugin metadata, config schema, entity panels, action types |
| 3 | `internal/api/server.go` | Edit | Register API routes (BEFORE wildcard/SPA routes) |
| 4 | `web/src/lib/types.ts` | Edit | TypeScript interfaces for API responses |
| 5 | `web/src/lib/api.ts` | Edit | API client methods |
| 6 | `web/src/pages/<PluginPage>.tsx` | Create | Full-page UI (if plugin has sidebar entry) |
| 7 | `web/src/App.tsx` | Edit | Add Route for new page |
| 8 | `web/src/components/Sidebar.tsx` | Edit | Conditional nav item (check plugin enabled via API) |
| 9 | `web/src/pages/Dashboard.tsx` | Edit | Dashboard widget (label, default config, render case) |
| 10 | `internal/api/handlers/dashboard.go` | Edit | Add widget ID to `knownWidgetIDs` map |
| 11 | `web/src/components/<PluginTab>.tsx` | Create | Entity detail tab (if plugin adds entity panels) |

### Backend Handler Pattern

```go
// internal/api/handlers/my_plugin.go
package handlers

func (h *Handlers) GetMyPluginData(w http.ResponseWriter, r *http.Request) {
    // 1. Check plugin is installed and enabled
    p, err := h.DB.GetPlugin(r.Context(), "my-plugin")
    if err != nil || p == nil {
        writeError(w, http.StatusNotFound, "my-plugin not installed")
        return
    }
    if !p.Enabled {
        writeError(w, http.StatusBadRequest, "my-plugin not enabled")
        return
    }

    // 2. Read custom config from p.Config (map[string]any)
    // 3. Business logic (external API calls, DB queries, etc.)
    // 4. For external HTTP: use http.NewRequest + set User-Agent header
    // 5. For expensive ops: cache with sync.RWMutex + TTL
    // 6. Return JSON with writeJSON(w, http.StatusOK, result)
}
```

### Registry Entry Pattern

Each entry in `registry.json` must include ALL of these fields:

```json
{
  "name": "my-plugin",
  "title": "My Plugin",
  "description": "Short description shown in plugin list.",
  "longDescription": "Detailed description shown in plugin detail view.",
  "features": ["Feature 1", "Feature 2"],
  "version": "1.0.0",
  "author": "Gantry",
  "category": "integration|widget|action-type",
  "homepage": "https://github.com/go2engle/gantry",
  "configSchema": {
    "type": "object",
    "properties": { ... },
    "required": [...]
  },
  "entityPanels": ["Service"],
  "actionTypes": ["my-action-type"]
}
```

### Dashboard Widget Pattern

In `Dashboard.tsx`:
1. Add to `WIDGET_LABELS`: `my_widget: 'My Widget'`
2. Add to `DEFAULT_WIDGETS`: `{ id: 'my_widget', visible: true, order: N, width: 'full' }`
3. Add state + fetch in the `useEffect` / `Promise.all`
4. Add `case 'my_widget':` in `renderWidget()` — return `null` if no data

In `dashboard.go`:
5. Add `"my_widget": true` to `knownWidgetIDs` map

### Common Pitfalls

- **CSS variables:** only the properties listed in the Frontend section exist (`success`, `warning`, and `mark-1`..`mark-6` included). Using undefined ones renders invisible.
- **Tailwind opacity:** `bg-[var(--gantry-accent)]/10` not `bg-[var(--gantry-accent)] bg-opacity-10`.
- **Dark mode contrast:** `text-[var(--gantry-bg-primary)]` on accent backgrounds, never `text-white`.
- **Entity schemas:** `additionalProperties: false` — check allowed spec fields before adding.
- **External HTTP APIs:** always set `User-Agent` header; some providers block default Go client.
- **Widget ID validation:** missing from `knownWidgetIDs` → dashboard save fails with "unknown widget id".
- **Route ordering:** plugin routes in `server.go` must come before wildcard routes.
- **Registry JSON:** missing `configSchema`/`entityPanels`/`actionTypes` causes parse errors.

## Build Commands

```bash
make build                  # full build (frontend + backend)
go build ./...              # backend only
cd web && npm run build     # frontend only
go test ./...               # run all Go tests
cd web && npx tsc --noEmit  # TypeScript type check
cd web && npm run dev       # frontend dev server (proxies API to :8080)
make dev-watch              # air (backend live reload) + vite HMR
golangci-lint run ./...     # lint Go
```

## Config

Env vars: `GANTRY_PORT`, `GANTRY_DB`, `GANTRY_DEV`, `GANTRY_ADMIN_PASSWORD`, `GANTRY_JWT_SECRET`, `GANTRY_DATA_DIR`, `GANTRY_ENCRYPTION_KEY`, `GANTRY_INSECURE_ALLOW`, `GANTRY_METRICS_TOKEN`, `GANTRY_DEV_ORIGINS`. Also configurable via YAML file with camelCase keys.

Production fail-fast (unless `GANTRY_INSECURE_ALLOW=true`): non-default admin password, explicit JWT secret, encryption key or persisted `encryption.key`, `GANTRY_DEV=false`. SSRF guard on webhooks/health-check; RBAC rules enforced on APIs; cookie-first SPA auth.
