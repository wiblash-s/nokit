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
| RCON Console | ✅ Implemented | Full terminal UI: timestamps, color-coded output, ↑↓ history, Ctrl+L clear, Tab autocomplete (5000+ CS2 CVARs/commands), RCON macros sidebar with localStorage persistence, clickable history panel, live/paused scroll toggle, Copy session & Export |
| Dashboard | ✅ Implemented |
| Live Logs | ❌ Not built (demo only / planned) |
| Players | ❌ Not built (demo only / planned) |
| Maps | ✅ Implemented | Standard map pool (12 maps), favorites system (localStorage), workshop map support, map cycle editor, RCON integration (changelevel, host_workshop_map) |
| CVAR Presets | ❌ Not built (demo only / planned) |
| Config Editor | ❌ Not built (demo only / planned) |
| Plugins | ❌ Not built (demo only / planned) |
| Scheduler | ❌ Not built (demo only / planned) |
| Admin | ❌ Not built (demo only / planned) |

See **[`featureDetail.md`](./featureDetail.md)** for the complete demo reference used to implement these features.

## Features

- **RCON console** with autocomplete for 5000+ commands and CVARs
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

## License

MIT
