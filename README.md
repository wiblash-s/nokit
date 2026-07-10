# Defuse

Self-hosted web admin panel for Counter-Strike 2 dedicated servers.

A **CS2 RCON web interface**, forked from [nokit / defuse](https://github.com/codevski/nokit). It provides an RCON console, live log streaming, player management, map control, and multi-server support — all in a single Go binary you can run alongside your CS2 container.

> ⚠️ Early development. Not yet production-ready.

## Feature Status

Only a subset of the panel is implemented today; the rest exists as a demo/planned reference. The full breakdown — a status table plus an exhaustive per-feature specification captured from the [nokit demo](https://nokit.app/demo) — lives in **[`featureDetail.md`](./featureDetail.md)**.

| Feature | Status |
|---------|--------|
| Login / Auth | ✅ Implemented |
| Multi-server switcher | ✅ Implemented |
| RCON Console | ✅ Implemented | Full terminal UI: timestamps, color-coded output, ↑↓ history, Ctrl+L clear, inline suggestion list autocomplete (top 8 ranked hits, prefix-highlighted, appears after first character; navigable with ↑↓, Tab/Enter to fill, Escape to dismiss, mouse click) for 5000+ CS2 CVARs/commands, RCON macros sidebar with localStorage persistence, clickable history panel, live/paused scroll toggle, Copy session & Export |
| Dashboard | ✅ Implemented |
| Live Logs | ✅ Implemented | Real-time server console streamed from `docker logs -f <container>` over SSE (`GET /api/logs/stream`); configurable line retention (default 500, 50–2000), auto-scroll with manual-scroll pause, Clear/Download, connection status indicator |
| Players | ❌ Not built (demo only / planned) |
| Maps | ✅ Implemented | Standard map pool (12 maps), favorites system (localStorage), workshop maps fetched live via RCON (`maps *`), map cycle editor, RCON integration (changelevel, host_workshop_map) |
| CVAR Presets | ❌ Not built (demo only / planned) |
| Config Editor | ✅ Implemented | File browser tree view, code editor with line numbers, unsaved changes tracking, save/reload/exec via RCON, support for .cfg and .json files |
| Plugins | ❌ Not built (demo only / planned) |
| Scheduler | ❌ Not built (demo only / planned) |
| Admin | ❌ Not built (demo only / planned) |

See **[`featureDetail.md`](./featureDetail.md)** for the complete demo reference used to implement these features.

## Features

- **RCON console** with inline suggestion list autocomplete for 5000+ commands and CVARs (auto-appears after the first character, prefix-highlighted, keyboard + mouse navigable)
- **Live logs** streamed over Server-Sent Events
- **Player management** — search, kick, SteamID resolution
- **Map control** — standard maps, workshop IDs, browser-stored favorites
- **CVAR presets** — pill buttons for common server configs
- **Multi-server** support with header dropdown switching
- **Auth** — sessions by default, optional reverse-proxy SSO pass-through

## Screenshots

_Coming soon._

## Requirements

- A CS2 dedicated server reachable over RCON (typically a Docker container which I have used joedwards32/cs2)
- Go 1.22+ (for building from source)
- Bun 1.x (for building the frontend from source)

## Quick start

```bash
git clone https://github.com/codevski/defuse
cd defuse
cp .env.example .env       # set PANEL_PASSWORD, RCON_PASSWORD, STEAM_GSLT
docker compose up -d
```

Open `http://localhost:8080`.

## Configuration

Servers are configured in `config.yml`. Secrets come from environment variables.

See [`config.example.yml`](./config.example.yaml) and [`.env.example`](./.env.example).

## Live Logs

The **Live Logs** panel (sidebar → *Live Logs*) streams your CS2 dedicated
server's console output in real time. It tails the server container with
`docker logs -f <container>` on the backend and relays each line to the browser
over **Server-Sent Events** (`GET /api/logs/stream`).

This is the quickest way to watch things like workshop-map downloads: after
issuing `host_workshop_map <id>` from the RCON console, the download progress
and map-load messages appear in the log stream.

**Panel features:**

- Real-time streaming with best-effort color coding (kills, chat, rounds,
  connects, workshop/download, errors).
- **Max lines** input — how many lines to retain in the browser (default `500`,
  range `50`–`2000`; persisted in `localStorage`).
- **Auto-scroll** that follows new output, and automatically **pauses** when you
  scroll up to read history (a *jump to latest* pill returns you to the bottom).
- **Clear** button and **Download `.log`** export.
- **Connection status indicator** (`connecting` / `connected` / `reconnecting` /
  `error`). The stream reconnects automatically and sends a heartbeat every 15s.

**Configuration:**

- `CS2_CONTAINER_NAME` — name of the CS2 container to tail. Defaults to `cs2`.
- The panel needs to run `docker` and reach the Docker daemon. In the provided
  `docker-compose.yml` this is wired up by installing `docker-cli` in the image
  and mounting the host socket read-only:

  ```yaml
  environment:
    CS2_CONTAINER_NAME: ${CS2_CONTAINER_NAME:-cs2}
  volumes:
    - /var/run/docker.sock:/var/run/docker.sock:ro
  ```

> ⚠️ Mounting the Docker socket grants the container control over the host's
> Docker daemon. Only enable this on hosts you trust. A read-only mount still
> allows `docker logs`.

## License

MIT
