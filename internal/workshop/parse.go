package workshop

import (
        "regexp"
        "strings"
)

// workshopPathRe matches the "workshop/<id>/<mapname>" pattern that appears in
// the output of CS2's "maps *" RCON command (used as a fallback when
// ds_workshop_listmaps is unavailable).
var workshopPathRe = regexp.MustCompile(`workshop[/\\](\d+)[/\\](\S+)`)

// mapNameRe matches a plausible CS2 map name token: letters, digits, and
// underscores. Real map names look like de_dust2, aim_redline, cs_office,
// de_drachenschanze, etc.
var mapNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_]*$`)

// ParsePathMaps extracts (workshopID, name) pairs from "maps *" output, where
// workshop maps are reported as "workshop/<id>/<name>". Results are de-duplicated
// by workshop ID. This is the legacy path-based parser, kept as a fallback.
func ParsePathMaps(output string) []Map {
        var maps []Map
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
                id := m[1]
                name := strings.TrimSuffix(m[2], ".vpk")
                if seen[id] {
                        continue
                }
                seen[id] = true
                maps = append(maps, Map{WorkshopID: id, Name: name})
        }
        return maps
}

// looksLikeUnknownCommand reports whether RCON output indicates the command was
// not recognised by the server (so we should fall back to another command).
func looksLikeUnknownCommand(output string) bool {
        o := strings.ToLower(output)
        return strings.Contains(o, "unknown command") || strings.Contains(o, "unknown console command")
}

// ParseListmaps extracts bare map names from `ds_workshop_listmaps` output.
//
// The command prints one map name per line, e.g.:
//
//	de_drachenschanze
//	aim_redline
//
// Some plugin builds prefix or annotate lines; this parser is deliberately
// lenient: it trims each line, strips any "workshop/<id>/" prefix and ".vpk"
// suffix, takes the first whitespace-separated token, and keeps it only if it
// looks like a map name. Blank lines and non-map noise are skipped. Results are
// de-duplicated, preserving first-seen order.
func ParseListmaps(output string) []string {
        var names []string
        seen := make(map[string]bool)

        for _, raw := range strings.Split(output, "\n") {
                line := strings.TrimSpace(raw)
                if line == "" {
                        continue
                }
                // A workshop path form still yields a usable name.
                if m := workshopPathRe.FindStringSubmatch(line); m != nil {
                        line = m[2]
                }
                // ds_workshop_listmaps prints exactly one map name per line. Anything with
                // embedded whitespace is status/echo noise (e.g. "Unknown command ..."),
                // so require a single token before treating it as a map name.
                fields := strings.Fields(line)
                if len(fields) != 1 {
                        continue
                }
                token := strings.TrimSuffix(fields[0], ".vpk")
                if !mapNameRe.MatchString(token) {
                        continue
                }
                if seen[token] {
                        continue
                }
                seen[token] = true
                names = append(names, token)
        }
        return names
}

// statusMapRe matches the map line of CS2 `status` output, tolerating the
// several shapes it takes across builds, e.g.:
//
//	map     : de_dust2
//	loaded spawngroup(  1)  : SV:  [1: de_dust2 | ...]
//	Map: workshop/3070900859/de_cache_redux
var statusMapRe = regexp.MustCompile(`(?im)^\s*map\s*:\s*(\S+)`)

// ParseStatusMap extracts the currently loaded map name from `status` output,
// stripping any "workshop/<id>/" prefix and ".vpk" suffix. It returns "" when no
// map line is found.
func ParseStatusMap(output string) string {
        m := statusMapRe.FindStringSubmatch(output)
        if m == nil {
                return ""
        }
        val := strings.TrimSpace(m[1])
        if p := workshopPathRe.FindStringSubmatch(val); p != nil {
                val = p[2]
        }
        val = strings.TrimSuffix(val, ".vpk")
        if !mapNameRe.MatchString(val) {
                return ""
        }
        return val
}
