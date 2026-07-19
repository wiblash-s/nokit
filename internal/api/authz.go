package api

import (
	"net/http"

	"github.com/codevski/defuse/internal/auth"
)

// RequirePerm wraps a Handler so it only runs when the authenticated identity
// holds the given permission. It returns 403 otherwise. The auth middleware is
// expected to have already attached an Identity to the request context; if none
// is present the request is treated as unauthorized.
func RequirePerm(p auth.Permission, h Handler) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		id, ok := auth.IdentityFrom(r.Context())
		if !ok {
			return Unauthorized("unauthorized")
		}
		if !id.Perms.Has(p) {
			return Forbidden("you do not have permission to perform this action")
		}
		return h(w, r)
	}
}

// actor returns the identity's audit fields (sub, username) for logging, with
// safe fallbacks when no identity is on the context.
func actor(r *http.Request) (sub, username string) {
	if id, ok := auth.IdentityFrom(r.Context()); ok {
		return id.Sub, id.Username
	}
	return "", "unknown"
}
