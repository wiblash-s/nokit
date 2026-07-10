import { useState } from "react"
import {
  Power,
  RotateCcw,
  Shuffle,
  FlagTriangleRight,
  Lock,
} from "lucide-react"
import { Button } from "@/components/ui/button"

type Mode = "match" | "practice" | "pug"

type Props = {
  exec: (command: string) => Promise<string>
  onAfterAction?: () => void
}

export function QuickActions({ exec, onAfterAction }: Props) {
  const [busy, setBusy] = useState<string | null>(null)
  const [mode, setMode] = useState<Mode>("match")
  const [message, setMessage] = useState<{
    kind: "ok" | "error"
    text: string
  } | null>(null)

  const run = async (
    key: string,
    command: string,
    opts?: { confirm?: string; label?: string }
  ) => {
    if (busy) return
    if (opts?.confirm && !confirm(opts.confirm)) return
    setBusy(key)
    setMessage(null)
    try {
      await exec(command)
      setMessage({ kind: "ok", text: `${opts?.label ?? command} — sent` })
      onAfterAction?.()
    } catch (err) {
      setMessage({
        kind: "error",
        text: err instanceof Error ? err.message : "failed",
      })
    } finally {
      setBusy(null)
    }
  }

  const switchMode = async (next: Mode) => {
    if (busy || next === mode) return
    setBusy(`mode-${next}`)
    setMessage(null)
    try {
      // Execs a server cfg named after the mode (match.cfg / practice.cfg /
      // pug.cfg), a common admin convention.
      await exec(`exec ${next}`)
      setMode(next)
      setMessage({ kind: "ok", text: `mode → ${next} (exec ${next}.cfg)` })
      onAfterAction?.()
    } catch (err) {
      setMessage({
        kind: "error",
        text: err instanceof Error ? err.message : "failed",
      })
    } finally {
      setBusy(null)
    }
  }

  return (
    <div className="rounded-lg border border-border bg-card p-4">
      <h2 className="mb-3 text-sm font-semibold">Quick actions</h2>

      <div className="flex flex-col gap-2">
        <Button
          variant="outline"
          className="justify-start"
          disabled={busy !== null}
          onClick={() =>
            run("restart", "_restart", {
              confirm: "Restart the server now?",
              label: "restart server",
            })
          }
        >
          <RotateCcw />
          {busy === "restart" ? "restarting…" : "Restart server"}
        </Button>

        {/* mode segmented toggle */}
        <div className="grid grid-cols-3 gap-1 rounded-lg border border-border p-1">
          {(["match", "practice", "pug"] as Mode[]).map((m) => (
            <button
              key={m}
              onClick={() => switchMode(m)}
              disabled={busy !== null}
              className={`rounded-md px-2 py-1.5 text-xs font-medium capitalize transition-colors disabled:opacity-50 ${
                mode === m
                  ? "bg-primary text-primary-foreground"
                  : "text-muted-foreground hover:bg-muted hover:text-foreground"
              }`}
            >
              {busy === `mode-${m}` ? "…" : m}
            </button>
          ))}
        </div>

        <Button
          variant="outline"
          className="justify-start"
          disabled={busy !== null}
          onClick={() =>
            run("nopass", 'sv_password ""', { label: "clear password" })
          }
        >
          <Lock />
          {busy === "nopass" ? "clearing…" : "No password"}
        </Button>

        <Button
          variant="outline"
          className="justify-start"
          disabled={busy !== null}
          onClick={() =>
            run("warmup", "mp_warmup_end", { label: "end warmup" })
          }
        >
          <FlagTriangleRight />
          {busy === "warmup" ? "ending…" : "End warmup"}
        </Button>

        <Button
          variant="outline"
          className="justify-start"
          disabled={busy !== null}
          onClick={() =>
            run("shuffle", "mp_scrambleteams 1", { label: "shuffle teams" })
          }
        >
          <Shuffle />
          {busy === "shuffle" ? "shuffling…" : "Auto-shuffle teams"}
        </Button>

        <Button
          variant="destructive"
          className="justify-start"
          disabled={busy !== null}
          onClick={() =>
            run("stop", "quit", {
              confirm: "Stop the server? This ends the current match.",
              label: "stop server",
            })
          }
        >
          <Power />
          {busy === "stop" ? "stopping…" : "Stop server"}
        </Button>
      </div>

      {message && (
        <p
          className={`mt-3 truncate font-mono text-xs ${
            message.kind === "ok" ? "text-green-500" : "text-destructive"
          }`}
        >
          {message.text}
        </p>
      )}
    </div>
  )
}
