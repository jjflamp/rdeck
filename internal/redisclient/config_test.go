package redisclient

import "testing"

func TestParseRedisURL(t *testing.T) {
	cases := []struct {
		url     string
		host    string
		port    int
		ssl     bool
		user    string
		auth    string
		wantErr bool
	}{
		{url: "redis://localhost:6379", host: "localhost", port: 6379},
		{url: "redis://:secretpw@127.0.0.1:6380", host: "127.0.0.1", port: 6380, auth: "secretpw"},
		{url: "redis://alice:pw@example.com:7000", host: "example.com", port: 7000, user: "alice", auth: "pw"},
		{url: "rediss://tls.example.com", host: "tls.example.com", port: 6379, ssl: true},
		{url: "http://example.com", wantErr: true},
		{url: "redis://", wantErr: true},
		{url: "redis://host:notaport", wantErr: true},
	}
	for _, c := range cases {
		cfg, err := ParseRedisURL(c.url)
		if c.wantErr {
			if err == nil {
				t.Errorf("%s: expected error", c.url)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.url, err)
			continue
		}
		if cfg.Host != c.host || cfg.Port != c.port || cfg.UseSSL != c.ssl ||
			cfg.Username != c.user || cfg.Auth != c.auth {
			t.Errorf("%s: got %+v", c.url, cfg)
		}
	}
}

func TestValidateSSH(t *testing.T) {
	cfg := &Config{Host: "r", Port: 6379, UseSSHTunnel: true, SSHHost: "b", SSHUser: "u"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing ssh auth error")
	}
	cfg.SSHPrivateKeyPath = "/some/key"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestDefaults(t *testing.T) {
	cfg := &Config{Host: "h"}
	cfg.Defaults()
	if cfg.Port != 6379 || cfg.TimeoutConnectMS != 60000 || cfg.KeysPattern != "*" ||
		cfg.NamespaceSeparator != ":" || cfg.DBScanLimit != 20 {
		t.Fatalf("defaults wrong: %+v", cfg)
	}
}
