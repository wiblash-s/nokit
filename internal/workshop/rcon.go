package workshop

import (
        "context"
        "fmt"
        "log/slog"
)

// rconProvider lists installed workshop maps via `ds_workshop_listmaps` and
// annotates each with a workshop ID from the download cache when one is known.
type rconProvider struct {
        rc       RCONExecutor
        cache    MapCache
        serverID string
        logger   *slog.Logger
}

func newRCONProvider(rc RCONExecutor, cache MapCache, serverID string, logger *slog.Logger) *rconProvider {
        return &rconProvider{rc: rc, cache: cache, serverID: serverID, logger: logger}
}

func (p *rconProvider) Mode() string   { return "rcon" }
func (p *rconProvider) Writable() bool { return false }

func (p *rconProvider) List(ctx context.Context) ([]Map, error) {
        // ds_workshop_listmaps reports installed workshop maps by name. Use
        // ExecuteMulti so a long list isn't truncated across RCON packets.
        out, err := p.rc.ExecuteMulti(p.serverID, "ds_workshop_listmaps")
        if err != nil {
                return nil, fmt.Errorf("workshop: ds_workshop_listmaps: %w", err)
        }

        var names []string
        if looksLikeUnknownCommand(out) || len(ParseListmaps(out)) == 0 {
                // Fall back to the legacy "maps *" path parser for servers without the
                // ds_workshop_* plugin. This yields both name and ID directly.
                p.logger.Debug("workshop: ds_workshop_listmaps unavailable/empty, falling back to 'maps *'",
                        "server", p.serverID)
                fout, ferr := p.rc.ExecuteMulti(p.serverID, "maps *")
                if ferr != nil {
                        // If the fallback also fails, surface whatever the listmaps call gave.
                        if len(ParseListmaps(out)) == 0 {
                                return []Map{}, nil
                        }
                        names = ParseListmaps(out)
                } else {
                        pathMaps := ParsePathMaps(fout)
                        // Cache the discovered ID<->name pairs so future RCON-only lists (and
                        // thumbnails) benefit even if the server later stops reporting paths.
                        for i := range pathMaps {
                                pathMaps[i].Source = SourceInstant
                                if p.cache != nil && pathMaps[i].WorkshopID != "" {
                                        _ = p.cache.UpsertWorkshopMap(p.serverID, pathMaps[i].WorkshopID, pathMaps[i].Name)
                                }
                        }
                        if len(pathMaps) > 0 {
                                return dedupeByName(pathMaps), nil
                        }
                        names = ParseListmaps(out)
                }
        } else {
                names = ParseListmaps(out)
        }

        maps := make([]Map, 0, len(names))
        for _, name := range names {
                entry := Map{Name: name, Source: SourceInstalled}
                if p.cache != nil {
                        if id, ok, err := p.cache.WorkshopIDForName(p.serverID, name); err == nil && ok && id != "" {
                                entry.WorkshopID = id
                                entry.Source = SourceInstant
                        }
                }
                maps = append(maps, entry)
        }
        return maps, nil
}

func (p *rconProvider) Uninstall(ctx context.Context, workshopID string) error {
        return fmt.Errorf("uninstall requires filesystem access: mount the CS2 workshop content dir at %s to enable it",
                ContentPathFor(p.serverID))
}

// dedupeByName collapses entries sharing a map name, keeping the first ID seen
// and recording additional IDs under Versions.
func dedupeByName(in []Map) []Map {
        order := make([]string, 0, len(in))
        byName := make(map[string]*Map)
        for i := range in {
                m := in[i]
                if existing, ok := byName[m.Name]; ok {
                        if m.WorkshopID != "" && m.WorkshopID != existing.WorkshopID {
                                existing.Versions = append(existing.Versions, m.WorkshopID)
                        }
                        continue
                }
                cp := m
                byName[m.Name] = &cp
                order = append(order, m.Name)
        }
        out := make([]Map, 0, len(order))
        for _, name := range order {
                m := *byName[name]
                if len(m.Versions) > 0 {
                        m.Versions = append([]string{m.WorkshopID}, m.Versions...)
                }
                out = append(out, m)
        }
        return out
}
