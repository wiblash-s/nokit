import { Outlet } from "react-router-dom"
import { Footer } from "@/components/footer"
import { Sidebar } from "@/components/sidebar"

type Props = {
  showSidebar?: boolean
  currentServerId?: string
}

export function Layout({ showSidebar, currentServerId }: Props) {
  return (
    <div className="flex min-h-screen">
      {showSidebar && currentServerId && (
        <Sidebar currentServerId={currentServerId} />
      )}
      <div className={`flex min-h-screen flex-1 flex-col ${showSidebar ? "ml-56" : ""}`}>
        <main className="flex flex-1 flex-col">
          <Outlet />
        </main>
        <Footer />
      </div>
    </div>
  )
}
