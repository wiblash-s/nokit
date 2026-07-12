package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/codevski/defuse/internal/configs"
	"github.com/codevski/defuse/internal/rcon"
	"github.com/codevski/defuse/internal/store"
)

// bannedUsersFile is the CS2 cfg that persists SteamID bans. `writeid` makes the
// game server append `banid <minutes> <steamid>` lines to it; the panel reads and
// edits it directly through the configs.Manager (mounted-fs or panel-DB mode).
const bannedUsersFile = "banned_users.cfg"

// Player is a single connected player parsed from the `status` RCON output and
// enriched with a GeoIP country lookup.
type Player struct {
	UserID      int    `json:"userid"`
	Name        string `json:"name"`
	SteamID     string `json:"steamid"`
	Team        string `json:"team"`
	Ping        int    `json:"ping"`
	Time        string `json:"time"`
	CountryCode string `json:"country_code"`
	CountryName string `json:"country_name"`

	// ipInternal holds the parsed address column for GeoIP resolution only. It
	// is unexported so it is never serialised into the API response.
	ipInternal string
}

// Ban is a single banned SteamID, merged from banned_users.cfg (persistent) and
// the `listid` RCON output (in-memory session bans).
type Ban struct {
	Name string `json:"name"`
	// SteamID is the canonical STEAM_x:y:z / [U:1:z] token as stored/reported.
	SteamID string `json:"steamid"`
	// ExpiresAt is a JSON string: a unix timestamp (seconds) or the literal
	// "permanent".
	ExpiresAt string `json:"expires_at"`
	// Source is one of "cfg", "session", or "both".
	Source string `json:"source"`
}

// ---------------------------------------------------------------------------
// GeoIP cache
// ---------------------------------------------------------------------------

// geoResult is one resolved country for an IP.
type geoResult struct {
	code string
	name string
}

// geoCache memoises ip-api.com lookups so repeated 10s refreshes of the live
// player list do not hammer the (rate-limited, key-less) free endpoint.
type geoCache struct {
	mu      sync.Mutex
	entries map[string]geoResult
	client  *http.Client
}

var geo = &geoCache{
	entries: make(map[string]geoResult),
	client:  &http.Client{Timeout: 6 * time.Second},
}

// lookup resolves country info for a set of IPs, using the cache first and a
// single ip-api.com batch request for the misses. Private/reserved/invalid IPs
// resolve to an empty result (and are cached as such).
func (g *geoCache) lookup(ips []string) map[string]geoResult {
	out := make(map[string]geoResult, len(ips))

	g.mu.Lock()
	var misses []string
	seen := make(map[string]bool)
	for _, ip := range ips {
		if ip == "" || seen[ip] {
			continue
		}
		seen[ip] = true
		if r, ok := g.entries[ip]; ok {
			out[ip] = r
			continue
		}
		if isPrivateIP(ip) {
			g.entries[ip] = geoResult{}
			out[ip] = geoResult{}
			continue
		}
		misses = append(misses, ip)
	}
	g.mu.Unlock()

	if len(misses) == 0 {
		return out
	}

	resolved := g.fetchBatch(misses)

	g.mu.Lock()
	for _, ip := range misses {
		r := resolved[ip] // zero value if the API omitted/failed this IP
		g.entries[ip] = r
		out[ip] = r
	}
	g.mu.Unlock()

	return out
}

