package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	stateCookie    = "defuse_oidc_state"
	verifierCookie = "defuse_oidc_verifier"
	flowCookieTTL  = 10 * time.Minute
)

// oidcProvider holds the discovered provider and OAuth2 client configuration.
type oidcProvider struct {
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth2   oauth2.Config
	logger   *slog.Logger
	// endSessionEndpoint is the provider's RP-initiated logout endpoint, if it
	// advertises one in discovery.
	endSessionEndpoint string
}

// discoveryClaims captures the optional end_session_endpoint from the provider
// discovery document (go-oidc does not expose it directly).
type discoveryClaims struct {
	EndSessionEndpoint string `json:"end_session_endpoint"`
}

func newOIDCProvider(ctx context.Context, cfg Config, logger *slog.Logger) (*oidcProvider, error) {
	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, err
	}
	var dc discoveryClaims
	_ = provider.Claims(&dc)

	return &oidcProvider{
		provider: provider,
		verifier: provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		oauth2: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Endpoint:     provider.Endpoint(),
			Scopes:       cfg.Scopes,
		},
		logger:             logger,
		endSessionEndpoint: dc.EndSessionEndpoint,
	}, nil
}

// endSessionURL builds the RP-initiated logout URL, appending the post-logout
// redirect if configured. Returns "" when the provider has no end-session
// endpoint.
func (p *oidcProvider) endSessionURL(postLogoutRedirect string) string {
	if p.endSessionEndpoint == "" {
		return ""
	}
	u, err := url.Parse(p.endSessionEndpoint)
	if err != nil {
		return ""
	}
	if postLogoutRedirect != "" {
		q := u.Query()
		q.Set("post_logout_redirect_uri", postLogoutRedirect)
		q.Set("client_id", p.oauth2.ClientID)
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// claims is the subset of ID token / userinfo claims we consume.
type claims struct {
	Sub               string   `json:"sub"`
	Email             string   `json:"email"`
	PreferredUsername string   `json:"preferred_username"`
	Name              string   `json:"name"`
	Groups            []string `json:"groups"`
}

func (c claims) username() string {
	switch {
	case c.PreferredUsername != "":
		return c.PreferredUsername
	case c.Name != "":
		return c.Name
	case c.Email != "":
		return c.Email
	default:
		return c.Sub
	}
}

// BeginLogin starts the authorization-code + PKCE flow: it stores a random
// state and PKCE verifier in short-lived cookies and redirects the browser to
// the provider's authorization endpoint.
func (a *Authenticator) BeginLogin(w http.ResponseWriter, r *http.Request) {
	if a.oidc == nil {
		http.Error(w, "oidc not enabled", http.StatusNotFound)
		return
	}
	state, err := randomURLToken()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	verifier := oauth2.GenerateVerifier()

	a.setFlowCookie(w, stateCookie, state)
	a.setFlowCookie(w, verifierCookie, verifier)

	authURL := a.oidc.oauth2.AuthCodeURL(state,
		oauth2.S256ChallengeOption(verifier),
	)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleCallback completes the flow: validates state, exchanges the code for
// tokens, verifies the ID token, extracts identity + groups, creates a session
// and redirects to the app root.
func (a *Authenticator) HandleCallback(w http.ResponseWriter, r *http.Request) {
	if a.oidc == nil {
		http.Error(w, "oidc not enabled", http.StatusNotFound)
		return
	}
	ctx := r.Context()

	// Surface provider-side errors (e.g. access_denied from an ACL rule).
	if e := r.URL.Query().Get("error"); e != "" {
		a.logger.Warn("oidc callback error", "error", e, "description", r.URL.Query().Get("error_description"))
		http.Redirect(w, r, "/login?error=access_denied", http.StatusSeeOther)
		return
	}

	stateCk, err := r.Cookie(stateCookie)
	if err != nil || stateCk.Value == "" || stateCk.Value != r.URL.Query().Get("state") {
		http.Redirect(w, r, "/login?error=state", http.StatusSeeOther)
		return
	}
	verifierCk, err := r.Cookie(verifierCookie)
	if err != nil || verifierCk.Value == "" {
		http.Redirect(w, r, "/login?error=state", http.StatusSeeOther)
		return
	}
	// Flow cookies are single-use.
	a.clearFlowCookie(w, stateCookie)
	a.clearFlowCookie(w, verifierCookie)

	oauth2Token, err := a.oidc.oauth2.Exchange(ctx, r.URL.Query().Get("code"),
		oauth2.VerifierOption(verifierCk.Value),
	)
	if err != nil {
		a.logger.Error("oidc token exchange failed", "error", err)
		http.Redirect(w, r, "/login?error=exchange", http.StatusSeeOther)
		return
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		a.logger.Error("oidc response missing id_token")
		http.Redirect(w, r, "/login?error=no_id_token", http.StatusSeeOther)
		return
	}
	idToken, err := a.oidc.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		a.logger.Error("oidc id_token verification failed", "error", err)
		http.Redirect(w, r, "/login?error=verify", http.StatusSeeOther)
		return
	}

	var cl claims
	if err := idToken.Claims(&cl); err != nil {
		a.logger.Error("oidc claims decode failed", "error", err)
		http.Redirect(w, r, "/login?error=claims", http.StatusSeeOther)
		return
	}

	// The groups claim may be omitted from the ID token (Authelia only includes
	// it when the 'groups' scope is granted and configured). Fall back to the
	// userinfo endpoint to be robust.
	if len(cl.Groups) == 0 {
		if ui, err := a.oidc.provider.UserInfo(ctx, oauth2.StaticTokenSource(oauth2Token)); err == nil {
			var uic claims
			if err := ui.Claims(&uic); err == nil {
				if len(uic.Groups) > 0 {
					cl.Groups = uic.Groups
				}
				if cl.PreferredUsername == "" {
					cl.PreferredUsername = uic.PreferredUsername
				}
				if cl.Email == "" {
					cl.Email = uic.Email
				}
			}
		}
	}

	perms := PermissionsForGroups(cl.Groups)
	if !perms.HasAnyRole() {
		a.logger.Warn("oidc login denied: no recognised role", "sub", cl.Sub, "groups", cl.Groups)
		http.Redirect(w, r, "/login?error=no_access", http.StatusSeeOther)
		return
	}

	groupsJSON, _ := json.Marshal(cl.Groups)
	token, err := randomToken()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := a.store.CreateOIDCSession(token, sessionTTL, cl.Sub, cl.username(), cl.Email, string(groupsJSON)); err != nil {
		a.logger.Error("create oidc session failed", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	a.setSessionCookie(w, token, sessionTTL)
	a.logger.Info("oidc login", "sub", cl.Sub, "user", cl.username(), "roles", perms.Roles())

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *Authenticator) setFlowCookie(w http.ResponseWriter, name, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(flowCookieTTL.Seconds()),
		HttpOnly: true,
		Secure:   a.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *Authenticator) clearFlowCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   a.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func randomURLToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.TrimRight(base64.URLEncoding.EncodeToString(b), "="), nil
}
