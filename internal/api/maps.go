package api

import (
        "log/slog"
        "net/http"
        "regexp"
        "time"

        "github.com/codevski/defuse/internal/rcon"
        "github.com/codevski/defuse/internal/steam"
        "github.com/codevski/defuse/internal/workshop"
)

// WorkshopMapInfo represents a single installed workshop map returned by the API.
type WorkshopMapInfo struct {
        // WorkshopID is the numeric Steam Workshop file ID, or "" when unknown
        // (RCON mode, map installed out-of-band). The UI shows a "no id" tag when empty.
        WorkshopID string `json:"workshopId"`
        Name       string `json:"name"`
        // Source is one of "instant", "installed", or "scanned" (see internal/workshop).
        Source string `json:"source"`
        // Versions lists all workshop IDs for this map name when more than one is
        // installed (filesystem mode only). Absent for single-version maps.
        Versions []string `json:"versions,omitempty"`
        // ThumbnailURL points at the panel's own thumbnail endpoint when a workshop
        // ID is known and Steam thumbnail resolution is enabled (STEAM_API_KEY set).
        // Empty otherwise, signalling the UI to fall back to a placeholder.
        ThumbnailURL string `json:"thumbnailUrl,omitempty"`
}

// WorkshopListResponse is the payload of the workshop maps endpoint. It reports
// the active provider mode and whether uninstall is possible, alongside the maps.
type WorkshopListResponse struct {
        // Mode is "rcon" or "filesystem".
        Mode string `json:"mode"`
        // Writable is true only in filesystem mode with a read-write mount, i.e. when
        // uninstall is supported.
        Writable bool              `json:"writable"`
        Maps     []WorkshopMapInfo `json:"maps"`
}

// numericIDRe validates a bare numeric workshop ID from a request.
var numericIDRe = regexp.MustCompile(`^\d+$`)

// WorkshopMapsHandler lists installed workshop maps for a server.
//
// It resolves a per-server provider (see internal/workshop): filesystem mode
// when the server's workshop content dir is mounted, otherwise RCON mode backed
// by ds_workshop_listmaps plus the download-ID cache. Maps with a known workshop
// ID are annotated with a thumbnail URL (and prefetched) when a STEAM_API_KEY is
// configured.
func WorkshopMapsHandler(mgr *rcon.Manager, steamClient *steam.Client, cache workshop.MapCache, logger *slog.Logger) Handler {
        return func(w http.ResponseWriter, r *http.Request) error {
                id := r.PathValue("id")
                if id == "" {
                        return BadRequest("missing server id")
                }

                prov := workshop.Resolve(id, mgr, cache, logger)
                logger.Info("syncing workshop maps", "server", id, "mode", prov.Mode())

                start := time.Now()
                entries, err := prov.List(r.Context())
                elapsed := time.Since(start)
                if err != nil {
                        logger.Error("workshop map sync failed", "server", id, "mode", prov.Mode(), "error", err, "elapsed", elapsed)
                        return WrapHTTP(err, http.StatusBadGateway, "workshop sync error")
                }

                maps := make([]WorkshopMapInfo, 0, len(entries))
                var thumbIDs []string
                for _, e := range entries {
                        m := WorkshopMapInfo{
                                WorkshopID: e.WorkshopID,
                                Name:       e.Name,
                                Source:     e.Source,
                                Versions:   e.Versions,
                        }
                        if e.WorkshopID != "" && steamClient.Enabled() {
                                m.ThumbnailURL = "/api/maps/thumbnail/" + e.WorkshopID
                                thumbIDs = append(thumbIDs, e.WorkshopID)
                        }
                        maps = append(maps, m)
                }

                if len(thumbIDs) > 0 {
                        steamClient.Prefetch(thumbIDs)
                }

                logger.Info("workshop maps synced",
                        "server", id, "mode", prov.Mode(), "writable", prov.Writable(),
                        "map_count", len(maps), "elapsed", elapsed,
                )

                return JSON(w, http.StatusOK, WorkshopListResponse{
                        Mode:     prov.Mode(),
                        Writable: prov.Writable(),
                        Maps:     maps,
                })
        }
}

// LoadWorkshopMapRequest is the body of the load endpoint.
type LoadWorkshopMapRequest struct {
        WorkshopID string `json:"workshopId"`
}

