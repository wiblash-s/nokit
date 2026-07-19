# lldap Groups Setup for nokit CS2 RCON Panel

This guide shows you how to create the required groups in lldap for nokit's permission system.

## Required Groups

Create these **4 groups** in lldap. The group names must match exactly (case-sensitive):

| Group Name | Role | Description |
|------------|------|-------------|
| `cs2-rcon-viewer` | Viewer | Read-only access: dashboard, players, logs |
| `cs2-rcon-operator` | Operator | Viewer + send console commands, unban, workshop, config edit/exec |
| `cs2-rcon-admin` | Admin | Operator + add servers, view audit log |
| `cs2-rcon-superadmin` | Superadmin | Admin + **delete** servers and configs |

## Step-by-Step: Creating Groups in lldap

### 1. Access lldap Web UI

Navigate to your lldap instance (usually `http://localhost:17170` or `https://ldap.example.com`)

**Login** with your admin credentials:
- Username: `admin`
- Password: (your `LLDAP_LDAP_USER_PASS`)

### 2. Create Each Group

For each of the 4 groups above:

1. Click **"Groups"** in the left sidebar
2. Click **"Create group"** button
3. Fill in the form:
   - **Display name**: `cs2-rcon-viewer` (use exact names from table above)
   - **Description**: *(optional)* e.g., "CS2 RCON Panel - Read-only access"
4. Click **"Create group"**

Repeat for all 4 groups:
- `cs2-rcon-viewer`
- `cs2-rcon-operator`
- `cs2-rcon-admin`
- `cs2-rcon-superadmin`

### 3. Assign Users to Groups

#### Important: Role Hierarchy

The permission system uses **additive inheritance**:
- A user in `cs2-rcon-operator` also gets `cs2-rcon-viewer` permissions
- A user in `cs2-rcon-admin` gets `operator` + `viewer` permissions
- A user in `cs2-rcon-superadmin` gets **all** permissions

**Best practice**: Only assign users to their **highest needed role**. The panel automatically grants lower-level permissions.

#### Assign a User

1. Click **"Users"** in the left sidebar
2. Click on the username you want to assign
3. Scroll down to **"Member of groups"**
4. Click **"Add to group"**
5. Select the appropriate group (e.g., `cs2-rcon-admin`)
6. Click **"Add"**

### 4. Verify Group Membership

To verify a user has the correct groups:

1. Go to **Users** → select the user
2. Check the **"Member of groups"** section shows the expected group(s)

Alternatively, go to **Groups** → select a group → see the **"Members"** list

## Example User Assignments

| User | Group(s) | What They Can Do |
|------|----------|------------------|
| `alice` | `cs2-rcon-superadmin` | Everything (full admin) |
| `bob` | `cs2-rcon-admin` | Add servers, view audit, manage configs, RCON commands (but cannot delete) |
| `charlie` | `cs2-rcon-operator` | Send RCON commands, manage workshop, edit configs (read-only for servers) |
| `dana` | `cs2-rcon-viewer` | View-only access to dashboard, players, logs |
| `eve` | *(no cs2-rcon groups)* | **Denied access** (403 error on login) |

## Troubleshooting

### User can log in but gets "Access Denied"

**Cause**: User is not a member of any `cs2-rcon-*` group.

**Fix**: Add the user to at least `cs2-rcon-viewer` in lldap.

### User doesn't see permissions they should have

**Causes**:
1. **Group name typo**: Group must be exactly `cs2-rcon-viewer` (not `cs2_rcon_viewer`, `CS2-RCON-VIEWER`, etc.)
2. **Authelia not configured**: The group must be included in Authelia's `access_control` rules AND the OIDC client's `scopes` must include `groups`
3. **Cache**: Log out and log back in to refresh the token

**Fix**: 
1. Verify group names match exactly
2. Check Authelia config has the group in `access_control.rules[].subject`
3. Verify Authelia LDAP `groups_filter` is working:
   ```yaml
   groups_filter: '(member={dn})'
   group_name_attribute: 'cn'
   additional_groups_dn: 'ou=groups'
   ```

### How to test group membership is working

1. **Check lldap**: User → "Member of groups" should show the group
2. **Check Authelia logs**: After login, Authelia logs should show group membership
3. **Check nokit**: 
   - Log in to the panel
   - Open browser DevTools → Network tab
   - Refresh the page
   - Check the response from `GET /api/me`
   - The `groups` field should contain your lldap group names:
     ```json
     {
       "username": "alice",
       "email": "alice@example.com",
       "groups": ["cs2-rcon-superadmin", "lldap_admin"],
       "roles": ["superadmin"],
       "isLocal": false,
       "permissions": {
         "view_dashboard": true,
         "send_console_command": true,
         ...
       }
     }
     ```

## Advanced: Custom Group Names

If you want to use different group names (e.g., `rcon-users` instead of `cs2-rcon-viewer`), you need to:

1. **Modify the panel code**: Edit `/home/ubuntu/nokit/internal/auth/permissions.go`:
   ```go
   const (
       GroupViewer    = "rcon-users"        // was "cs2-rcon-viewer"
       GroupOperator  = "rcon-operators"    // was "cs2-rcon-operator"
       // ... etc
   )
   ```
2. **Rebuild the panel**: `cd /home/ubuntu/nokit && go build ./cmd/defuse`
3. **Update Authelia**: Modify `access_control.rules[].subject` to use your custom group names
4. **Create groups in lldap** with your custom names

**Not recommended** unless you have a strong reason — keeping the defaults makes setup easier.

## Next Steps

After creating groups and assigning users:

1. ✅ **lldap groups created** (you are here)
2. ⬜ **Configure Authelia** → see `docs/authelia-config-example.yml`
3. ⬜ **Configure nokit env vars** → see `docs/docker-compose.oidc-example.yml`
4. ⬜ **Test login flow** → access your panel, click "Sign in with SSO"
