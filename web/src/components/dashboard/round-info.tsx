import type { ServerStats } from "@/hooks/useServerStats"

type Props = {
  data: ServerStats | null
  online: boolean
}

function Row({
  label,
  value,
  muted,
}: {
  label: string
  value: React.ReactNode
  muted?: boolean
}) {
  return (
    <div className="flex items-center justify-between gap-4 py-2">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span
        className={`truncate font-mono text-sm ${muted ? "text-muted-foreground" : ""}`}
      >
        {value}
      </span>
    </div>
  )
}

/**
 * Current-round overview. `map` and `players` come from `status`; live round
 * number / phase / score are not exposed by the `status`/`stats` RCON
 * commands and are surfaced by the Live Logs stream (planned), so they show
 * as "—" here with a footnote.
 */
export function RoundInfo({ data, online }: Props) {
  const phase = online ? "live" : "offline"

  return (
    <div className="rounded-lg border border-border bg-card p-4">
      <div className="mb-2 flex items-center justify-between">
        <h2 className="text-sm font-semibold">Current round</h2>
        <span className="font-mono text-xs text-muted-foreground">
          {data?.map ?? "—"}
        </span>
      </div>

      <div className="divide-y divide-border">
        <Row label="map" value={data?.map ?? "—"} />
        <Row
          label="phase"
          value={
            <span
              className={
                online ? "text-green-500" : "text-muted-foreground"
              }
            >
              {phase}
            </span>
          }
        />
        <Row label="round" value="—" muted />
        <Row
          label="score"
          value={
            <>
              <span className="text-blue-400">CT —</span>
              <span className="text-muted-foreground"> : </span>
              <span className="text-amber-500">— T</span>
            </>
          }
        />
        <Row label="round time" value="—" muted />
        <Row
          label="players"
          value={
            data?.players !== undefined
              ? `${data.players}${data.maxPlayers ? ` / ${data.maxPlayers}` : ""}`
              : "—"
          }
        />
      </div>

      <p className="mt-3 text-[11px] leading-snug text-muted-foreground">
        Live round number, phase and score stream from the Live Logs feed
        (coming soon). Map and player count are polled from{" "}
        <code className="font-mono">status</code>.
      </p>
    </div>
  )
}
