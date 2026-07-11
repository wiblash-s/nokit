# Workshop Maps Setup Guide

This guide shows how to configure the panel for **Mode B** workshop map management (filesystem scan with exact IDs + uninstall support).

## Quick Summary

- **Mode A (default)**: Lists maps via RCON `ds_workshop_listmaps`. Workshop IDs are cached in the DB when you load maps through the panel. **No mount required.**
- **Mode B (opt-in)**: Scans a mounted CS2 workshop directory for exact ID↔name mapping, multi-version detection, and optional uninstall. **Requires mounting a volume.**

Mode B activates automatically **per-server** when the panel finds a readable directory at `/workshop/<serverId>`.

---

## Scenario 1: CS2 Server in Docker (same compose stack)

If your CS2 server runs in the same `docker-compose.yml` stack, mount the CS2 container's volume path:

```yaml
services:
  defuse:
    # ... other config ...
    volumes:
      - ./config.yaml:/etc/defuse/config.yaml:ro
      - nokit_data:/data
      # Mount the cs2 volume's workshop content at /workshop/<serverId>
      - cs2_data/steamapps/workshop/content/730:/workshop/katzenstube:ro

  cs2:
    # ... other config ...
    volumes:
      - cs2_data:/home/steam/cs2-dedicated/

volumes:
  nokit_data:
  cs2_data:
```

**serverId**: Use the slug from the panel URL (e.g., `http://localhost:8080/servers/katzenstube/maps` → `katzenstube`).

**Read-only vs read-write**:
- `:ro` — Maps are read-only; uninstall button hidden.
- `:rw` — The panel can delete map folders; uninstall button shown (auto-detected via write-probe).

---

## Scenario 2: CS2 Server on Host (outside Docker)

If CS2 runs directly on the host at `/var/lib/cs2-server`, mount the absolute path:

```yaml
services:
  defuse:
    # ... other config ...
    volumes:
      - ./config.yaml:/etc/defuse/config.yaml:ro
      - nokit_data:/data
      # Mount host path
      - /var/lib/cs2-server/steamapps/workshop/content/730:/workshop/myserver:ro
```

---

## Scenario 3: Multiple Servers

Add one mount per server, each namespaced by its server ID:

```yaml
services:
  defuse:
    # ... other config ...
    volumes:
      - ./config.yaml:/etc/defuse/config.yaml:ro
      - nokit_data:/data
      # Server 1: katzenstube (read-only)
      - /path/to/server1/steamapps/workshop/content/730:/workshop/katzenstube:ro
      # Server 2: testserver (read-write, uninstall enabled)
      - /path/to/server2/steamapps/workshop/content/730:/workshop/testserver:rw
      # Server 3: competitive (read-only)
      - /path/to/server3/steamapps/workshop/content/730:/workshop/competitive:ro
```

Each server independently uses Mode A (RCON list) or Mode B (filesystem scan) based on whether its mount exists.

---

## Scenario 4: Custom Workshop Base Directory

By default, the panel looks for mounts under `/workshop/<serverId>`. To change the base:

```yaml
services:
  defuse:
    environment:
      # Change the base from /workshop to /cs2-workshop
      WORKSHOP_BASE: /cs2-workshop
    volumes:
      - ./config.yaml:/etc/defuse/config.yaml:ro
      - nokit_data:/data
      # Now mount at /cs2-workshop/<serverId>
      - /path/to/cs2/steamapps/workshop/content/730:/cs2-workshop/katzenstube:ro
```

---

## Scenario 5: Per-Server Override Path

For a single server with a non-standard location, use `WORKSHOP_PATH_<UPPER_ID>`:

```yaml
services:
  defuse:
    environment:
      # Override just for "katzenstube" (ID uppercased, hyphens→underscores)
      WORKSHOP_PATH_KATZENSTUBE: /custom/workshop/path
    volumes:
      - ./config.yaml:/etc/defuse/config.yaml:ro
      - nokit_data:/data
      # Mount at the custom path declared above
      - /path/to/cs2/steamapps/workshop/content/730:/custom/workshop/path:ro
```

This follows the same convention as `RCON_PASSWORD_<UPPER_ID>`.

---

## Verifying the Mount

After starting the panel:

1. Check the logs: `docker logs defuse | grep workshop`  
   You should see: `workshop base dir: /workshop`

2. Open the **Maps** page for your server: `http://localhost:8080/servers/katzenstube/maps`

3. Look at the **Workshop** section header:
   - **Mode A** badge: Panel is using RCON listing (mount not found or unreadable).
   - **Mode B** badge: Panel is scanning the mounted directory.

4. In Mode B:
   - Maps show exact workshop IDs with `scanned` badge.
   - Multi-version maps display a "X versions" chip.
   - Uninstall button appears only if the mount is writable.

---

## Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| Mode A shown, but I mounted a volume | Path mismatch or serverId doesn't match | Verify mount path is `/workshop/<serverId>` where `<serverId>` matches the panel URL slug exactly (case-sensitive) |
| Mode B shown, but no maps listed | Workshop folder is empty or has wrong structure | Check that the mounted directory contains numeric `<workshopId>/` subfolders with `.vpk` files |
| Uninstall button missing in Mode B | Mount is read-only | Change `:ro` to `:rw` in your `docker-compose.yml` and restart |
| "no id" tag on all maps in Mode A | Server returns only names, no IDs cached yet | Load a map via the panel's load input (top of Workshop section) to cache its ID |

---

## Example Files

- **`docker-compose.workshop-example.yml`** — Complete working example with Mode B mount for "katzenstube"
- **`docker-compose.yml`** — Production file with commented mount examples
- **`README.md`** — Feature overview and quick start

See the main [README.md](./README.md) for more details on Mode A vs Mode B behavior.
