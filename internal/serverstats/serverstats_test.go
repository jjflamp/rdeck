package serverstats

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestCollectInfo(t *testing.T) {
	mr := miniredis.RunT(t)
	cl := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer cl.Close()
	mr.Set("k", "v")

	// miniredis INFO has no Server section; assert parsing structure instead.
	snap, err := CollectInfo(context.Background(), cl)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Sections) == 0 {
		t.Fatal("no sections parsed")
	}
	found := false
	for _, kv := range snap.Sections {
		if len(kv) > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("sections empty: %+v", snap.Sections)
	}
}

func TestParseKeyspace(t *testing.T) {
	snap := &InfoSnapshot{Sections: map[string]map[string]string{
		"Keyspace": {"db0": "keys=12,expires=3", "db1": "keys=5"},
	}}
	sizes := DBSizes(snap)
	if sizes["db0"] != 12 || sizes["db1"] != 5 {
		t.Fatalf("sizes: %+v", sizes)
	}
}

func TestClientList(t *testing.T) {
	mr := miniredis.RunT(t)
	cl := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer cl.Close()

	rows, err := ClientList(context.Background(), cl)
	if err != nil {
		// miniredis lacks CLIENT LIST; nothing to assert there.
		t.Skip("server does not support CLIENT LIST:", err)
	}
	if len(rows) == 0 || rows[0].Addr == "" {
		t.Fatalf("no clients parsed: %+v", rows)
	}
}
