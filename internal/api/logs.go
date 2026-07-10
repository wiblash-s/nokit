// Live server-log streaming.
//
// This file implements an SSE (Server-Sent Events) endpoint that tails the
// stdout/stderr of the CS2 dedicated-server Docker container via
// `docker logs -f <container>` and relays each output line to the browser as
// an SSE `data:` event. The panel uses this to show live console output
// (map changes, workshop-map downloads, RCON echoes, plugin messages, etc.).
package api

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// defaultContainerName is used when CS2_CONTAINER_NAME is unset.
const defaultContainerName = "cs2"

// heartbeatInterval controls how often a comment frame is sent to keep the
// SSE connection (and any intermediary proxies) alive when the server is idle.
const heartbeatInterval = 15 * time.Second

// initialTailLines is how many historical lines `docker logs` emits before it
// starts following, so the panel isn't empty on connect.
const initialTailLines = "200"

// ContainerName returns the name of the CS2 Docker container to stream logs
// from, read from the CS2_CONTAINER_NAME env var (default "cs2").
func ContainerName() string {
	if name := strings.TrimSpace(os.Getenv("CS2_CONTAINER_NAME")); name != "" {
		return name
	}
	return defaultContainerName
}

// LogsStreamHandler streams `docker logs -f <container>` output to the client
// over Server-Sent Events.
//
// Behaviour:
//   - stdout and stderr of `docker logs` are merged into a single line stream.
//   - Each line is sent as an SSE event: `data: <line>\n\n`.
//   - A heartbeat comment (`: heartbeat\n\n`) is emitted every 15s.
//   - The child process is bound to the request context via CommandContext, so
//     it is killed automatically when the client disconnects; we also Wait on
//     it to reap the process and avoid zombies.
func LogsStreamHandler() Handler {
	return func(w http.ResponseWriter, r *http.Request) error {
		flusher, ok := w.(http.Flusher)
		if !ok {
			return WrapHTTP(nil, http.StatusInternalServerError, "streaming unsupported")
		}

		container := ContainerName()
		ctx := r.Context()

		// CommandContext ensures the process is killed when the client
		// disconnects (ctx cancelled) — this is our primary cleanup path.
		cmd := exec.CommandContext(ctx, "docker", "logs", "-f", "--tail", initialTailLines, container)

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return WrapHTTP(err, http.StatusInternalServerError, "could not open docker logs stdout")
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			return WrapHTTP(err, http.StatusInternalServerError, "could not open docker logs stderr")
		}

		if err := cmd.Start(); err != nil {
			// Most likely docker isn't installed / socket not mounted, or the
			// container name is wrong. Surface as JSON before any streaming.
			return WrapHTTP(err, http.StatusBadGateway, "could not start docker logs (is docker available and the container running?)")
		}

		// SSE response headers.
		h := w.Header()
		h.Set("Content-Type", "text/event-stream")
		h.Set("Cache-Control", "no-cache")
		h.Set("Connection", "keep-alive")
		// Disable proxy buffering (nginx) so events flush immediately.
		h.Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		// Merge stdout + stderr into a single channel of lines.
		lines := make(chan string, 512)
		var wg sync.WaitGroup
		scan := func(rc io.Reader) {
			defer wg.Done()
			sc := bufio.NewScanner(rc)
			// Allow long lines (workshop download URLs, stack traces, etc.).
			sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for sc.Scan() {
				select {
				case lines <- sc.Text():
				case <-ctx.Done():
					return
				}
			}
		}
		wg.Add(2)
		go scan(stdout)
		go scan(stderr)
		go func() {
			wg.Wait()
			close(lines)
		}()

		// Reap the child process on exit. CommandContext already sends a kill
		// on ctx cancellation; Wait releases its resources.
		defer func() { _ = cmd.Wait() }()

		heartbeat := time.NewTicker(heartbeatInterval)
		defer heartbeat.Stop()

		// Initial comment so the client's onopen fires with a real frame.
		fmt.Fprintf(w, ": connected to container %q\n\n", container)
		flusher.Flush()

		for {
			select {
			case <-ctx.Done():
				// Client disconnected; deferred cmd.Wait handles cleanup.
				return nil

			case <-heartbeat.C:
				if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
					return nil
				}
				flusher.Flush()

			case line, open := <-lines:
				if !open {
					// docker logs exited (container stopped/removed).
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

// writeSSEData writes a single log line as an SSE data event. A scanned line
// never contains a newline, so a single `data:` field is sufficient; we strip
// any trailing carriage return for clean rendering.
func writeSSEData(w io.Writer, line string) {
	line = strings.TrimRight(line, "\r")
	fmt.Fprintf(w, "data: %s\n\n", line)
}
