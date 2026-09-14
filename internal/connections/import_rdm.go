package connections

import (
	"encoding/json"
	"fmt"
	"os"

	"rdeck/internal/redisclient"
)

// ImportFromRDMFile imports connections.json exported by RESP.app /
// RedisDesktopManager. The original file is either a bare JSON array of
// connection objects or an object wrapping them; both are accepted.
// Field names are already aligned (see redisclient.Config), so most entries
// unmarshal directly; unknown fields in the original are ignored.
func (s *Store) ImportFromRDMFile(path string) (imported int, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		var wrapped struct {
			Connections []json.RawMessage `json:"connections"`
		}
		if err2 := json.Unmarshal(data, &wrapped); err2 != nil || wrapped.Connections == nil {
			return 0, fmt.Errorf("unrecognized connections file format: %w", err)
		}
		raws = wrapped.Connections
	}

	existing, err := s.List()
	if err != nil {
		return 0, err
	}
	byID := make(map[string]bool, len(existing))
	for _, c := range existing {
		byID[c.ID] = true
	}

	for _, raw := range raws {
		cfg := &redisclient.Config{}
		if err := json.Unmarshal(raw, cfg); err != nil {
			// Skip malformed entries but keep importing the rest.
			continue
		}
		if cfg.Host == "" && cfg.SSHHost == "" {
			continue
		}
		// Original configs have no explicit tunnel flag; derive it.
		if cfg.SSHHost != "" {
			cfg.UseSSHTunnel = true
		}
		cfg.Defaults()
		if byID[cfg.ID] {
			cfg.ID = "" // force a fresh ID to avoid silent overwrite
			cfg.Defaults()
		}
		if err := s.Save(cfg); err != nil {
			continue
		}
		imported++
	}
	return imported, nil
}
