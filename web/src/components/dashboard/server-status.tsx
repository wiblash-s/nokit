import type { ServerStats } from "@/hooks/useServerStats"
import { formatUptime } from "@/lib/rcon-parse"

type Props = {
  name: string
  host: string
  online: boolean
  data: ServerStats | null
}

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 py-2">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className="truncate font-mono text-sm">{value}</span>
    </div>
  )
}

export function ServerStatus({ name, host, online, data }: Props) {
  return (
    <div className="rounded-lg border border-border bg-card p-4">
      <div className="mb-2 flex items-center justify-between">
        <h2 className="text-sm font-semibold">Server status</h2>
        <span
          className={`flex items-center gap-1.5 text-xs ${
            online ? "text-green-500" : "text-destructive"
          }`}
        >
          <span
            className={`h-2 w-2 rounded-full ${
              online ? "bg-green-500" : "bg-destructive"
            }`}
          />
          {online ? "online" : "offline"}
        </span>
      </div>

      <div className="divide-y divide-border">
        <Row label="name" value={name} />
        <Row label="address" value={data?.status.address ?? host} />
        <Row label="map" value={data?.map ?? "—"} />
        <Row
          label="hostname"
          value={data?.status.hostname ?? "—"}
        />
        <Row
          label="players"
          value={
            data?.players !== undefined
              ? `${data.players} / ${data.maxPlayers ?? "?"}`
              : "—"
          }
        />
        <Row label="uptime" value={formatUptime(data?.uptimeMinutes)} />
        <Row label="version" value={data?.status.version ?? "—"} />
        <Row label="os" value={data?.status.os ?? "—"} />
      </div>
    </div>
  )
}
