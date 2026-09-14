package connections

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rdeck/internal/redisclient"
	"rdeck/internal/secrets"
)

// Secrets must live in the keyring (memory provider here), never in JSON.
func TestSecretsInKeyringNotJSON(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStoreWithSecrets(dir, secrets.ForMemory())
	if err != nil {
		t.Fatal(err)
	}

	cfg := &redisclient.Config{Name: "prod", Host: "h", Port: 6379,
		Auth: "redis-secret", SSHPassword: "ssh-secret"}
	if err := s.Save(cfg); err != nil {
		t.Fatal(err)
	}

	// JSON on disk must not contain the secrets.
	data, _ := os.ReadFile(filepath.Join(dir, "connections.json"))
	if strings.Contains(string(data), "redis-secret") || strings.Contains(string(data), "ssh-secret") {
		t.Fatalf("secrets leaked to JSON: %s", data)
	}

	// List hydrates them back.
	all, _ := s.List()
	if len(all) != 1 || all[0].Auth != "redis-secret" || all[0].SSHPassword != "ssh-secret" {
		t.Fatalf("hydrate failed: %+v", all)
	}

	// Delete wipes the keyring.
	id := all[0].ID
	if err := s.Delete(id); err != nil {
		t.Fatal(err)
	}
	all, _ = s.List()
	if len(all) != 0 {
		t.Fatal("not deleted")
	}
}
