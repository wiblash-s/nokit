package store

import (
        "database/sql"
        "errors"
        "fmt"
        "strings"
        "time"

        _ "modernc.org/sqlite"
)

type Server struct {
        ID        string
        Name      string
        Host      string
        RCONPass  string
        CreatedAt time.Time
}

type Store struct {
        db *sql.DB
}

func Open(path string) (*Store, error) {
        db, err := sql.Open("sqlite", path+"?_journal=WAL&_timeout=5000&_fk=true")
        if err != nil {
                return nil, fmt.Errorf("open db: %w", err)
        }
        db.SetMaxOpenConns(1)
        s := &Store{db: db}
        if err := s.migrate(); err != nil {
                db.Close()
                return nil, fmt.Errorf("migrate: %w", err)
        }
        return s, nil
}

func (s *Store) Close() error {
        return s.db.Close()
}

func (s *Store) migrate() error {
        _, err := s.db.Exec(`
                CREATE TABLE IF NOT EXISTS servers (
                        id         TEXT PRIMARY KEY,
                        name       TEXT NOT NULL,
                        host       TEXT NOT NULL,
                        rcon_pass  TEXT NOT NULL,
                        created_at INTEGER NOT NULL
                );

                CREATE TABLE IF NOT EXISTS sessions (
                        token      TEXT PRIMARY KEY,
                        created_at INTEGER NOT NULL,
                        expires_at INTEGER NOT NULL
                );

                -- Cache of Steam Workshop map ID<->name pairings, populated when maps are
                -- downloaded through the panel. Lets RCON mode (which only learns map
                -- names from ds_workshop_listmaps) resolve a name back to a workshop ID for
                -- thumbnails and "instantly loadable" indicators.
                CREATE TABLE IF NOT EXISTS workshop_maps (
                        server_id   TEXT NOT NULL,
                        workshop_id TEXT NOT NULL,
                        map_name    TEXT NOT NULL DEFAULT '',
                        updated_at  INTEGER NOT NULL,
                        PRIMARY KEY (server_id, workshop_id)
                );

                -- Panel-stored config files, used as the fallback for the config
                -- management feature when no cfg volume is mounted for a server. The
                -- panel keeps the .cfg contents here and, on exec, replays each command
                -- line over RCON instead of issuing a single exec of the file.
                CREATE TABLE IF NOT EXISTS panel_configs (
                        id         INTEGER PRIMARY KEY AUTOINCREMENT,
                        server_id  TEXT NOT NULL,
                        name       TEXT NOT NULL,
                        content    TEXT NOT NULL DEFAULT '',
                        created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
                        updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
                        UNIQUE(server_id, name)
                );
        `)
        return err
}

// WorkshopMapRecord is a cached workshop ID<->name pairing for a server.
type WorkshopMapRecord struct {
        ServerID   string
        WorkshopID string
        MapName    string
        UpdatedAt  time.Time
}

// UpsertWorkshopMap records a workshop ID for a server, optionally with its map
// name. An empty mapName never overwrites an existing non-empty one.
func (s *Store) UpsertWorkshopMap(serverID, workshopID, mapName string) error {
        _, err := s.db.Exec(
                `INSERT INTO workshop_maps (server_id, workshop_id, map_name, updated_at)
                 VALUES (?, ?, ?, ?)
                 ON CONFLICT(server_id, workshop_id) DO UPDATE SET
                   map_name   = CASE WHEN excluded.map_name != '' THEN excluded.map_name ELSE workshop_maps.map_name END,
                   updated_at = excluded.updated_at`,
                serverID, workshopID, mapName, time.Now().Unix(),
        )
        return err
}

// SetWorkshopMapName fills in / updates the map name for a cached workshop ID.
func (s *Store) SetWorkshopMapName(serverID, workshopID, mapName string) error {
        return s.UpsertWorkshopMap(serverID, workshopID, mapName)
}

