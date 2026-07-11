package api

import (
        "log/slog"
        "net/http"
        "regexp"
        "strings"
        "time"

        "github.com/codevski/defuse/internal/rcon"
        "github.com/codevski/defuse/internal/steam"
)

// WorkshopMapInfo represents a single installed workshop map returned by the API.
type WorkshopMapInfo struct {
        WorkshopID string `json:"workshopId"`
        Name       string `json:"name"`
        // ThumbnailURL points at the panel's own thumbnail endpoint when Steam
        // thumbnail resolution is enabled (a STEAM_API_KEY is configured). It is
        // empty otherwise, signalling the UI to fall back to a placeholder.
        ThumbnailURL string `json:"thumbnailUrl,omitempty"`
}

// WorkshopMapsHandler lists installed workshop maps by executing the RCON
// command "maps *" and extracting entries whose path starts with "workshop/".
//
// CS2 dedicated servers report workshop maps in the following format:
//
//      workshop/3070900859/de_cache_redux
//
// This handler parses that format and de-duplicates results by workshop ID.
//
// When a Steam client is configured (a STEAM_API_KEY is present), each map is
// annotated with a ThumbnailURL and the panel kicks off a background prefetch so
// thumbnails are cached and ready by the time the browser requests them. A nil
// steam client disables both behaviours.
func WorkshopMapsHandler(mgr *rcon.Manager, steamClient *steam.Client, logger *slog.Logger) Handler {
        return func(w http.ResponseWriter, r *http.Request) error {
                id := r.PathValue("id")
                if id == "" {
                        return BadRequest("missing server id")
                }

                logger.Info("syncing workshop maps", "server", id)

                // "maps *" returns a large, multi-packet response. ExecuteMulti reassembles
                // all packets over a dedicated connection so we get the complete map list
                // without truncation or desyncing the pooled RCON connection.
                start := time.Now()
                out, err := mgr.ExecuteMulti(id, "maps *")
                elapsed := time.Since(start)
                if err != nil {
                        logger.Error("workshop map sync failed", "server", id, "error", err, "elapsed", elapsed)
                        return WrapHTTP(err, http.StatusBadGateway, "rcon error")
                }

                maps := parseWorkshopMaps(out)
                logger.Info("workshop maps synced",
                        "server", id,
                        "map_count", len(maps),
                        "output_bytes", len(out),
                        "elapsed", elapsed,
                )

                if steamClient.Enabled() {
                        ids := make([]string, 0, len(maps))
                        for i := range maps {
                                maps[i].ThumbnailURL = "/api/maps/thumbnail/" + maps[i].WorkshopID
                                ids = append(ids, maps[i].WorkshopID)
                        }
                        // Warm the cache in the background so the UI gets real images without
                        // each card blocking on a Steam round-trip.
                        if len(ids) > 0 {
                                steamClient.Prefetch(ids)
                        }
                }

                return JSON(w, http.StatusOK, maps)
        }
}

// ThumbnailHandler serves a Steam Workshop map preview image for the workshop ID
// in the URL path. Images are cached on disk after the first request, so this
// hits the Steam API at most once per item.
//
// Responses are marked cacheable by the browser to avoid re-requesting images
// that rarely change. If the Steam client is disabled or the item has no preview
// image, an appropriate error status is returned and the UI falls back to a
// placeholder gradient.
func ThumbnailHandler(steamClient *steam.Client, logger *slog.Logger) Handler {
        return func(w http.ResponseWriter, r *http.Request) error {
                if !steamClient.Enabled() {
                        logger.Debug("thumbnail request rejected: Steam client disabled")
                        return NotFound("thumbnails unavailable: STEAM_API_KEY not configured")
                }

                id := r.PathValue("id")
                if id == "" {
                        return BadRequest("missing workshop id")
                }

                logger.Info("serving thumbnail", "workshop_id", id, "client_ip", r.RemoteAddr)

                path, err := steamClient.ThumbnailPath(r.Context(), id)
                if err != nil {
                        // Not found is the friendliest signal for the UI's onError fallback,
                        // whether the item lacks a preview or the ID was malformed.
                        logger.Debug("thumbnail unavailable", "workshop_id", id, "error", err)
                        return WrapHTTP(err, http.StatusNotFound, "thumbnail unavailable")
                }

                w.Header().Set("Cache-Control", "public, max-age=86400")
                http.ServeFile(w, r, path)
                return nil
        }
}

// workshopPathRe matches the "workshop/<id>/<mapname>" pattern that appears in
// the output of CS2's "maps *" RCON command.
var workshopPathRe = regexp.MustCompile(`workshop[/\\](\d+)[/\\](\S+)`)

func parseWorkshopMaps(output string) []WorkshopMapInfo {
        var maps []WorkshopMapInfo
        seen := make(map[string]bool)

        for _, line := range strings.Split(output, "\n") {
                line = strings.TrimSpace(line)
                if line == "" {
                        continue
                }

                m := workshopPathRe.FindStringSubmatch(line)
                if m == nil {
                        continue
                }

                workshopID := m[1]
                mapName := strings.TrimSuffix(m[2], ".vpk") // strip .vpk suffix if present

                if seen[workshopID] {
                        continue
                }
                seen[workshopID] = true

                maps = append(maps, WorkshopMapInfo{
                        WorkshopID: workshopID,
                        Name:       mapName,
                })
        }

        if maps == nil {
                maps = []WorkshopMapInfo{}
        }
        return maps
}
