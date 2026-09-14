package keymodel

import (
	"context"
	"fmt"
	"strconv"
)

// --- scan-style loaders (hash / set / zset) ---------------------------------
// Single-step cursor pagination via raw commands (plan §4.1-①: never go-redis
// SCAN helpers — they loop internally until exhaustion).

func scanPage(ctx context.Context, cl Client, cmdName, key, cursor, match string, limit int64, count int64) ([]interface{}, string, error) {
	if cursor == "" {
		// SCAN family requires a numeric cursor; the UI sends "" for page 1.
		cursor = "0"
	}
	args := []interface{}{cmdName, key, cursor}
	if match != "" && match != "*" {
		args = append(args, "MATCH", match)
	}
	args = append(args, "COUNT", count)
	raw, err := cl.Do(ctx, args...).Slice()
	if err != nil {
		return nil, "", fmt.Errorf("%s failed: %w", cmdName, err)
	}
	if len(raw) != 2 {
		return nil, "", fmt.Errorf("unexpected %s reply shape", cmdName)
	}
	next, err := parseCursorString(raw[0])
	if err != nil {
		return nil, "", err
	}
	if next == "0" {
		// Scan exhausted — normalize to "" so the frontend never resumes
		// from cursor 0 (which would rescan the whole collection and
		// duplicate previously shown rows).
		next = ""
	}
	items, _ := raw[1].([]interface{})
	// Keep consuming pages until the UI limit is reached or scan ends —
	// COUNT is a hint, servers may return few items per page. Guard against
	// a server repeating the same cursor (would loop forever).
	if int64(len(items)) < limit && next != "" && next != cursor {
		more, next2, err := scanPage(ctx, cl, cmdName, key, next, match, limit-int64(len(items)), count)
		if err != nil {
			return nil, "", err
		}
		return append(items, more...), next2, nil
	}
	return items, next, nil
}

func parseCursorString(v interface{}) (string, error) {
	switch t := v.(type) {
	case string:
		return t, nil
	case int64:
		return strconv.FormatInt(t, 10), nil
	default:
		return "", fmt.Errorf("unexpected scan cursor type %T", v)
	}
}

func toStr(v interface{}) string {
	s, _ := v.(string)
	return s
}

// HashLoader ------------------------------------------------------------------

type HashLoader struct{ Key string }

func (h *HashLoader) Meta(ctx context.Context, cl Client) (*KeyMeta, error) {
	n, err := cl.HLen(ctx, h.Key).Result()
	if err != nil {
		return nil, err
	}
	return &KeyMeta{Key: h.Key, Type: "hash", TTL: ttlOf(ctx, cl, h.Key), Rows: n}, nil
}

func (h *HashLoader) LoadPage(ctx context.Context, cl Client, cursor, match string, limit int64) (*RowsPage, error) {
	items, next, err := scanPage(ctx, cl, "HSCAN", h.Key, cursor, match, limit, limit)
	if err != nil {
		return nil, err
	}
	page := &RowsPage{NextCursor: next, Total: -1}
	for i := 0; i+1 < len(items); i += 2 {
		page.Rows = append(page.Rows, Row{
			Index: len(page.Rows),
			Key:   toStr(items[i]),
			Value: bytesToB64([]byte(toStr(items[i+1]))),
		})
	}
	return page, nil
}

// SetLoader -------------------------------------------------------------------

type SetLoader struct{ Key string }

func (s *SetLoader) Meta(ctx context.Context, cl Client) (*KeyMeta, error) {
	n, err := cl.SCard(ctx, s.Key).Result()
	if err != nil {
		return nil, err
	}
	return &KeyMeta{Key: s.Key, Type: "set", TTL: ttlOf(ctx, cl, s.Key), Rows: n}, nil
}

func (s *SetLoader) LoadPage(ctx context.Context, cl Client, cursor, match string, limit int64) (*RowsPage, error) {
	items, next, err := scanPage(ctx, cl, "SSCAN", s.Key, cursor, match, limit, limit)
	if err != nil {
		return nil, err
	}
	page := &RowsPage{NextCursor: next, Total: -1}
	for _, item := range items {
		page.Rows = append(page.Rows, Row{
			Index: len(page.Rows),
			Key:   toStr(item),
			Value: bytesToB64([]byte(toStr(item))),
		})
	}
	return page, nil
}

// ZSetLoader ------------------------------------------------------------------

type ZSetLoader struct{ Key string }

func (z *ZSetLoader) Meta(ctx context.Context, cl Client) (*KeyMeta, error) {
	n, err := cl.ZCard(ctx, z.Key).Result()
	if err != nil {
		return nil, err
	}
	return &KeyMeta{Key: z.Key, Type: "zset", TTL: ttlOf(ctx, cl, z.Key), Rows: n}, nil
}

func (z *ZSetLoader) LoadPage(ctx context.Context, cl Client, cursor, match string, limit int64) (*RowsPage, error) {
	items, next, err := scanPage(ctx, cl, "ZSCAN", z.Key, cursor, match, limit, limit)
	if err != nil {
		return nil, err
	}
	page := &RowsPage{NextCursor: next, Total: -1}
	for i := 0; i+1 < len(items); i += 2 {
		member := toStr(items[i])
		score, _ := strconv.ParseFloat(toStr(items[i+1]), 64)
		page.Rows = append(page.Rows, Row{
			Index: len(page.Rows),
			Key:   member,
			Value: bytesToB64([]byte(member)),
			Score: score,
		})
	}
	return page, nil
}
