package redisclient

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

func TestAuthArgsMatchGoRedisSemantics(t *testing.T) {
	cases := []struct {
		name     string
		username string
		auth     string
		want     []string
	}{
		{"no credentials", "", "", nil},
		{"username only must NOT auth", "default", "", nil},
		{"password only", "", "secret", []string{"AUTH", "secret"}},
		{"acl user+pass", "alice", "pw", []string{"AUTH", "alice", "pw"}},
	}
	for _, c := range cases {
		got := authArgs(&Config{Username: c.username, Auth: c.auth})
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

// The username-only config that produced WRONGPASS on real servers must now
// open a session without any AUTH attempt (regression for the console bug).
func TestSessionUsernameOnlyNoAuth(t *testing.T) {
	mr := miniredis.RunT(t)
	mr.Set("k", "v") // no auth required on the server

	port, err := strconv.Atoi(strings.TrimPrefix(mr.Addr(), "127.0.0.1:"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{ID: "t1", Name: "t", Host: "127.0.0.1", Port: port, Username: "default"}
	cfg.Defaults()

	// Open the connection the normal way (go-redis path also must not auth).
	conn, err := Open(t.Context(), cfg, t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()

	// The raw console session must succeed too (previously sent AUTH default).
	raw, err := conn.NewSession(t.Context(), 0)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	defer raw.Close()
	if _, err := raw.Cmd(t.Context(), "PING"); err != nil {
		t.Fatalf("ping: %v", err)
	}
}
