package auth

import "context"

// Identity is the authenticated principal for a request, attached to the
// request context by the auth middleware.
type Identity struct {
	Sub      string
	Username string
	Email    string
	Groups   []string
	Perms    PermissionSet
	// IsLocal is true for the single-user local auth mode account.
	IsLocal bool
}

type ctxKey struct{}

// WithIdentity returns a copy of ctx carrying the given identity.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// IdentityFrom extracts the identity from ctx. ok is false when the request is
// unauthenticated (should not happen behind the auth middleware).
func IdentityFrom(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}

// Can reports whether the identity in ctx holds the given permission.
func Can(ctx context.Context, p Permission) bool {
	id, ok := IdentityFrom(ctx)
	if !ok {
		return false
	}
	return id.Perms.Has(p)
}
