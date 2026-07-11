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
| Live Logs | ✅ Implemented | Real-time server console received over UDP (`logaddress_add`) **and HTTP (`logaddress_add_http`)** and streamed to the browser over SSE (`GET /api/logs/stream`); configurable line retention (default 500, 50–2000), auto-scroll with manual-scroll pause, Clear/Download, connection status indicator. No Docker socket required |
| HTTP Log Listener | ✅ Implemented | Public `POST /api/logs/http` endpoint accepts CS2 `logaddress_add_http` plain-text payloads and feeds them into the **same** loghub pipeline as the UDP listener — so HTTP-sourced logs appear in Live Logs and drive the same workshop-map download verification. Coexists with UDP (both run at once). Optional `CS2_LOG_HTTP_TOKEN` shared-secret guard |
| Players | ❌ Not built (demo only / planned) |
| Maps | ✅ Implemented | Standard map pool (12 maps), favorites system (localStorage), **two-mode workshop map management** — Mode A (default) lists installed maps via RCON `ds_workshop_listmaps` with workshop IDs cached in the DB for maps loaded through the panel; Mode B (opt-in) scans a mounted filesystem volume for exact ID↔name mapping incl. multiple versions and runtime-probed uninstall support — **workshop map thumbnails via the Steam Web API** (cached on disk, served from `GET /api/maps/thumbnail/{id}`; enable with `STEAM_API_KEY`), map cycle editor, RCON integration (changelevel, host_workshop_map) |
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

## Map thumbnails (Steam Web API)

The **Maps** page can show real Steam Workshop preview images for installed
workshop maps instead of placeholder gradients. To enable it:

1. Grab a Steam Web API key at <https://steamcommunity.com/dev/apikey>.
2. Set `STEAM_API_KEY` in your `.env` (see [`.env.example`](./.env.example)).
3. Restart the panel.

On startup the panel logs `steam workshop thumbnails enabled`. When the Maps
page loads workshop maps, the backend resolves each map's `preview_url` via the
Steam Web API, downloads the image **once**, and caches it under
`THUMBNAIL_CACHE_DIR` (default `thumbnails/`). Images are then served from
`GET /api/maps/thumbnail/{workshopId}`. If the key is unset — or Steam has no
preview for a given item — the UI falls back to the gradient placeholder.

> `STEAM_API_KEY` is **not** the same as `STEAM_GSLT`. GSLT makes the CS2 game
> server public; the API key is used only by the panel to fetch workshop
> metadata and thumbnails.

## Workshop maps (two modes)

The **Maps** page lists installed workshop maps using one of two modes,
selected **per server at runtime**. You don't declare the mode anywhere — the
panel picks Mode B automatically when a workshop volume is mounted for that
server, otherwise it falls back to Mode A.

### Mode A — RCON + DB cache (default, works everywhere)

Installed maps are listed by running `ds_workshop_listmaps` over RCON (falling
back to `maps *` on servers without that command). This returns bare map
**names** only (e.g. `de_drachenschanze`), with no workshop IDs.

- Workshop IDs are known **only** for maps that were downloaded through the
  panel. When you load a map with `host_workshop_map <id>`, the panel records
  the `id → name` mapping in its SQLite DB (`workshop_maps` table) after
  confirming the loaded map name from `status`.
- Maps with a known ID get a **thumbnail** and an **`instant`** badge (they can
  be switched to instantly by ID).
- Maps with no cached ID show a clean card with a **`no id`** tag and no
  thumbnail; they're switched by name via `ds_workshop_changelevel <name>`.
- Because only names are returned, Mode A **cannot disambiguate** multiple
  installed versions that share the same map name.

Mode A needs **no extra configuration** — it's the default for every server.

### Mode B — Filesystem scan (opt-in, exact IDs + uninstall)

If you mount CS2's workshop content directory into the panel container, the
panel scans it directly and gets an **exact ID ↔ name mapping** — including
multiple versions of the same map name — without relying on the DB cache.

Mount convention (namespaced by server ID):

```yaml
# docker-compose.yml — under the defuse service `volumes:`
# Mount your server's steamapps/workshop/content/730 at /workshop/<serverId>.
# <serverId> is the slug shown in the panel URL (e.g. "katzenstube").
- /path/to/cs2/steamapps/workshop/content/730:/workshop/katzenstube:ro
```

- The base directory defaults to `/workshop` and is overridable with the
  `WORKSHOP_BASE` env var. A single server can also be pointed at an arbitrary
  path with `WORKSHOP_PATH_<UPPER_ID>` (server ID uppercased, hyphens →
  underscores — same convention as `RCON_PASSWORD_<UPPER_ID>`).
