import { useCallback, useEffect, useRef, useState } from "react"
import {
  parseStats,
  parseStatus,
  type StatsInfo,
  type StatusInfo,
} from "@/lib/rcon-parse"

const POLL_INTERVAL_MS = 6000
const HISTORY_LIMIT = 40

export type ServerStats = {
  status: StatusInfo
  stats: StatsInfo
  /** merged, most-reliable player count */
  players?: number
  maxPlayers?: number
  /** effective tickrate (from stats ~tick) */
  tick?: number
  cpu?: number
  fps?: number
  uptimeMinutes?: number
  map?: string
}

export type StatsHistory = {
  cpu: number[]
  players: number[]
  tick: number[]
}

type State = {
  online: boolean
  loading: boolean
  error: string | null
  data: ServerStats | null
  history: StatsHistory
  lastUpdated: number | null
  /** raw text of the most recent `status` poll */
  rawStatus: string
}

async function rcon(serverId: string, command: string): Promise<string> {
  const res = await fetch(`/api/servers/${serverId}/rcon`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ command }),
  })
  const body = await res.json()
  if (!res.ok) throw new Error(body.error || `rcon failed (${res.status})`)
  return body.output ?? ""
}

/**
 * Polls a server's `status` and `stats` over RCON on an interval and keeps a
 * rolling history for sparklines. Also exposes an `exec` helper for one-off
 * commands (used by quick actions) and a manual `refresh`.
 */
export function useServerStats(serverId: string) {
  const [state, setState] = useState<State>({
    online: false,
    loading: true,
    error: null,
    data: null,
    history: { cpu: [], players: [], tick: [] },
    lastUpdated: null,
    rawStatus: "",
  })
  const [tick, setTick] = useState(0)
  const mounted = useRef(true)

  const refresh = useCallback(() => setTick((t) => t + 1), [])

  const exec = useCallback(
    (command: string) => rcon(serverId, command),
    [serverId]
  )

  useEffect(() => {
    mounted.current = true
    let timer: ReturnType<typeof setTimeout>

    const poll = async () => {
      try {
        const [statusRaw, statsRaw] = await Promise.all([
          rcon(serverId, "status"),
          rcon(serverId, "stats"),
        ])
        if (!mounted.current) return

        const status = parseStatus(statusRaw)
        const stats = parseStats(statsRaw)

        const players = status.players ?? stats.players
        const maxPlayers = status.maxPlayers
        const tickVal = stats.tick
        const cpu = stats.cpu

        const data: ServerStats = {
          status,
          stats,
          players,
          maxPlayers,
          tick: tickVal,
          cpu,
          fps: stats.fps,
          uptimeMinutes: stats.uptimeMinutes,
          map: status.map,
        }

        setState((prev) => {
          const push = (arr: number[], v?: number) =>
            v === undefined || Number.isNaN(v)
              ? arr
              : [...arr, v].slice(-HISTORY_LIMIT)
          return {
            online: true,
            loading: false,
            error: null,
            data,
            history: {
              cpu: push(prev.history.cpu, cpu),
              players: push(prev.history.players, players),
              tick: push(prev.history.tick, tickVal),
            },
            lastUpdated: Date.now(),
            rawStatus: statusRaw,
          }
        })
      } catch (err) {
        if (!mounted.current) return
        setState((prev) => ({
          ...prev,
          online: false,
          loading: false,
          error: err instanceof Error ? err.message : "unknown error",
        }))
      } finally {
        if (mounted.current) {
          timer = setTimeout(poll, POLL_INTERVAL_MS)
        }
      }
    }

    poll()

    return () => {
      mounted.current = false
      clearTimeout(timer)
    }
  }, [serverId, tick])

  return { ...state, refresh, exec }
}