// fetchBatch calls the ip-api.com batch endpoint for up to 100 IPs at a time and
// returns the resolved countries keyed by IP. On any transport/decoding error it
// returns whatever it managed to collect (possibly empty) — GeoIP is best-effort.
func (g *geoCache) fetchBatch(ips []string) map[string]geoResult {
	out := make(map[string]geoResult, len(ips))

	const maxBatch = 100
	for start := 0; start < len(ips); start += maxBatch {
		end := start + maxBatch
		if end > len(ips) {
			end = len(ips)
		}
		chunk := ips[start:end]

		type query struct {
			Query  string `json:"query"`
			Fields string `json:"fields"`
		}
		body := make([]query, 0, len(chunk))
		for _, ip := range chunk {
			body = append(body, query{Query: ip, Fields: "status,country,countryCode,query"})
		}
		payload, err := json.Marshal(body)
		if err != nil {
			continue
		}

		req, err := http.NewRequest(http.MethodPost, "http://ip-api.com/batch", bytes.NewReader(payload))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := g.client.Do(req)
		if err != nil {
			continue
		}
		var results []struct {
			Status      string `json:"status"`
			Country     string `json:"country"`
			CountryCode string `json:"countryCode"`
			Query       string `json:"query"`
		}
		decErr := json.NewDecoder(resp.Body).Decode(&results)
		resp.Body.Close()
		if decErr != nil {
			continue
		}
		for _, r := range results {
			if r.Status != "success" {
				continue
			}
			out[r.Query] = geoResult{code: strings.ToUpper(r.CountryCode), name: r.Country}
		}
	}

	return out
}

// isPrivateIP reports whether ip is loopback, private, link-local, or otherwise
// not a public address that ip-api.com can resolve.
func isPrivateIP(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return true
	}
	return parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast() ||
		parsed.IsUnspecified() || parsed.IsMulticast()
}

// ---------------------------------------------------------------------------
// status parsing
// ---------------------------------------------------------------------------

var (
	reQuotedName = regexp.MustCompile(`"([^"]*)"`)
	reSteamID    = regexp.MustCompile(`STEAM_[0-5]:[01]:\d+|\[U:\d:\d+\]|7656\d{13}`)
	reIPPort     = regexp.MustCompile(`(\d{1,3}(?:\.\d{1,3}){3}):\d+`)
	reTimeConn   = regexp.MustCompile(`\b(\d+:\d{2}(?::\d{2})?)\b`)
	reUserID     = regexp.MustCompile(`^#?\s*(\d+)`)
)

// parseStatusPlayers extracts one Player per player row in the raw `status`
// output. CS2's player-table layout shifts across builds, so parsing is tolerant:
// a line is treated as a player row only if it contains both a quoted name and a
// SteamID, and every other field degrades gracefully when absent.
func parseStatusPlayers(raw string) []Player {
	var players []Player
	if raw == "" {
		return players
	}

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" {
			continue
		}

		steam := reSteamID.FindString(line)
		nameMatch := reQuotedName.FindStringSubmatch(line)
		if steam == "" || nameMatch == nil {
			continue // not a player row (header/summary/etc.)
		}

		p := Player{
			Name:    nameMatch[1],
			SteamID: steam,
			Team:    detectTeam(line),
		}

		if m := reUserID.FindStringSubmatch(line); m != nil {
			if id, err := strconv.Atoi(m[1]); err == nil {
				p.UserID = id
			}
		}

		// The remainder of the line after the SteamID holds the columns
		// (connected time, ping, loss, state, rate, address). Parse them from
		// there to avoid picking up digits inside the name or SteamID.
		rest := line
		if idx := strings.Index(line, steam); idx >= 0 {
			rest = line[idx+len(steam):]
		}

		if m := reTimeConn.FindString(rest); m != "" {
			p.Time = m
		}
		if ipm := reIPPort.FindStringSubmatch(rest); ipm != nil {
			// address column is present but not surfaced to the client; used
			// only for GeoIP resolution below.
			p.ipInternal = ipm[1]
		}

		// Ping: first standalone integer after the connected-time column.
		p.Ping = firstPingAfterTime(rest)

		players = append(players, p)
	}

	return players
}

// firstPingAfterTime returns the first integer that appears after the connected
// time token in the row remainder, which in CS2/Source layouts is the ping.
func firstPingAfterTime(rest string) int {
	work := rest
	if loc := reTimeConn.FindStringIndex(rest); loc != nil {
		work = rest[loc[1]:]
	}
	for _, f := range strings.Fields(work) {
		if n, err := strconv.Atoi(f); err == nil {
			return n
		}
	}
	return 0
}

