// Per-database filter history (plan §4.2): persists the last 10 filter
// patterns per (connection, db) inside the connection config — matching the
// original filterHistoryTop10. Store secret handling applies to re-saves.
package app

import (
	"strconv"
)

// SaveFilterHistory records a used filter pattern (non-"*") for the db.
func (a *App) SaveFilterHistory(connID string, db uint, pattern string) error {
	if pattern == "" || pattern == "*" {
		return nil
	}
	all, err := a.store.List()
	if err != nil {
		return err
	}
	for _, cfg := range all {
		if cfg.ID != connID {
			continue
		}
		key := dbKey(db)
		hist := cfg.FilterHistory[key]

		// dedupe, newest first, cap 10
		next := []string{pattern}
		for _, p := range hist {
			if p != pattern {
				next = append(next, p)
			}
		}
		if len(next) > 10 {
			next = next[:10]
		}
		if cfg.FilterHistory == nil {
			cfg.FilterHistory = map[string][]string{}
		}
		cfg.FilterHistory[key] = next
		return a.store.Save(cfg)
	}
	return nil
}

// GetFilterHistory returns recent patterns for the db (newest first).
func (a *App) GetFilterHistory(connID string, db uint) ([]string, error) {
	all, err := a.store.List()
	if err != nil {
		return nil, err
	}
	for _, cfg := range all {
		if cfg.ID == connID {
			return cfg.FilterHistory[dbKey(db)], nil
		}
	}
	return []string{}, nil
}

func dbKey(db uint) string { return strconv.FormatUint(uint64(db), 10) }
