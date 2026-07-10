// Live server-log streaming.
//
// This file implements an SSE (Server-Sent Events) endpoint that relays CS2
// server log lines to the browser. Log lines are ingested over UDP via the
// classic Source `logaddress_add` mechanism (see internal/loghub) and fanned
// out to every connected SSE client. The panel uses this to show live console
// output (map changes, workshop-map downloads, RCON echoes, plugin messages,
// round events, etc.).
package api

import (
        "fmt"
        "io"
        "net/http"
        "strings"
        "time"

        "github.com/codevski/defuse/internal/loghub"
)

// heartbeatInterval controls how often a comment frame is sent to keep the
// SSE connection (and any intermediary proxies) alive when the server is idle.
const heartbeatInterval = 15 * time.Second

// maxLogBody caps the size of an HTTP log ingest body we will read, guarding
// against a runaway/oversized POST. CS2 log batches are small; 1 MiB is ample.
const maxLogBody = 1 << 20

// LogsIngestHTTPHandler ingests CS2 server logs delivered over HTTP via the
// `logaddress_add_http <url>` mechanism. Unlike the classic UDP `logaddress_add`
// transport (handled by the loghub's UDP socket), CS2 delivers HTTP logs as a
// plain-text POST body containing one or more log lines.
//
// Each received line is parsed (loghub.ParseHTTPBody) and published into the
// exact same hub used by the UDP listener (hub.Publish). This means HTTP-sourced
// lines appear in the same Live Logs SSE stream and drive the same workshop-map
// download verification — the two listeners coexist and share one pipeline.
//
// This endpoint is intentionally unauthenticated (a game server cannot present a
// panel session cookie). When CS2_LOG_HTTP_TOKEN is configured, the handler
// requires a matching token, supplied either as a `?token=` query parameter or
// an `X-Log-Token` header, so the endpoint can be locked down if desired.
func LogsIngestHTTPHandler(hub *loghub.Hub, token string) Handler {
        return func(w http.ResponseWriter, r *http.Request) error {
                if hub == nil {
                        return WrapHTTP(nil, http.StatusServiceUnavailable, "log hub is not configured")
                }

                if token != "" {
                        got := r.URL.Query().Get("token")
                        if got == "" {
                                got = r.Header.Get("X-Log-Token")
                        }
                        if got != token {
                                return WrapHTTP(nil, http.StatusUnauthorized, "invalid log token")
                        }
                }

                body, err := io.ReadAll(io.LimitReader(r.Body, maxLogBody))
                if err != nil {
                        return WrapHTTP(err, http.StatusBadRequest, "could not read log body")
                }

                for _, line := range loghub.ParseHTTPBody(body) {
                        hub.Publish(line)
                }

                // CS2 does not inspect the response body; a 200 is enough to ack.
                w.WriteHeader(http.StatusOK)
                return nil
        }
}

// LogsStreamHandler streams CS2 log lines (received over UDP by the loghub) to
// the client over Server-Sent Events.
//
// Behaviour:
//   - Subscribes to the hub; each received line is sent as `data: <line>\n\n`.
//   - A heartbeat comment (`: heartbeat\n\n`) is emitted every 15s.
//   - The subscription is removed when the client disconnects (ctx cancelled).
func LogsStreamHandler(hub *loghub.Hub) Handler {
        return func(w http.ResponseWriter, r *http.Request) error {
                flusher, ok := w.(http.Flusher)
                if !ok {
                        return WrapHTTP(nil, http.StatusInternalServerError, "streaming unsupported")
                }
                if hub == nil {
                        return WrapHTTP(nil, http.StatusServiceUnavailable, "log hub is not configured")
                }

                ctx := r.Context()

                // SSE response headers.
                h := w.Header()
                h.Set("Content-Type", "text/event-stream")
                h.Set("Cache-Control", "no-cache")
                h.Set("Connection", "keep-alive")
                // Disable proxy buffering (nginx) so events flush immediately.
                h.Set("X-Accel-Buffering", "no")
                w.WriteHeader(http.StatusOK)

                id, lines := hub.Subscribe()
                defer hub.Unsubscribe(id)

                heartbeat := time.NewTicker(heartbeatInterval)
                defer heartbeat.Stop()

                // Initial comment so the client's onopen fires with a real frame.
                fmt.Fprint(w, ": connected to CS2 log stream\n\n")
                flusher.Flush()

                for {
                        select {
                        case <-ctx.Done():
                                // Client disconnected; deferred Unsubscribe handles cleanup.
                                return nil

                        case <-heartbeat.C:
                                if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
                                        return nil
                                }
                                flusher.Flush()

                        case line, open := <-lines:
                                if !open {
                                        // Hub shut down.
                                        fmt.Fprint(w, "event: end\ndata: log stream ended\n\n")
                                        flusher.Flush()
                                        return nil
                                }
                                writeSSEData(w, line)
                                flusher.Flush()
                        }
                }
        }
}

// writeSSEData writes a single log line as an SSE data event. A log line never
// contains a newline, so a single `data:` field is sufficient; we strip any
// trailing carriage return for clean rendering.
func writeSSEData(w io.Writer, line string) {
        line = strings.TrimRight(line, "\r")
        fmt.Fprintf(w, "data: %s\n\n", line)
}
