package auth

import "sort"

// Permission is a single capability a user may hold. Permissions are derived
// from the OIDC groups claim via the role hierarchy below.
type Permission string

const (
	// Read-only capabilities (viewer and up).
	PermViewDashboard Permission = "view_dashboard"
	PermViewPlayers   Permission = "view_players"
	PermViewLogs      Permission = "view_logs"

	// Operator capabilities.
	PermSendConsoleCommand Permission = "send_console_command"
	PermKickPlayer         Permission = "kick_player"

	// Admin capabilities.
	PermBanPlayer      Permission = "ban_player"
	PermUnbanPlayer    Permission = "unban_player"
	PermManageWorkshop Permission = "manage_workshop"
	PermEditConfig     Permission = "edit_config"
	PermExecConfig     Permission = "exec_config"
	PermAddServer      Permission = "add_server"
	PermEditServer     Permission = "edit_server"

	// Superadmin capabilities.
	PermDeleteServer Permission = "delete_server"
	PermDeleteConfig Permission = "delete_config"
	PermViewAudit    Permission = "view_audit"
)

// Role names correspond 1:1 to the Authelia/lldap groups an operator creates.
// They are additive: each higher role includes every permission of the roles
// below it.
const (
	RoleViewer     = "cs2-rcon-viewer"
	RoleOperator   = "cs2-rcon-operator"
	RoleAdmin      = "cs2-rcon-admin"
	RoleSuperadmin = "cs2-rcon-superadmin"
)

// rolePermissions lists the permissions granted directly by each role, before
// hierarchical inheritance is applied.
var rolePermissions = map[string][]Permission{
	RoleViewer: {
		PermViewDashboard,
		PermViewPlayers,
		PermViewLogs,
	},
	RoleOperator: {
		PermSendConsoleCommand,
		PermKickPlayer,
	},
	RoleAdmin: {
		PermBanPlayer,
		PermUnbanPlayer,
		PermManageWorkshop,
		PermEditConfig,
		PermExecConfig,
		PermAddServer,
		PermEditServer,
	},
	RoleSuperadmin: {
		PermDeleteServer,
		PermDeleteConfig,
		PermViewAudit,
	},
}

// roleOrder defines the inheritance chain, lowest privilege first. Holding a
// role grants every permission of it and all preceding roles.
var roleOrder = []string{RoleViewer, RoleOperator, RoleAdmin, RoleSuperadmin}

// PermissionSet is the resolved set of permissions for an authenticated user.
type PermissionSet struct {
	perms map[Permission]struct{}
	roles map[string]struct{}
}

// PermissionsForGroups resolves a raw OIDC groups claim into a PermissionSet,
// applying the additive role hierarchy. Unknown groups are ignored.
func PermissionsForGroups(groups []string) PermissionSet {
	ps := PermissionSet{
		perms: make(map[Permission]struct{}),
		roles: make(map[string]struct{}),
	}
	held := make(map[string]bool)
	for _, g := range groups {
		held[g] = true
	}
	// Determine the highest role the user holds, then grant that role and all
	// roles below it in the hierarchy.
	highest := -1
	for i, role := range roleOrder {
		if held[role] {
			highest = i
		}
	}
	for i := 0; i <= highest; i++ {
		role := roleOrder[i]
		ps.roles[role] = struct{}{}
		for _, p := range rolePermissions[role] {
			ps.perms[p] = struct{}{}
		}
	}
	return ps
}

// AllPermissions returns a PermissionSet holding every permission. Used for the
// single-user local auth mode, where the one account is fully privileged.
func AllPermissions() PermissionSet {
	ps := PermissionSet{
		perms: make(map[Permission]struct{}),
		roles: make(map[string]struct{}),
	}
	for _, role := range roleOrder {
		ps.roles[role] = struct{}{}
		for _, p := range rolePermissions[role] {
			ps.perms[p] = struct{}{}
		}
	}
	return ps
}

// Has reports whether the set contains the given permission.
func (ps PermissionSet) Has(p Permission) bool {
	if ps.perms == nil {
		return false
	}
	_, ok := ps.perms[p]
	return ok
}

// HasAnyRole reports whether the user holds at least one recognised role, i.e.
// whether they are allowed into the app at all.
func (ps PermissionSet) HasAnyRole() bool {
	return len(ps.roles) > 0
}

// List returns the granted permissions as a sorted string slice, suitable for
// serialising to the frontend.
func (ps PermissionSet) List() []string {
	out := make([]string, 0, len(ps.perms))
	for p := range ps.perms {
		out = append(out, string(p))
	}
	sort.Strings(out)
	return out
}

// Map returns the permissions as a map keyed by permission name for JSON
// responses the frontend can index into (e.g. permissions.delete_server).
func (ps PermissionSet) Map() map[string]bool {
	// Enumerate every known permission so the frontend always receives a full,
	// explicit shape rather than only the granted keys.
	all := []Permission{
		PermViewDashboard, PermViewPlayers, PermViewLogs,
		PermSendConsoleCommand, PermKickPlayer,
		PermBanPlayer, PermUnbanPlayer, PermManageWorkshop, PermEditConfig, PermExecConfig, PermAddServer, PermEditServer,
		PermDeleteServer, PermDeleteConfig, PermViewAudit,
	}
	m := make(map[string]bool, len(all))
	for _, p := range all {
		m[string(p)] = ps.Has(p)
	}
	return m
}

// Roles returns the granted role names as a sorted slice.
func (ps PermissionSet) Roles() []string {
	out := make([]string, 0, len(ps.roles))
	for r := range ps.roles {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}
