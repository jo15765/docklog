# MS SQL Server log source

This fork adds `RUNTIME_MODE=mssql`, which streams container-style logs from SQL Server instead of the Docker socket.

## Quick start

```bash
git clone <your-fork-url>
cd docklog-mssql
export RUNTIME_MODE=mssql
export CLIENT_ACCESS=off   # optional for local dev
make build
./docklog
```

Open [http://localhost:8000](http://localhost:8000), sign in as `admin` / `admin123` (change the password after first login), then go to **MS SQL Settings** and enter **host**, **port**, **username**, and **password**.

Docker Compose (no Docker socket required):

```bash
docker compose -f docker-compose.mssql.yaml up --build -d
```

## Fixed log store

Connection settings always use:

| Setting  | Value |
| -------- | ----- |
| Database | `container_logs` |
| Table    | `dbo.container_logs` |

Expected columns include `id`, `logged_at`, `container_name`, `container_id`, `image`, `stream`, `compose_project`, `compose_service`, and `message`. The UI reads `id`, `logged_at`, `container_name`, `stream`, and `message` for display and streaming.

## MS SQL Settings UI

- **Host / port / username / password** — password is stored in local SQLite only (`DB_PATH`, default `docklog.db`)
- **Trust server certificate** — for self-signed or Docker SQL Server TLS
- **Disable encryption** — dev/local when TLS is not used
- **Test connection** — validates TCP, login, and table access before save

**Do not commit `docklog.db` or `data/`** — they contain the SQL password and notification webhook secrets. See [README — Before pushing to GitHub](../README.md#before-pushing-to-github).

## SQL Logs UI

- **All containers** — combined tail from the table
- **Per-container** — sidebar lists distinct `container_name` values; filter uses the `container` query parameter on API and WebSocket routes
- **Timezone** — display preset (default **America/New_York**) or custom IANA zone; persisted in browser local storage
- **Alternating row colors** — requires a fresh `pnpm --dir frontend build` after UI changes

## How streaming works

- **Initial / older pages**: `GET /api/mssql/logs?tail=10000` (newest 10k), then `&before_id={min_id}` for the next older batch; response includes `has_more_older`, `total`, `batch_size`, `entries` with RFC3339 `ts`
- **UI paging**: 100 lines per page; scrolling into unloaded history fetches the next 10k batch
- **WebSocket** (`/ws/mssql/logs`): polls for rows with `id` greater than the last seen id; payloads are JSON log entries (not plain text)
- **Full export**: `GET /api/mssql/logs?all=1` or **Download** (entire filter, may take time on large tables)

## Dashboard in MSSQL mode

The home dashboard uses **DashboardMssql** (SQL log source summary) instead of Docker container metrics. Docker container APIs return empty or unavailable responses; the frontend skips polling `/api/containers` when Docker is disabled.

## Environment

| Variable       | Description                    | Default  |
| -------------- | ------------------------------ | -------- |
| `RUNTIME_MODE` | Set to `mssql` for this mode   | `docker` |
| `DB_PATH`      | SQLite path for users/settings | `docklog.db` |
| `PORT`         | HTTP port                      | `8000`   |

Docker is not required in `mssql` mode.

## Notifications

Configure under **Admin → Notifications** (same as upstream DockLog). For Microsoft Teams:

- DockLog POSTs a **MessageCard** JSON body.
- **Teams Workflows** links that require OAuth will fail with **401** / `DirectApiAuthorizationRequired`; use classic **Incoming Webhook** connectors (if enabled in your tenant), an anonymous **When a HTTP request is received** flow from your admin, or **Custom webhook** for testing.
