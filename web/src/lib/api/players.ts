// API client for the Player Panel feature. It talks to the backend endpoints
// registered under /api/servers/{id}:
//   - GET    /players            live player list (parsed from `status`, GeoIP-enriched)
//   - GET    /bans               merged ban list (banned_users.cfg + `listid`)
//   - DELETE /bans/{steamid}     unban (removeid via RCON + remove from cfg)

export type Team = "CT" | "T" | "Spectator" | "Unassigned"

export interface Player {
  userid: number
  name: string
  steamid: string
  team: Team | string
  ping: number
  time: string
  country_code: string
  country_name: string
}

export type BanSource = "cfg" | "session" | "both"

export interface Ban {
  name: string
  steamid: string
  /** A unix timestamp as a string, or the literal "permanent". */
  expires_at: string
  source: BanSource
}

export interface UnbanResult {
  ok: boolean
  steamid: string
  cfg_removed: boolean
  rcon_error: string
  cfg_error: string
}

async function parseError(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { error?: string }
    if (body?.error) return body.error
  } catch {
    // ignore non-JSON bodies
  }
  return `request failed (${res.status})`
}

export async function fetchPlayers(serverId: string): Promise<Player[]> {
  const res = await fetch(`/api/servers/${encodeURIComponent(serverId)}/players`)
  if (!res.ok) throw new Error(await parseError(res))
  return (await res.json()) as Player[]
}

export async function fetchBans(serverId: string): Promise<Ban[]> {
  const res = await fetch(`/api/servers/${encodeURIComponent(serverId)}/bans`)
  if (!res.ok) throw new Error(await parseError(res))
  return (await res.json()) as Ban[]
}

export async function unbanPlayer(
  serverId: string,
  steamid: string
): Promise<UnbanResult> {
  const res = await fetch(
    `/api/servers/${encodeURIComponent(serverId)}/bans/${encodeURIComponent(steamid)}`,
    { method: "DELETE" }
  )
  if (!res.ok) throw new Error(await parseError(res))
  return (await res.json()) as UnbanResult
}