- Each numeric subfolder (`<workshopId>/` containing a `.vpk`) is one installed item, so the
  panel reports the exact workshop ID for every map and groups multiple
  versions under one card. Cards carry a **`scanned`** source badge.
- **Uninstall** is offered only in Mode B, and only when the mount is
  **writable**. Writability is **auto-detected at runtime** by a write-probe
  (the panel creates and deletes a temp file in the mount) — it is *not* a
  config flag. Mount the volume read-only (`:ro`) to keep uninstall disabled;
  mount it read-write to enable deleting a map's `<id>` folder from the panel.

## Live Logs

The **Live Logs** panel (sidebar → *Live Logs*) streams your CS2 dedicated
server's console output in real time. The backend receives the server's logs
over **UDP** using CS2's built-in `logaddress_add` mechanism, then relays each
line to the browser over **Server-Sent Events** (`GET /api/logs/stream`). No
Docker socket is required.

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

**How it works:**

1. The panel binds a UDP socket on `CS2_LOG_LISTEN_PORT` (default `27500`).
2. On startup it issues `logaddress_add <CS2_LOG_SINK_ADDR>; log on` to each
   server over RCON (retrying until the server is up), so the CS2 server begins
   streaming its console to the panel. The panel parses the Source log packets
   and fans each line out to connected browsers.

**Configuration:**

- `CS2_LOG_LISTEN_PORT` — UDP port the panel binds to receive logs. Default
  `27500`. Publish this port (`udp`) on the panel container.
- `CS2_LOG_SINK_ADDR` — address the CS2 server should send its logs to, **as
  reachable from the CS2 container** (e.g. the panel's service name on the
  shared Docker network, `defuse:27500`). Leave empty to configure
  `logaddress_add` yourself in the CS2 server instead.

  ```yaml
  ports:
    - "27500:27500/udp"
  environment:
    CS2_LOG_LISTEN_PORT: ${CS2_LOG_LISTEN_PORT:-27500}
    CS2_LOG_SINK_ADDR: ${CS2_LOG_SINK_ADDR:-defuse:27500}
  ```

> ℹ️ No Docker socket mount and no `docker-cli` are needed — logs travel over
> UDP directly from the game server, which is both simpler and more secure.
> Make sure the CS2 server has logging enabled (`CS2_LOG=on` for the
> `joedwards32/cs2` image) so `logaddress_add` has something to forward.

### HTTP log listener (`logaddress_add_http`)

Some CS2 servers/networks can't (or won't) deliver logs over UDP and instead use
the engine's **HTTP** log delivery, `logaddress_add_http <url>`, which POSTs each
log line as a plain-text HTTP body. The panel supports this **alongside** the UDP
listener — both run simultaneously and feed the **same** live log pipeline, so
HTTP-sourced lines show up in the Live Logs panel and drive the same workshop-map
download verification (e.g. the messages emitted after `host_workshop_map <id>`).

**Endpoint:** `POST /api/logs/http` on the panel (default port `8080`). It is
public/unauthenticated by design because a game server cannot present a panel
session cookie.

**Point your CS2 server at it** from the RCON console (or the server's
`autoexec`/config). Use the panel's address **as reachable from the CS2 server**:

```
logaddress_add_http "http://<panel-host>:8080/api/logs/http"
log on
```

For example, on the bundled Docker network where the panel's service name is
`defuse`:

```
logaddress_add_http "http://defuse:8080/api/logs/http"
log on
```

You can register both transports at once — UDP and HTTP will both flow into the
Live Logs view:

```
logaddress_add 10.0.0.5:27500
logaddress_add_http "http://defuse:8080/api/logs/http"
log on
```

**Auto-configuration:** set `CS2_LOG_HTTP_SINK_URL` and the panel will issue
`logaddress_add_http <url>` over RCON on startup (retrying until the server is
up), the same way `CS2_LOG_SINK_ADDR` drives the UDP `logaddress_add`.

**Configuration:**

- `CS2_LOG_HTTP_SINK_URL` — full URL of this endpoint as reachable from the CS2
  container (e.g. `http://defuse:8080/api/logs/http`). When set, the panel
  registers it via RCON automatically. Leave empty to run `logaddress_add_http`
  yourself.
- `CS2_LOG_HTTP_TOKEN` — optional shared secret. When set, the endpoint requires
  a matching token supplied as a `?token=<secret>` query parameter or an
  `X-Log-Token` header, so you can lock the endpoint down if it is exposed.
  Register the URL accordingly, e.g.
  `logaddress_add_http "http://defuse:8080/api/logs/http?token=mysecret"`.

> ℹ️ Because CS2 fires log lines as fire-and-forget POSTs, the endpoint always
> replies `200 OK` with an empty body. UDP and HTTP can be used independently or
> together — whichever your server supports.

## License

MIT
