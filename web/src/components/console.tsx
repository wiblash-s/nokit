import {
  useState,
  useRef,
  useEffect,
  useMemo,
  useCallback,
  type FormEvent,
  type KeyboardEvent,
} from "react"
import { Loader2, Play, Plus, X } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"
import cs2CommandsData from "@/data/cs2-commands.json"

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type LogLine = {
  id: number
  kind: "cmd" | "output" | "error"
  text: string
  ts: string
}

type Macro = {
  label: string
  command: string
  custom?: boolean
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

// CS2 commands / CVARs from comprehensive list (5000+ commands)
// Source: https://github.com/armync/ArminC-CS2-Cvars
const COMMANDS: string[] = cs2CommandsData.commands

const DEFAULT_MACROS: Macro[] = [
  { label: "warmup → live", command: "mp_warmuptime 15 ; mp_warmup_end" },
  { label: "reset score", command: "mp_restartgame 1" },
  { label: "knife → side pick", command: "matchzy_knife" },
  { label: "pause match", command: "matchzy_pause" },
  { label: "kick all bots", command: "bot_kick" },
  { label: "demo: start", command: "tv_record demos/manual_${date}" },
  { label: "demo: stop", command: "tv_stoprecord" },
  { label: "fix stuck round", command: "mp_unpause_match ; mp_restartgame 1" },
]

const MACROS_KEY = "nokit_console_macros"
const HISTORY_KEY = "nokit_console_history"
const MAX_HISTORY = 50

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function today(): string {
  return new Date().toISOString().slice(0, 10) // YYYY-MM-DD
}

function nowTime(): string {
  return new Date().toLocaleTimeString("en-GB", { hour12: false })
}

/** Substitute supported template tokens in a macro command. */
function expandCommand(command: string): string {
  return command.replace(/\$\{date\}/g, today())
}

function loadMacros(): Macro[] {
  try {
    const raw = localStorage.getItem(MACROS_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed
      .filter(
        (m): m is Macro =>
          m &&
          typeof m.label === "string" &&
          typeof m.command === "string"
      )
      .map((m) => ({ label: m.label, command: m.command, custom: true }))
  } catch {
    return []
  }
}

function loadHistory(): string[] {
  try {
    const raw = localStorage.getItem(HISTORY_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.filter((c): c is string => typeof c === "string")
  } catch {
    return []
  }
}

// ---------------------------------------------------------------------------
// Output line rendering
// ---------------------------------------------------------------------------

const CVAR_RE = /(" = )|("= \\?")|(\s=\s)/

function OutputText({ text }: { text: string }) {
  // Highlight CVAR-style lines: NAME = VALUE (or `"cvar" = "value"`)
  if (CVAR_RE.test(text)) {
    // Split around the first " = " / "= \"" style separator.
    const m = text.match(/^(.*?)( = |=\s*)(.+)$/)
    if (m) {
      return (
        <span>
          <span className="text-foreground/70">{m[1]}</span>
          <span className="text-foreground/50">{m[2]}</span>
          <span className="text-amber-400">{m[3]}</span>
        </span>
      )
    }
  }
  return <span className="text-foreground/80">{text}</span>
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function Console({ serverId }: { serverId: string }) {
  const [lines, setLines] = useState<LogLine[]>([])
  const [command, setCommand] = useState("")
  const [busy, setBusy] = useState(false)
  const [live, setLive] = useState(true)

  const [history, setHistory] = useState<string[]>(() => loadHistory())
  const [macros, setMacros] = useState<Macro[]>(() => [
    ...DEFAULT_MACROS,
    ...loadMacros(),
  ])

  // command-history cursor (for ↑/↓); -1 means "current draft"
  const [historyIdx, setHistoryIdx] = useState(-1)

  // autocomplete state
  const [acOpen, setAcOpen] = useState(false)
  const [acIdx, setAcIdx] = useState(0)

  // new-macro form
  const [showMacroForm, setShowMacroForm] = useState(false)
  const [newLabel, setNewLabel] = useState("")
  const [newCommand, setNewCommand] = useState("")

  const nextId = useRef(0)
  const outputRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  // ---- persistence --------------------------------------------------------

  useEffect(() => {
    const custom = macros.filter((m) => m.custom)
    try {
      localStorage.setItem(
        MACROS_KEY,
        JSON.stringify(custom.map((m) => ({ label: m.label, command: m.command })))
      )
    } catch {
      /* ignore quota errors */
    }
  }, [macros])

  useEffect(() => {
    try {
      localStorage.setItem(HISTORY_KEY, JSON.stringify(history.slice(0, MAX_HISTORY)))
    } catch {
      /* ignore */
    }
  }, [history])

  // ---- scroll to bottom on new output (unless paused) ---------------------

  useEffect(() => {
    if (!live) return
    outputRef.current?.scrollTo({ top: outputRef.current.scrollHeight })
  }, [lines, live])

  // ---- autocomplete matches ----------------------------------------------

  const matches = useMemo(() => {
    const prefix = command.trim().toLowerCase()
    if (!prefix) return []
    // Only autocomplete the (single-token) command word.
    if (/\s/.test(command.trimStart())) return []
    
    // With 5000+ commands, limit results to keep UI responsive
    const MAX_SUGGESTIONS = 50
    const results: string[] = []
    
    // Fast early exit: collect up to MAX_SUGGESTIONS matches
    for (const cmd of COMMANDS) {
      if (cmd.toLowerCase().startsWith(prefix) && cmd !== prefix) {
        results.push(cmd)
        if (results.length >= MAX_SUGGESTIONS) break
      }
    }
    
    return results
  }, [command])

  useEffect(() => {
    if (matches.length === 0) {
      setAcOpen(false)
      setAcIdx(0)
    }
  }, [matches.length])

  // ---- helpers ------------------------------------------------------------

  const append = useCallback((kind: LogLine["kind"], text: string) => {
    setLines((prev) => [...prev, { id: nextId.current++, kind, text, ts: nowTime() }])
  }, [])

  const pushHistory = useCallback((cmd: string) => {
    setHistory((prev) => {
      const next = [cmd, ...prev.filter((c) => c !== cmd)]
      return next.slice(0, MAX_HISTORY)
    })
  }, [])

  const runCommand = useCallback(
    async (rawCmd: string) => {
      const cmd = rawCmd.trim()
      if (!cmd || busy) return

      append("cmd", cmd)
      pushHistory(cmd)
      setBusy(true)

      try {
        const res = await fetch(`/api/servers/${serverId}/rcon`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ command: cmd }),
        })
        const body = await res.json()
        if (!res.ok) {
          append("error", body.error || `request failed (${res.status})`)
        } else {
          const out = (body.output ?? "").trimEnd()
          if (out) {
            for (const l of out.split(/\r?\n/)) {
              append(
                /error|failed|unknown command|not found/i.test(l) ? "error" : "output",
                l
              )
            }
          } else {
            append("output", "(no output)")
          }
        }
      } catch (err) {
        append(
          "error",
          `network error: ${err instanceof Error ? err.message : "unknown"}`
        )
      } finally {
        setBusy(false)
      }
    },
    [append, busy, pushHistory, serverId]
  )

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (acOpen && matches[acIdx]) {
      // accept the highlighted suggestion instead of submitting
      setCommand(matches[acIdx])
      setAcOpen(false)
      return
    }
    const cmd = command.trim()
    if (!cmd) return
    setCommand("")
    setHistoryIdx(-1)
    setAcOpen(false)
    void runCommand(cmd)
  }

  // ---- key handling -------------------------------------------------------

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    // Ctrl+L clears the terminal
    if (e.key === "l" && e.ctrlKey) {
      e.preventDefault()
      setLines([])
      return
    }

    // Tab: autocomplete
    if (e.key === "Tab") {
      if (matches.length === 0) return
      e.preventDefault()
      if (!acOpen) {
        setAcOpen(true)
        setAcIdx(0)
        setCommand(matches[0])
      } else {
        const nextIdx = (acIdx + 1) % matches.length
        setAcIdx(nextIdx)
        setCommand(matches[nextIdx])
      }
      return
    }

    // Escape dismisses autocomplete
    if (e.key === "Escape") {
      if (acOpen) {
        e.preventDefault()
        setAcOpen(false)
      }
      return
    }

    // When autocomplete open, arrows navigate suggestions
    if (acOpen && matches.length > 0 && (e.key === "ArrowUp" || e.key === "ArrowDown")) {
      e.preventDefault()
      const delta = e.key === "ArrowDown" ? 1 : -1
      const nextIdx = (acIdx + delta + matches.length) % matches.length
      setAcIdx(nextIdx)
      setCommand(matches[nextIdx])
      return
    }

    // ↑/↓ navigate command history (shell-style)
    if (e.key === "ArrowUp") {
      if (history.length === 0) return
      e.preventDefault()
      const nextIdx = Math.min(historyIdx + 1, history.length - 1)
      setHistoryIdx(nextIdx)
      setCommand(history[nextIdx] ?? "")
      return
    }
    if (e.key === "ArrowDown") {
      if (historyIdx < 0) return
      e.preventDefault()
      const nextIdx = historyIdx - 1
      setHistoryIdx(nextIdx)
      setCommand(nextIdx < 0 ? "" : history[nextIdx] ?? "")
      return
    }
  }

  // ---- macros -------------------------------------------------------------

  const runMacro = (macro: Macro) => {
    void runCommand(expandCommand(macro.command))
    inputRef.current?.focus()
  }

  const addMacro = (e: FormEvent) => {
    e.preventDefault()
    const label = newLabel.trim()
    const cmd = newCommand.trim()
    if (!label || !cmd) return
    setMacros((prev) => [...prev, { label, command: cmd, custom: true }])
    setNewLabel("")
    setNewCommand("")
    setShowMacroForm(false)
  }

  const deleteMacro = (idx: number) => {
    setMacros((prev) => prev.filter((_, i) => i !== idx))
  }

  // ---- toolbar actions ----------------------------------------------------

  const sessionText = useMemo(
    () =>
      lines
        .map((l) =>
          l.kind === "cmd" ? `rcon ${l.ts} > ${l.text}` : l.text
        )
        .join("\n"),
    [lines]
  )

  const copySession = () => {
    void navigator.clipboard?.writeText(sessionText).catch(() => {})
  }

  const exportSession = () => {
    const blob = new Blob([sessionText], { type: "text/plain" })
    const url = URL.createObjectURL(blob)
    const a = document.createElement("a")
    a.href = url
    a.download = `rcon-session-${today()}.txt`
    a.click()
    URL.revokeObjectURL(url)
  }

  const pasteToInput = (cmd: string) => {
    setCommand(cmd)
    setHistoryIdx(-1)
    inputRef.current?.focus()
  }

  const recentHistory = useMemo(() => history.slice(0, 20), [history])

  // ---- render -------------------------------------------------------------

  return (
    <div className="flex h-full flex-col gap-3 p-6">
      {/* top bar */}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h2 className="text-base font-semibold">RCON Console</h2>
          <p className="text-xs text-muted-foreground">
            tab to autocomplete · ↑↓ history · ctrl-l clears
          </p>
        </div>
        <div className="flex items-center gap-2">
          {/* live | paused segmented toggle */}
          <div className="inline-flex overflow-hidden rounded-lg border border-border text-xs">
            <button
              onClick={() => setLive(true)}
              className={cn(
                "px-2.5 py-1 font-medium transition-colors",
                live
                  ? "bg-primary text-primary-foreground"
                  : "text-muted-foreground hover:text-foreground"
              )}
            >
              live
            </button>
            <button
              onClick={() => setLive(false)}
              className={cn(
                "px-2.5 py-1 font-medium transition-colors",
                !live
                  ? "bg-primary text-primary-foreground"
                  : "text-muted-foreground hover:text-foreground"
              )}
            >
              paused
            </button>
          </div>
          <Button size="sm" variant="outline" onClick={copySession} disabled={lines.length === 0}>
            Copy session
          </Button>
          <Button size="sm" variant="outline" onClick={exportSession} disabled={lines.length === 0}>
            Export
          </Button>
        </div>
      </div>

      {/* two-column layout */}
      <div className="flex min-h-0 flex-1 flex-col gap-3 lg:flex-row">
        {/* LEFT: terminal + input */}
        <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-lg border border-border lg:basis-[65%]">
          <div
            ref={outputRef}
            className="min-h-0 flex-1 overflow-y-auto bg-zinc-950 p-3 font-mono text-xs leading-relaxed text-green-400"
          >
            {lines.length === 0 ? (
              <div className="text-muted-foreground">
                No commands yet. Try <code>status</code> or{" "}
                <code>mp_warmup_end</code>.
              </div>
            ) : (
              lines.map((line) => (
                <div key={line.id} className="whitespace-pre-wrap break-words">
                  {line.kind === "cmd" && (
                    <span className="text-green-400">
                      <span className="text-green-600">rcon {line.ts} {">"} </span>
                      {line.text}
                    </span>
                  )}
                  {line.kind === "output" && <OutputText text={line.text} />}
                  {line.kind === "error" && (
                    <span className="text-red-400">{line.text}</span>
                  )}
                </div>
              ))
            )}
            {busy && (
              <div className="flex items-center gap-1.5 text-green-600">
                <Loader2 className="size-3 animate-spin" />
                <span>running…</span>
              </div>
            )}
            {!live && (
              <div className="pointer-events-none sticky bottom-0 mt-2 inline-flex rounded bg-amber-500/20 px-2 py-0.5 text-[10px] font-medium text-amber-400">
                paused — auto-scroll off
              </div>
            )}
          </div>

          {/* input bar */}
          <form
            onSubmit={handleSubmit}
            className="relative flex items-center gap-2 border-t border-border bg-background p-2"
          >
            <span className="shrink-0 pl-1 font-mono text-xs text-green-600">rcon ⟩</span>
            <div className="relative flex-1">
              {/* autocomplete dropdown */}
              {acOpen && matches.length > 0 && (
                <div className="absolute bottom-full left-0 z-10 mb-1 max-h-48 w-56 overflow-y-auto rounded-lg border border-border bg-popover p-1 shadow-md">
                  {matches.map((m, i) => (
                    <button
                      key={m}
                      type="button"
                      onMouseDown={(e) => {
                        e.preventDefault()
                        setCommand(m)
                        setAcOpen(false)
                        inputRef.current?.focus()
                      }}
                      className={cn(
                        "block w-full rounded px-2 py-1 text-left font-mono text-xs",
                        i === acIdx
                          ? "bg-primary text-primary-foreground"
                          : "text-foreground hover:bg-muted"
                      )}
                    >
                      {m}
                    </button>
                  ))}
                </div>
              )}
              <Input
                ref={inputRef}
                value={command}
                onChange={(e) => {
                  setCommand(e.target.value)
                  setHistoryIdx(-1)
                  if (e.target.value.trim()) setAcOpen(false)
                }}
                onKeyDown={handleKeyDown}
                placeholder="status, say, mp_restartgame…"
                className="font-mono text-xs"
                spellCheck={false}
                autoComplete="off"
                autoFocus
              />
            </div>
            <Button type="submit" disabled={busy || !command.trim()} size="sm">
              {busy ? <Loader2 className="size-3.5 animate-spin" /> : "send"}
            </Button>
          </form>
        </div>

        {/* RIGHT: macros + history */}
        <div className="flex min-h-0 flex-col gap-3 lg:basis-[35%]">
          {/* macros */}
          <div className="flex min-h-0 flex-col overflow-hidden rounded-lg border border-border bg-card">
            <div className="border-b border-border px-3 py-2 text-xs font-semibold">
              ⚡ RCON macros
            </div>
            <div className="min-h-0 flex-1 space-y-1.5 overflow-y-auto p-2">
              {macros.map((macro, idx) => (
                <div key={`${macro.label}-${idx}`} className="group relative">
                  <button
                    onClick={() => runMacro(macro)}
                    className="w-full rounded-lg border border-border bg-background px-2.5 py-1.5 text-left transition-colors hover:bg-muted"
                  >
                    <div className="text-xs font-medium">{macro.label}</div>
                    <div className="truncate font-mono text-[10px] text-muted-foreground">
                      {macro.command}
                    </div>
                  </button>
                  {macro.custom && (
                    <button
                      onClick={() => deleteMacro(idx)}
                      title="Delete macro"
                      className="absolute top-1 right-1 hidden rounded p-0.5 text-muted-foreground hover:bg-destructive/20 hover:text-destructive group-hover:block"
                    >
                      <X className="size-3.5" />
                    </button>
                  )}
                </div>
              ))}

              {showMacroForm ? (
                <form
                  onSubmit={addMacro}
                  className="space-y-1.5 rounded-lg border border-border bg-background p-2"
                >
                  <Input
                    value={newLabel}
                    onChange={(e) => setNewLabel(e.target.value)}
                    placeholder="label"
                    className="text-xs"
                    autoFocus
                  />
                  <Input
                    value={newCommand}
                    onChange={(e) => setNewCommand(e.target.value)}
                    placeholder="command"
                    className="font-mono text-xs"
                  />
                  <div className="flex gap-1.5">
                    <Button
                      type="submit"
                      size="xs"
                      disabled={!newLabel.trim() || !newCommand.trim()}
                    >
                      Add
                    </Button>
                    <Button
                      type="button"
                      size="xs"
                      variant="ghost"
                      onClick={() => {
                        setShowMacroForm(false)
                        setNewLabel("")
                        setNewCommand("")
                      }}
                    >
                      Cancel
                    </Button>
                  </div>
                </form>
              ) : (
                <button
                  onClick={() => setShowMacroForm(true)}
                  className="flex w-full items-center justify-center gap-1 rounded-lg border border-dashed border-border px-2.5 py-1.5 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                >
                  <Plus className="size-3.5" /> New macro
                </button>
              )}
            </div>
          </div>

          {/* history */}
          <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border border-border bg-card">
            <div className="border-b border-border px-3 py-2 text-xs font-semibold">
              ↺ History
            </div>
            <div className="min-h-0 flex-1 overflow-y-auto p-2">
              {recentHistory.length === 0 ? (
                <div className="px-1 py-2 text-xs text-muted-foreground">
                  No history yet.
                </div>
              ) : (
                <div className="space-y-0.5">
                  {recentHistory.map((cmd, i) => (
                    <button
                      key={`${cmd}-${i}`}
                      onClick={() => pasteToInput(cmd)}
                      className="group flex w-full items-center gap-1.5 rounded px-2 py-1 text-left font-mono text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                    >
                      <Play className="size-3 shrink-0 opacity-0 group-hover:opacity-100" />
                      <span className="truncate">{cmd}</span>
                    </button>
                  ))}
                </div>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
