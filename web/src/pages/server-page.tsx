import { useState } from "react"
import { Navigate, useParams } from "react-router-dom"
import { useServers } from "@/hooks/useServers"
import { Header } from "@/components/header"
import { Console } from "@/components/console"
import { DashboardPage } from "@/pages/dashboard-page"

type Tab = "dashboard" | "console"

export function ServerPage() {
  const { id } = useParams<{ id: string }>()
  const state = useServers()
  const [tab, setTab] = useState<Tab>("dashboard")

  if (state.status === "loading") return null
  if (state.status === "error")
    return <div className="p-4 text-destructive">{state.message}</div>

  if (state.servers.length === 0) return <Navigate to="/" replace />

  const server = state.servers.find((s) => s.id === id)
  if (!server) return <Navigate to="/" replace />

  return (
    <div className="flex flex-1 flex-col">
      <Header servers={state.servers} currentId={id!} onRefresh={state.refresh} />

      {/* tab bar */}
      <div className="border-b border-border">
        <div className="mx-auto flex max-w-5xl gap-1 px-4">
          {(["dashboard", "console"] as Tab[]).map((t) => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={`-mb-px border-b-2 px-3 py-2.5 text-sm font-medium capitalize transition-colors ${
                tab === t
                  ? "border-primary text-foreground"
                  : "border-transparent text-muted-foreground hover:text-foreground"
              }`}
            >
              {t}
            </button>
          ))}
        </div>
      </div>

      <main className="mx-auto w-full max-w-5xl flex-1 p-4">
        {tab === "dashboard" ? (
          <DashboardPage
            key={id}
            serverId={id!}
            serverName={server.name}
            host={server.host}
            onOpenConsole={() => setTab("console")}
          />
        ) : (
          <Console serverId={id!} />
        )}
      </main>
    </div>
  )
}
