package api

import (
	"net/http"

	"github.com/codevski/defuse/internal/auth"
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AuthConfigHandler is public. It tells the frontend which auth mode is active
// so the login page can render either the local form or the "Sign in with SSO"
// button, without exposing anything sensitive.
func AuthConfigHandler(a *auth.Authenticator) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		return JSON(w, http.StatusOK, map[string]any{
			"mode": string(a.Mode()),
		})
	}
}

// LoginHandler handles local (username/password) login. In OIDC mode it is
// disabled — clients must use the OIDC redirect flow.
func LoginHandler(a *auth.Authenticator) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		if a.Mode() != auth.ModeLocal {
			return Forbidden("password login is disabled; use single sign-on")
		}
		var req LoginRequest
		if err := Decode(r, &req); err != nil {
			return err
		}
		if !a.LocalCreds().Verify(req.Username, req.Password) {
			return Unauthorized("invalid credentials")
		}
		if err := a.LoginLocal(w); err != nil {
			return WrapHTTP(err, http.StatusInternalServerError, "session error")
		}
		return JSON(w, http.StatusOK, map[string]string{"ok": "true"})
	}
}

// LogoutHandler clears the session. It returns a redirect URL when the provider
// supports RP-initiated logout so the frontend can complete SSO logout.
func LogoutHandler(a *auth.Authenticator) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		redirect := a.Logout(w, r)
		return JSON(w, http.StatusOK, map[string]string{"ok": "true", "redirect": redirect})
	}
}

// MeHandler returns the current identity and resolved permissions. Protected by
// the auth middleware, so an Identity is always present.
func MeHandler() Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		id, ok := auth.IdentityFrom(r.Context())
		if !ok {
			return Unauthorized("unauthorized")
		}
		return JSON(w, http.StatusOK, map[string]any{
			"authenticated": true,
			"username":      id.Username,
			"email":         id.Email,
			"groups":        id.Groups,
			"roles":         id.Perms.Roles(),
			"isLocal":       id.IsLocal,
			"permissions":   id.Perms.Map(),
		})
	}
}
