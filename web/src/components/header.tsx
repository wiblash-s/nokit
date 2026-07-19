import { useNavigate } from "react-router-dom"
import { Search, Settings, LogOut } from "lucide-react"
import type { ServerInfo } from "@/hooks/useServers"
import { ServerSwitcher } from "./server-switcher"
import { useUser } from "@/hooks/auth-context"

type Props = {
  servers: ServerInfo[]
  currentId: string
  onRefresh: () => void
  tickrate?: number
  players?: number
  maxPlayers?: number
  map?: string
}

export function Header({
  servers,
  currentId,
  onRefresh,
  tickrate,
  players,
  maxPlayers,
  map,
}: Props) {
  const navigate = useNavigate()
  const user = useUser()

  const handleLogout = async () => {
    try {
      const r = await fetch("/api/logout", { method: "POST" })
      const body = await r.json().catch(() => ({}))
      // In OIDC mode the backend may return a provider end-session URL so we can
      // complete single sign-out. Otherwise fall back to the local login page.
      if (body?.redirect) {
        window.location.assign(body.redirect)
        return
      }
    } catch {
      // ignore — still send the user to the login page below
    }
    navigate("/login", { replace: true })
  }

  const initial = (user?.username || "?").charAt(0).toUpperCase()
  const topRole = user?.roles?.[user.roles.length - 1]

  return (
    <header className="sticky top-0 z-20 border-b border-border bg-background">
      <div className="flex h-14 items-center gap-4 px-6">
        <ServerSwitcher
          servers={servers}
          currentId={currentId}
          onRefresh={onRefresh}
        />

        {/* Indicators */}
        {tickrate !== undefined && (
          <>
            <div className="h-4 w-px bg-border" />
            <div className="flex items-center gap-3 text-xs">
              <span className="text-muted-foreground">
                {tickrate.toFixed(1)}
              </span>
              {players !== undefined && maxPlayers !== undefined && (
                <span className="text-muted-foreground">
                  {players}/{maxPlayers}
                </span>
              )}
              {map && (
                <span className="font-mono text-muted-foreground">{map}</span>
              )}
            </div>
          </>
        )}

        {/* Right side */}
        <div className="ml-auto flex items-center gap-2">
          {/* Search placeholder */}
          <div className="relative">
            <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
            <input
              type="text"
              placeholder="cvars, players..."
              className="h-8 w-64 rounded-md border border-input bg-background pl-8 pr-3 text-xs placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
              disabled
            />
          </div>

          {user && (
            <div className="flex items-center gap-2 pl-1 pr-1">
              <div
                className="flex h-7 w-7 items-center justify-center rounded-full bg-primary text-xs font-semibold text-primary-foreground"
                title={user.email || user.username}
              >
                {initial}
              </div>
              <div className="hidden flex-col leading-tight sm:flex">
                <span className="text-xs font-medium">{user.username}</span>
                {topRole && (
                  <span className="text-[10px] uppercase tracking-wide text-muted-foreground">
                    {topRole.replace("cs2-rcon-", "")}
                  </span>
                )}
              </div>
            </div>
          )}

          <button
            className="flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            title="Settings"
          >
            <Settings className="h-4 w-4" />
          </button>

          <button
            onClick={handleLogout}
            className="flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            title="Sign out"
          >
            <LogOut className="h-4 w-4" />
          </button>
        </div>
      </div>
    </header>
  )
}
