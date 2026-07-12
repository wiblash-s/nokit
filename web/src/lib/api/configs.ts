// API client for the config management feature. It talks to the backend config
// endpoints registered under /api/servers/{id}/configs, which operate in one of
// two modes per server:
//   - "mounted": the .cfg files live on a mounted volume; exec runs `exec <name>`.
//   - "panel":   the .cfg files live in the panel DB; exec replays each line via RCON.

export interface ConfigInfo {
  name: string
  mode: string
}

export interface ConfigListResponse {
  mode: string
  writable: boolean
  configs: ConfigInfo[]
}

export interface ConfigDetail {
  name: string
  content: string
  mode: string
  writable: boolean
}

export interface ExecResult {
  mode: string
  commands_sent: number
  errors: string[]
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

export async function listConfigs(serverId: string): Promise<ConfigListResponse> {
  const res = await fetch(`/api/servers/${encodeURIComponent(serverId)}/configs`)
  if (!res.ok) throw new Error(await parseError(res))
  return (await res.json()) as ConfigListResponse
}

export async function getConfig(serverId: string, name: string): Promise<ConfigDetail> {
  const res = await fetch(
    `/api/servers/${encodeURIComponent(serverId)}/configs/${encodeURIComponent(name)}`
  )
  if (!res.ok) throw new Error(await parseError(res))
  return (await res.json()) as ConfigDetail
}

export async function saveConfig(serverId: string, name: string, content: string): Promise<void> {
  const res = await fetch(
    `/api/servers/${encodeURIComponent(serverId)}/configs/${encodeURIComponent(name)}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ content }),
    }
  )
  if (!res.ok) throw new Error(await parseError(res))
}

export async function deleteConfig(serverId: string, name: string): Promise<void> {
  const res = await fetch(
    `/api/servers/${encodeURIComponent(serverId)}/configs/${encodeURIComponent(name)}`,
    { method: "DELETE" }
  )
  if (!res.ok) throw new Error(await parseError(res))
}

export async function execConfig(serverId: string, name: string): Promise<ExecResult> {
  const res = await fetch(
    `/api/servers/${encodeURIComponent(serverId)}/configs/${encodeURIComponent(name)}/exec`,
    { method: "POST" }
  )
  if (!res.ok) throw new Error(await parseError(res))
  return (await res.json()) as ExecResult
}
