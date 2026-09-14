// Package app is the Wails binding layer — a thin facade over internal/*.
// Methods here correspond to the Q_INVOKABLE surface of RESP.app's
// ConnectionsManager; the frontend never touches internal packages directly.
package app

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/jjflamp/rdeck/internal/connections"
	"github.com/jjflamp/rdeck/internal/events"
	"github.com/jjflamp/rdeck/internal/redisclient"
	"github.com/jjflamp/rdeck/internal/settings"
	"github.com/jjflamp/rdeck/internal/tree"
)

type App struct {
	ctx      context.Context
	bus      *events.Bus
	store    *connections.Store
	settings *settings.Manager
	conns    *redisclient.Manager
	consoles *consoleManager
}

func New(bus *events.Bus, store *connections.Store, settings *settings.Manager, conns *redisclient.Manager) *App {
	return &App{bus: bus, store: store, settings: settings, conns: conns, consoles: newConsoleManager()}
}

func (a *App) Startup(ctx context.Context) { a.ctx = ctx }

func (a *App) Shutdown(ctx context.Context) { a.conns.CloseAll() }

// --- Connections CRUD -------------------------------------------------------

func (a *App) ListConnections() ([]*redisclient.Config, error) {
	return a.store.List()
}

func (a *App) SaveConnection(cfg *redisclient.Config) error {
	return a.store.Save(cfg)
}

func (a *App) DeleteConnection(id string) error {
	a.conns.Close(id)
	return a.store.Delete(id)
}

func (a *App) ImportFromRDM(path string) (int, error) {
	return a.store.ImportFromRDMFile(path)
}

// --- Open / browse ----------------------------------------------------------

// OpenConnection dials (direct/SSH/TLS), probes the mode and returns summary
// info for the connection header + db selector.
func (a *App) OpenConnection(cfg *redisclient.Config) (*ConnSummary, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	conn, err := a.conns.Open(a.ctx, *cfg)
	if err != nil {
		return nil, err
	}
	return &ConnSummary{
		ID:        cfg.ID,
		Mode:      string(conn.Mode()),
		Version:   conn.Version(),
		Address:   cfg.Addr(),
		Databases: conn.Databases(),
	}, nil
}

type ConnSummary struct {
	ID        string               `json:"id"`
	Mode      string               `json:"mode"`
	Version   string               `json:"version"`
	Address   string               `json:"address"`
	Databases []redisclient.DBInfo `json:"databases"`
}

// EnsureConnection returns the live summary for cfg, opening the connection
// if the manager doesn't have it (self-healing for stale frontend caches —
// e.g. after an app restart or a dropped entry). Frontend db clicks always
// route through this so "connection is not open" can't happen on browse.
func (a *App) EnsureConnection(cfg *redisclient.Config) (*ConnSummary, error) {
	if cfg.ID == "" {
		return nil, fmt.Errorf("connection config has no id")
	}
	if conn, err := a.conns.Get(cfg.ID); err == nil {
		return &ConnSummary{
			ID:        cfg.ID,
			Mode:      string(conn.Mode()),
			Version:   conn.Version(),
			Address:   cfg.Addr(),
			Databases: conn.Databases(),
		}, nil
	}
	return a.OpenConnection(cfg)
}

func (a *App) Disconnect(id string) { a.conns.Close(id) }

// RefreshKeys scans one db and returns the namespace tree.
func (a *App) RefreshKeys(connID string, dbIndex uint, pattern string) (*TreeResult, error) {
	conn, err := a.conns.Get(connID)
	if err != nil {
		return nil, err
	}
	res, err := conn.ScanKeys(a.ctx, dbIndex, pattern, a.settings.Get().ScanLimit)
	if err != nil {
		return nil, err
	}
	nodes := tree.Build(toKeyInputs(res.Keys), conn.Config().NamespaceSeparator)
	return &TreeResult{
		Nodes:    nodes,
		Total:    len(res.Keys),
		Complete: res.Complete,
	}, nil
}

type TreeResult struct {
	Nodes    []*tree.Node `json:"nodes"`
	Total    int          `json:"total"`
	Complete bool         `json:"complete"` // false => "load more" available
}

func toKeyInputs(keys []redisclient.KeyInfo) []tree.KeyInput {
	inputs := make([]tree.KeyInput, len(keys))
	for i, k := range keys {
		inputs[i] = tree.KeyInput{Name: k.Name, Type: k.Type}
	}
	return inputs
}

// --- Test connection --------------------------------------------------------

// TestConnection performs a full Open (so SSH/TLS paths are exercised) and
// reports the server mode + version on success.
func (a *App) TestConnection(cfg *redisclient.Config) (TestResult, error) {
	if err := cfg.Validate(); err != nil {
		return TestResult{}, err
	}
	settingsDir := filepath.Dir(a.store.Path())
	conn, err := redisclient.Open(a.ctx, *cfg, settingsDir)
	if err != nil {
		a.bus.Emit(events.Error, map[string]string{"source": "test-connection", "message": err.Error()})
		return TestResult{OK: false, Error: err.Error()}, nil
	}
	defer conn.Close()
	return TestResult{
		OK:      true,
		Message: string(conn.Mode()) + " · redis " + conn.Version(),
	}, nil
}

type TestResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// --- Settings ---------------------------------------------------------------

func (a *App) GetSettings() settings.Settings { return a.settings.Get() }

func (a *App) SetSettings(s settings.Settings) error { return a.settings.Set(s) }
