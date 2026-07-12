package server

import (
        "io/fs"
        "log/slog"
        "net/http"
        "os"
        "strings"

        "github.com/codevski/defuse/internal/api"
        "github.com/codevski/defuse/internal/auth"
        "github.com/codevski/defuse/internal/configs"
        "github.com/codevski/defuse/internal/loghub"
        "github.com/codevski/defuse/internal/rcon"
        "github.com/codevski/defuse/internal/steam"
        "github.com/codevski/defuse/internal/store"
)

type Server struct {
        logger  *slog.Logger
        dist    fs.FS
        store   *store.Store
        rcon    *rcon.Manager
        loghub  *loghub.Hub
        steam   *steam.Client
        creds   auth.Credentials
        configs *configs.Manager
}

func New(logger *slog.Logger, dist fs.FS, st *store.Store, mgr *rcon.Manager, hub *loghub.Hub, steamClient *steam.Client, creds auth.Credentials) *Server {
        return &Server{
                logger:  logger,
                dist:    dist,
                store:   st,
                rcon:    mgr,
                loghub:  hub,
                steam:   steamClient,
                creds:   creds,
                configs: configs.NewManager(st),
        }
}

func (s *Server) Handler() http.Handler {
        mux := http.NewServeMux()

        mux.Handle("POST /api/login", api.Wrap(s.logger, api.LoginHandler(s.creds, s.store)))
        mux.Handle("GET /api/health", api.Wrap(s.logger, s.health))

        // CS2 HTTP log ingest (`logaddress_add_http`). This is public and
        // unauthenticated by design — a game server cannot present a panel session
        // cookie. It shares the same loghub pipeline as the UDP listener, so HTTP
        // logs show up in the Live Logs view and drive workshop-map verification.
        // Set CS2_LOG_HTTP_TOKEN to require a `?token=`/`X-Log-Token` on the URL.
        mux.Handle("POST /api/logs/http", api.Wrap(s.logger, api.LogsIngestHTTPHandler(s.loghub, logHTTPToken())))

        protected := http.NewServeMux()
        protected.Handle("POST /api/logout", api.Wrap(s.logger, api.LogoutHandler(s.store)))
        protected.Handle("GET /api/me", api.Wrap(s.logger, api.MeHandler()))
        protected.Handle("GET /api/servers", api.Wrap(s.logger, api.ListServersHandler(s.store)))
        protected.Handle("POST /api/servers", api.Wrap(s.logger, api.AddServerHandler(s.store, s.rcon)))
        protected.Handle("DELETE /api/servers/{id}", api.Wrap(s.logger, api.DeleteServerHandler(s.store, s.rcon)))
        protected.Handle("POST /api/servers/{id}/rcon", api.Wrap(s.logger, api.RCONHandler(s.rcon)))
        protected.Handle("GET /api/servers/{id}/players", api.Wrap(s.logger, api.PlayersHandler(s.rcon, s.logger)))
        protected.Handle("GET /api/servers/{id}/bans", api.Wrap(s.logger, api.BansHandler(s.rcon, s.configs)))
        protected.Handle("DELETE /api/servers/{id}/bans/{steamid}", api.Wrap(s.logger, api.UnbanHandler(s.rcon, s.configs, s.logger)))
        protected.Handle("GET /api/servers/{id}/maps/workshop", api.Wrap(s.logger, api.WorkshopMapsHandler(s.rcon, s.steam, s.store, s.logger)))
        protected.Handle("POST /api/servers/{id}/maps/workshop/load", api.Wrap(s.logger, api.LoadWorkshopMapHandler(s.rcon, s.store, s.logger)))
        protected.Handle("DELETE /api/servers/{id}/maps/workshop/{workshopId}", api.Wrap(s.logger, api.UninstallWorkshopMapHandler(s.rcon, s.store, s.logger)))
        protected.Handle("GET /api/servers/{id}/configs", api.Wrap(s.logger, api.ListConfigsHandler(s.configs)))
        protected.Handle("GET /api/servers/{id}/configs/{name}", api.Wrap(s.logger, api.GetConfigHandler(s.configs)))
        protected.Handle("PUT /api/servers/{id}/configs/{name}", api.Wrap(s.logger, api.SaveConfigHandler(s.configs)))
        protected.Handle("DELETE /api/servers/{id}/configs/{name}", api.Wrap(s.logger, api.DeleteConfigHandler(s.configs)))
        protected.Handle("POST /api/servers/{id}/configs/{name}/exec", api.Wrap(s.logger, api.ExecConfigHandler(s.configs, s.rcon, s.logger)))
        protected.Handle("GET /api/maps/thumbnail/{id}", api.Wrap(s.logger, api.ThumbnailHandler(s.steam, s.logger)))
        protected.Handle("GET /api/logs/stream", api.Wrap(s.logger, api.LogsStreamHandler(s.loghub)))

        mux.Handle("/api/", auth.Middleware(s.store, protected))

        mux.Handle("/", s.spaHandler())

        return s.logRequests(s.recoverPanics(mux))
}

// logHTTPToken returns the optional shared secret that CS2 must present on the
// HTTP log ingest endpoint. Empty (the default) means the endpoint is open.
func logHTTPToken() string {
        return strings.TrimSpace(os.Getenv("CS2_LOG_HTTP_TOKEN"))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) error {
        return api.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) spaHandler() http.Handler {
        fileServer := http.FileServer(http.FS(s.dist))
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                f, err := s.dist.Open(r.URL.Path[1:])
                if err == nil {
                        f.Close()
                        fileServer.ServeHTTP(w, r)
                        return
                }
                r2 := r.Clone(r.Context())
                r2.URL.Path = "/"
                fileServer.ServeHTTP(w, r2)
        })
}

func (s *Server) logRequests(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                s.logger.Info("request", "method", r.Method, "path", r.URL.Path)
                next.ServeHTTP(w, r)
        })
}

func (s *Server) recoverPanics(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                defer func() {
                        if err := recover(); err != nil {
                                s.logger.Error("panic recovered", "error", err)
                                http.Error(w, "internal server error", http.StatusInternalServerError)
                        }
                }()
                next.ServeHTTP(w, r)
        })
}
