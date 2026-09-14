package redisclient

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ConnectionConfig mirrors RESP.app's RedisClient::ConnectionConfig + ServerConfig.
// JSON field names are aligned with the original toJsonObject() output so that
// connections.json exported by RESP.app can be imported losslessly.
type Config struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
	// Auth is the redis password (ACL password). Stored via keyring in a later
	// milestone; JSON for now, matching original behavior.
	Auth             string `json:"auth,omitempty"`
	Username         string `json:"username,omitempty"`
	TimeoutConnectMS uint   `json:"timeout_connect,omitempty"`
	TimeoutExecuteMS uint   `json:"timeout_execute,omitempty"`

	// TLS
	UseSSL             bool   `json:"ssl,omitempty"`
	SSLCACertPath      string `json:"ssl_ca_cert_path,omitempty"`
	SSLPrivateKeyPath  string `json:"ssl_private_key_path,omitempty"`
	SSLLocalCertPath   string `json:"ssl_local_cert_path,omitempty"`
	SSLIgnoreAllErrors bool   `json:"ssl_ignore_all_errors,omitempty"`

	// SSH tunnel
	UseSSHTunnel      bool   `json:"use_ssh_tunnel,omitempty"`
	SSHHost           string `json:"ssh_host,omitempty"`
	SSHPort           int    `json:"ssh_port,omitempty"`
	SSHUser           string `json:"ssh_user,omitempty"`
	SSHPassword       string `json:"ssh_password,omitempty"`
	SSHPrivateKeyPath string `json:"ssh_private_key_path,omitempty"`
	SSHPublicKeyPath  string `json:"ssh_public_key_path,omitempty"`
	SSHAgent          bool   `json:"ssh_agent,omitempty"`
	SSHAgentPath      string `json:"ssh_agent_path,omitempty"`
	AskForSSHPassword bool   `json:"ask_ssh_password,omitempty"`

	// Cluster
	ClusterHostOverride bool `json:"cluster_host_override,omitempty"`

	// App-level options (RESP.app ServerConfig extensions)
	KeysPattern        string              `json:"keys_pattern,omitempty"`
	NamespaceSeparator string              `json:"namespace_separator,omitempty"`
	DBScanLimit        uint                `json:"db_scan_limit,omitempty"`
	DefaultFormatter   string              `json:"default_formatter,omitempty"`
	IconColor          string              `json:"icon_color,omitempty"`
	FilterHistory      map[string][]string `json:"filter_history,omitempty"`
}

const (
	defaultPort        = 6379
	defaultSSHPort     = 22
	defaultTimeoutMS   = 60000
	defaultKeysPattern = "*"
	defaultNSSeparator = ":"
	defaultDBScanLimit = 20
)

// ErrSSHPasswordRequired signals the UI to prompt the user for the SSH
// password and retry with it filled in (askForSSHPassword flow, plan P1.5).
var ErrSSHPasswordRequired = errors.New("ssh password required")

// Defaults fills empty fields with the same defaults RESP.app uses.
func (c *Config) Defaults() {
	if c.ID == "" {
		c.ID = newID()
	}
	if c.Port == 0 {
		c.Port = defaultPort
	}
	if c.TimeoutConnectMS == 0 {
		c.TimeoutConnectMS = defaultTimeoutMS
	}
	if c.TimeoutExecuteMS == 0 {
		c.TimeoutExecuteMS = defaultTimeoutMS
	}
	if c.SSHPort == 0 {
		c.SSHPort = defaultSSHPort
	}
	if c.KeysPattern == "" {
		c.KeysPattern = defaultKeysPattern
	}
	if c.NamespaceSeparator == "" {
		c.NamespaceSeparator = defaultNSSeparator
	}
	if c.DBScanLimit == 0 {
		c.DBScanLimit = defaultDBScanLimit
	}
}

// Validate reports whether the config has enough information to connect.
func (c *Config) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("host is required")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port %d", c.Port)
	}
	if c.UseSSHTunnel {
		if c.SSHHost == "" {
			return fmt.Errorf("ssh host is required")
		}
		if c.SSHUser == "" {
			return fmt.Errorf("ssh user is required")
		}
		hasAuth := c.SSHPassword != "" || c.SSHPrivateKeyPath != "" || c.SSHAgent || c.AskForSSHPassword
		if !hasAuth {
			return fmt.Errorf("ssh auth method is required (password, private key or agent)")
		}
	}
	return nil
}

// ConnectTimeout / ExecuteTimeout return go-redis style durations.
func (c *Config) ConnectTimeout() time.Duration {
	return time.Duration(c.TimeoutConnectMS) * time.Millisecond
}
func (c *Config) ExecuteTimeout() time.Duration {
	return time.Duration(c.TimeoutExecuteMS) * time.Millisecond
}

// Addr returns "host:port" of the redis endpoint (as seen from inside the
// tunnel when UseSSHTunnel is set).
func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// ParseRedisURL parses redis:// / rediss:// / redis+sentinel style URLs,
// mirroring ConnectionsManager::parseConfigFromRedisConnectionString.
func ParseRedisURL(raw string) (*Config, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	switch strings.ToLower(u.Scheme) {
	case "redis", "rediss":
	default:
		return nil, fmt.Errorf("unsupported scheme %q (want redis:// or rediss://)", u.Scheme)
	}

	cfg := &Config{Port: defaultPort}
	if h := u.Hostname(); h != "" {
		cfg.Host = h
	}
	if p := u.Port(); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 || n > 65535 {
			return nil, fmt.Errorf("invalid port %q", p)
		}
		cfg.Port = n
	}
	if u.User != nil {
		cfg.Username = u.User.Username()
		cfg.Auth, _ = u.User.Password()
	}
	if strings.EqualFold(u.Scheme, "rediss") {
		cfg.UseSSL = true
	}
	q := u.Query()
	if db := q.Get("db"); db != "" {
		// P1 will store db navigation; URL db is informational for now.
		_ = db
	}
	if cfg.Host == "" {
		return nil, fmt.Errorf("host is required")
	}
	cfg.Name = cfg.Host
	cfg.Defaults()
	return cfg, nil
}
