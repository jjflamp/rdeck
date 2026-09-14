package redisclient

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// Scanner exercises the raw-SCAN path (plan §4.1-①) directly against
// miniredis, bypassing Connection.Open (miniredis INFO coverage is partial).
func testClient(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	cl := redis.NewClient(&redis.Options{Addr: mr.Addr(), Protocol: 2})
	t.Cleanup(func() { cl.Close() })
	return mr, cl
}

func TestScanNodeCollectsKeys(t *testing.T) {
	mr, cl := testClient(t)
	for _, k := range []string{"user:1", "user:2", "sess:1", "plain"} {
		mr.Set(k, "v")
	}
	mr.HSet("h:1", "f", "v")

	res := &ScanResult{}
	if err := scanNode(context.Background(), cl, "*", 100, res); err != nil {
		t.Fatal(err)
	}
	if !res.Complete || res.Scanned != 5 {
		t.Fatalf("want complete 5, got %d complete=%v", res.Scanned, res.Complete)
	}

	res.Keys = detectTypes(context.Background(), cl, res.Keys)
	types := map[string]string{}
	for _, k := range res.Keys {
		types[k.Name] = k.Type
	}
	if types["h:1"] != "hash" || types["plain"] != "string" {
		t.Fatalf("types wrong: %+v", types)
	}
}

func TestScanNodeRespectsLimit(t *testing.T) {
	mr, cl := testClient(t)
	for i := 0; i < 50; i++ {
		mr.Set(string(rune('a'+i%26)) + string(rune('a'+i/26)), "v")
	}

	res := &ScanResult{}
	if err := scanNode(context.Background(), cl, "*", 10, res); err != nil {
		t.Fatal(err)
	}
	if res.Scanned != 10 {
		t.Fatalf("want 10, got %d", res.Scanned)
	}
	if res.Complete {
		t.Fatal("limit hit must not report complete")
	}
}

func TestScanNodeMatchPattern(t *testing.T) {
	mr, cl := testClient(t)
	mr.Set("user:1", "v")
	mr.Set("user:2", "v")
	mr.Set("other", "v")

	res := &ScanResult{}
	if err := scanNode(context.Background(), cl, "user:*", 100, res); err != nil {
		t.Fatal(err)
	}
	if res.Scanned != 2 {
		t.Fatalf("want 2 matches, got %d", res.Scanned)
	}
}

func TestParseCursorTypes(t *testing.T) {
	for input, want := range map[interface{}]uint64{"0": 0, "123": 123, int64(7): 7} {
		got, err := parseCursor(input)
		if err != nil || got != want {
			t.Fatalf("parseCursor(%v) = %d, %v", input, got, err)
		}
	}
}
