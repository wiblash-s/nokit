// Package rcon manages connections to CS2 servers over the Source RCON
// protocol. The Manager owns a pool of clients keyed by server ID and
// handles reconnection on failure.
package rcon

import (
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

type Manager struct {
	logger *slog.Logger

	mu      sync.RWMutex
	clients map[string]Client
}

func New(logger *slog.Logger) *Manager {
	return &Manager{
		logger:  logger,
		clients: make(map[string]Client),
	}
}

func (m *Manager) Connect(serverID, host, password string) error {
	conn, err := rcon.Dial(host, password,
		rcon.SetDialTimeout(5*time.Second),
		rcon.SetDeadline(10*time.Second),
	)
	if err != nil {
		return fmt.Errorf("rcon dial %s: %w", host, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.clients[serverID]; ok {
		_ = existing.Close()
	}
	m.clients[serverID] = conn
	m.logger.Info("rcon connected", "server", serverID, "host", host)
	return nil
}

func (m *Manager) Get(serverID string) (Client, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.clients[serverID]
	return c, ok
}

func (m *Manager) Execute(serverID, command string) (string, error) {
	c, ok := m.Get(serverID)
	if !ok {
		return "", fmt.Errorf("no rcon client for server %q", serverID)
	}
	return c.Execute(command)
}

func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, c := range m.clients {
		if err := c.Close(); err != nil {
			m.logger.Warn("rcon close", "server", id, "error", err)
		}
		delete(m.clients, id)
	}
	return nil
}
