package connections

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jjflamp/rdeck/internal/redisclient"
	"github.com/jjflamp/rdeck/internal/secrets"
)

// Store persists connection configs as JSON in the settings directory.
// Layout: <settingsDir>/connections.json. Secret fields (auth,
// ssh_password) go to the OS keychain when available; JSON keeps them
// empty in that case (plan §3.2). When the keychain write fails the
// plaintext stays in JSON as graceful degradation.
type Store struct {
	path    string
	secrets secrets.ProviderFactory
}

type fileLayout struct {
	Version     int                   `json:"version"`
	Connections []*redisclient.Config `json:"connections"`
}

func NewStore(settingsDir string) (*Store, error) {
	return NewStoreWithSecrets(settingsDir, secrets.For)
}

// NewStoreWithSecrets allows injecting a secrets provider (tests).
func NewStoreWithSecrets(settingsDir string, factory secrets.ProviderFactory) (*Store, error) {
	if err := os.MkdirAll(settingsDir, 0o700); err != nil {
		return nil, fmt.Errorf("create settings dir: %w", err)
	}
	return &Store{path: filepath.Join(settingsDir, "connections.json"), secrets: factory}, nil
}

func (s *Store) Path() string { return s.path }

// List returns all saved connections (never nil), with secret fields
// hydrated from the keyring when they were stored there.
func (s *Store) List() ([]*redisclient.Config, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return []*redisclient.Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	var fl fileLayout
	if err := json.Unmarshal(data, &fl); err != nil {
		return nil, fmt.Errorf("parse %s: %w", s.path, err)
	}
	if fl.Connections == nil {
		fl.Connections = []*redisclient.Config{}
	}
	for _, c := range fl.Connections {
		s.hydrate(c)
	}
	return fl.Connections, nil
}

// hydrate pulls secret fields back from the keyring (best effort).
func (s *Store) hydrate(c *redisclient.Config) {
	p := s.secrets(c.ID)
	if c.Auth == "" {
		if v, err := p.Get("auth"); err == nil {
			c.Auth = v
		}
	}
	if c.SSHPassword == "" {
		if v, err := p.Get("ssh_password"); err == nil {
			c.SSHPassword = v
		}
	}
}

// Save upserts a connection (matched by ID).
func (s *Store) Save(cfg *redisclient.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	cfg.Defaults()

	// Move secrets to the keyring; on success strip them from the JSON copy.
	// On keyring failure the plaintext stays in JSON (graceful degradation).
	persisted := *cfg
	if p := s.secrets(cfg.ID); p != nil {
		if cfg.Auth != "" && p.Set("auth", cfg.Auth) == nil {
			persisted.Auth = ""
		}
		if cfg.SSHPassword != "" && p.Set("ssh_password", cfg.SSHPassword) == nil {
			persisted.SSHPassword = ""
		}
	}

	all, err := s.List()
	if err != nil {
		return err
	}
	replaced := false
	for i, existing := range all {
		if existing.ID == cfg.ID {
			all[i] = &persisted
			replaced = true
			break
		}
	}
	if !replaced {
		all = append(all, &persisted)
	}
	return s.write(all)
}

// Delete removes a connection by ID and wipes its keyring secrets.
func (s *Store) Delete(id string) error {
	if p := s.secrets(id); p != nil {
		p.WipeAll()
	}
	all, err := s.List()
	if err != nil {
		return err
	}
	kept := all[:0]
	for _, c := range all {
		if c.ID != id {
			kept = append(kept, c)
		}
	}
	return s.write(kept)
}

func (s *Store) write(conns []*redisclient.Config) error {
	data, err := json.MarshalIndent(fileLayout{Version: 1, Connections: conns}, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path) // atomic on POSIX
}
