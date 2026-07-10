import { useState } from "react"
import { File, Folder, RefreshCw, Terminal, Save, Plus, ChevronRight, ChevronDown } from "lucide-react"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type FileNode = {
  name: string
  path: string
  type: "file" | "folder"
  modified?: string
  children?: FileNode[]
  expanded?: boolean
}

// ---------------------------------------------------------------------------
// Mock Data
// ---------------------------------------------------------------------------

const MOCK_FILE_TREE: FileNode[] = [
  {
    name: "autoexec.cfg",
    path: "/cfg/autoexec.cfg",
    type: "file",
    modified: "2d",
  },
  {
    name: "server.cfg",
    path: "/cfg/server.cfg",
    type: "file",
    modified: "1h",
  },
  {
    name: "banned_users.cfg",
    path: "/cfg/banned_users.cfg",
    type: "file",
    modified: "3h",
  },
  {
    name: "banned_ip.cfg",
    path: "/cfg/banned_ip.cfg",
    type: "file",
    modified: "7d",
  },
  {
    name: "gamemodes",
    path: "/cfg/gamemodes",
    type: "folder",
    expanded: true,
    children: [
      {
        name: "gamemode_competitive.cfg",
        path: "/cfg/gamemodes/gamemode_competitive.cfg",
        type: "file",
        modified: "12m",
      },
      {
        name: "gamemode_competitive_server.cfg",
        path: "/cfg/gamemodes/gamemode_competitive_server.cfg",
        type: "file",
        modified: "0h",
      },
      {
        name: "gamemode_casual.cfg",
        path: "/cfg/gamemodes/gamemode_casual.cfg",
        type: "file",
      },
      {
        name: "gamemode_deathmatch.cfg",
        path: "/cfg/gamemodes/gamemode_deathmatch.cfg",
        type: "file",
        modified: "14d",
      },
    ],
  },
  {
    name: "matchzy",
    path: "/cfg/matchzy",
    type: "folder",
    children: [
      {
        name: "mr12_competitive.cfg",
        path: "/cfg/matchzy/mr12_competitive.cfg",
        type: "file",
        modified: "8h",
      },
      {
        name: "live.cfg",
        path: "/cfg/matchzy/live.cfg",
        type: "file",
        modified: "0h",
      },
      {
        name: "warmup.cfg",
        path: "/cfg/matchzy/warmup.cfg",
        type: "file",
        modified: "0h",
      },
    ],
  },
  {
    name: "cs_sharp",
    path: "/cfg/cs_sharp",
    type: "folder",
    children: [
      {
        name: "WeaponPaints.json",
        path: "/cfg/cs_sharp/WeaponPaints.json",
        type: "file",
        modified: "5d",
      },
      {
        name: "MatchZy.json",
        path: "/cfg/cs_sharp/MatchZy.json",
        type: "file",
        modified: "8h",
      },
    ],
  },
]

const DEMO_FILE_CONTENT = `// gamemode_competitive_server.cfg
// matchzy + tournament defaults — fra1.mm
// last edit: m0use@ — 12:35

mp_maxrounds              24
mp_overtime_enable        1
mp_overtime_maxrounds     6
mp_overtime_startmoney    16000
mp_freezetime             15
mp_roundtime              1.92
mp_warmuptime             15
mp_startmoney             800
mp_maxmoney               16000
mp_buy_anywhere           0
mp_team_intro_period      0

sv_pure                   2
sv_minrate                128000
sv_maxupdaterate          128
sv_minupdaterate          128
sv_cheats                 0

// matchzy
matchzy_minimum_ready_required   5
matchzy_kick_when_no_match_loaded 0
matchzy_demo_path                "matchzy_demos"
matchzy_chat_prefix              "[≫nokit≪]"
`

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

type ConfigEditorPageProps = {
  serverId: string
}

