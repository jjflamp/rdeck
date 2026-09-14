// Bulk operations bindings: preview scan + async execution with throttled
// progress events ("bulk:progress" / "bulk:done").
package app

import (
	"sync"
	"time"

	"rdeck/internal/bulkops"
	"rdeck/internal/events"
	"rdeck/internal/redisclient"
)

type BulkRequest struct {
	Op      string `json:"op"` // delete | copy | ttl | rdb_import
	ConnID  string `json:"conn_id"`
	DB      uint   `json:"db"`
	Pattern string `json:"pattern"`

	// copy
	TargetConnID string `json:"target_conn_id,omitempty"`
	TargetDB     uint   `json:"target_db,omitempty"`

	// ttl
	TTL int64 `json:"ttl,omitempty"`

	// rdb import
	RDBPath string   `json:"rdb_path,omitempty"`
	Include []string `json:"include,omitempty"` // key glob patterns
}

type BulkPreview struct {
	Keys     []redisclient.KeyInfo `json:"keys"`
	Total    int                   `json:"total"`
	Complete bool                  `json:"complete"`
}

// BulkPreview scans matching keys (up to the scan limit) for confirmation.
func (a *App) BulkPreview(connID string, db uint, pattern string) (*BulkPreview, error) {
	conn, err := a.conns.Get(connID)
	if err != nil {
		return nil, err
	}
	res, err := conn.ScanKeys(a.ctx, db, pattern, 10000)
	if err != nil {
		return nil, err
	}
	return &BulkPreview{Keys: res.Keys, Total: len(res.Keys), Complete: res.Complete}, nil
}

// BulkRun executes asynchronously; returns a run id. Progress arrives via
// "bulk:progress" [runID, done, total, msg]; completion via "bulk:done"
// [runID, summary].
func (a *App) BulkRun(req BulkRequest) (string, error) {
	runID := randomID()

	switch req.Op {
	case "delete", "ttl", "copy":
		src, err := a.clientFor(req.ConnID, req.DB)
		if err != nil {
			return "", err
		}
		preview, err := a.BulkPreview(req.ConnID, req.DB, req.Pattern)
		if err != nil {
			return "", err
		}
		keys := keyNames(preview.Keys)
		if len(keys) == 0 {
			a.bus.Emit(events.BulkDone, runID, map[string]any{"ok": true, "message": "没有匹配的 key"})
			return runID, nil
		}

		switch req.Op {
		case "delete":
			go func() {
				n, err := bulkops.DeleteKeys(a.ctx, src, keys, a.throttle(runID, len(keys)))
				a.emitDone(runID, err, map[string]any{"deleted": n})
			}()
		case "ttl":
			go func() {
				n, err := bulkops.BulkTTL(a.ctx, src, keys, req.TTL, a.throttle(runID, len(keys)))
				a.emitDone(runID, err, map[string]any{"changed": n})
			}()
		case "copy":
			dst, err := a.clientFor(req.TargetConnID, req.TargetDB)
			if err != nil {
				return "", err
			}
			go func() {
				n, failed, err := bulkops.CopyKeys(a.ctx, src, dst, keys, a.throttle(runID, len(keys)))
				a.emitDone(runID, err, map[string]any{"copied": n, "failed": failed})
			}()
		}

	case "rdb_import":
		dst, err := a.clientFor(req.ConnID, req.DB)
		if err != nil {
			return "", err
		}
		go func() {
			imported, skipped, err := bulkops.ImportRDB(a.ctx, dst, req.RDBPath,
				bulkops.RDBFilter{Include: req.Include},
				a.throttleBytes(runID))
			a.emitDone(runID, err, map[string]any{"imported": imported, "skipped": skipped})
		}()

	default:
		return "", errUnknownOp(req.Op)
	}
	return runID, nil
}

type unknownOpError struct{ op string }

func (e unknownOpError) Error() string { return "unknown bulk op: " + e.op }

func errUnknownOp(op string) error { return unknownOpError{op} }

// throttle wraps a Progress callback to emit at most every 200ms (the
// plan §3.1-④ rule), plus a final emission.
func (a *App) throttle(runID string, total int) bulkops.Progress {
	var mu sync.Mutex
	var last time.Time
	return func(done, _ int, msg string) {
		mu.Lock()
		if done < total && time.Since(last) < 200*time.Millisecond {
			mu.Unlock()
			return
		}
		last = time.Now()
		mu.Unlock()
		a.bus.Emit(events.BulkProgress, runID, done, total, msg)
	}
}

// throttleBytes adapts RDB import byte-based progress into a percent event.
func (a *App) throttleBytes(runID string) bulkops.Progress {
	var mu sync.Mutex
	var last time.Time
	return func(done, total int, msg string) {
		mu.Lock()
		if time.Since(last) < 200*time.Millisecond {
			mu.Unlock()
			return
		}
		last = time.Now()
		mu.Unlock()
		pct := 0
		if total > 0 {
			pct = done * 100 / total
		}
		a.bus.Emit(events.BulkProgress, runID, pct, 100, msg)
	}
}

func (a *App) emitDone(runID string, err error, summary map[string]any) {
	summary["ok"] = err == nil
	if err != nil {
		summary["error"] = err.Error()
	}
	a.bus.Emit(events.BulkDone, runID, summary)
}

func keyNames(keys []redisclient.KeyInfo) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = k.Name
	}
	return out
}
