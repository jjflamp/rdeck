package keymodel

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func testClient(t *testing.T) Client {
	t.Helper()
	mr := miniredis.RunT(t)
	cl := redis.NewClient(&redis.Options{Addr: mr.Addr(), Protocol: 2})
	t.Cleanup(func() { cl.Close() })
	return cl
}

func TestHashLoadAndEdit(t *testing.T) {
	ctx := context.Background()
	cl := testClient(t)
	cl.HSet(ctx, "h", "f1", "v1", "f2", "v2")

	l, err := NewLoader("hash", "h", 0)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := l.Meta(ctx, cl)
	if err != nil || meta.Rows != 2 || meta.Type != "hash" {
		t.Fatalf("meta: %+v %v", meta, err)
	}

	page, err := l.LoadPage(ctx, cl, "0", "*", 10)
	if err != nil || len(page.Rows) != 2 {
		t.Fatalf("page: %v %v", page, err)
	}
	if page.Rows[0].Key != "f1" || page.Rows[0].Value != "djE=" { // b64("v1")
		t.Fatalf("row wrong: %+v", page.Rows[0])
	}

	// Regression: the UI sends "" as the first-page cursor; SCAN types must
	// normalize it to "0" instead of erroring with "invalid cursor".
	page2, err := l.LoadPage(ctx, cl, "", "*", 10)
	if err != nil || len(page2.Rows) != 2 {
		t.Fatalf("empty-cursor page: %v %v", page2, err)
	}

	// add / update / remove
	if err := EditRow(ctx, cl, "h", EditRequest{Op: "add", Type: "hash", Key: "f3", Value: "djM="}); err != nil {
		t.Fatal(err)
	}
	if err := EditRow(ctx, cl, "h", EditRequest{Op: "update", Type: "hash", Key: "f1", Value: "eHg="}); err != nil {
		t.Fatal(err)
	}
	if err := EditRow(ctx, cl, "h", EditRequest{Op: "remove", Type: "hash", Key: "f2"}); err != nil {
		t.Fatal(err)
	}
	if n, _ := cl.HLen(ctx, "h").Result(); n != 2 {
		t.Fatalf("want 2 fields, got %d", n)
	}
	if v, _ := cl.HGet(ctx, "h", "f1").Result(); v != "xx" {
		t.Fatalf("update failed: %q", v)
	}
}

func TestListRangeAndUpdate(t *testing.T) {
	ctx := context.Background()
	cl := testClient(t)
	cl.RPush(ctx, "l", "a", "b", "c", "d")

	l, _ := NewLoader("list", "l", 0)
	page, err := l.LoadPage(ctx, cl, "0", "", 2)
	if err != nil || len(page.Rows) != 2 || page.NextCursor == "" {
		t.Fatalf("page1: %+v %v", page, err)
	}
	page2, err := l.LoadPage(ctx, cl, page.NextCursor, "", 2)
	if err != nil || len(page2.Rows) != 2 || page2.NextCursor != "" {
		t.Fatalf("page2: %+v %v", page2, err)
	}

	if err := EditRow(ctx, cl, "l", EditRequest{Op: "update", Type: "list", Index: 1, Value: "Qg=="}); err != nil {
		t.Fatal(err)
	}
	if v, _ := cl.LIndex(ctx, "l", 1).Result(); v != "B" {
		t.Fatalf("LSet failed: %q", v)
	}
	if err := EditRow(ctx, cl, "l", EditRequest{Op: "remove", Type: "list", Value: "Qg=="}); err != nil {
		t.Fatal(err)
	}
	if n, _ := cl.LLen(ctx, "l").Result(); n != 3 {
		t.Fatalf("LRem failed: %d", n)
	}
}

func TestZSetScoreUpdate(t *testing.T) {
	ctx := context.Background()
	cl := testClient(t)
	cl.ZAdd(ctx, "z", redis.Z{Score: 1, Member: "m1"}, redis.Z{Score: 2, Member: "m2"})

	l, _ := NewLoader("zset", "z", 0)
	page, err := l.LoadPage(ctx, cl, "0", "*", 10)
	if err != nil || len(page.Rows) != 2 {
		t.Fatalf("page: %v %v", page, err)
	}
	if page.Rows[0].Key != "m1" || page.Rows[0].Score != 1 {
		t.Fatalf("zset row: %+v", page.Rows[0])
	}

	if err := EditRow(ctx, cl, "z", EditRequest{Op: "update", Type: "zset", Key: "m1", Score: 9}); err != nil {
		t.Fatal(err)
	}
	if s, _ := cl.ZScore(ctx, "z", "m1").Result(); s != 9 {
		t.Fatalf("score update failed: %v", s)
	}
}

func TestTTLAndKeyOps(t *testing.T) {
	ctx := context.Background()
	cl := testClient(t)
	cl.Set(ctx, "k", "v", 0)

	if err := SetTTL(ctx, cl, "k", 100); err != nil {
		t.Fatal(err)
	}
	if ttl, _ := cl.TTL(ctx, "k").Result(); ttl < 99 {
		t.Fatalf("ttl wrong: %v", ttl)
	}
	if err := SetTTL(ctx, cl, "k", 0); err != nil {
		t.Fatal(err)
	}
	if ttl, _ := cl.TTL(ctx, "k").Result(); ttl != -1 {
		t.Fatalf("persist failed: %v", ttl)
	}

	if err := RenameKey(ctx, cl, "k", "k2"); err != nil {
		t.Fatal(err)
	}
	if err := DeleteKey(ctx, cl, "k2"); err != nil {
		t.Fatal(err)
	}
	if n, _ := cl.Exists(ctx, "k2").Result(); n != 0 {
		t.Fatal("delete failed")
	}
}

func TestStringValueSizeLimit(t *testing.T) {
	ctx := context.Background()
	cl := testClient(t)
	cl.Set(ctx, "big", "0123456789", 0)

	l, _ := NewLoader("string", "big", 5)
	meta, _ := l.Meta(ctx, cl)
	if !meta.Truncated || meta.Size != 10 {
		t.Fatalf("meta: %+v", meta)
	}
	page, err := l.LoadPage(ctx, cl, "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Rows[0].Value) == 0 {
		t.Fatal("no value")
	}
}
