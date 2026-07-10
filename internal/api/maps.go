package api

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/codevski/defuse/internal/rcon"
)

// WorkshopMapInfo represents a single installed workshop map returned by the API.
type WorkshopMapInfo struct {
	WorkshopID string `json:"workshopId"`
	Name       string `json:"name"`
}

// WorkshopMapsHandler lists installed workshop maps by executing the RCON
// command "maps *" and extracting entries whose path starts with "workshop/".
//
// CS2 dedicated servers report workshop maps in the following format:
//
//	workshop/3070900859/de_cache_redux
//
// This handler parses that format and de-duplicates results by workshop ID.
func WorkshopMapsHandler(mgr *rcon.Manager) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		id := r.PathValue("id")
		if id == "" {
			return BadRequest("missing server id")
		}

		out, err := mgr.Execute(id, "maps *")
		if err != nil {
			return WrapHTTP(err, http.StatusBadGateway, "rcon error")
		}

		maps := parseWorkshopMaps(out)
		return JSON(w, http.StatusOK, maps)
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
