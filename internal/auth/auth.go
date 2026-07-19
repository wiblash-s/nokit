// Package auth implements panel authentication. It supports two modes:
//
//   - local: a single username/password account (legacy default).
//   - oidc:  delegated auth against an OIDC provider such as Authelia, with
//     authorization derived from the provider's groups claim.
//
// In both modes a server-side session is stored in the database and referenced
// by an HttpOnly cookie. The middleware resolves that session into an Identity
// (including a resolved PermissionSet) and attaches it to the request context.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/codevski/defuse/internal/store"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookie = "defuse_session"
	sessionTTL    = 30 * 24 * time.Hour // 30 days
)

// Credentials holds the single local-mode account.
type Credentials struct {
	username     string
	passwordHash []byte
}

// Store is the persistence contract the authenticator needs.
type Store interface {
	CreateLocalSession(token string, ttl time.Duration) error
	CreateOIDCSession(token string, ttl time.Duration, sub, username, email, groupsJSON string) error
	SessionByToken(token string) (store.SessionInfo, bool, error)
	DeleteSession(token string) error
}

// LoadCredentials reads the local-mode account from PANEL_USERNAME/PANEL_PASSWORD.
func LoadCredentials() (Credentials, error) {
	u := os.Getenv("PANEL_USERNAME")
	p := os.Getenv("PANEL_PASSWORD")
	if u == "" {
		return Credentials{}, errors.New("PANEL_USERNAME env var is required")
	}
	if p == "" {
		return Credentials{}, errors.New("PANEL_PASSWORD env var is required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	if err != nil {
		return Credentials{}, err
	}
	return Credentials{username: u, passwordHash: hash}, nil
}

// Verify checks a username/password pair against the local account.
func (c Credentials) Verify(username, password string) bool {
	if username != c.username || len(c.passwordHash) == 0 {
		return false
	}
	return bcrypt.CompareHashAndPassword(c.passwordHash, []byte(password)) == nil
}

// Authenticator ties the auth configuration, session store and (for OIDC mode)
// the provider together, and exposes HTTP middleware and flow handlers.
type Authenticator struct {
	cfg    Config
	store  Store
	logger *slog.Logger
	oidc   *oidcProvider // nil in local mode
}

// New constructs an Authenticator for the given configuration. For OIDC mode it
// performs provider discovery against the issuer, which requires the issuer to
// be reachable at startup.
func New(ctx context.Context, cfg Config, st Store, logger *slog.Logger) (*Authenticator, error) {
	a := &Authenticator{cfg: cfg, store: st, logger: logger}
	if cfg.Mode == ModeOIDC {
		p, err := newOIDCProvider(ctx, cfg, logger)
		if err != nil {
			return nil, err
		}
		a.oidc = p
	}
	return a, nil
}

// Mode reports the active authentication mode.
func (a *Authenticator) Mode() Mode { return a.cfg.Mode }

// LocalCreds exposes the local account for the local login handler.
func (a *Authenticator) LocalCreds() Credentials { return a.cfg.Local }

// LoginLocal establishes a session for the single local account.
func (a *Authenticator) LoginLocal(w http.ResponseWriter) error {
	token, err := randomToken()
	if err != nil {
		return err
	}
	if err := a.store.CreateLocalSession(token, sessionTTL); err != nil {
		return err
	}
	a.setSessionCookie(w, token, sessionTTL)
	return nil
}

// Logout clears the local session. For OIDC it also returns the provider's
// end-session URL (if available) so the caller can redirect the browser there.
func (a *Authenticator) Logout(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(sessionCookie); err == nil {
		_ = a.store.DeleteSession(c.Value)
	}
	a.clearSessionCookie(w)
	if a.oidc != nil {
		return a.oidc.endSessionURL(a.cfg.PostLogoutRedirectURL)
	}
	return ""
}

// identityFor resolves a session token into an Identity. found is false when the
// session is unknown or expired.
func (a *Authenticator) identityFor(token string) (Identity, bool, error) {
	info, ok, err := a.store.SessionByToken(token)
	if err != nil || !ok {
		return Identity{}, false, err
	}
	if info.IsLocal {
		// The single local account is fully privileged.
		return Identity{
			Username: "local",
			IsLocal:  true,
			Perms:    AllPermissions(),
		}, true, nil
	}
	var groups []string
	if info.Groups != "" {
		_ = json.Unmarshal([]byte(info.Groups), &groups)
	}
	return Identity{
		Sub:      info.Sub,
		Username: info.Username,
		Email:    info.Email,
		Groups:   groups,
		Perms:    PermissionsForGroups(groups),
	}, true, nil
}

// Middleware authenticates the request, attaches the resolved Identity to the
// context, and rejects unauthenticated requests. API paths get a 401 JSON body;
// everything else is redirected to the login page.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := a.authenticate(r)
		if !ok {
			a.reject(w, r)
			return
		}
		// A user with a valid session but no recognised role is authenticated but
		// unauthorized for the whole app.
		if !id.IsLocal && !id.Perms.HasAnyRole() {
			a.forbidNoRole(w, r)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
	})
}

func (a *Authenticator) authenticate(r *http.Request) (Identity, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return Identity{}, false
	}
	id, ok, err := a.identityFor(c.Value)
	if err != nil {
		a.logger.Error("session lookup failed", "error", err)
		return Identity{}, false
	}
	return id, ok
}

func isAPIPath(p string) bool {
	return len(p) >= 5 && p[:5] == "/api/"
}

func (a *Authenticator) reject(w http.ResponseWriter, r *http.Request) {
	if isAPIPath(r.URL.Path) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (a *Authenticator) forbidNoRole(w http.ResponseWriter, r *http.Request) {
	if isAPIPath(r.URL.Path) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"no_access","message":"your account has no panel role assigned"}`))
		return
	}
	http.Redirect(w, r, "/login?error=no_access", http.StatusSeeOther)
}

func (a *Authenticator) setSessionCookie(w http.ResponseWriter, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   a.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *Authenticator) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   a.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
