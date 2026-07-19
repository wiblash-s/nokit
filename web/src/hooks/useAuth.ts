import { useEffect, useState } from "react"

// Permission keys mirror the auth.Permission constants in the Go backend.
export type Permission =
  | "view_dashboard"
  | "view_players"
  | "view_logs"
  | "send_console_command"
  | "kick_player"
  | "ban_player"
  | "unban_player"
  | "manage_workshop"
  | "edit_config"
  | "exec_config"
  | "add_server"
  | "edit_server"
  | "delete_server"
  | "delete_config"
  | "view_audit"

export type Permissions = Record<Permission, boolean>

export type User = {
  username: string
  email: string
  roles: string[]
  groups: string[]
  isLocal: boolean
  permissions: Permissions
}

type AuthState =
  | { status: "loading" }
  | { status: "authenticated"; user: User }
  | { status: "unauthenticated" }

const EMPTY_PERMISSIONS: Permissions = {
  view_dashboard: false,
  view_players: false,
  view_logs: false,
  send_console_command: false,
  kick_player: false,
  ban_player: false,
  unban_player: false,
  manage_workshop: false,
  edit_config: false,
  exec_config: false,
  add_server: false,
  edit_server: false,
  delete_server: false,
  delete_config: false,
  view_audit: false,
}

export function useAuth(): AuthState {
  const [state, setState] = useState<AuthState>({ status: "loading" })

  useEffect(() => {
    fetch("/api/me")
      .then(async (r) => {
        if (!r.ok) {
          setState({ status: "unauthenticated" })
          return
        }
        const body = await r.json()
        setState({
          status: "authenticated",
          user: {
            username: body.username ?? "",
            email: body.email ?? "",
            roles: body.roles ?? [],
            groups: body.groups ?? [],
            isLocal: Boolean(body.isLocal),
            permissions: { ...EMPTY_PERMISSIONS, ...(body.permissions ?? {}) },
          },
        })
      })
      .catch(() => setState({ status: "unauthenticated" }))
  }, [])

  return state
}
