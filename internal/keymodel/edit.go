package keymodel

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// EditRequest describes one row-level mutation. Field usage depends on type:
//
//	hash:   Key=field, Value=value            Add→HSET  Update→HSET  Remove→HDEL field
//	list:   Value=value, Index=row            Add→RPUSH Update→LSET  Remove→LREM (first occurrence)
//	set:    Key=member (old), NewKey=new      Add→SADD  Update→SREM+SADD  Remove→SREM
//	zset:   Key=member, Score, NewKey=new     Add→ZADD  Update→ZADD / ZREM+SADD  Remove→ZREM
//	stream: Key=id ("" → *), Value=b64(JSON object of field:value pairs)
type EditRequest struct {
	Op     string  `json:"op"` // add | update | remove
	Type   string  `json:"type"`
	Key    string  `json:"key,omitempty"`
	NewKey string  `json:"newKey,omitempty"`
	Index  int     `json:"index,omitempty"`
	Value  string  `json:"value"` // base64
	Score  float64 `json:"score,omitempty"`
}

// EditRow applies one row mutation. Values arrive base64 and are decoded here.
func EditRow(ctx context.Context, cl Client, key string, req EditRequest) error {
	value, err := b64ToBytes(req.Value)
	if err != nil {
		return fmt.Errorf("bad value encoding: %w", err)
	}

	switch req.Type {
	case "hash":
		switch req.Op {
		case "add", "update":
			return cl.HSet(ctx, key, req.Key, value).Err()
		case "remove":
			return cl.HDel(ctx, key, req.Key).Err()
		}
	case "list":
		switch req.Op {
		case "add":
			return cl.RPush(ctx, key, value).Err()
		case "update":
			return cl.LSet(ctx, key, int64(req.Index), value).Err()
		case "remove":
			return cl.LRem(ctx, key, 1, value).Err()
		}
	case "set":
		switch req.Op {
		case "add":
			return cl.SAdd(ctx, key, value).Err()
		case "update":
			if req.NewKey == "" || req.NewKey == req.Key {
				return nil // set members are immutable; nothing to do
			}
			pipe := cl.TxPipeline()
			pipe.SRem(ctx, key, req.Key)
			pipe.SAdd(ctx, key, req.NewKey)
			_, err := pipe.Exec(ctx)
			return err
		case "remove":
			return cl.SRem(ctx, key, req.Key).Err()
		}
	case "zset":
		switch req.Op {
		case "add":
			return cl.ZAdd(ctx, key, redis.Z{Score: req.Score, Member: value}).Err()
		case "update":
			if req.NewKey != "" && req.NewKey != req.Key {
				pipe := cl.TxPipeline()
				pipe.ZRem(ctx, key, req.Key)
				pipe.ZAdd(ctx, key, redis.Z{Score: req.Score, Member: req.NewKey})
				_, err := pipe.Exec(ctx)
				return err
			}
			return cl.ZAddXX(ctx, key, redis.Z{Score: req.Score, Member: req.Key}).Err()
		case "remove":
			return cl.ZRem(ctx, key, req.Key).Err()
		}
	case "stream":
		switch req.Op {
		case "add":
			var pairs map[string]interface{}
			if err := json.Unmarshal(value, &pairs); err != nil {
				return fmt.Errorf("stream entry must be a JSON object of field:value: %w", err)
			}
			id := req.Key
			if id == "" {
				id = "*"
			}
			args := []interface{}{"XADD", key, id}
			for f, v := range pairs {
				args = append(args, f, v)
			}
			return cl.Do(ctx, args...).Err()
		case "remove":
			return cl.XDel(ctx, key, req.Key).Err()
		default:
			return fmt.Errorf("stream entries cannot be updated in place")
		}
	default:
		return fmt.Errorf("type %q does not support row editing", req.Type)
	}
	return fmt.Errorf("unknown op %q", req.Op)
}

// --- key-level operations ----------------------------------------------------

// SetTTL applies EXPIRE (ttl > 0) or PERSIST (ttl <= 0).
func SetTTL(ctx context.Context, cl Client, key string, ttl int64) error {
	if ttl > 0 {
		return cl.Expire(ctx, key, time.Duration(ttl)*time.Second).Err()
	}
	return cl.Persist(ctx, key).Err()
}

// RenameKey renames within the same db (RENAME overwrites the target).
func RenameKey(ctx context.Context, cl Client, from, to string) error {
	return cl.Rename(ctx, from, to).Err()
}

// DeleteKey removes the key.
func DeleteKey(ctx context.Context, cl Client, key string) error {
	return cl.Del(ctx, key).Err()
}

// SetString writes a full string value (b64) — the string editor save path.
func SetString(ctx context.Context, cl Client, key, valueB64 string) error {
	raw, err := b64ToBytes(valueB64)
	if err != nil {
		return err
	}
	return cl.Set(ctx, key, raw, 0).Err()
}
