// Package configs manages CS2 server .cfg files for the panel. It mirrors the
// two-mode design of the workshop package, selected PER SERVER at runtime:
//
//   - Mounted mode (primary): when a directory <CONFIG_BASE>/<serverID> exists
//     in the container (a volume mount of the CS2 server's cfg folder), the
//     panel reads and writes .cfg files directly on that filesystem. Executing a
//     config issues a single `exec <name>` RCON command, which the game server
//     runs from its own cfg directory.
//
//   - Panel-only fallback: when no config directory is mounted, configs live in
//     the panel's SQLite database. Executing a config replays its contents over
//     RCON, sending each non-empty, non-comment line as a separate command.
//
// Mode selection is a pure function of whether <CONFIG_BASE>/<serverID> is a
// readable directory; see IsMounted / GetMode.
package configs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Mode strings surfaced to the API and UI.
const (
	ModeMount = "mounted"
	ModePanel = "panel"
)

// defaultBase is the base directory under which each server's cfg mount is
// expected at <base>/<serverID>. Overridable via CONFIG_BASE.
const defaultBase = "/configs"

// Config is a single config file as surfaced to the API layer.
type Config struct {
	// Name is the filename without any directory, e.g. "practice.cfg".
	Name string `json:"name"`
	// Content is the full file body. It is only populated by GetConfig; ListConfigs
	// leaves it empty to keep listings cheap.
	Content string `json:"content"`
	// Mode is "mounted" or "panel".
	Mode string `json:"mode"`
	// Writable reports whether the config can be written. Always true in panel
	// mode; in mounted mode it reflects whether the mount is read-write.
	Writable bool `json:"writable"`
}

// Manager reads and writes a server's configs in whichever mode applies.
type Manager struct {
	// baseDir is the mount base; default /configs, overridable via CONFIG_BASE.
	baseDir string
	// store backs panel-fallback mode. It may be nil, in which case panel mode
	// operations fail gracefully.
	store ConfigStore
}

// NewManager builds a Manager. The panel-fallback store is used only when a
// server has no mounted config directory.
func NewManager(store ConfigStore) *Manager {
	base := strings.TrimSpace(os.Getenv("CONFIG_BASE"))
	if base == "" {
		base = defaultBase
	}
	return &Manager{baseDir: base, store: store}
}

// dirFor returns the filesystem path that would hold serverID's configs.
func (m *Manager) dirFor(serverID string) string {
	return filepath.Join(m.baseDir, serverID)
}

// isReadableDir reports whether path exists, is a directory, and can be opened.
func isReadableDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

// IsMounted reports whether a config directory is mounted for this server.
func (m *Manager) IsMounted(serverID string) bool {
	return isReadableDir(m.dirFor(serverID))
}

// IsWritable reports whether the mounted config directory is writable. It is
// only meaningful when IsMounted is true; it probes with a temp file rather than
// inspecting mode bits, since a read-only bind mount can still report writable
// permission bits yet fail the actual write.
func (m *Manager) IsWritable(serverID string) bool {
	dir := m.dirFor(serverID)
	if !isReadableDir(dir) {
		return false
	}
	f, err := os.CreateTemp(dir, ".nokit-cfg-write-probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// GetMode returns the active mode and, for mounted mode, whether it is writable.
// Panel mode is always writable.
func (m *Manager) GetMode(serverID string) (mode string, writable bool) {
	if m.IsMounted(serverID) {
		return ModeMount, m.IsWritable(serverID)
	}
	return ModePanel, true
}

// validName rejects names that are empty, contain a path separator, or try to
// traverse out of the config directory. It also enforces the .cfg extension so
// mounted-mode writes stay confined to config files.
func validName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("config name is required")
	}
	if name != filepath.Base(name) || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return fmt.Errorf("invalid config name %q", name)
	}
	if !strings.HasSuffix(strings.ToLower(name), ".cfg") {
		return fmt.Errorf("config name must end in .cfg")
	}
	return nil
}

