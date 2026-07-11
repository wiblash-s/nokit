// Package rcon manages connections to CS2 servers over the Source RCON
// protocol. The Manager owns a pool of clients keyed by server ID and
// handles reconnection on failure.
package rcon

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gorcon/rcon"
)

type Client interface {
	Execute(command string) (string, error)
	Close() error
}

var ErrNotConnected = errors.New("rcon: not connected")

type connection struct {
	id       string
	host     string
	password string

	mu           sync.Mutex
	client       Client
	lastFailTime time.Time
	failCount    int
}

type Manager struct {
	logger *slog.Logger

	mu          sync.RWMutex
	connections map[string]*connection
}

func New(logger *slog.Logger) *Manager {
	return &Manager{
		logger:      logger,
		connections: make(map[string]*connection),
	}
}

func (m *Manager) Connect(serverID, host, password string) error {
	conn := &connection{
		id:       serverID,
		host:     host,
		password: password,
	}
	m.mu.Lock()
	m.connections[serverID] = conn
	m.mu.Unlock()
	if err := m.dial(conn); err != nil {
		m.logger.Warn("rcon initial connect failed",
			"server", serverID,
			"host", host,
			"error", err,
		)
		return err
	}
	return nil
}

func (m *Manager) dial(conn *connection) error {
	// Check if we should apply backoff due to recent failures
	conn.mu.Lock()
	if conn.failCount > 0 {
		backoffDuration := m.calculateBackoff(conn.failCount)
		timeSinceLastFail := time.Since(conn.lastFailTime)
		if timeSinceLastFail < backoffDuration {
			conn.mu.Unlock()
			return fmt.Errorf("rcon dial %s: backing off for %v (attempt %d)",
				conn.host, backoffDuration-timeSinceLastFail, conn.failCount)
		}
	}
	conn.mu.Unlock()

	c, err := rcon.Dial(conn.host, conn.password,
		rcon.SetDialTimeout(5*time.Second),
		rcon.SetDeadline(10*time.Second),
	)
	if err != nil {
		conn.mu.Lock()
		conn.failCount++
		conn.lastFailTime = time.Now()
		conn.mu.Unlock()
		return fmt.Errorf("rcon dial %s: %w", conn.host, err)
	}

	conn.mu.Lock()
	if conn.client != nil {
		_ = conn.client.Close()
	}
	conn.client = c
	// Reset failure tracking on successful connection
	conn.failCount = 0
	conn.lastFailTime = time.Time{}
	conn.mu.Unlock()

	m.logger.Info("rcon connected", "server", conn.id, "host", conn.host)
	return nil
}

// calculateBackoff returns exponential backoff duration based on failure count
// Max backoff is capped at 30 seconds
func (m *Manager) calculateBackoff(failCount int) time.Duration {
	const (
		baseDelay = 1 * time.Second
		maxDelay  = 30 * time.Second
	)

	delay := baseDelay * (1 << uint(failCount-1)) // exponential: 1s, 2s, 4s, 8s, 16s, 32s...
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}

func (m *Manager) Execute(serverID, command string) (string, error) {
	m.mu.RLock()
	conn, ok := m.connections[serverID]
	m.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("rcon: unknown server %q", serverID)
	}

	// First attempt: try existing connection
	conn.mu.Lock()
	client := conn.client
	conn.mu.Unlock()

	if client != nil {
		out, err := client.Execute(command)
		if err == nil {
			return out, nil
		}
		m.logger.Warn("rcon execute failed, retrying",
			"server", serverID,
			"error", err,
		)
		// Close the failed connection
		conn.mu.Lock()
		if conn.client != nil {
			_ = conn.client.Close()
			conn.client = nil
		}
		conn.mu.Unlock()
	}

	// Second attempt: reconnect and retry
	if err := m.dial(conn); err != nil {
		return "", fmt.Errorf("%w: %v", ErrNotConnected, err)
	}

	// Hold lock during execute to prevent race condition where another
	// goroutine closes the client between retrieval and execution
	conn.mu.Lock()
	defer conn.mu.Unlock()

	if conn.client == nil {
		return "", fmt.Errorf("%w: client is nil after reconnection", ErrNotConnected)
	}

	out, err := conn.client.Execute(command)
	if err != nil {
		return "", fmt.Errorf("rcon execute after reconnect: %w", err)
	}
	return out, nil
}

// ExecuteMulti runs a command that may produce a large, multi-packet response
// (such as CS2's "maps *") and returns the full, reassembled output.
//
// It uses a dedicated short-lived TCP connection rather than the pooled gorcon
// client, because gorcon only reads the first response packet — which truncates
// large outputs and desyncs the pooled connection for subsequent commands.
func (m *Manager) ExecuteMulti(serverID, command string) (string, error) {
	m.mu.RLock()
	conn, ok := m.connections[serverID]
	m.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("rcon: unknown server %q", serverID)
	}

	out, err := executeMultiPacket(conn.host, conn.password, command, 15*time.Second)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNotConnected, err)
	}
	return out, nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, conn := range m.connections {
		conn.mu.Lock()
		if conn.client != nil {
			if err := conn.client.Close(); err != nil {
				m.logger.Warn("rcon close", "server", id, "error", err)
			}
			conn.client = nil
		}
		conn.mu.Unlock()
	}
	return nil
}

func (m *Manager) Disconnect(serverID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	conn, ok := m.connections[serverID]
	if !ok {
		return
	}
	conn.mu.Lock()
	if conn.client != nil {
		_ = conn.client.Close()
		conn.client = nil
	}
	conn.mu.Unlock()
	delete(m.connections, serverID)
}
