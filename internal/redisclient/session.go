package redisclient

import (
	"context"
	"time"

	"github.com/jjflamp/rdeck/internal/resp"
)

// NewSession opens a dedicated raw RESP2 connection for this connection
// (through the same dialer, so SSH tunnels apply), authenticates and
// selects the given db. Used by Console / MONITOR / PubSub where pooled
// clients are wrong (plan §3.1-②).
func (c *Connection) NewSession(ctx context.Context, db uint) (*resp.Conn, error) {
	d := &dialer{cfg: &c.cfg}
	if c.cfg.UseSSHTunnel {
		if c.ssh == nil {
			c.ssh = newSSHSession(&c.cfg, c.settingsDir)
		}
		d.ssh = c.ssh
	}

	conn, err := resp.Dial(ctx, d.DialContext, c.addr(), c.cfg.ConnectTimeout())
	if err != nil {
		return nil, err
	}

	// Auth & select on this dedicated connection. AUTH semantics must match
	// go-redis: only authenticate when a password is present (a username
	// alone must NOT trigger AUTH — redis would read it as a password and
	// reject with WRONGPASS).
	if args := authArgs(&c.cfg); args != nil {
		if _, err := conn.Cmd(ctx, args...); err != nil {
			conn.Close()
			return nil, err
		}
	}
	if db != 0 && c.cluster == nil {
		if _, err := conn.Cmd(ctx, "SELECT", itoa(db)); err != nil {
			conn.Close()
			return nil, err
		}
	}
	return conn, nil
}

// authArgs builds the AUTH command per go-redis semantics:
//   - password only           → AUTH <pass>
//   - username + password     → AUTH <user> <pass>   (ACL)
//   - no password             → nil (no authentication)
func authArgs(cfg *Config) []string {
	if cfg.Auth == "" {
		return nil
	}
	if cfg.Username != "" {
		return []string{"AUTH", cfg.Username, cfg.Auth}
	}
	return []string{"AUTH", cfg.Auth}
}

func itoa(n uint) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

var _ = time.Second
