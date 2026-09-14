package redisclient

import (
	"context"
	"fmt"
	"sync"
)

// Manager owns open connections keyed by config ID. App-layer bindings talk
// to connections through it; each Open replaces any previous connection with
// the same ID.
type Manager struct {
	settingsDir string

	mu    sync.RWMutex
	conns map[string]*Connection
}

func NewManager(settingsDir string) *Manager {
	return &Manager{settingsDir: settingsDir, conns: map[string]*Connection{}}
}

// Open establishes (or replaces) the connection for cfg.ID.
func (m *Manager) Open(ctx context.Context, cfg Config) (*Connection, error) {
	if cfg.ID == "" {
		return nil, fmt.Errorf("connection config has no id")
	}
	conn, err := Open(ctx, cfg, m.settingsDir)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if old, ok := m.conns[cfg.ID]; ok {
		old.Close()
	}
	m.conns[cfg.ID] = conn
	return conn, nil
}

func (m *Manager) Get(id string) (*Connection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.conns[id]
	if !ok {
		return nil, fmt.Errorf("connection %s is not open", id)
	}
	return c, nil
}

func (m *Manager) Close(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.conns[id]; ok {
		c.Close()
		delete(m.conns, id)
	}
}

func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, c := range m.conns {
		c.Close()
		delete(m.conns, id)
	}
}
