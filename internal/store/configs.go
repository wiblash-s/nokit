package store

import (
	"database/sql"
	"errors"
	"time"

	"github.com/codevski/defuse/internal/configs"
)

// This file implements configs.ConfigStore on *Store, backing the panel-fallback
// config mode. Returning the configs.PanelConfig type keeps the dependency
// one-way (store -> configs), mirroring how the workshop layer's MapCache is
// satisfied here without an import cycle.

// sqliteTimeLayouts are the timestamp formats modernc/sqlite may hand back for a
// DATETIME column populated by CURRENT_TIMESTAMP.
var sqliteTimeLayouts = []string{
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05Z",
	time.RFC3339,
}

func parseSQLiteTime(s string) time.Time {
	for _, layout := range sqliteTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

// ListPanelConfigs returns every stored config for a server, ordered by name.
func (s *Store) ListPanelConfigs(serverID string) ([]configs.PanelConfig, error) {
	rows, err := s.db.Query(
		`SELECT id, server_id, name, content, created_at, updated_at
		 FROM panel_configs WHERE server_id = ? ORDER BY name ASC`, serverID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []configs.PanelConfig
	for rows.Next() {
		var c configs.PanelConfig
		var created, updated string
		if err := rows.Scan(&c.ID, &c.ServerID, &c.Name, &c.Content, &created, &updated); err != nil {
			return nil, err
		}
		c.CreatedAt = parseSQLiteTime(created)
		c.UpdatedAt = parseSQLiteTime(updated)
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetPanelConfig fetches a single config by name, returning ErrNotFound when it
// does not exist.
func (s *Store) GetPanelConfig(serverID, name string) (configs.PanelConfig, error) {
	var c configs.PanelConfig
	var created, updated string
	err := s.db.QueryRow(
		`SELECT id, server_id, name, content, created_at, updated_at
		 FROM panel_configs WHERE server_id = ? AND name = ?`, serverID, name,
	).Scan(&c.ID, &c.ServerID, &c.Name, &c.Content, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return configs.PanelConfig{}, ErrNotFound
	}
	if err != nil {
		return configs.PanelConfig{}, err
	}
	c.CreatedAt = parseSQLiteTime(created)
	c.UpdatedAt = parseSQLiteTime(updated)
	return c, nil
}

// SavePanelConfig creates or updates a config's content (upsert on
// server_id+name), bumping updated_at on update.
func (s *Store) SavePanelConfig(serverID, name, content string) error {
	_, err := s.db.Exec(
		`INSERT INTO panel_configs (server_id, name, content, created_at, updated_at)
		 VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		 ON CONFLICT(server_id, name) DO UPDATE SET
		   content    = excluded.content,
		   updated_at = CURRENT_TIMESTAMP`,
		serverID, name, content,
	)
	return err
}

// DeletePanelConfig removes a stored config.
func (s *Store) DeletePanelConfig(serverID, name string) error {
	_, err := s.db.Exec(
		`DELETE FROM panel_configs WHERE server_id = ? AND name = ?`, serverID, name,
	)
	return err
}
