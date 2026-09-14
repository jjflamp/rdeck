// Package keymodel implements per-type key loading and editing — the Go
// counterpart of RESP.app's src/app/models/key-models. Pagination is
// stateless: the caller passes back the returned cursor (scan cursor for
// hash/set/zset, offset for list, last stream id for stream), mirroring the
// original semantics without server-side row cache.
package keymodel

import (
	"context"
	"encoding/base64"

	"github.com/redis/go-redis/v9"
)

// Row is one displayable row. Value is base64 (binary-safe transport rule,
// plan §3.1-①) — callers decode/encode at the edges.
type Row struct {
	Index int     `json:"index"`           // position within the collection
	Key   string  `json:"key,omitempty"`   // hash field / set member / zset member / stream id
	Value string  `json:"value"`           // base64 of raw value bytes
	Score float64 `json:"score,omitempty"` // zset only
}

// RowsPage is one page of rows plus resumption info.
type RowsPage struct {
	Rows       []Row  `json:"rows"`
	NextCursor string `json:"nextCursor"` // "" means exhausted
	Total      int64  `json:"total"`
}

// KeyMeta is the header info for an opened key.
type KeyMeta struct {
	Key       string `json:"key"`
	Type      string `json:"type"`
	TTL       int64  `json:"ttl"`       // seconds, -1 = no expiry, -2 = missing
	Rows      int64  `json:"rows"`      // total row count (1 for strings)
	Size      int64  `json:"size"`      // payload size in bytes (best effort)
	Truncated bool   `json:"truncated"` // value exceeded size limit
}

// Client is the redis client surface keymodel needs: typed commands
// (redis.Cmdable) plus raw Do for single-step SCAN pagination.
// *redis.Client and *redis.ClusterClient both satisfy it.
type Client interface {
	redis.Cmdable
	Do(ctx context.Context, args ...interface{}) *redis.Cmd
}

// Loader loads and edits one key. Implementations are per redis type.
type Loader interface {
	// Meta returns type/TTL/total rows for the header.
	Meta(ctx context.Context, cl Client) (*KeyMeta, error)
	// LoadPage fetches rows starting at cursor (type-specific semantics:
	// scan cursor for hash/set/zset, list offset, last stream id).
	LoadPage(ctx context.Context, cl Client, cursor string, match string, limit int64) (*RowsPage, error)
}

// bytesToB64 / b64ToBytes centralize the base64 transport rule.
func bytesToB64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func b64ToBytes(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

// ttlOf returns TTL in seconds (-1 = persistent, -2 = key missing).
// go-redis encodes redis TTL -1/-2 as negative-nanosecond durations.
func ttlOf(ctx context.Context, cl Client, key string) int64 {
	ttl, err := cl.TTL(ctx, key).Result()
	if err != nil {
		return -1
	}
	if ttl > 0 {
		return int64(ttl.Seconds())
	}
	if ttl == -2 {
		return -2
	}
	return -1
}
