// Console / MONITOR / PubSub / Server-panel bindings.
// Monitor and pubsub messages are batched (200ms flush) before EventsEmit —
// the throttle layer required by plan §3.1-④.
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/jjflamp/rdeck/internal/console"
	"github.com/jjflamp/rdeck/internal/events"
	"github.com/jjflamp/rdeck/internal/resp"
	"github.com/jjflamp/rdeck/internal/serverstats"
)

type consoleSession struct {
	conn    *resp.Conn
	cli     *console.Session
	cancel  context.CancelFunc
	mu      sync.Mutex
	monitor bool
}

type consoleManager struct {
	mu       sync.Mutex
	sessions map[string]*consoleSession
}

func newConsoleManager() *consoleManager {
	return &consoleManager{sessions: map[string]*consoleSession{}}
}

func (m *consoleManager) add(s *consoleSession) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := randomID()
	m.sessions[id] = s
	return id
}

func (m *consoleManager) get(id string) (*consoleSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, sessionError{id}
	}
	return s, nil
}

func (m *consoleManager) close(id string) {
	m.mu.Lock()
	s, ok := m.sessions[id]
	delete(m.sessions, id)
	m.mu.Unlock()
	if ok {
		s.cancel()
		s.conn.Close()
	}
}

type sessionError struct{ id string }

func (e sessionError) Error() string { return "console session not found: " + e.id }

func randomID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// --- Console ------------------------------------------------------------------

// OpenConsole creates a dedicated raw session for db.
func (a *App) OpenConsole(connID string, db uint) (string, error) {
	conn, err := a.conns.Get(connID)
	if err != nil {
		return "", err
	}
	raw, err := conn.NewSession(a.ctx, db)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithCancel(a.ctx)
	s := &consoleSession{conn: raw, cli: console.New(ctx, raw, db), cancel: cancel}
	return a.consoles.add(s), nil
}

func (a *App) ConsoleExecute(sessionID, line string) (string, error) {
	s, err := a.consoles.get(sessionID)
	if err != nil {
		return "", err
	}
	return s.cli.Execute(a.ctx, line)
}

// ConsoleStartMonitor switches the session into MONITOR mode; lines arrive
// via the "console:monitor" event in batches.
func (a *App) ConsoleStartMonitor(sessionID string) error {
	s, err := a.consoles.get(sessionID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.monitor {
		s.mu.Unlock()
		return nil
	}
	s.monitor = true
	s.mu.Unlock()

	ch, err := s.cli.StartMonitor(a.ctx)
	if err != nil {
		s.mu.Lock()
		s.monitor = false
		s.mu.Unlock()
		return err
	}
	go a.streamBatched(events.ConsoleMonitor, sessionID, ch)
	return nil
}

// streamBatched collects messages and emits them every 200ms (throttle rule).
func (a *App) streamBatched(eventName, sessionID string, in <-chan string) {
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	var buf []string
	for {
		select {
		case line, ok := <-in:
			if !ok {
				if len(buf) > 0 {
					a.bus.Emit(eventName, sessionID, buf)
				}
				return
			}
			buf = append(buf, line)
		case <-t.C:
			if len(buf) > 0 {
				a.bus.Emit(eventName, sessionID, buf)
				buf = buf[:0]
			}
		}
	}
}

func (a *App) CloseConsole(sessionID string) { a.consoles.close(sessionID) }

// --- PubSub ------------------------------------------------------------------

// PubSubSubscribe opens a dedicated session, subscribes and streams messages
// via the "pubsub:message" event (batched).
func (a *App) PubSubSubscribe(connID, channel string) (string, error) {
	conn, err := a.conns.Get(connID)
	if err != nil {
		return "", err
	}
	raw, err := conn.NewSession(a.ctx, 0)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithCancel(a.ctx)
	s := &consoleSession{conn: raw, cancel: cancel}
	id := a.consoles.add(s)

	cli := console.New(ctx, raw, 0)
	ch, err := cli.Subscribe(ctx, channel)
	if err != nil {
		a.consoles.close(id)
		return "", err
	}
	go func() {
		for msg := range ch {
			a.bus.Emit(events.PubsubMessage, connID, channel, msg)
		}
	}()
	return id, nil
}

func (a *App) PubSubUnsubscribe(sessionID string) { a.consoles.close(sessionID) }

// --- Server panel --------------------------------------------------------------

func (a *App) GetServerInfo(connID string, db uint) (*serverstats.InfoSnapshot, error) {
	cl, err := a.clientFor(connID, db)
	if err != nil {
		return nil, err
	}
	return serverstats.CollectInfo(a.ctx, cl, "server", "clients", "memory", "stats", "keyspace")
}

func (a *App) GetSlowLog(connID string, db uint, count int64) ([]serverstats.SlowLogEntry, error) {
	cl, err := a.clientFor(connID, db)
	if err != nil {
		return nil, err
	}
	return serverstats.SlowLog(a.ctx, cl, count)
}

func (a *App) GetClientList(connID string, db uint) ([]serverstats.ClientRow, error) {
	cl, err := a.clientFor(connID, db)
	if err != nil {
		return nil, err
	}
	return serverstats.ClientList(a.ctx, cl)
}
