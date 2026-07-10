import {
  useState,
  useRef,
  useEffect,
  useCallback,
  useMemo,
} from "react"
import { Trash2, Download, ScrollText } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"

// ---------------------------------------------------------------------------
// Live server logs panel.
//
// Connects to the backend SSE endpoint `GET /api/logs/stream`. The backend
// receives CS2 server logs over UDP (the classic `logaddress_add` mechanism)
// and fans each line out to connected clients. Renders each incoming line in
// real time. Supports a configurable retention window, auto-scroll with a
// manual-scroll pause, a clear button, a download, and a connection status
// indicator.
// ---------------------------------------------------------------------------

type LogLine = {
  id: number
  text: string
  ts: string
}

type ConnStatus = "connecting" | "connected" | "reconnecting" | "error"

const STREAM_URL = "/api/logs/stream"

const DEFAULT_MAX_LINES = 500
const MIN_MAX_LINES = 50
const MAX_MAX_LINES = 2000

const MAX_LINES_KEY = "nokit_logs_max_lines"

function nowTime(): string {
  return new Date().toLocaleTimeString("en-GB", { hour12: false })
}

function today(): string {
  return new Date().toISOString().slice(0, 10)
}

function loadMaxLines(): number {
  try {
    const raw = localStorage.getItem(MAX_LINES_KEY)
    if (!raw) return DEFAULT_MAX_LINES
    const n = parseInt(raw, 10)
    if (Number.isNaN(n)) return DEFAULT_MAX_LINES
    return Math.min(MAX_MAX_LINES, Math.max(MIN_MAX_LINES, n))
  } catch {
    return DEFAULT_MAX_LINES
  }
}

// Best-effort colorization of common CS2/srcds log line categories.
function lineClass(text: string): string {
  if (/killed .* with /.test(text)) return "text-rose-300"
  if (/"\s*say(_team)?\s*"|"\s+say(_team)?\s+"/.test(text) || /\bsay(_team)?\b/.test(text))
    return "text-sky-300"
  if (/Round_Start|Round_End|triggered|Warmup/.test(text)) return "text-amber-300"
  if (/connected|disconnected|switched from team|entered the game/.test(text))
    return "text-emerald-300"
  if (/error|fail|exception|fatal|panic/i.test(text)) return "text-red-400"
  if (/workshop|download|Success|host_workshop_map/i.test(text))
    return "text-fuchsia-300"
  return "text-foreground/80"
}

const STATUS_META: Record<
  ConnStatus,
  { label: string; dot: string; text: string }
> = {
  connecting: {
    label: "connecting",
    dot: "bg-amber-400 animate-pulse",
    text: "text-amber-400",
  },
  connected: {
    label: "connected",
    dot: "bg-emerald-400",
    text: "text-emerald-400",
  },
  reconnecting: {
    label: "reconnecting",
    dot: "bg-amber-400 animate-pulse",
    text: "text-amber-400",
  },
  error: {
    label: "error",
    dot: "bg-red-500",
    text: "text-red-400",
  },
}

