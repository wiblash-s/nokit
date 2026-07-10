package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/codevski/defuse/assets"
	"github.com/codevski/defuse/internal/auth"
	"github.com/codevski/defuse/internal/loghub"
	"github.com/codevski/defuse/internal/rcon"
	"github.com/codevski/defuse/internal/server"
	"github.com/codevski/defuse/internal/store"
)

// defaultLogPort is the UDP port the log hub binds to receive CS2 logs when
// CS2_LOG_LISTEN_PORT is unset.
const defaultLogPort = 27500

// logListenPort returns the UDP port to bind for CS2 log ingestion.
func logListenPort() int {
	if v := strings.TrimSpace(os.Getenv("CS2_LOG_LISTEN_PORT")); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 && p < 65536 {
			return p
		}
	}
	return defaultLogPort
}

// configureLogAddress points each CS2 server at our UDP log sink via RCON.
// The sink address (what the CS2 server should send its logs to, as reachable
// from the CS2 container — e.g. "defuse:27500") comes from CS2_LOG_SINK_ADDR.
// If unset, we assume the user configures logaddress themselves (autoexec /
// container env) and skip auto-configuration.
//
// It retries for a while because the CS2 server may still be starting up when
// the panel boots.
func configureLogAddress(logger *slog.Logger, mgr *rcon.Manager, servers []store.Server) {
	sink := strings.TrimSpace(os.Getenv("CS2_LOG_SINK_ADDR"))
	if sink == "" {
		logger.Info("CS2_LOG_SINK_ADDR unset; skipping RCON logaddress auto-config",
			"hint", "set it (e.g. defuse:27500) or configure logaddress_add in the CS2 server yourself")
		return
	}

	for _, sv := range servers {
		go func(id string) {
			cmds := []string{
				"logaddress_delall",
				"logaddress_add " + sink,
				"log on",
			}
			for attempt := 1; attempt <= 30; attempt++ {
				ok := true
				for _, c := range cmds {
					if _, err := mgr.Execute(id, c); err != nil {
						ok = false
						break
					}
				}
				if ok {
					logger.Info("configured CS2 logaddress", "server", id, "sink", sink)
					return
				}
				time.Sleep(10 * time.Second)
			}
			logger.Warn("gave up configuring CS2 logaddress via RCON", "server", id, "sink", sink)
		}(sv.ID)
	}
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	creds, err := auth.LoadCredentials()
	if err != nil {
		logger.Error("credentials", "error", err)
		os.Exit(1)
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "nokit.db"
	}
	st, err := store.Open(dbPath)
	if err != nil {
		logger.Error("store", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	mgr := rcon.New(logger)
	servers, err := st.ListServers()
	if err != nil {
		logger.Error("list servers", "error", err)
		os.Exit(1)
	}
	for _, sv := range servers {
		if err := mgr.Connect(sv.ID, sv.Host, sv.RCONPass); err != nil {
			logger.Warn("rcon connect", "server", sv.ID, "error", err)
		} else {
			logger.Info("rcon connected", "server", sv.ID, "host", sv.Host)
		}
	}

	// Live Logs: bind a UDP socket that receives CS2 server logs (delivered via
	// `logaddress_add`) and fans them out to the SSE panel.
	hub := loghub.New(logger)
	if err := hub.Listen(logListenPort()); err != nil {
		// Non-fatal: the rest of the panel still works; Live Logs just won't
		// receive anything until the port is available.
		logger.Warn("log hub", "error", err)
	}
	defer hub.Close()

	// Tell each CS2 server where to send its logs (best-effort, retries).
	configureLogAddress(logger, mgr, servers)

	dist, err := assets.FS()
	if err != nil {
		logger.Error("assets", "error", err)
		os.Exit(1)
	}

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	srv := server.New(logger, dist, st, mgr, hub, creds)
	httpSrv := &http.Server{
		Addr:    addr,
		Handler: srv.Handler(),
	}

	go func() {
		logger.Info("listening", "addr", addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("listen", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		logger.Error("shutdown", "error", err)
	}
	logger.Info("shutdown complete")
}
