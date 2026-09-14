package keymodel

import (
	"context"
	"encoding/json"
	"path"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// StringLoader — single-value key. cursor/match ignored.
type StringLoader struct {
	Key       string
	SizeLimit int64 // valueSizeLimit; <=0 means unlimited
}

func (s *StringLoader) Meta(ctx context.Context, cl Client) (*KeyMeta, error) {
	n, err := cl.StrLen(ctx, s.Key).Result()
	if err != nil {
		return nil, err
	}
	truncated := s.SizeLimit > 0 && n > s.SizeLimit
	return &KeyMeta{Key: s.Key, Type: "string", TTL: ttlOf(ctx, cl, s.Key), Rows: 1, Size: n, Truncated: truncated}, nil
}

// LoadPage returns the value, truncated at SizeLimit when configured.
func (s *StringLoader) LoadPage(ctx context.Context, cl Client, cursor, match string, limit int64) (*RowsPage, error) {
	var (
		val []byte
		err error
	)
	if s.SizeLimit > 0 {
		val, err = cl.GetRange(ctx, s.Key, 0, s.SizeLimit-1).Bytes()
	} else {
		val, err = cl.Get(ctx, s.Key).Bytes()
	}
	if err != nil && err != redis.Nil {
		return nil, err
	}
	return &RowsPage{Rows: []Row{{Index: 0, Value: bytesToB64(val)}}}, nil
}

// ListLoader — LRANGE range pagination (cursor = start offset).
type ListLoader struct{ Key string }

func (l *ListLoader) Meta(ctx context.Context, cl Client) (*KeyMeta, error) {
	n, err := cl.LLen(ctx, l.Key).Result()
	if err != nil {
		return nil, err
	}
	return &KeyMeta{Key: l.Key, Type: "list", TTL: ttlOf(ctx, cl, l.Key), Rows: n}, nil
}

func (l *ListLoader) LoadPage(ctx context.Context, cl Client, cursor, match string, limit int64) (*RowsPage, error) {
	start, err := strconv.ParseInt(cursor, 10, 64)
	if err != nil || start < 0 {
		start = 0
	}
	end := start + limit - 1
	vals, err := cl.LRange(ctx, l.Key, start, end).Result()
	if err != nil {
		return nil, err
	}
	total, _ := cl.LLen(ctx, l.Key).Result()
	page := &RowsPage{Total: total}
	for i, v := range vals {
		if match != "" && match != "*" && !globMatch(match, v) {
			continue
		}
		page.Rows = append(page.Rows, Row{
			Index: int(start) + i,
			Value: bytesToB64([]byte(v)),
		})
	}
	nextStart := start + int64(len(vals))
	if nextStart >= total {
		page.NextCursor = ""
	} else {
		page.NextCursor = strconv.FormatInt(nextStart, 10)
	}
	return page, nil
}

// StreamLoader — XRANGE id pagination (cursor = last seen id, "" = from start).
// Entry field/value pairs are serialized to a JSON object then base64'd so
// the UI can render them like other rows.
type StreamLoader struct{ Key string }

func (s *StreamLoader) Meta(ctx context.Context, cl Client) (*KeyMeta, error) {
	n, err := cl.XLen(ctx, s.Key).Result()
	if err != nil {
		return nil, err
	}
	return &KeyMeta{Key: s.Key, Type: "stream", TTL: ttlOf(ctx, cl, s.Key), Rows: n}, nil
}

func (s *StreamLoader) LoadPage(ctx context.Context, cl Client, cursor, match string, limit int64) (*RowsPage, error) {
	start := "-"
	if cursor != "" {
		start = "(" + cursor // exclusive: resume after last seen id
	}
	msgs, err := cl.XRangeN(ctx, s.Key, start, "+", limit).Result()
	if err != nil {
		return nil, err
	}
	total, _ := cl.XLen(ctx, s.Key).Result()
	page := &RowsPage{Total: total}
	var lastID string
	for _, m := range msgs {
		lastID = m.ID
		pairs := make(map[string]string, len(m.Values))
		for f, v := range m.Values {
			pairs[f] = toStr(v)
		}
		var enc string
		if b, err := json.Marshal(pairs); err == nil {
			enc = bytesToB64(b)
		}
		page.Rows = append(page.Rows, Row{Index: len(page.Rows), Key: m.ID, Value: enc})
	}
	if int64(len(msgs)) < limit {
		page.NextCursor = ""
	} else {
		page.NextCursor = lastID
	}
	return page, nil
}

func globMatch(pattern, s string) bool {
	ok, err := path.Match(pattern, s)
	return err == nil && ok
}
