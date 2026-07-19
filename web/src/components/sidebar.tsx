import { NavLink } from "react-router-dom"
import {
  LayoutDashboard,
  Terminal,
  ScrollText,
  Users,
  Map,
  ListChecks,
  FileCode,
  Puzzle,
  Clock,
  Shield,
} from "lucide-react"
import { usePermissions } from "@/hooks/auth-context"
import type { Permission } from "@/hooks/useAuth"

type NavItem = {
  to: string
  icon: React.ElementType
  label: string
  /** Permission required to see this item. Omit for items everyone with a role sees. */
  perm?: Permission
}

type NavSection = {
  title: string
  items: NavItem[]
}

const NAV_SECTIONS: NavSection[] = [
  {
    title: "SERVER",
    items: [
      { to: "dashboard", icon: LayoutDashboard, label: "Dashboard", perm: "view_dashboard" },
      { to: "console", icon: Terminal, label: "RCON Console", perm: "send_console_command" },
      { to: "logs", icon: ScrollText, label: "Live Logs", perm: "view_logs" },
      { to: "players", icon: Users, label: "Players", perm: "view_players" },
    ],
  },
  {
    title: "CONFIGURATION",
    items: [
      { to: "maps", icon: Map, label: "Maps", perm: "view_dashboard" },
      { to: "presets", icon: ListChecks, label: "CVAR Presets", perm: "edit_config" },
      { to: "config", icon: FileCode, label: "Config Editor", perm: "edit_config" },
    ],
  },
  {
    title: "SYSTEM",
    items: [
      { to: "plugins", icon: Puzzle, label: "Plugins", perm: "edit_config" },
      { to: "scheduler", icon: Clock, label: "Scheduler", perm: "edit_config" },
      { to: "admin", icon: Shield, label: "Admin", perm: "view_audit" },
    ],
  },
]

export function Sidebar({ currentServerId }: { currentServerId: string }) {
  const { can } = usePermissions()
  const sections = NAV_SECTIONS.map((section) => ({
    ...section,
    items: section.items.filter((item) => !item.perm || can(item.perm)),
  })).filter((section) => section.items.length > 0)

  return (
    <aside className="fixed left-0 top-0 z-30 flex h-screen w-56 flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground">
      {/* Logo */}
      <div className="flex h-14 items-center border-b border-sidebar-border px-4">
        <div className="flex items-center gap-2">
          <div className="flex h-7 w-7 items-center justify-center rounded bg-primary text-primary-foreground font-bold text-sm">
            n
          </div>
          <span className="font-semibold text-base tracking-tight">nokit</span>
        </div>
      </div>

      {/* Navigation */}
      <nav className="flex-1 space-y-6 overflow-y-auto py-4">
        {sections.map((section) => (
          <div key={section.title} className="px-3">
            <div className="mb-2 px-3 text-xs font-medium uppercase tracking-wider text-muted-foreground">
              {section.title}
            </div>
            <div className="space-y-1">
              {section.items.map((item) => (
                <NavLink
                  key={item.to}
                  to={`/servers/${currentServerId}/${item.to}`}
                  className={({ isActive }) =>
                    `flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors ${
                      isActive
                        ? "bg-sidebar-accent text-sidebar-primary border-l-4 border-primary pl-[8px]"
                        : "text-sidebar-foreground hover:bg-sidebar-accent/50"
                    }`
                  }
                >
                  <item.icon className="h-4 w-4 flex-shrink-0" />
                  <span>{item.label}</span>
                </NavLink>
              ))}
            </div>
          </div>
        ))}
      </nav>

      {/* Footer */}
      <div className="border-t border-sidebar-border px-4 py-3">
        <div className="space-y-0.5 text-xs text-muted-foreground">
          <div>relay v0.4.2</div>
        </div>
      </div>
    </aside>
  )
}
