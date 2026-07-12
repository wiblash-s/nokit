package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/codevski/defuse/internal/configs"
	"github.com/codevski/defuse/internal/rcon"
	"github.com/codevski/defuse/internal/store"
)

// ConfigInfo is a single config in a list response (no content).
type ConfigInfo struct {
	Name string `json:"name"`
	Mode string `json:"mode"`
}

// ConfigListResponse is the payload of the list endpoint. It reports the active
// mode and, for mounted mode, whether the mount is writable.
type ConfigListResponse struct {
	// Mode is "mounted" or "panel".
	Mode string `json:"mode"`
	// Writable is true in panel mode, and in mounted mode when the mount is
	// read-write.
	Writable bool         `json:"writable"`
	Configs  []ConfigInfo `json:"configs"`
}

// ConfigDetail is the payload of the single-config endpoint.
type ConfigDetail struct {
	Name     string `json:"name"`
	Content  string `json:"content"`
	Mode     string `json:"mode"`
	Writable bool   `json:"writable"`
}

// SaveConfigRequest is the body of the PUT endpoint.
type SaveConfigRequest struct {
	Content string `json:"content"`
}

// ExecResult summarizes a config exec. In mounted mode a single `exec <name>`
// command is sent; in panel mode each non-empty, non-comment line is replayed.
type ExecResult struct {
	Mode         string   `json:"mode"`
	CommandsSent int      `json:"commands_sent"`
	Errors       []string `json:"errors"`
}

// ListConfigsHandler lists all configs for a server, in mounted or panel mode.
func ListConfigsHandler(mgr *configs.Manager) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		id := r.PathValue("id")
		if id == "" {
			return BadRequest("missing server id")
		}
		mode, writable := mgr.GetMode(id)
		list, err := mgr.ListConfigs(id)
		if err != nil {
			return WrapHTTP(err, http.StatusInternalServerError, "failed to list configs")
		}
		out := make([]ConfigInfo, 0, len(list))
		for _, c := range list {
			out = append(out, ConfigInfo{Name: c.Name, Mode: c.Mode})
		}
		return JSON(w, http.StatusOK, ConfigListResponse{Mode: mode, Writable: writable, Configs: out})
	}
}

// GetConfigHandler returns a single config's content.
func GetConfigHandler(mgr *configs.Manager) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		id := r.PathValue("id")
		name := r.PathValue("name")
		if id == "" || name == "" {
			return BadRequest("missing server id or config name")
		}
		c, err := mgr.GetConfig(id, name)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return NotFound("config not found")
			}
			return WrapHTTP(err, http.StatusBadRequest, "failed to read config")
		}
		return JSON(w, http.StatusOK, ConfigDetail{
			Name:     c.Name,
			Content:  c.Content,
			Mode:     c.Mode,
			Writable: c.Writable,
		})
	}
}

// SaveConfigHandler creates or updates a config.
func SaveConfigHandler(mgr *configs.Manager) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		id := r.PathValue("id")
		name := r.PathValue("name")
		if id == "" || name == "" {
			return BadRequest("missing server id or config name")
		}
		var req SaveConfigRequest
		if err := Decode(r, &req); err != nil {
			return err
		}
		if err := mgr.SaveConfig(id, name, req.Content); err != nil {
			return WrapHTTP(err, http.StatusBadRequest, "failed to save config")
		}
		return JSON(w, http.StatusOK, map[string]string{"ok": "true"})
	}
}

// DeleteConfigHandler removes a config.
func DeleteConfigHandler(mgr *configs.Manager) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		id := r.PathValue("id")
		name := r.PathValue("name")
		if id == "" || name == "" {
			return BadRequest("missing server id or config name")
		}
		if err := mgr.DeleteConfig(id, name); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return NotFound("config not found")
			}
			return WrapHTTP(err, http.StatusBadRequest, "failed to delete config")
		}
		return JSON(w, http.StatusOK, map[string]string{"ok": "true"})
	}
}

// ExecConfigHandler runs a config on the server.
//
//   - Mounted mode: sends a single `exec <name>` RCON command, letting the game
//     server read the file from its own cfg directory.
//   - Panel mode: reads the stored config, splits it into individual commands
//     (dropping blanks and // comments) and sends each over RCON, collecting any
//     per-command errors into the response.
func ExecConfigHandler(mgr *configs.Manager, rc *rcon.Manager, logger *slog.Logger) Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		id := r.PathValue("id")
		name := r.PathValue("name")
		if id == "" || name == "" {
			return BadRequest("missing server id or config name")
		}

		mode, _ := mgr.GetMode(id)

		if mode == configs.ModeMount {
			cmd := "exec " + configs.StripExt(name)
			if _, err := rc.Execute(id, cmd); err != nil {
				return WrapHTTP(err, http.StatusBadGateway, "rcon error")
			}
			logger.Info("executed config via exec", "server", id, "config", name)
			return JSON(w, http.StatusOK, ExecResult{Mode: mode, CommandsSent: 1, Errors: []string{}})
		}

		// Panel mode: replay each command line over RCON.
		cfg, err := mgr.GetConfig(id, name)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return NotFound("config not found")
			}
			return WrapHTTP(err, http.StatusBadRequest, "failed to read config")
		}
		lines := configs.ExecLines(cfg.Content)
		sent := 0
		errs := []string{}
		for _, line := range lines {
			if _, err := rc.Execute(id, line); err != nil {
				errs = append(errs, line+": "+err.Error())
				continue
			}
			sent++
		}
		logger.Info("executed config via panel replay", "server", id, "config", name, "sent", sent, "errors", len(errs))
		return JSON(w, http.StatusOK, ExecResult{Mode: mode, CommandsSent: sent, Errors: errs})
	}
}