export function ConfigEditorPage({ serverId }: ConfigEditorPageProps) {
  const [fileTree, setFileTree] = useState<FileNode[]>(MOCK_FILE_TREE)
  const [selectedPath, setSelectedPath] = useState("/cfg/gamemodes/gamemode_competitive_server.cfg")
  const [fileContent, setFileContent] = useState(DEMO_FILE_CONTENT)
  const [originalContent, setOriginalContent] = useState(DEMO_FILE_CONTENT)
  const [loading, setLoading] = useState(false)

  const hasUnsavedChanges = fileContent !== originalContent

  const selectedFile = selectedPath.split("/").pop() || ""
  const lineCount = fileContent.split("\n").length

  const toggleFolder = (path: string) => {
    const updateTree = (nodes: FileNode[]): FileNode[] => {
      return nodes.map((node) => {
        if (node.path === path && node.type === "folder") {
          return { ...node, expanded: !node.expanded }
        }
        if (node.children) {
          return { ...node, children: updateTree(node.children) }
        }
        return node
      })
    }
    setFileTree(updateTree(fileTree))
  }

  const selectFile = (node: FileNode) => {
    if (node.type === "file") {
      setSelectedPath(node.path)
      // TODO: fetch file content from backend
      setFileContent(DEMO_FILE_CONTENT)
      setOriginalContent(DEMO_FILE_CONTENT)
    }
  }

  const saveFile = async () => {
    setLoading(true)
    try {
      // TODO: implement save to server filesystem
      console.log("Saving file:", selectedPath, fileContent)
      setOriginalContent(fileContent)
    } catch (err) {
      console.error("Failed to save file:", err)
    } finally {
      setLoading(false)
    }
  }

  const reloadFile = () => {
    setFileContent(originalContent)
  }

  const execViaRcon = async () => {
    setLoading(true)
    try {
      const res = await fetch(`/api/servers/${serverId}/rcon`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ command: `exec ${selectedFile}` }),
      })
      if (res.ok) {
        // Success feedback
      }
    } catch (err) {
      console.error("Failed to exec via RCON:", err)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex flex-1 overflow-hidden">
      {/* File Browser Sidebar */}
      <div className="flex w-64 flex-col border-r border-border bg-card">
        <div className="flex items-center justify-between border-b border-border p-3">
          <h2 className="font-semibold">cfg /</h2>
          <Button variant="ghost" size="icon" className="h-7 w-7">
            <Plus className="h-4 w-4" />
          </Button>
        </div>
        <div className="flex-1 overflow-y-auto p-2">
          <FileTreeView
            nodes={fileTree}
            selectedPath={selectedPath}
            onSelect={selectFile}
            onToggle={toggleFolder}
          />
        </div>
      </div>

      {/* Editor Panel */}
      <div className="flex flex-1 flex-col">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-border bg-card p-3">
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <span>/home/cs2/server/csgo{selectedPath}</span>
          </div>
          <div className="flex items-center gap-2">
            {hasUnsavedChanges && (
              <span className="rounded bg-amber-500/10 px-2 py-1 text-xs font-medium text-amber-600">
                unsaved
              </span>
            )}
            <Button variant="outline" size="sm" onClick={reloadFile} disabled={!hasUnsavedChanges}>
              <RefreshCw className="mr-2 h-4 w-4" />
              Reload from disk
            </Button>
            <Button variant="outline" size="sm" onClick={execViaRcon} disabled={loading}>
              <Terminal className="mr-2 h-4 w-4" />
              exec via rcon
            </Button>
            <Button size="sm" onClick={saveFile} disabled={!hasUnsavedChanges || loading}>
              <Save className="mr-2 h-4 w-4" />
              Save & apply
            </Button>
          </div>
        </div>

        {/* Editor */}
        <div className="flex flex-1 overflow-hidden bg-zinc-950">
          {/* Line numbers */}
          <div className="border-r border-zinc-800 bg-zinc-950 px-3 py-4 font-mono text-xs text-zinc-600">
            {Array.from({ length: lineCount }, (_, i) => (
              <div key={i} className="text-right leading-6">
                {i + 1}
              </div>
            ))}
          </div>

          {/* Content */}
          <div className="flex-1 overflow-y-auto">
            <textarea
              value={fileContent}
              onChange={(e) => setFileContent(e.target.value)}
              className="h-full w-full resize-none bg-transparent p-4 font-mono text-sm text-foreground outline-none"
              spellCheck={false}
              style={{ lineHeight: "1.5rem" }}
            />
          </div>
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between border-t border-border bg-card px-4 py-2 text-xs text-muted-foreground">
          <div className="flex items-center gap-4">
            <span>utf-8</span>
            <span>·</span>
            <span>LF</span>
            <span>·</span>
            <span>{lineCount} lines</span>
          </div>
          <div>last edit 12m ago by m0use@</div>
        </div>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// FileTreeView Component
// ---------------------------------------------------------------------------

type FileTreeViewProps = {
  nodes: FileNode[]
  selectedPath: string
  onSelect: (node: FileNode) => void
  onToggle: (path: string) => void
  depth?: number
}

function FileTreeView({ nodes, selectedPath, onSelect, onToggle, depth = 0 }: FileTreeViewProps) {
  return (
    <div className="space-y-0.5">
      {nodes.map((node) => (
        <div key={node.path}>
          <button
            onClick={() => {
              if (node.type === "folder") {
                onToggle(node.path)
              } else {
                onSelect(node)
              }
            }}
            className={cn(
              "flex w-full items-center gap-1.5 rounded px-2 py-1 text-left text-sm transition-colors hover:bg-accent",
              selectedPath === node.path && node.type === "file" && "bg-accent text-accent-foreground"
            )}
            style={{ paddingLeft: `${depth * 12 + 8}px` }}
          >
            {node.type === "folder" ? (
              <>
                {node.expanded ? (
                  <ChevronDown className="h-3 w-3 shrink-0 text-muted-foreground" />
                ) : (
                  <ChevronRight className="h-3 w-3 shrink-0 text-muted-foreground" />
                )}
                <Folder className="h-4 w-4 shrink-0 text-blue-500" />
              </>
            ) : (
              <>
                <div className="w-3" />
                <File className="h-4 w-4 shrink-0 text-muted-foreground" />
              </>
            )}
            <span className="flex-1 truncate">{node.name}</span>
            {node.modified && (
              <span className="shrink-0 text-xs text-muted-foreground">{node.modified}</span>
            )}
          </button>
          {node.type === "folder" && node.expanded && node.children && (
            <FileTreeView
              nodes={node.children}
              selectedPath={selectedPath}
              onSelect={onSelect}
              onToggle={onToggle}
              depth={depth + 1}
            />
          )}
        </div>
      ))}
    </div>
  )
}
