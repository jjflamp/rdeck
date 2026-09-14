package bulkops

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/hdt3213/rdb/model"
	"github.com/hdt3213/rdb/parser"
	"github.com/redis/go-redis/v9"
)

// RDBFilter narrows which RDB objects are imported.
type RDBFilter struct {
	DBs     map[int]bool    // nil = all dbs
	Types   map[string]bool // nil = all base types (module types always skipped)
	Include []string        // key glob patterns; empty = all keys
}

func (f *RDBFilter) match(o model.RedisObject) bool {
	if f.DBs != nil && !f.DBs[o.GetDBIndex()] {
		return false
	}
	t := o.GetType()
	if f.Types != nil && !f.Types[t] {
		return false
	}
	if len(f.Include) > 0 {
		ok := false
		for _, pat := range f.Include {
			if m, err := path.Match(pat, o.GetKey()); err == nil && m {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// ImportRDB streams an RDB file into the destination client, replaying
// objects as write commands with their TTLs (replacing hdt3213-agnostic
// python rdbtools from the original). Module types (ReJSON etc.) are skipped
// and counted, mirroring plan §4.2.
func ImportRDB(ctx context.Context, dst redis.Cmdable, path string, filter RDBFilter, p Progress) (imported, skipped int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, fmt.Errorf("open rdb: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}
	total := int(stat.Size())

	dec := parser.NewDecoder(f)
	err = dec.Parse(func(o model.RedisObject) bool {
		if p != nil {
			p(imported+skipped, total, o.GetKey())
		}

		t := o.GetType()
		if t != model.StringType && t != model.ListType && t != model.SetType &&
			t != model.HashType && t != model.ZSetType && t != model.StreamType {
			skipped++ // module / aux / metadata objects
			return true
		}
		if !filter.match(o) {
			return true
		}
		if werr := writeObject(ctx, dst, o); werr != nil {
			skipped++
			return true // keep going; surface counts at the end
		}
		imported++

		// Apply TTL after data exists.
		if exp := o.GetExpiration(); exp != nil {
			_ = dst.ExpireAt(ctx, o.GetKey(), *exp).Err()
		}
		return true
	})
	return imported, skipped, err
}

func writeObject(ctx context.Context, dst redis.Cmdable, o model.RedisObject) error {
	key := o.GetKey()
	switch obj := o.(type) {
	case *model.StringObject:
		return dst.Set(ctx, key, obj.Value, 0).Err()

	case *model.ListObject:
		pipe := dst.Pipeline()
		for start := 0; start < len(obj.Values); start += batchSize {
			end := min(start+batchSize, len(obj.Values))
			pipe.RPush(ctx, key, toAnySlice(obj.Values[start:end]))
		}
		_, err := pipe.Exec(ctx)
		return err

	case *model.SetObject:
		pipe := dst.Pipeline()
		for start := 0; start < len(obj.Members); start += batchSize {
			end := min(start+batchSize, len(obj.Members))
			pipe.SAdd(ctx, key, toAnySlice(obj.Members[start:end]))
		}
		_, err := pipe.Exec(ctx)
		return err

	case *model.HashObject:
		pipe := dst.Pipeline()
		args := make([]any, 0, batchSize*2)
		for field, val := range obj.Hash {
			args = append(args, field, val)
			if len(args) >= batchSize*2 {
				pipe.HSet(ctx, key, args...)
				args = args[:0]
			}
		}
		if len(args) > 0 {
			pipe.HSet(ctx, key, args...)
		}
		_, err := pipe.Exec(ctx)
		return err

	case *model.ZSetObject:
		pipe := dst.Pipeline()
		for start := 0; start < len(obj.Entries); start += batchSize {
			end := min(start+batchSize, len(obj.Entries))
			members := make([]redis.Z, 0, end-start)
			for _, e := range obj.Entries[start:end] {
				members = append(members, redis.Z{Score: e.Score, Member: e.Member})
			}
			pipe.ZAdd(ctx, key, members...)
		}
		_, err := pipe.Exec(ctx)
		return err

	case *model.StreamObject:
		pipe := dst.Pipeline()
		for _, entry := range obj.Entries {
			for _, msg := range entry.Msgs {
				if msg.Deleted || msg.Id == nil {
					continue
				}
				idText, _ := msg.Id.MarshalText()
				args := []any{"XADD", key, string(idText)}
				for f, v := range msg.Fields {
					args = append(args, f, v)
				}
				pipe.Do(ctx, args...)
			}
		}
		_, err := pipe.Exec(ctx)
		return err

	default:
		return fmt.Errorf("unsupported rdb object type %T", o)
	}
}

func toAnySlice(bs [][]byte) []any {
	out := make([]any, len(bs))
	for i, b := range bs {
		out[i] = b
	}
	return out
}

