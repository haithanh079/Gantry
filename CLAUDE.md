# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Gantry is an open-source internal developer platform (IDP) — a self-hostable alternative to Backstage that deploys as a single binary. It provides a unified service catalog, self-service portal, and GitOps-native configuration management. Apache 2.0 licensed.

The PRD is in `Gantry-PRD-v1.0.md`.

## Build Commands

```bash
# Full build (frontend + backend)
make build

# Backend only (Go binary → bin/gantry)
go build -o bin/gantry ./cmd/gantry

# Frontend only (React → web/dist/)
cd web && npm install && npm run build

# Run in dev mode
bin/gantry serve --dev

# Frontend dev server with hot reload (proxies API to :8080)
cd web && npm run dev

# Live reload dev (air + vite, requires air: go install github.com/air-verse/air@latest)
make dev-watch

# Run all Go tests
go test ./...

# Run a single Go test
go test ./... -run TestEntityCreate -v

# TypeScript type check
cd web && npx tsc --noEmit

# Lint Go code
golangci-lint run ./...

# Format Go code
make fmt
```

## Tech Stack

- **Backend:** Go — single binary, chi router, embedded SQLite (modernc.org/sqlite, pure Go/no CGO)
- **Frontend:** React 18 + Vite + Tailwind CSS + lucide-react icons
- **Auth:** bcrypt passwords + JWT (golang-jwt/jwt/v5)
- **Real-time:** gorilla/websocket
- **Search:** SQLite FTS5 (full-text search)
- **Schema validation:** santhosh-tekuri/jsonschema/v5
- **CLI:** spf13/cobra
- **Encryption:** AES-256-GCM for plugin config secrets (`internal/crypto/`)
- **Live reload:** github.com/air-verse/air (backend), Vite HMR (frontend)

## Architecture

Modular monolith — single process. Key packages:

- `cmd/gantry/` — CLI entry point (serve, apply, get, describe, export, run, version)
- `internal/api/` — HTTP server, routes, SPA serving
- `internal/api/handlers/` — REST handlers (entities, auth, apikeys, search, schemas, actions, audit, health, graph, plugins, github, dashboard, history)
- `internal/api/middleware/` — Auth (JWT + API key), request logging + metrics
- `internal/api/websocket/` — WebSocket hub with channel subscriptions
- `internal/auth/` — Password hashing (bcrypt) and JWT token lifecycle
- `internal/config/` — Config loading from env vars + YAML file
- `internal/crypto/` — AES-256-GCM encryption for DB-stored secrets
- `internal/db/` — Database layer (SQLite), migrations, entity CRUD queries
- `internal/dispatcher/` — Action dispatchers (webhook, etc.)
- `internal/entity/` — Entity types, built-in kinds, JSON Schema validation (schemas embedded via go:embed)
- `internal/events/` — In-process pub/sub event bus
- `internal/metrics/` — Custom Prometheus text format metrics (no external dep)
- `internal/plugins/` — Plugin manifest types, bundled registry, Kubernetes/GitHub/ArgoCD integrations
- `internal/search/` — FTS5 search service
- `web/` — React SPA (served from web/dist/ by Go binary)
  - `web/src/pages/` — Dashboard, Catalog, EntityDetail, Actions, AuditLog, Settings, Login, Plugins, Users
  - `web/src/components/` — Sidebar, CommandPalette, EntityCard, EntityTable, SchemaForm, ThemeToggle, ErrorBoundary, EntityGraph, GitHubTab, KubernetesTab, ArgoCDTab, ActionWizard, ActionFormBuilder
  - `web/src/hooks/` — useAuth, useTheme
  - `web/src/lib/` — api.ts (API client), types.ts, plugin-runtime.ts
  - `web/src/styles/globals.css` — global styles + markdown rendering
- `website/` — Docusaurus documentation site (separate from the app)

### Entity Model

Everything is an entity with `kind`, `apiVersion`, `metadata`, and `spec` fields. Each kind has a JSON Schema in `internal/entity/schemas/` that drives both backend validation and frontend form generation.

Built-in kinds: Service, API, Infrastructure, Team, Environment, Documentation, Action.

### Data Flow

- **UI Path:** User → API handler → schema validation → DB write → event bus → WebSocket broadcast
- **API auth:** JWT Bearer token in Authorization header; claims stored in request context via middleware. API keys use `gantry_<hex>` prefix format.

### Key Patterns

- DB queries use `database/sql` directly (no ORM). JSON fields (tags, annotations, labels, spec) stored as TEXT and marshaled via `encoding/json`.
- Handlers are methods on `handlers.Handlers` struct which holds all service dependencies.
- The `entity.SchemaValidator` uses `go:embed` to load JSON schemas from `internal/entity/schemas/`.
- Frontend uses CSS custom properties (`--gantry-*`) for theming; dark mode via `.dark` class on `<html>`.
- `SchemaForm` component auto-generates forms from JSON Schema for entity creation/editing and action execution.
- Plugin configs with secret fields are encrypted at rest using AES-256-GCM via `internal/crypto/`.
- Browsing history is tracked per-user in the `history` DB table; surfaced on the Dashboard.