// LoadWorkshopMapHandler downloads and switches to a workshop map by ID via
// `host_workshop_map <id>`, records the ID in the cache immediately, and kicks a
// bounded background job that resolves the loaded map's name (by polling
// `status`) so future lists can show its thumbnail and mark it instantly loadable.
func LoadWorkshopMapHandler(mgr *rcon.Manager, cache workshop.MapCache, logger *slog.Logger) Handler {
        return func(w http.ResponseWriter, r *http.Request) error {
                id := r.PathValue("id")
                if id == "" {
                        return BadRequest("missing server id")
                }
                var req LoadWorkshopMapRequest
                if err := Decode(r, &req); err != nil {
                        return err
                }
                if !numericIDRe.MatchString(req.WorkshopID) {
                        return BadRequest("workshopId must be numeric")
                }

                if _, err := mgr.Execute(id, "host_workshop_map "+req.WorkshopID); err != nil {
                        return WrapHTTP(err, http.StatusBadGateway, "rcon error")
                }

                // Remember the ID right away (name filled in by the reconcile job).
                if cache != nil {
                        if err := cache.UpsertWorkshopMap(id, req.WorkshopID, ""); err != nil {
                                logger.Warn("failed to cache workshop id", "server", id, "workshop_id", req.WorkshopID, "error", err)
                        }
                }

                go reconcileMapName(mgr, cache, id, req.WorkshopID, logger)

                return JSON(w, http.StatusAccepted, map[string]string{
                        "ok":         "true",
                        "workshopId": req.WorkshopID,
                })
        }
}

// reconcileMapName polls `status` until the server reports a loaded map name,
// then caches the ID<->name pairing. It gives up after a bounded window so it
// never leaks a goroutine on a server that never finishes loading.
func reconcileMapName(mgr *rcon.Manager, cache workshop.MapCache, serverID, workshopID string, logger *slog.Logger) {
        if cache == nil {
                return
        }
        deadline := time.Now().Add(90 * time.Second)
        for attempt := 0; time.Now().Before(deadline); attempt++ {
                time.Sleep(5 * time.Second)
                out, err := mgr.Execute(serverID, "status")
                if err != nil {
                        continue
                }
                if name := workshop.ParseStatusMap(out); name != "" {
                        if err := cache.SetWorkshopMapName(serverID, workshopID, name); err != nil {
                                logger.Warn("failed to cache workshop map name", "server", serverID, "workshop_id", workshopID, "error", err)
                                return
                        }
                        logger.Info("reconciled workshop map name", "server", serverID, "workshop_id", workshopID, "map", name)
                        return
                }
        }
        logger.Debug("gave up reconciling workshop map name", "server", serverID, "workshop_id", workshopID)
}

// UninstallWorkshopMapHandler removes an installed workshop map by ID. It is only
// possible in filesystem mode with a writable mount; otherwise it returns 409.
func UninstallWorkshopMapHandler(mgr *rcon.Manager, cache workshop.MapCache, logger *slog.Logger) Handler {
        return func(w http.ResponseWriter, r *http.Request) error {
                id := r.PathValue("id")
                workshopID := r.PathValue("workshopId")
                if id == "" || workshopID == "" {
                        return BadRequest("missing server id or workshop id")
                }
                if !numericIDRe.MatchString(workshopID) {
                        return BadRequest("workshopId must be numeric")
                }

                prov := workshop.Resolve(id, mgr, cache, logger)
                if prov.Mode() != "filesystem" || !prov.Writable() {
                        return Conflict("uninstall unavailable: mount the workshop content dir read-write for this server")
                }

                if err := prov.Uninstall(r.Context(), workshopID); err != nil {
                        return WrapHTTP(err, http.StatusBadGateway, "uninstall failed")
                }
                if cache != nil {
                        _ = cache.DeleteWorkshopMap(id, workshopID)
                }
                logger.Info("uninstalled workshop map", "server", id, "workshop_id", workshopID)
                return JSON(w, http.StatusOK, map[string]string{"ok": "true"})
        }
}

// ThumbnailHandler serves a Steam Workshop map preview image for the workshop ID
// in the URL path. Images are cached on disk after the first request, so this
// hits the Steam API at most once per item.
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
                        logger.Debug("thumbnail unavailable", "workshop_id", id, "error", err)
                        return WrapHTTP(err, http.StatusNotFound, "thumbnail unavailable")
                }

                w.Header().Set("Cache-Control", "public, max-age=86400")
                http.ServeFile(w, r, path)
                return nil
        }
}
