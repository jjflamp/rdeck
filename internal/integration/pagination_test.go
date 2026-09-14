// Pagination regression: "load more" must never repeat the previous page's
// last row (user-reported bug).
package integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/redis/go-redis/v9"

	"rdeck/internal/keymodel"
)

func seedAndClean(t *testing.T, cl keymodel.Client, key string, n int, seed func(i int)) {
	t.Helper()
	for i := 0; i < n; i++ {
		seed(i)
	}
	t.Cleanup(func() { cl.Del(context.Background(), key) })
}

func assertNoOverlap(t *testing.T, prevLast, nextFirst string) {
	t.Helper()
	if prevLast == nextFirst {
		t.Fatalf("page overlap: %q appears at end of page1 and start of page2", prevLast)
	}
}

func TestListPaginationNoOverlap(t *testing.T) {
	ctx := context.Background()
	conn := realConn(t)
	cl, _ := conn.Client(0)
	kmCl := cl.(keymodel.Client)

	key := "it:pag:list"
	cl.Del(ctx, key)
	seedAndClean(t, kmCl, key, 250, func(i int) {
		cl.RPush(ctx, key, fmt.Sprintf("i%d", i))
	})

	l, _ := keymodel.NewLoader("list", key, 0)
	p1, err := l.LoadPage(ctx, kmCl, "", "*", 100)
	if err != nil || len(p1.Rows) != 100 {
		t.Fatalf("p1: %d %v", len(p1.Rows), err)
	}
	p2, err := l.LoadPage(ctx, kmCl, p1.NextCursor, "*", 100)
	if err != nil || len(p2.Rows) != 100 {
		t.Fatalf("p2: %d %v", len(p2.Rows), err)
	}
	assertNoOverlap(t, p1.Rows[99].Value, p2.Rows[0].Value)
}

func TestHashPaginationNoOverlap(t *testing.T) {
	ctx := context.Background()
	conn := realConn(t)
	cl, _ := conn.Client(0)
	kmCl := cl.(keymodel.Client)

	key := "it:pag:hash"
	cl.Del(ctx, key)
	seedAndClean(t, kmCl, key, 250, func(i int) {
		cl.HSet(ctx, key, fmt.Sprintf("f%d", i), fmt.Sprintf("v%d", i))
	})

	l, _ := keymodel.NewLoader("hash", key, 0)
	seen := map[string]bool{}
	pages := 0
	cursor := ""
	for cursor != "" || pages == 0 {
		p, err := l.LoadPage(ctx, kmCl, cursor, "*", 100)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range p.Rows {
			if seen[r.Key] {
				t.Fatalf("duplicate field across pages: %s", r.Key)
			}
			seen[r.Key] = true
		}
		cursor = p.NextCursor
		pages++
		if pages > 10 {
			t.Fatal("too many pages")
		}
	}
	if len(seen) != 250 {
		t.Fatalf("expected 250 unique fields, got %d", len(seen))
	}
}

func TestZSetPaginationNoOverlap(t *testing.T) {
	ctx := context.Background()
	conn := realConn(t)
	cl, _ := conn.Client(0)
	kmCl := cl.(keymodel.Client)

	key := "it:pag:zset"
	cl.Del(ctx, key)
	seedAndClean(t, kmCl, key, 250, func(i int) {
		cl.ZAdd(ctx, key, redis.Z{Score: float64(i), Member: fmt.Sprintf("m%d", i)})
	})

	l, _ := keymodel.NewLoader("zset", key, 0)
	seen := map[string]bool{}
	cursor := ""
	pages := 0
	for cursor != "" || pages == 0 {
		p, err := l.LoadPage(ctx, kmCl, cursor, "*", 100)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range p.Rows {
			if seen[r.Key] {
				t.Fatalf("duplicate member across pages: %s", r.Key)
			}
			seen[r.Key] = true
		}
		cursor = p.NextCursor
		pages++
		if pages > 10 {
			t.Fatal("too many pages")
		}
	}
	if len(seen) != 250 {
		t.Fatalf("expected 250 unique members, got %d", len(seen))
	}
}
