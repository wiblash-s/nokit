package api

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/codevski/defuse/internal/loghub"
)

// freeUDPPort asks the OS for an available UDP port and returns it.
func freeUDPPort(t *testing.T) int {
	t.Helper()
	c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("probe udp port: %v", err)
	}
	port := c.LocalAddr().(*net.UDPAddr).Port
	c.Close()
	return port
}

// netDialUDP dials a UDP "connection" to 127.0.0.1:<port>.
func netDialUDP(port int) (net.Conn, error) {
	return net.Dial("udp", net.JoinHostPort("127.0.0.1", strconvItoa(port)))
}

func strconvItoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [6]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// feed simulates a CS2 server sending a UDP log datagram to the hub-bound port.
func feedUDP(t *testing.T, port int, body string) {
	t.Helper()
	conn, err := netDialUDP(port)
	if err != nil {
		t.Fatalf("dial udp: %v", err)
	}
	defer conn.Close()
	packet := append([]byte{0xFF, 0xFF, 0xFF, 0xFF, 'R'}, []byte("L "+body+"\x00")...)
	if _, err := conn.Write(packet); err != nil {
		t.Fatalf("write udp: %v", err)
	}
}

func TestLogsStreamHandler(t *testing.T) {
	hub := loghub.New(discardLogger())
	port := freeUDPPort(t)
	if err := hub.Listen(port); err != nil {
		t.Fatalf("hub listen: %v", err)
	}
	defer hub.Close()

	handler := Wrap(discardLogger(), LogsStreamHandler(hub))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/logs/stream", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	// Give the handler a moment to subscribe before feeding a line.
	time.Sleep(100 * time.Millisecond)
	feedUDP(t, port, "07/10/2026 - 16:16:14: host_workshop_map 3070263842")

	// Wait until the recorder has captured the data frame.
	deadline := time.After(3 * time.Second)
	for {
		if strings.Contains(rec.Body.String(), "host_workshop_map 3070263842") {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("did not receive log line; got:\n%s", rec.Body.String())
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	body := rec.Body.String()
	if !strings.Contains(body, ": connected to CS2 log stream") {
		t.Errorf("missing initial connect comment; got:\n%s", body)
	}
	if !strings.Contains(body, "data: 07/10/2026 - 16:16:14: host_workshop_map 3070263842") {
		t.Errorf("missing SSE data frame; got:\n%s", body)
	}

	// Check SSE headers.
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not exit after client disconnect")
	}
}

func TestLogsStreamHandlerNilHub(t *testing.T) {
	handler := Wrap(discardLogger(), LogsStreamHandler(nil))
	req := httptest.NewRequest(http.MethodGet, "/api/logs/stream", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

// scanSSE is a small helper kept for potential future streaming assertions.
func scanSSE(r io.Reader, n int) []string {
	sc := bufio.NewScanner(r)
	var out []string
	for sc.Scan() && len(out) < n {
		out = append(out, sc.Text())
	}
	return out
}

var _ = scanSSE
