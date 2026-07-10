import { Link, useParams } from "react-router-dom"

type Props = {
  rawStatus: string
  lastUpdated: number | null
}

function ago(ts: number | null): string {
  if (!ts) return "—"
  const secs = Math.max(0, Math.round((Date.now() - ts) / 1000))
  if (secs < 60) return `${secs}s ago`
  return `${Math.round(secs / 60)}m ago`
}

export function RecentOutput({ rawStatus, lastUpdated }: Props) {
  const { id } = useParams<{ id: string }>()
  const lines = rawStatus
    .split(/\r?\n/)
    .map((l) => l.trimEnd())
    .filter((l) => l.length > 0)
    .slice(-10)

  return (
    <div className="rounded-lg border border-border bg-card p-4">
      <div className="mb-2 flex items-center justify-between">
        <h2 className="text-sm font-semibold">Recent output</h2>
        <Link
          to={`/servers/${id}/console`}
          className="text-xs text-muted-foreground transition-colors hover:text-foreground"
        >
          open console →
        </Link>
      </div>

      <div className="max-h-56 overflow-y-auto rounded-md bg-muted/50 p-3 font-mono text-xs leading-relaxed">
        {lines.length === 0 ? (
          <span className="text-muted-foreground">
            waiting for first poll…
          </span>
        ) : (
          <>
            <div className="mb-1 text-muted-foreground">
              {"> "}status
            </div>
            {lines.map((line, i) => (
              <div key={i} className="whitespace-pre-wrap text-foreground/80">
                {line}
              </div>
            ))}
          </>
        )}
      </div>

      <p className="mt-2 text-right font-mono text-[11px] text-muted-foreground">
        updated {ago(lastUpdated)}
      </p>
    </div>
  )
}
