import { useCallback, useEffect, useRef, useState } from "react"
import { RefreshCw, Users, Ban as BanIcon, ShieldOff, Loader2 } from "lucide-react"

import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Toaster } from "@/components/ui/toaster"
import { toast } from "@/hooks/use-toast"
import { Can } from "@/components/can"
import {
  fetchBans,
  fetchPlayers,
  unbanPlayer,
  type Ban,
  type Player,
} from "@/lib/api/players"

const LIVE_REFRESH_MS = 10_000

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Convert a 2-letter ISO country code to its flag emoji (regional indicators). */
function flagEmoji(code: string): string {
  if (!code || code.length !== 2) return "🏳️"
  const base = 0x1f1e6
  const chars = code
    .toUpperCase()
    .split("")
    .map((c) => base + (c.charCodeAt(0) - 65))
  if (chars.some((n) => n < base || n > base + 25)) return "🏳️"
  return String.fromCodePoint(...chars)
}

const TEAM_STYLES: Record<string, string> = {
  CT: "border-blue-500/30 bg-blue-500/10 text-blue-600 dark:text-blue-400",
  T: "border-orange-500/30 bg-orange-500/10 text-orange-600 dark:text-orange-400",
  Spectator: "border-border bg-muted text-muted-foreground",
  Unassigned: "border-border bg-muted text-muted-foreground",
}

const TEAM_LABEL: Record<string, string> = {
  CT: "CT",
  T: "T",
  Spectator: "SPEC",
  Unassigned: "—",
}

function TeamBadge({ team }: { team: string }) {
  const style = TEAM_STYLES[team] ?? TEAM_STYLES.Unassigned
  const label = TEAM_LABEL[team] ?? team
  return (
    <span
      className={cn(
        "inline-flex w-fit items-center rounded-md border px-2 py-0.5 text-xs font-medium",
        style
      )}
    >
      {label}
    </span>
  )
}

function pingColor(ping: number): string {
  if (ping <= 0) return "text-muted-foreground"
  if (ping < 60) return "text-green-600 dark:text-green-400"
  if (ping < 120) return "text-amber-600 dark:text-amber-400"
  return "text-destructive"
}

function formatExpiry(expires: string): string {
  if (!expires || expires === "permanent") return "Permanent"
  const n = Number(expires)
  // A value that looks like a unix timestamp (seconds) → render as a date.
  if (Number.isFinite(n) && n > 1_000_000_000) {
    return new Date(n * 1000).toLocaleString()
  }
  // Otherwise it is a raw minutes value passed through from the cfg/listid.
  return `${expires} min`
}

