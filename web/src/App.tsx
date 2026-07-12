import { BrowserRouter, Navigate, Route, Routes, useParams } from "react-router-dom"
import { useAuth } from "@/hooks/useAuth"
import { useServers } from "@/hooks/useServers"
import { LoginPage } from "@/pages/login-page"
import { ServerPage } from "@/pages/server-page"
import { DashboardPage } from "@/pages/dashboard-page"
import { ConfigEditorPage } from "@/pages/config-editor-page"
import { MapsPage } from "@/pages/maps-page"
import { PlayersPage } from "@/pages/players-page"
import { Console } from "@/components/console"
import { LogsPanel } from "@/components/logs"
import { Header } from "./components/header"
import { Layout } from "./components/layout"
import { Sidebar } from "./components/sidebar"
import { Footer } from "./components/footer"

function RequireAuth({ children }: { children: React.ReactNode }) {
  const auth = useAuth()
  if (auth.status === "loading") return null
  if (auth.status === "unauthenticated") {
    return <Navigate to="/login" replace />
  }
  return <>{children}</>
}

function RootRedirect() {
  const state = useServers()
  if (state.status === "loading") return null
  if (state.status === "error")
    return <div className="p-4 text-destructive">{state.message}</div>
  if (state.servers.length === 0)
    return (
      <div className="flex flex-1 flex-col">
        <Header servers={[]} currentId="" onRefresh={state.refresh} />
        <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
          no servers configured - add one using the switcher above
        </div>
      </div>
    )
  return <Navigate to={`/servers/${state.servers[0].id}/dashboard`} replace />
}

function PlaceholderPage({ title }: { title: string }) {
  return (
    <div className="flex flex-1 items-center justify-center p-6">
      <div className="text-center">
        <h1 className="text-lg font-semibold text-muted-foreground">
          {title}
        </h1>
        <p className="mt-2 text-sm text-muted-foreground">Coming soon</p>
      </div>
    </div>
  )
}

function DashboardRoute() {
  const { id } = useParams<{ id: string }>()
  const state = useServers()
  if (state.status !== "ready") return null
  const server = state.servers.find((s) => s.id === id)
  if (!server) return null
  return (
    <DashboardPage
      serverId={id!}
      serverName={server.name}
      host={server.host}
    />
  )
}

function ConsoleRoute() {
  const { id } = useParams<{ id: string }>()
  return <Console serverId={id!} />
}

function ConfigEditorRoute() {
  const { id } = useParams<{ id: string }>()
  return <ConfigEditorPage serverId={id!} />
}

function MapsRoute() {
  const { id } = useParams<{ id: string }>()
  return <MapsPage serverId={id!} />
}

function PlayersRoute() {
  const { id } = useParams<{ id: string }>()
  return <PlayersPage serverId={id!} />
}

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<Layout />}>
          <Route index element={<LoginPage />} />
        </Route>
        
        <Route
          path="/"
          element={
            <RequireAuth>
              <Layout />
            </RequireAuth>
          }
        >
          <Route index element={<RootRedirect />} />
        </Route>

        <Route
          path="/servers/:id"
          element={
            <RequireAuth>
              <ServerLayoutWrapper />
            </RequireAuth>
          }
        >
          <Route index element={<Navigate to="dashboard" replace />} />
          <Route path="dashboard" element={<DashboardRoute />} />
          <Route path="console" element={<ConsoleRoute />} />
          <Route path="logs" element={<LogsPanel />} />
          <Route path="players" element={<PlayersRoute />} />
          <Route path="maps" element={<MapsRoute />} />
          <Route path="presets" element={<PlaceholderPage title="CVAR Presets" />} />
          <Route path="config" element={<ConfigEditorRoute />} />
          <Route path="plugins" element={<PlaceholderPage title="Plugins" />} />
          <Route path="scheduler" element={<PlaceholderPage title="Scheduler" />} />
          <Route path="admin" element={<PlaceholderPage title="Admin" />} />
        </Route>

        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  )
}

function ServerLayoutWrapper() {
  const { id } = useParams<{ id: string }>()
  if (!id) return <Navigate to="/" replace />
  return (
    <>
      <Sidebar currentServerId={id} />
      <div className="ml-56 flex min-h-screen flex-1 flex-col">
        <ServerPage />
        <Footer />
      </div>
    </>
  )
}