// WorkshopIDForName returns the most recently updated workshop ID whose map name
// matches (case-insensitive) for the given server. ok is false when none exists.
func (s *Store) WorkshopIDForName(serverID, mapName string) (string, bool, error) {
        var id string
        err := s.db.QueryRow(
                `SELECT workshop_id FROM workshop_maps
                 WHERE server_id = ? AND map_name != '' AND lower(map_name) = lower(?)
                 ORDER BY updated_at DESC, rowid DESC LIMIT 1`,
                serverID, mapName,
        ).Scan(&id)
        if errors.Is(err, sql.ErrNoRows) {
                return "", false, nil
        }
        if err != nil {
                return "", false, err
        }
        return id, true, nil
}

// ListWorkshopMaps returns all cached workshop ID<->name pairings for a server.
func (s *Store) ListWorkshopMaps(serverID string) ([]WorkshopMapRecord, error) {
        rows, err := s.db.Query(
                `SELECT server_id, workshop_id, map_name, updated_at FROM workshop_maps
                 WHERE server_id = ? ORDER BY updated_at DESC`, serverID,
        )
        if err != nil {
                return nil, err
        }
        defer rows.Close()

        var out []WorkshopMapRecord
        for rows.Next() {
                var r WorkshopMapRecord
                var ts int64
                if err := rows.Scan(&r.ServerID, &r.WorkshopID, &r.MapName, &ts); err != nil {
                        return nil, err
                }
                r.UpdatedAt = time.Unix(ts, 0)
                out = append(out, r)
        }
        return out, rows.Err()
}

// DeleteWorkshopMap forgets a cached workshop ID for a server.
func (s *Store) DeleteWorkshopMap(serverID, workshopID string) error {
        _, err := s.db.Exec(
                `DELETE FROM workshop_maps WHERE server_id = ? AND workshop_id = ?`,
                serverID, workshopID,
        )
        return err
}

func (s *Store) ListServers() ([]Server, error) {
        rows, err := s.db.Query(
                `SELECT id, name, host, rcon_pass, created_at FROM servers ORDER BY created_at ASC`,
        )
        if err != nil {
                return nil, err
        }
        defer rows.Close()

        var out []Server
        for rows.Next() {
                var sv Server
                var ts int64
                if err := rows.Scan(&sv.ID, &sv.Name, &sv.Host, &sv.RCONPass, &ts); err != nil {
                        return nil, err
                }
                sv.CreatedAt = time.Unix(ts, 0)
                out = append(out, sv)
        }
        return out, rows.Err()
}

func (s *Store) GetServer(id string) (Server, error) {
        var sv Server
        var ts int64
        err := s.db.QueryRow(
                `SELECT id, name, host, rcon_pass, created_at FROM servers WHERE id = ?`, id,
        ).Scan(&sv.ID, &sv.Name, &sv.Host, &sv.RCONPass, &ts)
        if errors.Is(err, sql.ErrNoRows) {
                return Server{}, ErrNotFound
        }
        if err != nil {
                return Server{}, err
        }
        sv.CreatedAt = time.Unix(ts, 0)
        return sv, nil
}

func (s *Store) AddServer(sv Server) error {
        _, err := s.db.Exec(
                `INSERT INTO servers (id, name, host, rcon_pass, created_at) VALUES (?, ?, ?, ?, ?)`,
                sv.ID, sv.Name, sv.Host, sv.RCONPass, sv.CreatedAt.Unix(),
        )
        if err != nil && strings.Contains(err.Error(), "UNIQUE constraint") {
                return ErrConflict
        }
        return err
}

func (s *Store) DeleteServer(id string) error {
        _, err := s.db.Exec(`DELETE FROM servers WHERE id = ?`, id)
        return err
}

func (s *Store) CreateSession(token string, ttl time.Duration) error {
        now := time.Now()
        _, err := s.db.Exec(
                `INSERT INTO sessions (token, created_at, expires_at) VALUES (?, ?, ?)`,
                token, now.Unix(), now.Add(ttl).Unix(),
        )
        return err
}

func (s *Store) ValidSession(token string) (bool, error) {
        _, _ = s.db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now().Unix())

        var count int
        err := s.db.QueryRow(
                `SELECT COUNT(*) FROM sessions WHERE token = ? AND expires_at >= ?`,
                token, time.Now().Unix(),
        ).Scan(&count)
        return count > 0, err
}

func (s *Store) DeleteSession(token string) error {
        _, err := s.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
        return err
}

var (
        ErrNotFound = errors.New("not found")
        ErrConflict = errors.New("already exists")
)