// detectTeam maps team hints in a status row to CT / T / Spectator / Unassigned.
// Standard `status` output rarely includes team, so this defaults to Unassigned.
func detectTeam(line string) string {
	l := strings.ToLower(line)
	switch {
	case strings.Contains(l, "counter-terrorist") || strings.Contains(l, " ct ") || strings.Contains(l, "team_ct"):
		return "CT"
	case strings.Contains(l, "terrorist") || strings.Contains(l, " t ") || strings.Contains(l, "team_t"):
		return "T"
	case strings.Contains(l, "spectat"):
		return "Spectator"
	default:
		return "Unassigned"
	}
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// PlayersHandler returns the live player list for a server: it runs `status`
// over RCON, parses the per-player rows, and resolves each player's country from
// their IP via a cached ip-api.com lookup.
func PlayersHandler(rc *rcon.Manager, logger *slog.Logger) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		id := r.PathValue("id")
		if id == "" {
			return BadRequest("missing server id")
		}

		out, err := rc.Execute(id, "status")
		if err != nil {
			return WrapHTTP(err, http.StatusBadGateway, "rcon error")
		}

		players := parseStatusPlayers(out)

		ips := make([]string, 0, len(players))
		for _, p := range players {
			if p.ipInternal != "" {
				ips = append(ips, p.ipInternal)
			}
		}
		geos := geo.lookup(ips)
		for i := range players {
			if g, ok := geos[players[i].ipInternal]; ok {
				players[i].CountryCode = g.code
				players[i].CountryName = g.name
			}
		}

		// Never emit the internal IP field; return a clean array.
		resp := make([]Player, len(players))
		copy(resp, players)
		return JSON(w, http.StatusOK, resp)
	}
}

// BansHandler returns the merged ban list for a server: persistent bans from
// banned_users.cfg (via the configs.Manager) unioned with in-memory session bans
// from the `listid` RCON command, deduplicated by SteamID.
func BansHandler(rc *rcon.Manager, mgr *configs.Manager) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		id := r.PathValue("id")
		if id == "" {
			return BadRequest("missing server id")
		}

		// Keyed by normalised SteamID so cfg and session entries merge.
		bans := make(map[string]*Ban)

		// 1) Persistent bans from banned_users.cfg (best-effort: absent file is fine).
		if cfg, err := mgr.GetConfig(id, bannedUsersFile); err == nil {
			for _, b := range parseBannedUsersCfg(cfg.Content) {
				key := normSteamID(b.SteamID)
				b := b
				bans[key] = &b
			}
		} else if !errors.Is(err, store.ErrNotFound) {
			// A missing file surfaces as a read error (mounted) or ErrNotFound
			// (panel); both mean "no persistent bans", so we only log unexpected
			// panel-store failures implicitly by ignoring here.
			_ = err
		}

		// 2) Session bans from `listid`.
		if out, err := rc.Execute(id, "listid"); err == nil {
			for _, b := range parseListID(out) {
				key := normSteamID(b.SteamID)
				if existing, ok := bans[key]; ok {
					existing.Source = "both"
					if existing.ExpiresAt == "" {
						existing.ExpiresAt = b.ExpiresAt
					}
				} else {
					b := b
					bans[key] = &b
				}
			}
		}

		resp := make([]Ban, 0, len(bans))
		for _, b := range bans {
			if b.ExpiresAt == "" {
				b.ExpiresAt = "permanent"
			}
			resp = append(resp, *b)
		}
		sort.Slice(resp, func(i, j int) bool { return resp[i].SteamID < resp[j].SteamID })
		return JSON(w, http.StatusOK, resp)
	}
}

