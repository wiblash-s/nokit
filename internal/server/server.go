package server

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/codevski/defuse/internal/api"
	"github.com/codevski/defuse/internal/rcon"
)

type Server struct {
	logger  *slog.Logger
	dist    fs.FS
	rconMgr *rcon.Manager
}

func New(logger *slog.Logger, dist fs.FS, rconMgr *rcon.Manager) *Server {
	return &Server{
		logger:  logger,
		dist:    dist,
		rconMgr: rconMgr,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /api/ping", api.Wrap(s.logger, s.ping))
	mux.Handle("POST /api/servers/{id}/rcon", api.Wrap(s.logger, s.execRCON))

	mux.Handle("/", http.FileServer(http.FS(s.dist)))

	return s.recoverPanics(s.logRequests(mux))
}

func (s *Server) ping(w http.ResponseWriter, r *http.Request) error {
	return api.JSON(w, http.StatusOK, map[string]string{"message": "pong from defuse"})
}

type rconRequest struct {
	Command string `json:"command"`
}

type rconResponse struct {
	Output string `json:"output"`
}

func (s *Server) execRCON(w http.ResponseWriter, r *http.Request) error {
	serverID := r.PathValue("id")
	if serverID == "" {
		return api.BadRequest("missing server id")
	}

	var req rconRequest
	if err := api.Decode(r, &req); err != nil {
		return err
	}
	req.Command = strings.TrimSpace(req.Command)
	if req.Command == "" {
		return api.BadRequest("command is required")
	}

	output, err := s.rconMgr.Execute(serverID, req.Command)
	if err != nil {
		return api.WrapHTTP(err, http.StatusBadGateway, "rcon execute failed")
	}

	return api.JSON(w, http.StatusOK, rconResponse{Output: output})
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start),
		)
	})
}

func (s *Server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.logger.Error("panic recovered",
					"method", r.Method,
					"path", r.URL.Path,
					"panic", rec,
				)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
