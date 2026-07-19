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
        authn   *auth.Authenticator
        configs *configs.Manager
}

func New(logger *slog.Logger, dist fs.FS, st *store.Store, mgr *rcon.Manager, hub *loghub.Hub, steamClient *steam.Client, authn *auth.Authenticator) *Server {
        return &Server{
                logger:  logger,
                dist:    dist,
                store:   st,
                rcon:    mgr,
                loghub:  hub,
                steam:   steamClient,
                authn:   authn,
                configs: configs.NewManager(st),
        }
}

func (s *Server) Handler() http.Handler {
        mux := http.NewServeMux()

        // Public auth endpoints.
        mux.Handle("GET /api/auth/config", api.Wrap(s.logger, api.AuthConfigHandler(s.authn)))
        mux.Handle("POST /api/login", api.Wrap(s.logger, api.LoginHandler(s.authn)))
        // OIDC redirect flow. These write redirects directly rather than JSON, so
        // they are registered as raw handlers (not wrapped by api.Wrap).
        mux.HandleFunc("GET /api/auth/login", s.authn.BeginLogin)
        mux.HandleFunc("GET /api/auth/callback", s.authn.HandleCallback)
        mux.Handle("GET /api/health", api.Wrap(s.logger, s.health))

        // CS2 HTTP log ingest (`logaddress_add_http`). This is public and
        // unauthenticated by design — a game server cannot present a panel session
        // cookie. It shares the same loghub pipeline as the UDP listener, so HTTP
        // logs show up in the Live Logs view and drive workshop-map verification.
        // Set CS2_LOG_HTTP_TOKEN to require a `?token=`/`X-Log-Token` on the URL.
        mux.Handle("POST /api/logs/http", api.Wrap(s.logger, api.LogsIngestHTTPHandler(s.loghub, logHTTPToken())))

        protected := http.NewServeMux()

        // Any authenticated user (with a role) may inspect their own session and log out.
        protected.Handle("POST /api/logout", api.Wrap(s.logger, api.LogoutHandler(s.authn)))
        protected.Handle("GET /api/me", api.Wrap(s.logger, api.MeHandler()))

        // Read-only (viewer and up).
        protected.Handle("GET /api/servers", api.Wrap(s.logger, api.RequirePerm(auth.PermViewDashboard, api.ListServersHandler(s.store))))
        protected.Handle("GET /api/servers/{id}/players", api.Wrap(s.logger, api.RequirePerm(auth.PermViewPlayers, api.PlayersHandler(s.rcon, s.logger))))
        protected.Handle("GET /api/servers/{id}/bans", api.Wrap(s.logger, api.RequirePerm(auth.PermViewPlayers, api.BansHandler(s.rcon, s.configs))))
        protected.Handle("GET /api/servers/{id}/maps/workshop", api.Wrap(s.logger, api.RequirePerm(auth.PermViewDashboard, api.WorkshopMapsHandler(s.rcon, s.steam, s.store, s.logger))))
        protected.Handle("GET /api/maps/thumbnail/{id}", api.Wrap(s.logger, api.RequirePerm(auth.PermViewDashboard, api.ThumbnailHandler(s.steam, s.logger))))
        protected.Handle("GET /api/logs/stream", api.Wrap(s.logger, api.RequirePerm(auth.PermViewLogs, api.LogsStreamHandler(s.loghub))))

        // Operator: RCON console.
        protected.Handle("POST /api/servers/{id}/rcon", api.Wrap(s.logger, api.RequirePerm(auth.PermSendConsoleCommand, api.Audited(s.store, "rcon.command", api.RCONHandler(s.rcon)))))

        // Admin: server management, workshop maps, config editing.
        protected.Handle("POST /api/servers", api.Wrap(s.logger, api.RequirePerm(auth.PermAddServer, api.Audited(s.store, "server.add", api.AddServerHandler(s.store, s.rcon)))))
        protected.Handle("DELETE /api/servers/{id}/bans/{steamid}", api.Wrap(s.logger, api.RequirePerm(auth.PermUnbanPlayer, api.Audited(s.store, "player.unban", api.UnbanHandler(s.rcon, s.configs, s.logger)))))
        protected.Handle("POST /api/servers/{id}/maps/workshop/load", api.Wrap(s.logger, api.RequirePerm(auth.PermManageWorkshop, api.Audited(s.store, "workshop.load", api.LoadWorkshopMapHandler(s.rcon, s.store, s.logger)))))
        protected.Handle("DELETE /api/servers/{id}/maps/workshop/{workshopId}", api.Wrap(s.logger, api.RequirePerm(auth.PermManageWorkshop, api.Audited(s.store, "workshop.uninstall", api.UninstallWorkshopMapHandler(s.rcon, s.store, s.logger)))))
        protected.Handle("GET /api/servers/{id}/configs", api.Wrap(s.logger, api.RequirePerm(auth.PermEditConfig, api.ListConfigsHandler(s.configs))))
        protected.Handle("GET /api/servers/{id}/configs/{name}", api.Wrap(s.logger, api.RequirePerm(auth.PermEditConfig, api.GetConfigHandler(s.configs))))
        protected.Handle("PUT /api/servers/{id}/configs/{name}", api.Wrap(s.logger, api.RequirePerm(auth.PermEditConfig, api.Audited(s.store, "config.save", api.SaveConfigHandler(s.configs)))))
        protected.Handle("POST /api/servers/{id}/configs/{name}/exec", api.Wrap(s.logger, api.RequirePerm(auth.PermExecConfig, api.Audited(s.store, "config.exec", api.ExecConfigHandler(s.configs, s.rcon, s.logger)))))

        // Superadmin: destructive operations + audit log.
        protected.Handle("DELETE /api/servers/{id}", api.Wrap(s.logger, api.RequirePerm(auth.PermDeleteServer, api.Audited(s.store, "server.delete", api.DeleteServerHandler(s.store, s.rcon)))))
        protected.Handle("DELETE /api/servers/{id}/configs/{name}", api.Wrap(s.logger, api.RequirePerm(auth.PermDeleteConfig, api.Audited(s.store, "config.delete", api.DeleteConfigHandler(s.configs)))))
        protected.Handle("GET /api/audit", api.Wrap(s.logger, api.RequirePerm(auth.PermViewAudit, api.AuditLogHandler(s.store))))

        mux.Handle("/api/", s.authn.Middleware(protected))

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