// ListConfigs returns all .cfg files for this server, from the mounted directory
// or the panel database depending on mode. Content is not populated.
func (m *Manager) ListConfigs(serverID string) ([]Config, error) {
	if m.IsMounted(serverID) {
		dir := m.dirFor(serverID)
		writable := m.IsWritable(serverID)
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("configs: read dir %s: %w", dir, err)
		}
		out := make([]Config, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if !strings.HasSuffix(strings.ToLower(e.Name()), ".cfg") {
				continue
			}
			out = append(out, Config{Name: e.Name(), Mode: ModeMount, Writable: writable})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
		return out, nil
	}

	if m.store == nil {
		return []Config{}, nil
	}
	records, err := m.store.ListPanelConfigs(serverID)
	if err != nil {
		return nil, fmt.Errorf("configs: list panel configs: %w", err)
	}
	out := make([]Config, 0, len(records))
	for _, r := range records {
		out = append(out, Config{Name: r.Name, Mode: ModePanel, Writable: true})
	}
	return out, nil
}

// GetConfig reads a single config's content by name.
func (m *Manager) GetConfig(serverID, name string) (Config, error) {
	if err := validName(name); err != nil {
		return Config{}, err
	}
	if m.IsMounted(serverID) {
		path := filepath.Join(m.dirFor(serverID), name)
		b, err := os.ReadFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("configs: read %s: %w", name, err)
		}
		return Config{
			Name:     name,
			Content:  string(b),
			Mode:     ModeMount,
			Writable: m.IsWritable(serverID),
		}, nil
	}

	if m.store == nil {
		return Config{}, fmt.Errorf("configs: panel store unavailable")
	}
	rec, err := m.store.GetPanelConfig(serverID, name)
	if err != nil {
		return Config{}, err
	}
	return Config{Name: rec.Name, Content: rec.Content, Mode: ModePanel, Writable: true}, nil
}

// SaveConfig writes a config's content, to the mounted filesystem or the panel
// database depending on mode. In mounted read-only mode it returns an error.
func (m *Manager) SaveConfig(serverID, name, content string) error {
	if err := validName(name); err != nil {
		return err
	}
	if m.IsMounted(serverID) {
		if !m.IsWritable(serverID) {
			return fmt.Errorf("configs: mount for server %q is read-only; remount read-write to edit", serverID)
		}
		path := filepath.Join(m.dirFor(serverID), name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("configs: write %s: %w", name, err)
		}
		return nil
	}

	if m.store == nil {
		return fmt.Errorf("configs: panel store unavailable")
	}
	return m.store.SavePanelConfig(serverID, name, content)
}

// DeleteConfig removes a config from the mounted filesystem or the panel database.
func (m *Manager) DeleteConfig(serverID, name string) error {
	if err := validName(name); err != nil {
		return err
	}
	if m.IsMounted(serverID) {
		if !m.IsWritable(serverID) {
			return fmt.Errorf("configs: mount for server %q is read-only; remount read-write to delete", serverID)
		}
		path := filepath.Join(m.dirFor(serverID), name)
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("configs: delete %s: %w", name, err)
		}
		return nil
	}

	if m.store == nil {
		return fmt.Errorf("configs: panel store unavailable")
	}
	return m.store.DeletePanelConfig(serverID, name)
}

// ExecLines splits a config body into the individual RCON commands to send in
// panel mode: it drops blank lines and `//` comment lines (including inline
// trailing comments) and trims surrounding whitespace.
func ExecLines(content string) []string {
	var cmds []string
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		// Strip an inline trailing comment (// ...) that is not inside quotes.
		if idx := inlineCommentIndex(line); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			continue
		}
		cmds = append(cmds, line)
	}
	return cmds
}

// inlineCommentIndex returns the index of the start of a // comment that is not
// inside a double-quoted string, or -1 if there is none.
func inlineCommentIndex(line string) int {
	inQuote := false
	for i := 0; i < len(line)-1; i++ {
		switch line[i] {
		case '"':
			inQuote = !inQuote
		case '/':
			if !inQuote && line[i+1] == '/' {
				return i
			}
		}
	}
	return -1
}

// StripExt returns name without a trailing .cfg extension, used to build the
// `exec <name>` command in mounted mode. CS2 accepts either form; dropping the
// extension matches the most common convention.
func StripExt(name string) string {
	if strings.HasSuffix(strings.ToLower(name), ".cfg") {
		return name[:len(name)-len(".cfg")]
	}
	return name
}