export function LogsPanel() {
  const [lines, setLines] = useState<LogLine[]>([])
  const [status, setStatus] = useState<ConnStatus>("connecting")
  const [maxLines, setMaxLines] = useState<number>(() => loadMaxLines())
  // Whether auto-scroll follows new lines. Toggled off when the user scrolls up.
  const [follow, setFollow] = useState(true)

  const nextId = useRef(0)
  const outputRef = useRef<HTMLDivElement>(null)
  const followRef = useRef(follow)
  const maxLinesRef = useRef(maxLines)

  followRef.current = follow
  maxLinesRef.current = maxLines

  // ---- persist max-lines preference --------------------------------------
  useEffect(() => {
    try {
      localStorage.setItem(MAX_LINES_KEY, String(maxLines))
    } catch {
      /* ignore quota errors */
    }
  }, [maxLines])

  // If the retention window shrinks, trim existing lines immediately.
  useEffect(() => {
    setLines((prev) => (prev.length > maxLines ? prev.slice(-maxLines) : prev))
  }, [maxLines])

  // ---- SSE connection -----------------------------------------------------
  useEffect(() => {
    let es: EventSource | null = null
    let closed = false

    const connect = () => {
      if (closed) return
      es = new EventSource(STREAM_URL, { withCredentials: true })

      es.onopen = () => {
        if (!closed) setStatus("connected")
      }

      es.onmessage = (ev) => {
        if (closed) return
        const text = ev.data as string
        setLines((prev) => {
          const next = [
            ...prev,
            { id: nextId.current++, text, ts: nowTime() },
          ]
          const limit = maxLinesRef.current
          return next.length > limit ? next.slice(-limit) : next
        })
      }

      // Server signals the docker-logs process ended.
      es.addEventListener("end", () => {
        if (!closed) setStatus("reconnecting")
      })

      es.onerror = () => {
        if (closed) return
        // EventSource auto-reconnects unless the connection is fully closed.
        setStatus(es && es.readyState === EventSource.CLOSED ? "error" : "reconnecting")
      }
    }

    connect()

    return () => {
      closed = true
      es?.close()
    }
  }, [])

  // ---- auto-scroll to bottom on new lines (unless paused) -----------------
  useEffect(() => {
    if (!follow) return
    const el = outputRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [lines, follow])

  // Detect manual scroll: pause following when the user scrolls up, resume
  // when they return to (near) the bottom.
  const handleScroll = useCallback(() => {
    const el = outputRef.current
    if (!el) return
    const distanceFromBottom =
      el.scrollHeight - el.scrollTop - el.clientHeight
    const atBottom = distanceFromBottom < 40
    if (atBottom && !followRef.current) setFollow(true)
    else if (!atBottom && followRef.current) setFollow(false)
  }, [])

  const clear = useCallback(() => {
    setLines([])
    nextId.current = 0
  }, [])

  const download = useCallback(() => {
    const text = lines.map((l) => l.text).join("\n")
    const blob = new Blob([text], { type: "text/plain" })
    const url = URL.createObjectURL(blob)
    const a = document.createElement("a")
    a.href = url
    a.download = `cs2-logs-${today()}.log`
    a.click()
    URL.revokeObjectURL(url)
  }, [lines])

  const jumpToBottom = useCallback(() => {
    const el = outputRef.current
    if (el) el.scrollTop = el.scrollHeight
    setFollow(true)
  }, [])

  const onMaxLinesChange = (raw: string) => {
    const n = parseInt(raw, 10)
    if (Number.isNaN(n)) return
    setMaxLines(Math.min(MAX_MAX_LINES, Math.max(MIN_MAX_LINES, n)))
  }

  const meta = useMemo(() => STATUS_META[status], [status])

  // ---- render -------------------------------------------------------------
  return (
    <div className="flex h-full flex-col gap-3 p-6">
      {/* header */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="flex items-center gap-2 text-base font-semibold">
            <ScrollText className="size-4" /> Live Logs
          </h2>
          <p className="text-xs text-muted-foreground">
            <code>logaddress</code> (UDP) over SSE ·{" "}
            {lines.length} line{lines.length === 1 ? "" : "s"} shown
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {/* connection status indicator */}
          <div
            className={cn(
              "inline-flex items-center gap-1.5 rounded-lg border border-border px-2.5 py-1 text-xs font-medium",
              meta.text
            )}
            title={`SSE: ${meta.label}`}
          >
            <span className={cn("size-2 rounded-full", meta.dot)} />
            {meta.label}
          </div>

          {/* max lines input */}
          <label className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
            Max lines
            <Input
              type="number"
              min={MIN_MAX_LINES}
              max={MAX_MAX_LINES}
              step={50}
              value={maxLines}
              onChange={(e) => onMaxLinesChange(e.target.value)}
              className="h-7 w-20 text-xs"
              title={`Retain between ${MIN_MAX_LINES} and ${MAX_MAX_LINES} lines`}
            />
          </label>

          <Button
            size="sm"
            variant="outline"
            onClick={download}
            disabled={lines.length === 0}
          >
            <Download className="size-3.5" /> Download
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={clear}
            disabled={lines.length === 0}
          >
            <Trash2 className="size-3.5" /> Clear
          </Button>
        </div>
      </div>

      {/* log stream */}
      <div className="relative flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border border-border">
        <div
          ref={outputRef}
          onScroll={handleScroll}
          className="min-h-0 flex-1 overflow-y-auto bg-zinc-950 p-3 font-mono text-xs leading-relaxed"
        >
          {lines.length === 0 ? (
            <div className="text-muted-foreground">
              {status === "connected"
                ? "Connected — waiting for log output…"
                : "Connecting to log stream…"}
            </div>
          ) : (
            lines.map((line) => (
              <div
                key={line.id}
                className="flex gap-2 whitespace-pre-wrap break-all"
              >
                <span className="shrink-0 select-none text-zinc-600">
                  {line.ts}
                </span>
                <span className={lineClass(line.text)}>{line.text}</span>
              </div>
            ))
          )}
        </div>

        {/* paused / jump-to-bottom pill */}
        {!follow && (
          <button
            onClick={jumpToBottom}
            className="absolute bottom-3 right-3 inline-flex items-center gap-1 rounded-full bg-amber-500/20 px-3 py-1 text-[11px] font-medium text-amber-300 shadow-sm backdrop-blur transition-colors hover:bg-amber-500/30"
          >
            paused — jump to latest ↓
          </button>
        )}
      </div>
    </div>
  )
}
