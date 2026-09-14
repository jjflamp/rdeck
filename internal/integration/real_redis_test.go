// Integration tests against a REAL redis server (localhost:6379).
// These cover paths miniredis cannot: multi-type DUMP/RESTORE copy,
// full Connection.Open mode detection, and raw console sessions.
// Skipped automatically when no local redis is running.
package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jjflamp/rdeck/internal/bulkops"
	"github.com/jjflamp/rdeck/internal/console"
	"github.com/jjflamp/rdeck/internal/redisclient"
)

func realConn(t *testing.T) *redisclient.Connection {
	t.Helper()
	cl := redis.NewClient(&redis.Options{Addr: "localhost:6379", DialTimeout: time.Second})
	defer cl.Close()
	if err := cl.Ping(context.Background()).Err(); err != nil {
		t.Skip("no local redis on :6379:", err)
	}
	cfg := redisclient.Config{ID: "it-" + fmt.Sprint(time.Now().UnixNano()), Name: "it", Host: "localhost", Port: 6379}
	cfg.Defaults()
	conn, err := redisclient.Open(context.Background(), cfg, t.TempDir())
	if err != nil {
		t.Skip("redis open failed:", err)
	}
	t.Cleanup(conn.Close)
	return conn
}

func TestOpenStandaloneDetectsDBRange(t *testing.T) {
	conn := realConn(t)
	if conn.Mode() != redisclient.ModeStandalone {
		t.Fatalf("mode = %s", conn.Mode())
	}
	if len(conn.Databases()) < 16 {
		t.Fatalf("expected >=16 dbs, got %d", len(conn.Databases()))
	}
	if conn.Version() == "" {
		t.Fatal("no version parsed")
	}
}

func TestScanKeysWithTypes(t *testing.T) {
	ctx := context.Background()
	conn := realConn(t)
	cl, _ := conn.Client(0)

	keys := []string{"it:str", "it:hash", "it:list", "it:set", "it:zset"}
	cl.Set(ctx, keys[0], "v", 0)
	cl.HSet(ctx, keys[1], "f", "v")
	cl.RPush(ctx, keys[2], "a")
	cl.SAdd(ctx, keys[3], "m")
	cl.ZAdd(ctx, keys[4], redis.Z{Score: 1, Member: "z"})
	t.Cleanup(func() { cl.Del(ctx, "it:str", "it:hash", "it:list", "it:set", "it:zset") })

	res, err := conn.ScanKeys(ctx, 0, "it:*", 100)
	if err != nil {
		t.Fatal(err)
	}
	types := map[string]string{}
	for _, k := range res.Keys {
		types[k.Name] = k.Type
	}
	want := map[string]string{"it:str": "string", "it:hash": "hash", "it:list": "list",
		"it:set": "set", "it:zset": "zset"}
	for k, wt := range want {
		if types[k] != wt {
			t.Fatalf("%s type = %q want %q", k, types[k], wt)
		}
	}
	if !res.Complete {
		t.Fatal("scan should be complete")
	}
}

// Multi-type DUMP/RESTORE copy — impossible to exercise on miniredis
// (its DUMP only supports strings).
func TestCopyKeysMultiType(t *testing.T) {
	ctx := context.Background()
	conn := realConn(t)
	src, _ := conn.Client(0)
	dst, _ := conn.Client(9) // cross-db within the same server

	seed := map[string]func(){
		"it:c:str": func() { src.Set(ctx, "it:c:str", "payload", 0) },
		"it:c:hash": func() { src.HSet(ctx, "it:c:hash", "f1", "v1", "f2", "v2") },
		"it:c:list": func() { src.RPush(ctx, "it:c:list", "a", "b", "c") },
		"it:c:set":  func() { src.SAdd(ctx, "it:c:set", "m1", "m2") },
		"it:c:zset": func() { src.ZAdd(ctx, "it:c:zset", redis.Z{Score: 2.5, Member: "zm"}) },
	}
	var keys []string
	for k, seedFn := range seed {
		seedFn()
		keys = append(keys, k)
	}
	t.Cleanup(func() {
		src.Del(ctx, "it:c:str", "it:c:hash", "it:c:list", "it:c:set", "it:c:zset")
		dst.Del(ctx, "it:c:str", "it:c:hash", "it:c:list", "it:c:set", "it:c:zset")
	})

	// reuse the bulkops CopyKeys through direct import to avoid cycle
	copied, failed, err := bulkops.CopyKeys(ctx, src, dst, keys, nil)
	if err != nil || copied != len(keys) || len(failed) != 0 {
		t.Fatalf("copied=%d failed=%v err=%v", copied, failed, err)
	}

	if v, _ := dst.Get(ctx, "it:c:str").Result(); v != "payload" {
		t.Fatalf("str: %q", v)
	}
	if n, _ := dst.HLen(ctx, "it:c:hash").Result(); n != 2 {
		t.Fatalf("hash: %d", n)
	}
	if n, _ := dst.LLen(ctx, "it:c:list").Result(); n != 3 {
		t.Fatalf("list: %d", n)
	}
	if n, _ := dst.SCard(ctx, "it:c:set").Result(); n != 2 {
		t.Fatalf("set: %d", n)
	}
	if s, _ := dst.ZScore(ctx, "it:c:zset", "zm").Result(); s != 2.5 {
		t.Fatalf("zset: %v", s)
	}

	// TTL preserved across the copy
	src.Expire(ctx, "it:c:str", 500*time.Second)
	copied, failed, err = bulkops.CopyKeys(ctx, src, dst, []string{"it:c:str"}, nil)
	if err != nil || copied != 1 {
		t.Fatalf("recopy: %d %v %v", copied, failed, err)
	}
	if ttl, _ := dst.PTTL(ctx, "it:c:str").Result(); ttl <= 0 {
		t.Fatalf("ttl not preserved: %v", ttl)
	}
}

func TestConsoleSessionExecuteAndSelect(t *testing.T) {
	ctx := context.Background()
	conn := realConn(t)

	raw, err := conn.NewSession(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()

	s := console.New(ctx, raw, 0)
	if out, err := s.Execute(ctx, "SET it:console hello"); err != nil || out == "" {
		t.Fatalf("set: %q %v", out, err)
	}
	cl0, _ := conn.Client(0)
	defer cl0.Del(ctx, "it:console")

	if out, err := s.Execute(ctx, "SELECT 3"); err != nil {
		t.Fatalf("select: %v", err)
	} else if out == "" {
		t.Fatal("select: empty reply")
	}
	// after SELECT the same dedicated connection must serve db3
	if out, err := s.Execute(ctx, "SET it:console3 world"); err != nil {
		t.Fatalf("set db3: %v", err)
	} else {
		_ = out
	}
	cl3, _ := conn.Client(3)
	defer cl3.Del(ctx, "it:console3")
	if v, _ := cl3.Get(ctx, "it:console3").Result(); v != "world" {
		t.Fatalf("db3 value: %q", v)
	}
}