// UnbanHandler lifts a ban for a SteamID: it issues `removeid <steamid>` over
// RCON (removing any in-memory session ban) and removes the matching line from
// banned_users.cfg via the configs.Manager, so the ban does not return on the
// next map/config load.
func UnbanHandler(rc *rcon.Manager, mgr *configs.Manager, logger *slog.Logger) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		id := r.PathValue("id")
		steamid := r.PathValue("steamid")
		if id == "" || steamid == "" {
			return BadRequest("missing server id or steamid")
		}
		if reSteamID.FindString(steamid) == "" {
			return BadRequest("invalid steamid")
		}

		var rconErr, cfgErr string

		// 1) Remove the session ban via RCON.
		if _, err := rc.Execute(id, "removeid "+steamid); err != nil {
			rconErr = err.Error()
		}

		// 2) Remove the persistent line from banned_users.cfg (if the file exists
		//    and contains it).
		removed := false
		if cfg, err := mgr.GetConfig(id, bannedUsersFile); err == nil {
			newContent, changed := removeBanLine(cfg.Content, steamid)
			if changed {
				if err := mgr.SaveConfig(id, bannedUsersFile, newContent); err != nil {
					cfgErr = err.Error()
				} else {
					removed = true
				}
			}
		} else if !errors.Is(err, store.ErrNotFound) {
			// Non-fatal: file may simply not be present in this deployment.
			_ = err
		}

		if rconErr != "" && cfgErr != "" {
			return WrapHTTP(errors.New(rconErr+"; "+cfgErr), http.StatusBadGateway,
				"failed to unban: "+rconErr+"; "+cfgErr)
		}

		logger.Info("unban", "server", id, "steamid", steamid,
			"cfg_removed", removed, "rcon_error", rconErr, "cfg_error", cfgErr)

		return JSON(w, http.StatusOK, map[string]any{
			"ok":          true,
			"steamid":     steamid,
			"cfg_removed": removed,
			"rcon_error":  rconErr,
			"cfg_error":   cfgErr,
		})
	}
}

// ---------------------------------------------------------------------------
// ban parsing helpers
// ---------------------------------------------------------------------------

// parseBannedUsersCfg parses `banid <minutes> <steamid>` lines from a
// banned_users.cfg body. A minutes value of 0 means a permanent ban; any other
// numeric value is surfaced verbatim as expires_at (its semantics — remaining
// minutes vs. absolute unix — vary by writer, so it is passed through unchanged).
func parseBannedUsersCfg(content string) []Ban {
	var out []Ban
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		steam := reSteamID.FindString(line)
		if steam == "" {
			continue
		}
		b := Ban{SteamID: steam, Source: "cfg", ExpiresAt: "permanent"}

		// Look for the numeric argument in `banid <n> <steamid>`.
		fields := strings.Fields(line)
		for i, f := range fields {
			if strings.EqualFold(f, "banid") && i+1 < len(fields) {
				if n, err := strconv.ParseInt(fields[i+1], 10, 64); err == nil && n != 0 {
					b.ExpiresAt = strconv.FormatInt(n, 10)
				}
				break
			}
		}
		out = append(out, b)
	}
	return out
}

// parseListID parses the `listid` RCON output into session bans. The command
// prints one SteamID per entry line (formats vary across builds); we extract the
// SteamID token and any trailing minutes value.
func parseListID(raw string) []Ban {
	var out []Ban
	for _, raw := range strings.Split(raw, "\n") {
		line := strings.TrimSpace(raw)
		steam := reSteamID.FindString(line)
		if steam == "" {
			continue
		}
		b := Ban{SteamID: steam, Source: "session", ExpiresAt: "permanent"}
		// e.g. "... : 30 min" -> non-permanent; 0 min stays permanent.
		if m := regexp.MustCompile(`(\d+)\s*min`).FindStringSubmatch(line); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n != 0 {
				b.ExpiresAt = strconv.Itoa(n)
			}
		}
		out = append(out, b)
	}
	return out
}

// removeBanLine returns content with every line referencing steamid removed, and
// whether any line was removed. Matching is done on the normalised SteamID so
// cosmetic differences (case/spacing) do not prevent removal.
func removeBanLine(content, steamid string) (string, bool) {
	target := normSteamID(steamid)
	var kept []string
	changed := false
	for _, line := range strings.Split(content, "\n") {
		found := reSteamID.FindString(line)
		if found != "" && normSteamID(found) == target {
			changed = true
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n"), changed
}

// normSteamID lower-cases and trims a SteamID so equivalent tokens compare equal.
func normSteamID(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
