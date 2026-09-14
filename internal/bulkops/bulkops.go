// Package bulkops implements bulk key operations — the Go counterpart of
// RESP.app's bulk-operations module (DELETE_KEYS / COPY_KEYS / TTL /
// IMPORT_RDB_KEYS). All operations are pipeline-batched and report progress
// through a callback (the binding layer throttles it into events).
package bulkops

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Progress reports (done, total, last message). total may be 0 while scanning.
type Progress func(done, total int, msg string)

const batchSize = 200

// DeleteKeys removes keys in pipeline batches. Returns the number deleted.
func DeleteKeys(ctx context.Context, cl redis.Cmdable, keys []string, p Progress) (int, error) {
	deleted := 0
	for start := 0; start < len(keys); start += batchSize {
		end := min(start+batchSize, len(keys))
		pipe := cl.Pipeline()
		for _, k := range keys[start:end] {
			pipe.Del(ctx, k)
		}
		cmds, err := pipe.Exec(ctx)
		if err != nil && err != redis.Nil {
			return deleted, fmt.Errorf("delete batch: %w", err)
		}
		for _, cmd := range cmds {
			if n, _ := cmd.(*redis.IntCmd).Result(); n > 0 {
				deleted++
			}
		}
		if p != nil {
			p(end, len(keys), "deleting")
		}
	}
	return deleted, nil
}

// BulkTTL sets expiry (ttl > 0) or persists (ttl <= 0) for many keys.
func BulkTTL(ctx context.Context, cl redis.Cmdable, keys []string, ttl int64, p Progress) (int, error) {
	changed := 0
	for start := 0; start < len(keys); start += batchSize {
		end := min(start+batchSize, len(keys))
		pipe := cl.Pipeline()
		for _, k := range keys[start:end] {
			if ttl > 0 {
				pipe.Expire(ctx, k, time.Duration(ttl)*time.Second)
			} else {
				pipe.Persist(ctx, k)
			}
		}
		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			return changed, fmt.Errorf("ttl batch: %w", err)
		}
		changed = end
		if p != nil {
			p(end, len(keys), "setting ttl")
		}
	}
	return changed, nil
}

// CopyKeys copies keys from source to destination via DUMP + RESTORE
// (preserves binary payloads and TTL, REPLACE overwrites targets) — the
// original copyoperation.cpp semantics. Cross-connection and cross-db are
// handled by the caller passing different clients.
func CopyKeys(ctx context.Context, src, dst redis.Cmdable, keys []string, p Progress) (copied int, failed []string, err error) {
	for start := 0; start < len(keys); start += batchSize {
		end := min(start+batchSize, len(keys))
		batch := keys[start:end]

		// Phase 1: DUMP + PTTL from source.
		dumpPipe := src.Pipeline()
		ttlPipe := src.Pipeline()
		dumps := make([]*redis.StringCmd, len(batch))
		ttls := make([]*redis.DurationCmd, len(batch))
		for i, k := range batch {
			dumps[i] = dumpPipe.Dump(ctx, k)
			ttls[i] = dumpPipe.PTTL(ctx, k)
			_ = ttlPipe
		}
		if _, err := dumpPipe.Exec(ctx); err != nil && err != redis.Nil {
			return copied, failed, fmt.Errorf("dump batch: %w", err)
		}

		// Phase 2: RESTORE into destination; count per-key results.
		restorePipe := dst.Pipeline()
		restores := make([]*redis.StatusCmd, len(batch))
		for i, k := range batch {
			payload, derr := dumps[i].Bytes()
			if derr != nil || len(payload) == 0 {
				failed = append(failed, k) // key vanished or dump failed
				continue
			}
			var ttl time.Duration
			if t := ttls[i].Val(); t > 0 {
				ttl = t
			}
			restores[i] = restorePipe.RestoreReplace(ctx, k, ttl, string(payload))
		}
		_, rerr := restorePipe.Exec(ctx)
		for i, k := range batch {
			if restores[i] == nil {
				continue // already counted as failed at dump phase
			}
			if rerr == nil || restores[i].Err() == nil {
				copied++
			} else {
				failed = append(failed, k)
			}
		}
		if rerr != nil {
			return copied, failed, fmt.Errorf("restore batch: %w", rerr)
		}
		if p != nil {
			p(end, len(keys), "copying")
		}
	}
	return copied, failed, nil
}
