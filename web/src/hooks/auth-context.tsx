import { createContext, useContext } from "react"
import { useAuth, type Permission, type Permissions, type User } from "./useAuth"

type AuthContextValue =
  | { status: "loading" }
  | { status: "authenticated"; user: User }
  | { status: "unauthenticated" }

const AuthContext = createContext<AuthContextValue>({ status: "loading" })

/**
 * AuthProvider fetches the current session once and shares it with the whole
 * app so components don't each re-hit /api/me.
 */
export function AuthProvider({ children }: { children: React.ReactNode }) {
  const state = useAuth()
  return <AuthContext.Provider value={state}>{children}</AuthContext.Provider>
}

export function useAuthContext(): AuthContextValue {
  return useContext(AuthContext)
}

/** Returns the authenticated user, or null while loading / when signed out. */
export function useUser(): User | null {
  const state = useContext(AuthContext)
  return state.status === "authenticated" ? state.user : null
}

const NONE: Permissions = {
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

/**
 * usePermissions returns a helper to check the current user's permissions.
 * When signed out / loading, every check returns false.
 */
export function usePermissions() {
  const user = useUser()
  const perms = user?.permissions ?? NONE
  return {
    permissions: perms,
    can: (p: Permission) => Boolean(perms[p]),
  }
}
