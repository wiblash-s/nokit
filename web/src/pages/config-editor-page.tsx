import { useCallback, useEffect, useState } from "react"
import {
  File,
  RefreshCw,
  Terminal,
  Save,
  Plus,
  Trash2,
  AlertCircle,
  CheckCircle2,
  Loader2,
  FolderOpen,
  Database,
  Lock,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { CfgEditor } from "@/components/cfg-editor"
import { cn } from "@/lib/utils"
import {
  listConfigs,
  getConfig,
  saveConfig,
  deleteConfig,
  execConfig,
  type ConfigInfo,
} from "@/lib/api/configs"

// ---------------------------------------------------------------------------
// Toast (minimal, self-contained — the app has no toast library)
// ---------------------------------------------------------------------------

type Toast = { id: number; kind: "success" | "error"; message: string }

function useToasts() {
  const [toasts, setToasts] = useState<Toast[]>([])
  const push = useCallback((kind: Toast["kind"], message: string) => {
    const id = Date.now() + Math.random()
    setToasts((t) => [...t, { id, kind, message }])
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 4000)
  }, [])
  return { toasts, success: (m: string) => push("success", m), error: (m: string) => push("error", m) }
}

function ToastStack({ toasts }: { toasts: Toast[] }) {
  return (
    <div className="pointer-events-none fixed right-4 bottom-4 z-50 flex flex-col gap-2">
      {toasts.map((t) => (
        <div
          key={t.id}
          className={cn(
            "pointer-events-auto flex max-w-sm items-start gap-2 rounded-lg border px-3 py-2 text-sm shadow-lg",
            t.kind === "success"
              ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-500"
              : "border-destructive/30 bg-destructive/10 text-destructive"
          )}
        >
          {t.kind === "success" ? (
            <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" />
          ) : (
            <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
          )}
          <span className="break-words">{t.message}</span>
        </div>
      ))}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

type ConfigEditorPageProps = {
  serverId: string
}

const NEW_DRAFT = "__new__"

export function ConfigEditorPage({ serverId }: ConfigEditorPageProps) {
  const { toasts, success, error } = useToasts()

  const [mode, setMode] = useState<string>("panel")
  const [writable, setWritable] = useState(true)
  const [files, setFiles] = useState<ConfigInfo[]>([])
  const [listLoading, setListLoading] = useState(true)

  // Selection + editor state.
  const [selected, setSelected] = useState<string | null>(null)
  const [draftName, setDraftName] = useState("") // for new configs
  const [content, setContent] = useState("")
  const [originalContent, setOriginalContent] = useState("")
  const [contentLoading, setContentLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [execing, setExecing] = useState(false)

  const isNew = selected === NEW_DRAFT
  const hasUnsavedChanges = content !== originalContent
  const effectiveName = isNew ? draftName.trim() : selected ?? ""
  const canWrite = mode === "panel" || writable

  const refreshList = useCallback(async () => {
    setListLoading(true)
    try {
      const res = await listConfigs(serverId)
      setMode(res.mode)
      setWritable(res.writable)
      setFiles(res.configs)
    } catch (e) {
      error(e instanceof Error ? e.message : "Failed to load configs")
    } finally {
      setListLoading(false)
    }
  }, [serverId, error])

  useEffect(() => {
    void refreshList()
  }, [refreshList])

  const openConfig = useCallback(
    async (name: string) => {
      setSelected(name)
      setDraftName("")
      setContentLoading(true)
      try {
        const cfg = await getConfig(serverId, name)
        setContent(cfg.content)
        setOriginalContent(cfg.content)
      } catch (e) {
        error(e instanceof Error ? e.message : "Failed to open config")
        setContent("")
        setOriginalContent("")
      } finally {
        setContentLoading(false)
      }
    },
    [serverId, error]
  )

  const startNewConfig = () => {
    setSelected(NEW_DRAFT)
    setDraftName("")
    setContent("")
    setOriginalContent("")
  }

  const onSave = async () => {
    let name = effectiveName
    if (!name) {
      error("Config name is required")
      return
    }
    if (!name.toLowerCase().endsWith(".cfg")) {
      name = `${name}.cfg`
    }
    setSaving(true)
    try {
      await saveConfig(serverId, name, content)
      setOriginalContent(content)
      success(`Saved ${name}`)
      if (isNew) {
        setSelected(name)
        setDraftName("")
      }
      await refreshList()
    } catch (e) {
      error(e instanceof Error ? e.message : "Failed to save config")
    } finally {
      setSaving(false)
    }
  }

  const onDelete = async (name: string) => {
    if (!confirm(`Delete ${name}? This cannot be undone.`)) return
    try {
      await deleteConfig(serverId, name)
      success(`Deleted ${name}`)
      if (selected === name) {
        setSelected(null)
        setContent("")
        setOriginalContent("")
      }
      await refreshList()
    } catch (e) {
      error(e instanceof Error ? e.message : "Failed to delete config")
    }
  }

  const onExec = async () => {
    if (isNew || !selected) {
      error("Save the config before executing it")
      return
    }
    setExecing(true)
    try {
      const res = await execConfig(serverId, selected)
      if (res.errors && res.errors.length > 0) {
        error(`Sent ${res.commands_sent} command(s), ${res.errors.length} failed`)
      } else if (res.mode === "mounted") {
        success(`Executed ${selected} via RCON`)
      } else {
        success(`Sent ${res.commands_sent} command(s) via RCON`)
      }
    } catch (e) {
      error(e instanceof Error ? e.message : "Failed to execute config")
    } finally {
      setExecing(false)
    }
  }

  const onReload = () => {
    if (selected && !isNew) void openConfig(selected)
  }

  const execLabel = mode === "mounted" ? "exec via RCON" : "Send Commands via RCON"

  return (
    <div className="flex flex-1 overflow-hidden">
      {/* File tree sidebar */}
      <div className="flex w-64 shrink-0 flex-col border-r border-border bg-card">
        <div className="flex items-center justify-between border-b border-border p-3">
          <h2 className="text-sm font-semibold">Configs</h2>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={startNewConfig}
            disabled={!canWrite}
            title={canWrite ? "New config" : "Mount is read-only"}
          >
            <Plus className="h-4 w-4" />
          </Button>
        </div>

        {/* Mode badge */}
        <div className="border-b border-border px-3 py-2">
          <span
            className={cn(
              "inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-xs font-medium",
              mode === "mounted"
                ? "bg-blue-500/10 text-blue-500"
                : "bg-purple-500/10 text-purple-500"
            )}
          >
            {mode === "mounted" ? (
              <>
                <FolderOpen className="h-3.5 w-3.5" /> 📁 Mounted
              </>
            ) : (
              <>
                <Database className="h-3.5 w-3.5" /> 🗄️ Panel
              </>
            )}
          </span>
          {mode === "mounted" && !writable && (
            <span className="mt-1 flex items-center gap-1 text-[11px] text-muted-foreground">
              <Lock className="h-3 w-3" /> read-only mount
            </span>
          )}
        </div>

        <div className="flex-1 overflow-y-auto p-2">
          {listLoading ? (
            <div className="flex items-center justify-center py-6 text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
            </div>
          ) : files.length === 0 ? (
            <p className="px-2 py-4 text-center text-xs text-muted-foreground">
              No configs yet.
              {canWrite && ' Use "New Config" to create one.'}
            </p>
          ) : (
            <div className="space-y-0.5">
              {files.map((f) => (
                <div
                  key={f.name}
                  className={cn(
                    "group flex items-center gap-1.5 rounded px-2 py-1 text-sm transition-colors hover:bg-accent",
                    selected === f.name && "bg-accent text-accent-foreground"
                  )}
                >
                  <button
                    onClick={() => void openConfig(f.name)}
                    className="flex flex-1 items-center gap-1.5 truncate text-left"
                  >
                    <File className="h-4 w-4 shrink-0 text-muted-foreground" />
                    <span className="truncate">{f.name}</span>
                  </button>
                  {canWrite && (
                    <button
                      onClick={() => void onDelete(f.name)}
                      className="shrink-0 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100 hover:text-destructive"
                      title="Delete config"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Editor panel */}
      <div className="flex flex-1 flex-col">
        {selected === null ? (
          <div className="flex flex-1 flex-col items-center justify-center gap-2 text-muted-foreground">
            <File className="h-10 w-10 opacity-40" />
            <p className="text-sm">Select a config to edit or create a new one.</p>
          </div>
        ) : (
          <>
            {/* Toolbar */}
            <div className="flex items-center justify-between gap-2 border-b border-border bg-card p-3">
              <div className="flex min-w-0 flex-1 items-center gap-2">
                {isNew ? (
                  <Input
                    autoFocus
                    value={draftName}
                    onChange={(e) => setDraftName(e.target.value)}
                    placeholder="new-config.cfg"
                    className="max-w-xs font-mono text-sm"
                  />
                ) : (
                  <span className="truncate font-mono text-sm font-medium">{selected}</span>
                )}
                {hasUnsavedChanges && (
                  <span className="shrink-0 rounded bg-amber-500/10 px-2 py-0.5 text-xs font-medium text-amber-500">
                    unsaved
                  </span>
                )}
              </div>
              <div className="flex shrink-0 items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={onReload}
                  disabled={isNew || contentLoading || !hasUnsavedChanges}
                >
                  <RefreshCw className={cn("h-4 w-4", contentLoading && "animate-spin")} />
                  Reload
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => void onExec()}
                  disabled={isNew || execing}
                >
                  {execing ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <Terminal className="h-4 w-4" />
                  )}
                  {execLabel}
                </Button>
                <Button size="sm" onClick={() => void onSave()} disabled={!canWrite || saving}>
                  {saving ? (
                    <Loader2 className="h-4 w-4 animate-spin" />
                  ) : (
                    <Save className="h-4 w-4" />
                  )}
                  Save
                </Button>
              </div>
            </div>

            {/* Mode indicator under the toolbar */}
            <div className="flex items-center gap-2 border-b border-border bg-card/50 px-3 py-1.5 text-xs text-muted-foreground">
              {mode === "mounted" ? (
                <span>
                  Mounted mode — files are read/written on the server's cfg volume. Exec runs{" "}
                  <code className="font-mono">
                    exec {selected && !isNew ? selected.replace(/\.cfg$/, "") : "<name>"}
                  </code>
                  .
                </span>
              ) : (
                <span>
                  Panel mode — files are stored in the panel database. Exec replays each line as an
                  RCON command.
                </span>
              )}
              {!canWrite && <span className="text-amber-500">· read-only</span>}
            </div>

            {/* Editor */}
            <div className="relative flex-1 overflow-hidden">
              {contentLoading ? (
                <div className="flex h-full items-center justify-center text-muted-foreground">
                  <Loader2 className="h-5 w-5 animate-spin" />
                </div>
              ) : (
                <CfgEditor value={content} onChange={setContent} readOnly={!canWrite} />
              )}
            </div>
          </>
        )}
      </div>

      <ToastStack toasts={toasts} />
    </div>
  )
}
