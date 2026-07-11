package workshop

import (
        "context"
        "fmt"
        "log/slog"
        "os"
        "path/filepath"
        "regexp"
        "sort"
        "strings"
)

// numericDirRe matches a workshop content subfolder, which Steam names after the
// numeric published file (workshop) ID.
var numericDirRe = regexp.MustCompile(`^\d+$`)

// fsProvider scans a mounted CS2 workshop content directory
// (steamapps/workshop/content/730) for installed maps. Because the on-disk
// layout is <root>/<workshopID>/<mapname>.vpk, it yields an exact ID<->name
// mapping and can distinguish multiple versions of the same map name.
type fsProvider struct {
        root     string
        writable bool
        logger   *slog.Logger
}

func newFilesystemProvider(root string, logger *slog.Logger) *fsProvider {
        p := &fsProvider{root: root, logger: logger}
        p.writable = probeWritable(root, logger)
        logger.Info("workshop: filesystem provider ready", "root", root, "writable", p.writable)
        return p
}

func (p *fsProvider) Mode() string   { return "filesystem" }
func (p *fsProvider) Writable() bool { return p.writable }

// probeWritable determines at runtime whether root is writable by attempting to
// create and remove a temp file. This is more reliable than inspecting mode bits
// or a declared flag: a read-only bind mount can still report writable
// permission bits yet fail the actual write.
func probeWritable(root string, logger *slog.Logger) bool {
        f, err := os.CreateTemp(root, ".nokit-write-probe-*")
        if err != nil {
                logger.Debug("workshop: write probe failed (read-only mount)", "root", root, "error", err)
                return false
        }
        name := f.Name()
        _ = f.Close()
        _ = os.Remove(name)
        return true
}

func (p *fsProvider) List(ctx context.Context) ([]Map, error) {
        entries, err := os.ReadDir(p.root)
        if err != nil {
                return nil, fmt.Errorf("workshop: read content dir %s: %w", p.root, err)
        }

        // Collect all (name -> []id) so we can group multiple versions of a name.
        idsByName := make(map[string][]string)
        var nameOrder []string

        for _, e := range entries {
                if !e.IsDir() || !numericDirRe.MatchString(e.Name()) {
                        continue
                }
                id := e.Name()
                name := p.mapNameIn(filepath.Join(p.root, id))
                if name == "" {
                        // A workshop folder without a recognisable .vpk — skip it rather than
                        // surfacing a bogus entry.
                        p.logger.Debug("workshop: no .vpk in workshop dir, skipping", "id", id)
                        continue
                }
                if _, seen := idsByName[name]; !seen {
                        nameOrder = append(nameOrder, name)
                }
                idsByName[name] = append(idsByName[name], id)
        }

        maps := make([]Map, 0, len(nameOrder))
        for _, name := range nameOrder {
                ids := idsByName[name]
                sort.Strings(ids)
                m := Map{Name: name, WorkshopID: ids[0], Source: SourceScanned}
                if len(ids) > 1 {
                        m.Versions = ids
                }
                maps = append(maps, m)
        }
        return maps, nil
}

// mapNameIn returns the map name (vpk filename without extension) inside a
// workshop item directory, or "" if none is found. It searches one level deep
// as well, since some layouts nest the vpk under a subdirectory.
func (p *fsProvider) mapNameIn(dir string) string {
        entries, err := os.ReadDir(dir)
        if err != nil {
                return ""
        }
        for _, e := range entries {
                if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".vpk") {
                        return strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
                }
        }
        // One level deeper.
        for _, e := range entries {
                if e.IsDir() {
                        sub, err := os.ReadDir(filepath.Join(dir, e.Name()))
                        if err != nil {
                                continue
                        }
                        for _, se := range sub {
                                if !se.IsDir() && strings.HasSuffix(strings.ToLower(se.Name()), ".vpk") {
                                        return strings.TrimSuffix(se.Name(), filepath.Ext(se.Name()))
                                }
                        }
                }
        }
        return ""
}

func (p *fsProvider) Uninstall(ctx context.Context, workshopID string) error {
        if !p.writable {
                return fmt.Errorf("workshop: content dir %s is mounted read-only; remount read-write to uninstall", p.root)
        }
        if !numericDirRe.MatchString(workshopID) {
                return fmt.Errorf("workshop: invalid workshop id %q", workshopID)
        }
        target := filepath.Join(p.root, workshopID)
        if _, err := os.Stat(target); err != nil {
                if os.IsNotExist(err) {
                        return fmt.Errorf("workshop: %s is not installed", workshopID)
                }
                return fmt.Errorf("workshop: stat %s: %w", workshopID, err)
        }
        if err := os.RemoveAll(target); err != nil {
                return fmt.Errorf("workshop: remove %s: %w", workshopID, err)
        }
        p.logger.Info("workshop: uninstalled map", "id", workshopID, "path", target)
        return nil
}
