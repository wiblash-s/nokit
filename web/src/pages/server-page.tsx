import { Navigate, Outlet, useParams } from "react-router-dom"
import { useServers } from "@/hooks/useServers"
import { useServerStats } from "@/hooks/useServerStats"
import { Header } from "@/components/header"

export function ServerPage() {
  const { id } = useParams<{ id: string }>()
  const state = useServers()
  const { data } = useServerStats(id!)

  if (state.status === "loading") return null
  if (state.status === "error")
    return <div className="p-4 text-destructive">{state.message}</div>

  if (state.servers.length === 0) return <Navigate to="/" replace />

  const server = state.servers.find((s) => s.id === id)
  if (!server) return <Navigate to="/" replace />

  return (
    <div className="flex flex-1 flex-col">
      <Header
        servers={state.servers}
        currentId={id!}
        onRefresh={state.refresh}
        tickrate={data?.tick}
        players={data?.players}
        maxPlayers={data?.maxPlayers}
        map={data?.map}
      />
      <main className="flex flex-1 flex-col">
        <Outlet />
      </main>
    </div>
  )
}