## Config

Env vars: `GANTRY_PORT`, `GANTRY_DB`, `GANTRY_DEV`, `GANTRY_ADMIN_PASSWORD`, `GANTRY_JWT_SECRET`, `GANTRY_DATA_DIR`, `GANTRY_ENCRYPTION_KEY`, `GANTRY_INSECURE_ALLOW`, `GANTRY_METRICS_TOKEN`, `GANTRY_DEV_ORIGINS`. CLI flags override env vars. Default: port 8080, SQLite at `./data/gantry.db`, admin/changeme.

Production (`GANTRY_DEV=false`) **fail-fast** unless `GANTRY_INSECURE_ALLOW=true`: bootstrap admin password must not be default, `GANTRY_JWT_SECRET` must be set explicitly, and encryption key must be configured or persisted in `data/encryption.key`.

`GANTRY_ENCRYPTION_KEY` — any stable string (SHA-256 derived to 32 bytes) for AES-256-GCM plugin secrets. Auto-generated `encryption.key` in DataDir if unset.

Security: SSRF guard on outbound webhooks/health-check proxy; RBAC permission rules enforced on entity/action APIs via `handlers.CanAccess`; login rate limit; optional `GANTRY_METRICS_TOKEN` on `/metrics`; SPA uses HttpOnly cookies (no JWT in localStorage).

Config can also be loaded from a YAML file (same field names in camelCase).

## API Patterns

REST endpoints: `/api/v1/entities`, `/api/v1/entities/{kind}`, `/api/v1/entities/{kind}/{name}`.
Actions: `/api/v1/actions/{name}/execute`, `/api/v1/actions/{name}/runs/{id}`.
Auth: `POST /api/v1/auth/login`, `GET /api/v1/auth/me`, `GET /api/v1/auth/github` (OAuth SSO).
Search: `GET /api/v1/search?q=`.
WebSocket: `GET /api/v1/ws`.
Health: `/healthz`, `/readyz`.
Metrics: `GET /metrics` (Prometheus text format).
Dashboard: `GET /api/v1/dashboard`.
History: `GET /api/v1/history`, `POST /api/v1/history`.
Graph: `GET /api/v1/graph/{kind}/{name}`.
Plugins: `GET/POST /api/v1/plugins`, `GET/PUT/DELETE /api/v1/plugins/{name}`, `POST /api/v1/plugins/{name}/enable`, `POST /api/v1/plugins/{name}/disable`, `POST /api/v1/plugins/{name}/sync`.
API Keys: `GET/POST /api/v1/apikeys`, `DELETE /api/v1/apikeys/{id}`.

## CSS Theming

Only these CSS custom properties are defined (do NOT use others):
`--gantry-bg-primary`, `--gantry-bg-secondary`, `--gantry-bg-tertiary`, `--gantry-text-primary`, `--gantry-text-secondary`, `--gantry-border`, `--gantry-accent`, `--gantry-accent-hover`, `--gantry-danger`, `--gantry-success`, `--gantry-warning`, `--gantry-mark-1`, `--gantry-mark-2`, `--gantry-mark-3`, `--gantry-mark-4`, `--gantry-mark-5`, `--gantry-mark-6`.

For transparent accent backgrounds use `bg-[var(--gantry-accent)]/10` (Tailwind opacity modifier), NOT `bg-opacity-10`.

## Creating Plugins

Gantry plugins follow a consistent pattern. Every plugin touches up to 11 files. Use the status-monitor plugin as a reference implementation.

### Plugin Anatomy — Files to Create or Modify

| # | File | Action | Purpose |
|---|------|--------|---------|
| 1 | `internal/api/handlers/<plugin>.go` | **Create** | Backend handler(s) — business logic, external API calls, caching |
| 2 | `internal/plugins/bundled/registry.json` | **Edit** | Add plugin metadata, config schema, entity panels, action types |
| 3 | `internal/api/server.go` | **Edit** | Register API routes (must be BEFORE wildcard/SPA catch-all routes) |
| 4 | `web/src/lib/types.ts` | **Edit** | Add TypeScript interfaces for API response types |
| 5 | `web/src/lib/api.ts` | **Edit** | Add API client methods |
| 6 | `web/src/pages/<PluginPage>.tsx` | **Create** | Full-page UI (if plugin has a sidebar entry) |
| 7 | `web/src/App.tsx` | **Edit** | Add `<Route>` for the new page |
| 8 | `web/src/components/Sidebar.tsx` | **Edit** | Add conditional nav item (check if plugin is enabled via API) |
| 9 | `web/src/pages/Dashboard.tsx` | **Edit** | Add dashboard widget (label, default config, render case) |
| 10 | `internal/api/handlers/dashboard.go` | **Edit** | Add widget ID to `knownWidgetIDs` map |
| 11 | `web/src/components/<PluginTab>.tsx` | **Create** | Entity detail tab (if plugin adds entity panels) |

