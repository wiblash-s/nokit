package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// writeFakeDocker installs a fake `docker` executable on PATH that, for
// `docker logs -f ...`, prints one line to stdout and one to stderr and then
// blocks (simulating `-f` following) until it is killed. This lets us exercise
// the SSE handler's framing, stderr-merge and cleanup paths without a real
// Docker daemon.
func writeFakeDocker(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker shim is POSIX-only")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"echo \"stdout hello from $3\"\n" +
		"echo \"stderr warning line\" 1>&2\n" +
		"# block to emulate `-f` follow mode until killed\n" +
		"while true; do sleep 0.05; done\n"
	p := filepath.Join(dir, "docker")
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake docker: %v", err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+oldPath)
}

func TestContainerNameDefault(t *testing.T) {
	t.Setenv("CS2_CONTAINER_NAME", "")
	if got := ContainerName(); got != "cs2" {
		t.Fatalf("default container name = %q, want cs2", got)
	}
	t.Setenv("CS2_CONTAINER_NAME", "  my-cs2  ")
	if got := ContainerName(); got != "my-cs2" {
		t.Fatalf("container name = %q, want my-cs2 (trimmed)", got)
	}
}

func TestLogsStreamHandler(t *testing.T) {
	writeFakeDocker(t)
	t.Setenv("CS2_CONTAINER_NAME", "cs2")

	h := LogsStreamHandler()
	wrapped := func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			t.Errorf("handler returned error: %v", err)
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(wrapped))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	// Read SSE frames until we've seen both the stdout and stderr lines.
	sc := bufio.NewScanner(resp.Body)
	var gotStdout, gotStderr, gotConnect bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, ": connected"):
				gotConnect = true
			case strings.HasPrefix(line, "data: stdout hello"):
				gotStdout = true
			case strings.HasPrefix(line, "data: stderr warning line"):
				gotStderr = true
			}
			if gotStdout && gotStderr {
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for SSE frames")
	}

	if !gotConnect {
		t.Error("missing initial ': connected' comment frame")
	}
	if !gotStdout {
		t.Error("missing stdout data frame")
	}
	if !gotStderr {
		t.Error("missing merged stderr data frame")
	}

	// Disconnecting the client should cause the handler to return and the
	// fake docker child to be killed (CommandContext). Cancelling here
	// exercises that cleanup path without hanging.
	cancel()
	resp.Body.Close()
	time.Sleep(200 * time.Millisecond)
}