function timeAgo(date: Date | null): string {
  if (!date) return "never"
  const secs = Math.round((Date.now() - date.getTime()) / 1000)
  if (secs < 5) return "just now"
  if (secs < 60) return `${secs}s ago`
  const mins = Math.floor(secs / 60)
  return `${mins}m ago`
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export function PlayersPage({ serverId }: { serverId: string }) {
  const [tab, setTab] = useState<"live" | "bans">("live")

  return (
    <div className="flex flex-1 flex-col gap-6 p-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Players</h1>
          <p className="text-sm text-muted-foreground">
            Live connections and ban management
          </p>
        </div>
      </div>

      <Tabs value={tab} onValueChange={(v) => setTab(v as "live" | "bans")}>
        <TabsList>
          <TabsTrigger value="live">
            <Users className="size-4" />
            Live
          </TabsTrigger>
          <TabsTrigger value="bans">
            <BanIcon className="size-4" />
            Bans
          </TabsTrigger>
        </TabsList>

        <TabsContent value="live">
          <LiveTab serverId={serverId} active={tab === "live"} />
        </TabsContent>
        <TabsContent value="bans">
          <BansTab serverId={serverId} active={tab === "bans"} />
        </TabsContent>
      </Tabs>

      <Toaster />
    </div>
  )
}

// ---------------------------------------------------------------------------
// Live tab
// ---------------------------------------------------------------------------

function LiveTab({ serverId, active }: { serverId: string; active: boolean }) {
  const [players, setPlayers] = useState<Player[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null)
  const [, forceTick] = useState(0)

  const load = useCallback(
    async (opts: { silent?: boolean } = {}) => {
      if (!opts.silent) setLoading(true)
      try {
        const data = await fetchPlayers(serverId)
        setPlayers(data)
        setError(null)
        setLastUpdated(new Date())
      } catch (err) {
        setError(err instanceof Error ? err.message : "failed to load players")
      } finally {
        setLoading(false)
      }
    },
    [serverId]
  )

  // Initial load + auto-refresh every 10s while the tab is active.
  useEffect(() => {
    if (!active) return
    load()
    const iv = setInterval(() => load({ silent: true }), LIVE_REFRESH_MS)
    return () => clearInterval(iv)
  }, [active, load])

  // Keep the "last updated" label fresh.
  useEffect(() => {
    const iv = setInterval(() => forceTick((n) => n + 1), 1000)
    return () => clearInterval(iv)
  }, [])

  return (
    <div className="rounded-xl border border-border bg-card">
      <div className="flex items-center justify-between border-b border-border px-4 py-2.5">
        <div className="flex items-center gap-2 text-sm">
          <span className="font-medium">
            {players.length} {players.length === 1 ? "player" : "players"} online
          </span>
          <span className="text-xs text-muted-foreground">
            · updated {timeAgo(lastUpdated)}
          </span>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => load()}
          disabled={loading}
        >
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
          Refresh
        </Button>
      </div>

      {error ? (
        <div className="p-6 text-sm text-destructive">{error}</div>
      ) : loading && players.length === 0 ? (
        <div className="flex items-center justify-center gap-2 p-10 text-sm text-muted-foreground">
          <Loader2 className="size-4 animate-spin" /> Loading players…
        </div>
      ) : players.length === 0 ? (
        <div className="p-10 text-center text-sm text-muted-foreground">
          No players connected.
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>SteamID</TableHead>
              <TableHead>Team</TableHead>
              <TableHead className="text-right">Ping</TableHead>
              <TableHead>Time</TableHead>
              <TableHead>Country</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {players.map((p) => (
              <TableRow key={`${p.userid}-${p.steamid}`}>
                <TableCell className="font-medium">{p.name}</TableCell>
                <TableCell className="font-mono text-xs text-muted-foreground">
                  {p.steamid}
                </TableCell>
                <TableCell>
                  <TeamBadge team={p.team} />
                </TableCell>
                <TableCell className={cn("text-right tabular-nums", pingColor(p.ping))}>
                  {p.ping > 0 ? `${p.ping} ms` : "—"}
                </TableCell>
                <TableCell className="tabular-nums text-muted-foreground">
                  {p.time || "—"}
                </TableCell>
                <TableCell>
                  {p.country_code ? (
                    <span className="flex items-center gap-1.5">
                      <span className="text-base leading-none">
                        {flagEmoji(p.country_code)}
                      </span>
                      <span className="text-sm">{p.country_name || p.country_code}</span>
                    </span>
                  ) : (
                    <span className="text-muted-foreground">—</span>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Bans tab
// ---------------------------------------------------------------------------

function BansTab({ serverId, active }: { serverId: string; active: boolean }) {
  const [bans, setBans] = useState<Ban[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [target, setTarget] = useState<Ban | null>(null)
  const [unbanning, setUnbanning] = useState(false)
  const loadedRef = useRef(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const data = await fetchBans(serverId)
      setBans(data)
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : "failed to load bans")
    } finally {
      setLoading(false)
    }
  }, [serverId])

  useEffect(() => {
    if (active && !loadedRef.current) {
      loadedRef.current = true
      load()
    }
  }, [active, load])

  const confirmUnban = async () => {
    if (!target) return
    setUnbanning(true)
    try {
      const result = await unbanPlayer(serverId, target.steamid)
      if (result.rcon_error || result.cfg_error) {
        toast("Unban partially applied", {
          variant: "error",
          description: [result.rcon_error, result.cfg_error]
            .filter(Boolean)
            .join(" · "),
        })
      } else {
        toast("Player unbanned", {
          variant: "success",
          description: target.steamid,
        })
      }
      setTarget(null)
      await load()
    } catch (err) {
      toast("Unban failed", {
        variant: "error",
        description: err instanceof Error ? err.message : "unknown error",
      })
    } finally {
      setUnbanning(false)
    }
  }

  return (
    <div className="rounded-xl border border-border bg-card">
      <div className="flex items-center justify-between border-b border-border px-4 py-2.5">
        <div className="text-sm font-medium">
          {bans.length} {bans.length === 1 ? "ban" : "bans"}
        </div>
        <Button variant="outline" size="sm" onClick={load} disabled={loading}>
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
          Refresh
        </Button>
      </div>

      {error ? (
        <div className="p-6 text-sm text-destructive">{error}</div>
      ) : loading && bans.length === 0 ? (
        <div className="flex items-center justify-center gap-2 p-10 text-sm text-muted-foreground">
          <Loader2 className="size-4 animate-spin" /> Loading bans…
        </div>
      ) : bans.length === 0 ? (
        <div className="p-10 text-center text-sm text-muted-foreground">
          No active bans.
        </div>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>SteamID</TableHead>
              <TableHead>Expires</TableHead>
              <TableHead>Source</TableHead>
              <TableHead className="text-right">Action</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {bans.map((b) => (
              <TableRow key={b.steamid}>
                <TableCell className="font-medium">
                  {b.name || <span className="text-muted-foreground">unknown</span>}
                </TableCell>
                <TableCell className="font-mono text-xs text-muted-foreground">
                  {b.steamid}
                </TableCell>
                <TableCell>
                  {b.expires_at === "permanent" ? (
                    <Badge variant="destructive">Permanent</Badge>
                  ) : (
                    <span className="text-sm text-muted-foreground">
                      {formatExpiry(b.expires_at)}
                    </span>
                  )}
                </TableCell>
                <TableCell>
                  <Badge variant="outline" className="uppercase">
                    {b.source}
                  </Badge>
                </TableCell>
                <TableCell className="text-right">
                  <Can perm="unban_player">
                    <Button
                      variant="destructive"
                      size="sm"
                      onClick={() => setTarget(b)}
                    >
                      <ShieldOff className="size-3.5" />
                      Unban
                    </Button>
                  </Can>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <AlertDialog
        open={target !== null}
        onOpenChange={(open) => {
          if (!open && !unbanning) setTarget(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Unban this player?</AlertDialogTitle>
            <AlertDialogDescription>
              This removes{" "}
              <span className="font-mono text-foreground">{target?.steamid}</span>{" "}
              from the server's session bans (<code>removeid</code>) and from{" "}
              <code>banned_users.cfg</code>. The player will be able to reconnect
              immediately.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={unbanning}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={(e) => {
                e.preventDefault()
                confirmUnban()
              }}
              disabled={unbanning}
              className="bg-destructive/10 text-destructive hover:bg-destructive/20 dark:bg-destructive/20 dark:hover:bg-destructive/30"
            >
              {unbanning ? (
                <>
                  <Loader2 className="size-3.5 animate-spin" /> Unbanning…
                </>
              ) : (
                <>
                  <ShieldOff className="size-3.5" /> Unban
                </>
              )}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