### Step-by-Step

**1. Backend Handler** (`internal/api/handlers/<plugin>.go`)
- Handlers are methods on `*Handlers` struct: `func (h *Handlers) GetMyData(w http.ResponseWriter, r *http.Request)`
- Always check plugin is installed and enabled first:
  ```go
  p, err := h.DB.GetPlugin(r.Context(), "my-plugin")
  if err != nil || p == nil { writeError(w, 404, "plugin not installed"); return }
  if !p.Enabled { writeError(w, 400, "plugin not enabled"); return }
  ```
- For external HTTP calls: always set `User-Agent` header (some APIs block default Go client), use `http.NewRequest` + `client.Do(req)` instead of `client.Get()`
- For expensive operations: use an in-memory cache with `sync.RWMutex` and a TTL (e.g., 60s)
- For concurrent work: use `sync.WaitGroup` + goroutines with a `sync.Mutex` to protect shared state
- Read custom config from `p.Config` (it's a `map[string]any` parsed from JSON)

**2. Registry Entry** (`internal/plugins/bundled/registry.json`)
- Must include ALL fields: `name`, `title`, `description`, `longDescription`, `features`, `version`, `author`, `category`, `homepage`, `configSchema`
- `configSchema` is a JSON Schema object — drives the config modal UI via `SchemaForm`
- Optional: `entityPanels` (array of kind strings), `actionTypes` (array of action type strings)
- Categories: `integration`, `widget`, `action-type`

**3. Route Registration** (`internal/api/server.go`)
- Add routes inside the `protected` group (requires auth)
- Place BEFORE any wildcard or SPA catch-all routes
- Pattern: `protected.Get("/plugins/<name>/<endpoint>", h.HandlerMethod)`

**4. Frontend Types** (`web/src/lib/types.ts`)
- Add interfaces for any new API response shapes
- Export them so pages/components can import

**5. API Client** (`web/src/lib/api.ts`)
- Add methods to the `api` object: `getMyData: () => request<MyType>('GET', '/plugins/my-plugin/data')`
- Import new types from `types.ts`

**6. Plugin Page** (`web/src/pages/<PluginPage>.tsx`)
- Standard page with loading state, error handling, auto-refresh
- Use only `--gantry-*` CSS variables for colors
- For text contrasting against accent background: use `text-[var(--gantry-bg-primary)]` (NOT `text-white`)
- For category/filter pills: selected state = `bg-[var(--gantry-accent)] text-[var(--gantry-bg-primary)]`

**7. Route** (`web/src/App.tsx`)
- Add: `<Route path="/my-plugin" element={<MyPluginPage />} />`

**8. Sidebar Entry** (`web/src/components/Sidebar.tsx`)
- Add state: `const [myPluginEnabled, setMyPluginEnabled] = useState(false)`
- Check on mount: `api.listPlugins().then(ps => setMyPluginEnabled(ps.some(p => p.name === 'my-plugin' && p.enabled)))`
- Conditionally render nav item with a lucide-react icon

**9. Dashboard Widget** (`web/src/pages/Dashboard.tsx`)
- Add to `WIDGET_LABELS`: `my_widget: 'My Widget'`
- Add to `DEFAULT_WIDGETS`: `{ id: 'my_widget', visible: true, order: N, width: 'full' }`
- Add `case 'my_widget':` in `renderWidget()` switch
- Add state + fetch in the main `useEffect` / `Promise.all`
- Return `null` from render case if no data (plugin not enabled)

**10. Backend Widget Validation** (`internal/api/handlers/dashboard.go`)
- Add widget ID to the `knownWidgetIDs` map — without this, saving dashboard config will fail with "unknown widget id"

### Common Pitfalls

- **CSS variables:** only the properties listed under CSS Theming exist, including `--gantry-success`, `--gantry-warning`, and `--gantry-mark-1` through `--gantry-mark-6`. Using others → invisible/transparent elements
- **Tailwind opacity:** `bg-[var(--gantry-accent)]/10` not `bg-[var(--gantry-accent)] bg-opacity-10`
- **Dark mode contrast:** never use `text-white` on accent backgrounds; use `text-[var(--gantry-bg-primary)]` which adapts
- **Entity schemas:** all have `additionalProperties: false` — check allowed spec fields before adding new ones
- **External HTTP APIs:** always set User-Agent header; some providers block the default Go HTTP client
- **Widget ID validation:** forgetting to add the ID to `knownWidgetIDs` in `dashboard.go` causes save failures
- **Route ordering:** plugin routes in `server.go` must come before wildcard routes
- **Registry JSON:** missing `configSchema`, `entityPanels`, or `actionTypes` fields will cause parse errors

### Verification Checklist

```bash
# After making changes, always verify:
go build ./...              # Backend compiles
cd web && npx tsc --noEmit  # Frontend type-checks
cd web && npm run build     # Frontend builds
```
