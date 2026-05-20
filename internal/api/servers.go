package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/codevski/defuse/internal/config"
)

type ServerInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Host string `json:"host"`
}

func ServersHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out := make([]ServerInfo, 0, len(cfg.Servers))
		for _, s := range cfg.Servers {
			out = append(out, ServerInfo{
				ID:   s.ID,
				Name: s.Name,
				Host: s.RCONHost,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(out); err != nil {
			slog.Error("encode servers list", "err", err)
		}
	}
}
