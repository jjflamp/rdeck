package bulkops

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func clients(t *testing.T) (src, dst redis.Cmdable, mr *miniredis.Miniredis) {
	t.Helper()
	mr1 := miniredis.RunT(t)
	mr2 := miniredis.RunT(t)
	src = redis.NewClient(&redis.Options{Addr: mr1.Addr(), Protocol: 2})
	dst = redis.NewClient(&redis.Options{Addr: mr2.Addr(), Protocol: 2})
	t.Cleanup(func() { src.(*redis.Client).Close(); dst.(*redis.Client).Close() })
	return src, dst, mr1
}

func TestDeleteKeys(t *testing.T) {
	ctx := context.Background()
	cl, _, mr := clients(t)
	for i := 0; i < 500; i++ {
		mr.Set(fmt.Sprintf("k:%d", i), "v")
	}
	keys := make([]string, 500)
	for i := range keys {
		keys[i] = fmt.Sprintf("k:%d", i)
	}

	var lastDone, lastTotal int
	n, err := DeleteKeys(ctx, cl, keys, func(done, total int, _ string) { lastDone, lastTotal = done, total })
	if err != nil {
		t.Fatal(err)
	}
	if n != 500 {
		t.Fatalf("deleted %d", n)
	}
	if lastDone != 500 || lastTotal != 500 {
		t.Fatalf("progress: %d/%d", lastDone, lastTotal)
	}
	if n, _ := cl.DBSize(ctx).Result(); n != 0 {
		t.Fatalf("dbsize %d", n)
	}
}

func TestBulkTTL(t *testing.T) {
	ctx := context.Background()
	cl, _, mr := clients(t)
	mr.Set("a", "1")
	mr.Set("b", "2")

	n, err := BulkTTL(ctx, cl, []string{"a", "b"}, 100, nil)
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if ttl, _ := cl.TTL(ctx, "a").Result(); ttl <= 0 {
		t.Fatalf("ttl not set: %v", ttl)
	}

	// persist
	n, err = BulkTTL(ctx, cl, []string{"a", "b"}, 0, nil)
	if err != nil || n != 2 {
		t.Fatalf("persist n=%d err=%v", n, err)
	}
	if ttl, _ := cl.TTL(ctx, "a").Result(); ttl != -1 {
		t.Fatalf("persist failed: %v", ttl)
	}
}

func TestCopyKeysRoundTrip(t *testing.T) {
	// NOTE: miniredis implements DUMP for strings only, so the round trip
	// here covers string keys; multi-type copy is exercised against real
	// redis in integration tests (plan §3.2 测试策略).
	ctx := context.Background()
	src, dst, mr := clients(t)
	mr.Set("str", "payload")

	copied, failed, err := CopyKeys(ctx, src, dst, []string{"str"}, nil)
	if err != nil || copied != 1 || len(failed) != 0 {
		t.Fatalf("copied=%d failed=%v err=%v", copied, failed, err)
	}
	if v, _ := dst.Get(ctx, "str").Result(); v != "payload" {
		t.Fatalf("str: %q", v)
	}

	// TTL preservation
	mr.SetTTL("str", 300*time.Second)
	if _, _, err := CopyKeys(ctx, src, dst, []string{"str"}, nil); err != nil {
		t.Fatal(err)
	}
	if ttl, _ := dst.(*redis.Client).PTTL(ctx, "str").Result(); ttl <= 0 {
		t.Fatalf("ttl not preserved: %v", ttl)
	}
}

func TestCopyMissingKey(t *testing.T) {
	ctx := context.Background()
	src, dst, _ := clients(t)
	copied, failed, err := CopyKeys(ctx, src, dst, []string{"nope"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if copied != 0 || len(failed) != 1 {
		t.Fatalf("copied=%d failed=%v", copied, failed)
	}
}
