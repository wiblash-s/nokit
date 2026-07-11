// Package workshop discovers and manages the Steam Workshop maps installed on a
// CS2 dedicated server.
//
// It supports two modes, selected PER SERVER at runtime so a multi-server panel
// can mix them freely:
//
//   - RCON mode (default, works for any server): the list of installed maps
//     comes from the `ds_workshop_listmaps` RCON command, which reports only bare
//     map NAMES (e.g. "de_drachenschanze") with no workshop IDs. IDs are known
//     only for maps that were downloaded through the panel and cached in the
//     database, so thumbnails are available for those but not for maps installed
//     out-of-band. Multiple versions of the same map name cannot be told apart.
//
//   - Filesystem mode (opt-in): when the CS2 server's workshop content directory
//     (steamapps/workshop/content/730) is mounted into the panel at
//     /workshop/<serverID> (or an explicit WORKSHOP_PATH_<ID> override), the
//     provider scans it directly. Each subfolder is a numeric workshop ID
//     containing the map's .vpk, giving an exact ID<->name mapping — including
//     several versions of the same map name. Uninstall (deleting the folder) is
//     enabled when the mount is writable, which is detected at runtime with a
//     write probe rather than declared in config.
//
// Mode selection is a pure function of whether the per-server content path is a
// readable directory; see Resolve.
package workshop

import (
        "context"
        "log/slog"
        "os"
        "path/filepath"
        "strings"
)

// Source describes where a listed map (and its ID, if any) came from.
const (
        // SourceInstant: the name came from ds_workshop_listmaps AND we know its
        // workshop ID from the download cache, so it is instantly loadable with a
        // real thumbnail.
        SourceInstant = "instant"
        // SourceInstalled: the name came from ds_workshop_listmaps but we have no
        // cached workshop ID for it (installed out-of-band). Shown with a "no id" tag
        // and no thumbnail.
        SourceInstalled = "installed"
        // SourceScanned: the entry was discovered by scanning the mounted workshop
        // content directory, so its ID (and any duplicate-name versions) is exact.
        SourceScanned = "scanned"
)

// Map is a single installed workshop map as surfaced to the API layer.
type Map struct {
        // Name is the CS2 map name (e.g. "de_drachenschanze"), without any
        // "workshop/<id>/" prefix or ".vpk" suffix.
        Name string
        // WorkshopID is the numeric Steam Workshop file ID, or "" when unknown
        // (RCON mode, map installed out-of-band).
        WorkshopID string
        // Source is one of the Source* constants above.
        Source string
        // Versions lists all workshop IDs found for this map name. It is only
        // populated (len > 1) in filesystem mode when the same name is installed
        // under multiple IDs, signalling an ambiguous/multi-version install.
        Versions []string
}

// Provider lists and (optionally) uninstalls the workshop maps for one server.
type Provider interface {
        // Mode returns "rcon" or "filesystem".
        Mode() string
        // Writable reports whether Uninstall is supported (filesystem mode with a
        // writable mount). Always false in RCON mode.
        Writable() bool
        // List returns the installed workshop maps.
        List(ctx context.Context) ([]Map, error)
        // Uninstall removes an installed workshop map by ID. It returns an error in
        // RCON mode or when the mount is read-only.
        Uninstall(ctx context.Context, workshopID string) error
}

// RCONExecutor is the subset of *rcon.Manager the RCON provider needs. Declaring
// it here keeps the provider testable with a fake.
type RCONExecutor interface {
        Execute(serverID, command string) (string, error)
        ExecuteMulti(serverID, command string) (string, error)
}

// MapCache is the subset of the store the workshop layer needs to remember the
// ID<->name pairing of maps downloaded through the panel. The store satisfies
// this without importing the workshop package (avoiding an import cycle).
type MapCache interface {
        // WorkshopIDForName returns the most recently cached workshop ID whose map
        // name matches (case-insensitive). ok is false when nothing is cached.
        WorkshopIDForName(serverID, mapName string) (id string, ok bool, err error)
        // UpsertWorkshopMap records an ID (with an optional name) for a server. An
        // empty name must not clobber an existing non-empty one.
        UpsertWorkshopMap(serverID, workshopID, mapName string) error
        // SetWorkshopMapName fills in / updates the name for a cached ID.
        SetWorkshopMapName(serverID, workshopID, mapName string) error
        // DeleteWorkshopMap forgets a cached ID (e.g. after uninstall).
        DeleteWorkshopMap(serverID, workshopID string) error
}

// defaultBase is the base directory under which each server's workshop content
// mount is expected at <base>/<serverID>. Overridable via WORKSHOP_BASE.
const defaultBase = "/workshop"

// BaseDir returns the configured workshop mount base directory.
func BaseDir() string {
        if v := strings.TrimSpace(os.Getenv("WORKSHOP_BASE")); v != "" {
                return v
        }
        return defaultBase
}

// envKeyForID mirrors the RCON_PASSWORD_<UPPER_ID> convention used elsewhere:
// upper-case, hyphens -> underscores.
func envKeyForID(serverID string) string {
        return "WORKSHOP_PATH_" + strings.ToUpper(strings.ReplaceAll(serverID, "-", "_"))
}

// ContentPathFor returns the filesystem path that would hold serverID's workshop
// content, honouring a per-server WORKSHOP_PATH_<ID> override before falling back
// to <WORKSHOP_BASE>/<serverID>.
func ContentPathFor(serverID string) string {
        if override := strings.TrimSpace(os.Getenv(envKeyForID(serverID))); override != "" {
                return override
        }
        return filepath.Join(BaseDir(), serverID)
}

// isReadableDir reports whether path exists, is a directory, and can be opened
// for reading.
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

// Resolve picks the provider for serverID: filesystem mode when the per-server
// content path is a readable directory, otherwise RCON mode.
func Resolve(serverID string, rc RCONExecutor, cache MapCache, logger *slog.Logger) Provider {
        if logger == nil {
                logger = slog.Default()
        }
        path := ContentPathFor(serverID)
        if isReadableDir(path) {
                logger.Info("workshop: filesystem mode", "server", serverID, "path", path)
                return newFilesystemProvider(path, logger)
        }
        logger.Info("workshop: rcon mode", "server", serverID, "content_path_checked", path)
        return newRCONProvider(rc, cache, serverID, logger)
}
