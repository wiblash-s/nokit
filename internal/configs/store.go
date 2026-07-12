package configs

import "time"

// PanelConfig is a config file stored in the panel's own database. This is the
// fallback storage used when no config volume is mounted for a server: the
// panel keeps the .cfg contents itself and, on exec, replays each command line
// over RCON rather than issuing a single `exec <file>`.
type PanelConfig struct {
	ID        int64
	ServerID  string
	Name      string
	Content   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ConfigStore is the persistence the panel-fallback config mode needs. It is
// satisfied by *store.Store without that package importing this one (mirroring
// the workshop.MapCache pattern), keeping the dependency one-way: store ->
// configs for the shared PanelConfig type only.
type ConfigStore interface {
	// ListPanelConfigs returns every stored config for a server, ordered by name.
	ListPanelConfigs(serverID string) ([]PanelConfig, error)
	// GetPanelConfig fetches a single config by name. It returns an error that
	// wraps store.ErrNotFound when the config does not exist.
	GetPanelConfig(serverID, name string) (PanelConfig, error)
	// SavePanelConfig creates or updates a config's content (upsert on
	// server_id+name).
	SavePanelConfig(serverID, name, content string) error
	// DeletePanelConfig removes a stored config.
	DeletePanelConfig(serverID, name string) error
}
