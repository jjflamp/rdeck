package keymodel

import (
	"context"
	"fmt"
)

// NewLoader returns the loader for a redis TYPE. Module types (ReJSON etc.)
// fall back to string-style access, matching keyfactory.cpp behavior; truly
// unknown module types use the catch-all unknown loader.
func NewLoader(keyType, key string, sizeLimit int64) (Loader, error) {
	switch keyType {
	case "string", "ReJSON-RL", "ReJSON", "AC": // AC = RedisJSON v2 arrow? keep string-ish
		return &StringLoader{Key: key, SizeLimit: sizeLimit}, nil
	case "hash":
		return &HashLoader{Key: key}, nil
	case "list":
		return &ListLoader{Key: key}, nil
	case "set":
		return &SetLoader{Key: key}, nil
	case "zset":
		return &ZSetLoader{Key: key}, nil
	case "stream":
		return &StreamLoader{Key: key}, nil
	case "BF", "CF", "CMS", "TOPK", "TDIGEST": // probabilistic modules: meta only
		return &UnknownLoader{Key: key, Type: keyType}, nil
	case "none":
		return nil, fmt.Errorf("key %q does not exist", key)
	default:
		return &UnknownLoader{Key: key, Type: keyType}, nil
	}
}

// UnknownLoader exposes metadata only (original unknownkey behavior).
type UnknownLoader struct {
	Key  string
	Type string
}

func (u *UnknownLoader) Meta(ctx context.Context, cl Client) (*KeyMeta, error) {
	return &KeyMeta{Key: u.Key, Type: u.Type, TTL: ttlOf(ctx, cl, u.Key), Rows: 0}, nil
}

func (u *UnknownLoader) LoadPage(ctx context.Context, cl Client, cursor, match string, limit int64) (*RowsPage, error) {
	return &RowsPage{}, nil
}
