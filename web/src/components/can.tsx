import { usePermissions } from "@/hooks/auth-context"
import type { Permission } from "@/hooks/useAuth"

type CanProps = {
  /** Permission required to render the children. */
  perm: Permission
  children: React.ReactNode
  /** Optional fallback rendered when the user lacks the permission. */
  fallback?: React.ReactNode
}

/**
 * Can renders its children only when the current user holds `perm`. Use it to
 * hide destructive actions (delete, restart) and admin-only surfaces from users
 * whose group does not grant them.
 */
export function Can({ perm, children, fallback = null }: CanProps) {
  const { can } = usePermissions()
  return <>{can(perm) ? children : fallback}</>
}
