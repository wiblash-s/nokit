import { useServerStats } from "@/hooks/useServerStats"
import { StatsCard } from "@/components/dashboard/stats-card"
import { ServerStatus } from "@/components/dashboard/server-status"
import { QuickActions } from "@/components/dashboard/quick-actions"
import { RoundInfo } from "@/components/dashboard/round-info"
import { RecentOutput } from "@/components/dashboard/recent-output"

const CHART_GREEN = "#22c55e"
const CHART_BLUE = "#3b82f6"
const CHART_ORANGE = "#f97316"

type Props = {
  serverId: string
  serverName: string
  host: string
  onOpenConsole: () => void
}

export function DashboardPage({
  serverId,
  serverName,
  host,
  onOpenConsole,
}: Props) {
  const { online, loading, error, data, history, lastUpdated, rawStatus, exec } =
    useServerStats(serverId)

  const playersValue =
    data?.players !== undefined
      ? `${data.players} / ${data.maxPlayers ?? "?"}`
      : "—"
  const cpuValue = data?.cpu !== undefined ? `${data.cpu.toFixed(1)} %` : "—"
  const tickValue = data?.tick !== undefined ? `${data.tick.toFixed(1)} hz` : "—"

  return (
    <div className="space-y-4">
      {/* page header */}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="text-lg font-semibold">Dashboard</h1>
          <p className="font-mono text-xs text-muted-foreground">
            {serverName} · {host} · connected via rcon
          </p>
        </div>
        <span
          className={`flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs ${
            online
              ? "border-green-500/30 text-green-500"
              : "border-destructive/30 text-destructive"
          }`}
        >
          <span
            className={`h-1.5 w-1.5 rounded-full ${
              online ? "bg-green-500" : "bg-destructive"
            }`}
          />
          {loading ? "connecting…" : online ? "live" : "offline"}
        </span>
      </div>

      {error && !online && (
        <div className="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-xs text-destructive">
          Could not reach the server over RCON: {error}
        </div>
      )}

      {/* stat cards */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatsCard
          label="Players"
          value={playersValue}
          sub={online ? "connected" : "—"}
          history={history.players}
          color={CHART_GREEN}
        />
        <StatsCard
          label="Tick rate"
          value={tickValue}
          sub="target 128"
          history={history.tick}
          color={CHART_ORANGE}
          max={128}
        />
        <StatsCard
          label="CPU"
          value={cpuValue}
          sub={data?.fps !== undefined ? `${data.fps.toFixed(0)} fps` : "srcds"}
          history={history.cpu}
          color={CHART_BLUE}
          area
        />
        <StatsCard
          label="RAM"
          value="n/a"
          sub="not reported via rcon"
        />
      </div>

      {/* main grid: status + round (left ~2/3), quick actions (right ~1/3) */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <div className="space-y-4 lg:col-span-2">
          <ServerStatus
            name={serverName}
            host={host}
            online={online}
            data={data}
          />
          <RoundInfo data={data} online={online} />
        </div>
        <div className="space-y-4">
          <QuickActions exec={exec} />
          <RecentOutput
            rawStatus={rawStatus}
            lastUpdated={lastUpdated}
            onOpenConsole={onOpenConsole}
          />
        </div>
      </div>
    </div>
  )
}
