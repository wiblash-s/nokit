package main

import (
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
)

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestResolveSink(t *testing.T) {
	log := quietLogger()

	// A literal IPv4 host must pass through unchanged.
	if got := resolveSink(log, "10.0.0.5:27500"); got != "10.0.0.5:27500" {
		t.Errorf("literal IP: got %q, want unchanged", got)
	}

	// A malformed value (no port) must pass through unchanged.
	if got := resolveSink(log, "defuse"); got != "defuse" {
		t.Errorf("no port: got %q, want unchanged", got)
	}

	// A resolvable hostname must be converted to ip:port, preserving the port.
	got := resolveSink(log, "localhost:27500")
	host, port, err := net.SplitHostPort(got)
	if err != nil {
		t.Fatalf("resolved value %q is not host:port: %v", got, err)
	}
	if port != "27500" {
		t.Errorf("port not preserved: got %q", port)
	}
	if net.ParseIP(host) == nil {
		t.Errorf("host not resolved to an IP: got %q", host)
	}
	if strings.Contains(got, "localhost") {
		t.Errorf("hostname was not replaced with an IP: %q", got)
	}
}
