package redisclient

import (
	"context"
	"net"
)

// dialer produces the go-redis Dialer hook for a connection config:
// direct TCP, or TCP tunneled through the shared SSH session. TLS is applied
// by go-redis itself (it wraps the returned conn), so SSH+TLS composes.
type dialer struct {
	cfg *Config
	ssh *SSHSession // nil when not tunneling
}

func (d *dialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if d.ssh != nil {
		return d.ssh.DialContext(ctx, network, addr)
	}
	nd := &net.Dialer{Timeout: d.cfg.ConnectTimeout()}
	return nd.DialContext(ctx, network, addr)
}
