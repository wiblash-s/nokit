package auth

import "testing"

func TestPermissionsForGroups_Hierarchy(t *testing.T) {
	cases := []struct {
		name       string
		groups     []string
		wantHas    []Permission
		wantHasNot []Permission
		wantRole   bool
	}{
		{
			name:       "viewer",
			groups:     []string{RoleViewer},
			wantHas:    []Permission{PermViewDashboard, PermViewPlayers, PermViewLogs},
			wantHasNot: []Permission{PermSendConsoleCommand, PermDeleteServer, PermEditConfig},
			wantRole:   true,
		},
		{
			name:       "operator inherits viewer",
			groups:     []string{RoleOperator},
			wantHas:    []Permission{PermViewDashboard, PermSendConsoleCommand, PermKickPlayer},
			wantHasNot: []Permission{PermEditConfig, PermDeleteServer},
			wantRole:   true,
		},
		{
			name:       "admin inherits operator+viewer",
			groups:     []string{RoleAdmin},
			wantHas:    []Permission{PermViewLogs, PermSendConsoleCommand, PermEditConfig, PermAddServer},
			wantHasNot: []Permission{PermDeleteServer, PermDeleteConfig, PermViewAudit},
			wantRole:   true,
		},
		{
			name:       "superadmin has everything",
			groups:     []string{RoleSuperadmin},
			wantHas:    []Permission{PermViewDashboard, PermSendConsoleCommand, PermEditConfig, PermDeleteServer, PermDeleteConfig, PermViewAudit},
			wantHasNot: []Permission{},
			wantRole:   true,
		},
		{
			name:       "highest role wins even if only superadmin present",
			groups:     []string{"unrelated", RoleSuperadmin},
			wantHas:    []Permission{PermViewDashboard, PermDeleteServer},
			wantHasNot: []Permission{},
			wantRole:   true,
		},
		{
			name:       "no recognised group grants nothing",
			groups:     []string{"some-other-app-group"},
			wantHas:    []Permission{},
			wantHasNot: []Permission{PermViewDashboard, PermDeleteServer},
			wantRole:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ps := PermissionsForGroups(tc.groups)
			for _, p := range tc.wantHas {
				if !ps.Has(p) {
					t.Errorf("expected permission %q to be granted", p)
				}
			}
			for _, p := range tc.wantHasNot {
				if ps.Has(p) {
					t.Errorf("expected permission %q NOT to be granted", p)
				}
			}
			if ps.HasAnyRole() != tc.wantRole {
				t.Errorf("HasAnyRole = %v, want %v", ps.HasAnyRole(), tc.wantRole)
			}
		})
	}
}

func TestAllPermissions(t *testing.T) {
	ps := AllPermissions()
	for _, p := range []Permission{PermViewDashboard, PermSendConsoleCommand, PermEditConfig, PermDeleteServer, PermViewAudit} {
		if !ps.Has(p) {
			t.Errorf("AllPermissions missing %q", p)
		}
	}
	if !ps.HasAnyRole() {
		t.Error("AllPermissions should report HasAnyRole")
	}
}

func TestPermissionMapIsComplete(t *testing.T) {
	m := PermissionsForGroups([]string{RoleViewer}).Map()
	// The map must always contain every permission key, granted or not.
	for _, key := range []string{
		string(PermViewDashboard), string(PermSendConsoleCommand),
		string(PermDeleteServer), string(PermViewAudit),
	} {
		if _, ok := m[key]; !ok {
			t.Errorf("permission map missing key %q", key)
		}
	}
	if m[string(PermDeleteServer)] {
		t.Error("viewer should not have delete_server in map")
	}
}
