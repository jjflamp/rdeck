package redisclient

import (
	"context"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// KeyInfo is one scanned key with its TYPE.
type KeyInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// ScanResult is the outcome of one key-space scan pass.
type ScanResult struct {
	Keys     []KeyInfo `json:"keys"`     // full key paths
	Complete bool      `json:"complete"` // false when stopped at limit ("load more")
	Scanned  int       `json:"scanned"`
}

// scanBatch is the raw SCAN page size; small enough for responsive paging.
const scanBatch = 500

// ScanKeys walks the key space of a db with raw SCAN (plan §4.1-①: never use
// go-redis SCAN helpers — they loop internally until the cursor is exhausted).
// Stops after limit keys, reporting Complete=false so the UI can offer
// "load more".
func (c *Connection) ScanKeys(ctx context.Context, db uint, pattern string, limit int) (*ScanResult, error) {
	if pattern == "" {
		pattern = c.cfg.KeysPattern
	}
	if limit <= 0 {
		limit = int(c.cfg.DBScanLimit) * 500 // original default scanLimit semantics
	}

	cl, err := c.Client(db)
	if err != nil {
		return nil, err
	}

	res := &ScanResult{}
	var node doer
	switch typed := cl.(type) {
	case *redis.ClusterClient:
		node = typed
		err = c.scanCluster(ctx, typed, pattern, limit, res)
	case *redis.Client:
		node = typed
		err = scanNode(ctx, typed, pattern, limit, res)
	default:
		err = fmt.Errorf("unsupported client type %T", cl)
	}
	if err != nil {
		return nil, err
	}

	res.Keys = detectTypes(ctx, node, res.Keys)
	return res, nil
}

func (c *Connection) scanCluster(ctx context.Context, cl *redis.ClusterClient, pattern string, limit int, res *ScanResult) error {
	// Scan every master like the original getClusterKeys; go-redis routes
	// TYPE lookups automatically for the pipeline later.
	return cl.ForEachMaster(ctx, func(ctx context.Context, node *redis.Client) error {
		if res.Scanned >= limit {
			return nil
		}
		return scanNode(ctx, node, pattern, limit, res)
	})
}

// doer matches *redis.Client and *redis.ClusterClient raw-command surface
// (redis.Cmdable itself does not expose Do).
type doer interface {
	Do(ctx context.Context, args ...interface{}) *redis.Cmd
	Pipeline() redis.Pipeliner
}

func scanNode(ctx context.Context, cl doer, pattern string, limit int, res *ScanResult) error {
	var cursor uint64
	for {
		raw, err := cl.Do(ctx, "SCAN", cursor, "MATCH", pattern, "COUNT", scanBatch).Slice()
		if err != nil {
			return fmt.Errorf("SCAN failed: %w", err)
		}
		if len(raw) != 2 {
			return fmt.Errorf("unexpected SCAN reply shape")
		}
		next, err := parseCursor(raw[0])
		if err != nil {
			return err
		}
		keysRaw, _ := raw[1].([]interface{})
		for _, k := range keysRaw {
			name, _ := k.(string)
			if name == "" {
				continue
			}
			res.Keys = append(res.Keys, KeyInfo{Name: name})
			res.Scanned++
			if res.Scanned >= limit {
				// Stopped before the cursor was exhausted: more keys exist
				// than collected. Resume requires a fresh scan with a higher
				// limit (cursor is gone), so never report Complete here.
				res.Complete = false
				return nil
			}
		}
		if next == 0 {
			res.Complete = true
			return nil
		}
		if next == cursor { // defensive: server repeating cursor would loop forever
			res.Complete = false
			return nil
		}
		cursor = next
	}
}

func parseCursor(v interface{}) (uint64, error) {
	switch t := v.(type) {
	case string:
		return strconv.ParseUint(t, 10, 64)
	case int64:
		return uint64(t), nil
	default:
		return 0, fmt.Errorf("unexpected SCAN cursor type %T", v)
	}
}

func detectTypes(ctx context.Context, cl doer, keys []KeyInfo) []KeyInfo {
	const batch = 200
	for start := 0; start < len(keys); start += batch {
		end := min(start+batch, len(keys))
		pipe := cl.Pipeline()
		cmds := make([]*redis.StatusCmd, end-start)
		for i := start; i < end; i++ {
			cmds[i-start] = pipe.Type(ctx, keys[i].Name)
		}
		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			// Leave types empty on failure; value loading will surface errors.
			continue
		}
		for i := start; i < end; i++ {
			keys[i].Type = cmds[i-start].Val()
		}
	}
	return keys
}
