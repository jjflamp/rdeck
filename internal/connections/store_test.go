package connections

import (
	"os"
	"path/filepath"
	"testing"

	"rdeck/internal/redisclient"
	"rdeck/internal/secrets"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStoreWithSecrets(t.TempDir(), secrets.ForMemory())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSaveListDelete(t *testing.T) {
	s := newTestStore(t)

	cfg := &redisclient.Config{Name: "local", Host: "127.0.0.1", Port: 6379}
	if err := s.Save(cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	if cfg.ID == "" {
		t.Fatal("expected ID to be generated")
	}

	all, err := s.List()
	if err != nil || len(all) != 1 {
		t.Fatalf("list: %v (%d)", err, len(all))
	}

	// upsert by ID
	cfg.Name = "local-renamed"
	if err := s.Save(cfg); err != nil {
		t.Fatal(err)
	}
	all, _ = s.List()
	if len(all) != 1 || all[0].Name != "local-renamed" {
		t.Fatalf("upsert failed: %+v", all)
	}

	if err := s.Delete(cfg.ID); err != nil {
		t.Fatal(err)
	}
	all, _ = s.List()
	if len(all) != 0 {
		t.Fatalf("delete failed: %+v", all)
	}
}

func TestSaveRejectsInvalid(t *testing.T) {
	s := newTestStore(t)
	if err := s.Save(&redisclient.Config{Port: 6379}); err == nil {
		t.Fatal("expected validation error for empty host")
	}
	if err := s.Save(&redisclient.Config{Host: "h", Port: 6379, UseSSHTunnel: true}); err == nil {
		t.Fatal("expected validation error for ssh without user/auth")
	}
}

func TestImportFromRDMArray(t *testing.T) {
	s := newTestStore(t)

	// RESP.app exports a bare array of connection objects using the
	// original parameter key names.
	orig := `[
	  {"id":"abc123","name":"prod","host":"10.0.0.1","port":6379,
	   "auth":"secret","timeout_connect":60000,"timeout_execute":60000,
	   "keys_pattern":"*","namespace_separator":":",
	   "ssh_host":"bastion","ssh_port":22,"ssh_user":"ops",
	   "ssh_private_key_path":"/home/ops/id_rsa"},
	  {"name":"tls","host":"redis.example.com","port":6380,"ssl":true},
	  {"host":""},
	  "not-an-object"
	]`
	path := filepath.Join(t.TempDir(), "connections.json")
	if err := os.WriteFile(path, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}

	n, err := s.ImportFromRDMFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("expected 2 imported, got %d", n)
	}

	all, _ := s.List()
	byName := map[string]*redisclient.Config{}
	for _, c := range all {
		byName[c.Name] = c
	}

	prod := byName["prod"]
	if prod == nil || !prod.UseSSHTunnel || prod.SSHUser != "ops" {
		t.Fatalf("ssh fields not imported: %+v", prod)
	}
	// Original ID collided with nothing but must be preserved when unique.
	if prod.ID != "abc123" {
		t.Fatalf("id not preserved: %q", prod.ID)
	}
	if tls := byName["tls"]; tls == nil || !tls.UseSSL {
		t.Fatalf("ssl flag not imported: %+v", tls)
	}
}

func TestImportPreservesOriginalFile(t *testing.T) {
	s := newTestStore(t)
	orig := `[{"name":"a","host":"h"}]`
	path := filepath.Join(t.TempDir(), "connections.json")
	os.WriteFile(path, []byte(orig), 0o600)

	if _, err := s.ImportFromRDMFile(path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != orig {
		t.Fatal("source file must not be modified")
	}
}
