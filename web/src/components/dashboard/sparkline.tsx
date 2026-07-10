type Props = {
  data: number[]
  /** stroke/fill color (any CSS color) */
  color?: string
  /** render as a filled area chart rather than a plain line */
  area?: boolean
  className?: string
  /** optional fixed max for the y-axis; otherwise derived from data */
  max?: number
}

/**
 * A tiny dependency-free SVG sparkline. Scales to its container width via a
 * viewBox and preserveAspectRatio="none".
 */
export function Sparkline({
  data,
  color = "var(--color-primary)",
  area = false,
  className,
  max,
}: Props) {
  const width = 100
  const height = 32

  if (data.length === 0) {
    return (
      <svg
        viewBox={`0 0 ${width} ${height}`}
        preserveAspectRatio="none"
        className={className}
      />
    )
  }

  const lo = Math.min(...data)
  const hi = max ?? Math.max(...data)
  const range = hi - lo || 1
  const stepX = data.length > 1 ? width / (data.length - 1) : width

  const points = data.map((v, i) => {
    const x = i * stepX
    // padding of 2px top/bottom so peaks aren't clipped
    const y = height - 2 - ((v - lo) / range) * (height - 4)
    return [x, y] as const
  })

  const line = points.map(([x, y]) => `${x.toFixed(2)},${y.toFixed(2)}`).join(" ")
  const areaPath =
    `M0,${height} ` +
    points.map(([x, y]) => `L${x.toFixed(2)},${y.toFixed(2)}`).join(" ") +
    ` L${width},${height} Z`

  return (
    <svg
      viewBox={`0 0 ${width} ${height}`}
      preserveAspectRatio="none"
      className={className}
      style={{ overflow: "visible" }}
    >
      {area && <path d={areaPath} fill={color} opacity={0.15} />}
      <polyline
        points={line}
        fill="none"
        stroke={color}
        strokeWidth={1.5}
        strokeLinejoin="round"
        strokeLinecap="round"
        vectorEffect="non-scaling-stroke"
      />
    </svg>
  )
}
