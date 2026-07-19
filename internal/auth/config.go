package auth

import (
	"errors"
	"os"
	"strings"
)

// Mode selects how the panel authenticates users.
type Mode string

const (
	// ModeLocal is the legacy single-user username/password mode.
	ModeLocal Mode = "local"
	// ModeOIDC delegates authentication to an OIDC provider (e.g. Authelia).
	ModeOIDC Mode = "oidc"
)

// Config is the resolved authentication configuration loaded from the
// environment.
type Config struct {
	Mode Mode

	// Local mode credentials (only used when Mode == ModeLocal).
	Local Credentials

	// OIDC settings (only used when Mode == ModeOIDC).
	IssuerURL    string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	// Scopes requested from the provider. Defaults to openid, profile, email,
	// groups.
	Scopes []string
	// PostLogoutRedirectURL is where the provider sends the user after
	// RP-initiated logout. Defaults to the app root derived from RedirectURL.
	PostLogoutRedirectURL string
	// CookieSecure controls the Secure flag on session/state cookies. Should be
	// true in production (HTTPS). Defaults to true unless AUTH_COOKIE_INSECURE=1.
	CookieSecure bool
}

// LoadConfig reads the authentication configuration from the environment.
//
// AUTH_MODE selects the mode (defaults to "local" for backwards compatibility).
// In local mode PANEL_USERNAME/PANEL_PASSWORD are required. In oidc mode the
// OIDC_* variables are required.
func LoadConfig() (Config, error) {
	mode := Mode(strings.ToLower(strings.TrimSpace(os.Getenv("AUTH_MODE"))))
	if mode == "" {
		mode = ModeLocal
	}

	cfg := Config{
		Mode:         mode,
		CookieSecure: os.Getenv("AUTH_COOKIE_INSECURE") != "1",
	}

	switch mode {
	case ModeLocal:
		creds, err := LoadCredentials()
		if err != nil {
			return Config{}, err
		}
		cfg.Local = creds
		return cfg, nil

	case ModeOIDC:
		cfg.IssuerURL = strings.TrimSpace(os.Getenv("OIDC_ISSUER_URL"))
		cfg.ClientID = strings.TrimSpace(os.Getenv("OIDC_CLIENT_ID"))
		cfg.ClientSecret = strings.TrimSpace(os.Getenv("OIDC_CLIENT_SECRET"))
		cfg.RedirectURL = strings.TrimSpace(os.Getenv("OIDC_REDIRECT_URL"))
		cfg.PostLogoutRedirectURL = strings.TrimSpace(os.Getenv("OIDC_POST_LOGOUT_REDIRECT_URL"))

		if cfg.IssuerURL == "" {
			return Config{}, errors.New("OIDC_ISSUER_URL is required when AUTH_MODE=oidc")
		}
		if cfg.ClientID == "" {
			return Config{}, errors.New("OIDC_CLIENT_ID is required when AUTH_MODE=oidc")
		}
		if cfg.ClientSecret == "" {
			return Config{}, errors.New("OIDC_CLIENT_SECRET is required when AUTH_MODE=oidc")
		}
		if cfg.RedirectURL == "" {
			return Config{}, errors.New("OIDC_REDIRECT_URL is required when AUTH_MODE=oidc (e.g. https://rcon.example.com/api/auth/callback)")
		}

		if scopes := strings.TrimSpace(os.Getenv("OIDC_SCOPES")); scopes != "" {
			cfg.Scopes = strings.Fields(strings.ReplaceAll(scopes, ",", " "))
		} else {
			cfg.Scopes = []string{"openid", "profile", "email", "groups"}
		}
		return cfg, nil

	default:
		return Config{}, errors.New("AUTH_MODE must be 'local' or 'oidc', got: " + string(mode))
	}
}
