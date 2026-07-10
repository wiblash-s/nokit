import { Sparkline } from "./sparkline"

type Props = {
  label: string
  value: string
  sub?: string
  history?: number[]
  color?: string
  area?: boolean
  /** fixed max for the sparkline y-axis */
  max?: number
}

export function StatsCard({
  label,
  value,
  sub,
  history,
  color = "var(--color-primary)",
  area = false,
  max,
}: Props) {
  return (
    <div className="flex flex-col gap-2 rounded-lg border border-border bg-card p-4">
      <div className="flex items-start justify-between">
        <span className="text-[11px] font-medium tracking-wider text-muted-foreground uppercase">
          {label}
        </span>
      </div>
      <div className="flex items-end justify-between gap-2">
        <div className="min-w-0">
          <div className="truncate font-mono text-2xl leading-none font-semibold">
            {value}
          </div>
          {sub && (
            <div className="mt-1.5 truncate text-xs text-muted-foreground">
              {sub}
            </div>
          )}
        </div>
        {history && history.length > 1 && (
          <Sparkline
            data={history}
            color={color}
            area={area}
            max={max}
            className="h-8 w-24 flex-shrink-0"
          />
        )}
      </div>
    </div>
  )
}
