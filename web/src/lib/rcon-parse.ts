// Parsers for the raw text returned by CS2 RCON `status` and `stats`
// commands. CS2 has shifted output formats a few times across builds, so
// these parsers are written to be tolerant: every field is optional and we
// degrade gracefully to `null`/`undefined` when a value cannot be found.

export type StatusInfo = {
  hostname?: string
  version?: string
  map?: string
  os?: string
  address?: string
  players?: number
  bots?: number
  maxPlayers?: number
}

export type StatsInfo = {
  cpu?: number // percent
  netIn?: number // kb/s
  netOut?: number // kb/s
  uptimeMinutes?: number
  fps?: number
  players?: number
  tick?: number // ~tick, effectively the current tickrate
}

/** Parse the output of the `status` command. */
export function parseStatus(raw: string): StatusInfo {
  const info: StatusInfo = {}
  if (!raw) return info

  for (const line of raw.split(/\r?\n/)) {
    const trimmed = line.trim()

    const kv = trimmed.match(/^([a-z/]+)\s*:\s*(.+)$/i)
    if (!kv) continue
    const key = kv[1].toLowerCase()
    const value = kv[2].trim()

    switch (key) {
      case "hostname":
        info.hostname = value
        break
      case "version":
        info.version = value.split(/[/\s]/)[0]
        break
      case "map": {
        // e.g. "de_mirage" or "de_mirage @ ... " — keep first token
        info.map = value.split(/[\s@]/)[0]
        break
      }
      case "os":
        info.os = value
        break
      case "udp/ip":
      case "ip":
        info.address = value.split(/\s/)[0]
        break
      case "players": {
        // Handles both:
        //   "9 humans, 0 bots (12/0 max)"
        //   "2 humans, 0 bots (10 max)"
        //   "9 (12 max)"
        const humans = value.match(/(\d+)\s+humans?/i)
        const bots = value.match(/(\d+)\s+bots?/i)
        const max = value.match(/\((\d+)(?:\/\d+)?\s*max\)/i)
        if (humans) info.players = Number(humans[1])
        if (bots) info.bots = Number(bots[1])
        if (max) info.maxPlayers = Number(max[1])
        if (info.players === undefined) {
          const lead = value.match(/^(\d+)/)
          if (lead) info.players = Number(lead[1])
        }
        break
      }
    }
  }

  return info
}

// Column keys, in the order CS2 prints them in the `stats` table.
const STATS_COLUMNS = [
  "cpu",
  "netin",
  "netout",
  "uptime",
  "maps",
  "fps",
  "players",
  "svms",
  "ms",
  "tick",
] as const

/** Parse the output of the `stats` command (a two-row header + values table). */
export function parseStats(raw: string): StatsInfo {
  const info: StatsInfo = {}
  if (!raw) return info

  const lines = raw
    .split(/\r?\n/)
    .map((l) => l.trim())
    .filter(Boolean)

  // Find the header row (contains "CPU") then use the following row of
  // numbers. Fall back to the first all-numeric row if no header is found.
  const headerIdx = lines.findIndex(
    (l) => /\bcpu\b/i.test(l) && /\bfps\b/i.test(l)
  )
  let valueLine: string | undefined

  if (headerIdx !== -1 && lines[headerIdx + 1]) {
    valueLine = lines[headerIdx + 1]
  } else {
    valueLine = lines.find((l) => /^[\d.\s+-]+$/.test(l) && /\d/.test(l))
  }

  if (!valueLine) return info

  const nums = valueLine.match(/-?\d+(?:\.\d+)?/g)
  if (!nums) return info

  nums.forEach((n, i) => {
    const col = STATS_COLUMNS[i]
    if (!col) return
    const val = Number(n)
    switch (col) {
      case "cpu":
        info.cpu = val
        break
      case "netin":
        info.netIn = val
        break
      case "netout":
        info.netOut = val
        break
      case "uptime":
        info.uptimeMinutes = val
        break
      case "fps":
        info.fps = val
        break
      case "players":
        info.players = val
        break
      case "tick":
        info.tick = val
        break
    }
  })

  return info
}

/** Format an uptime given in minutes as e.g. "11h 02m" or "47m". */
export function formatUptime(minutes?: number): string {
  if (minutes === undefined || Number.isNaN(minutes)) return "—"
  const total = Math.floor(minutes)
  const days = Math.floor(total / 1440)
  const hours = Math.floor((total % 1440) / 60)
  const mins = total % 60
  if (days > 0) return `${days}d ${String(hours).padStart(2, "0")}h`
  if (hours > 0) return `${hours}h ${String(mins).padStart(2, "0")}m`
  return `${mins}m`
}
