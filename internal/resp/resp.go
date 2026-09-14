// Package resp is a minimal RESP2 client over a dedicated net.Conn —
// the "专用单连接" of plan §3.1-②. It backs the Console (arbitrary commands
// incl. SELECT/MONITOR) and PubSub, where pooled go-redis clients would be
// semantically wrong.
package resp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Conn is a single RESP2 connection. Writes are serialized; reads are up to
// the owner (command/reply pairing, or a push loop for MONITOR/SUBSCRIBE).
type Conn struct {
	c    net.Conn
	br   *bufio.Reader
	wmu  sync.Mutex
	once sync.Once
}

// Dial establishes the raw connection. dialer may tunnel over SSH (the same
// hook go-redis uses), so SSH/TLS combos keep working.
func Dial(ctx context.Context, dialer func(ctx context.Context, network, addr string) (net.Conn, error), addr string, timeout time.Duration) (*Conn, error) {
	nd := &net.Dialer{Timeout: timeout}
	var c net.Conn
	var err error
	if dialer != nil {
		c, err = dialer(ctx, "tcp", addr)
	} else {
		c, err = nd.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, err
	}
	return &Conn{c: c, br: bufio.NewReader(c)}, nil
}

// Cmd writes the command and reads one reply (request/response mode).
func (c *Conn) Cmd(ctx context.Context, args ...string) (any, error) {
	if err := c.WriteCommand(args...); err != nil {
		return nil, err
	}
	deadline := time.Time{}
	if d, ok := ctx.Deadline(); ok {
		deadline = d
	}
	_ = c.c.SetReadDeadline(deadline)
	return c.ReadReply()
}

// WriteCommand serializes args as an array of bulk strings.
func (c *Conn) WriteCommand(args ...string) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	var sb strings.Builder
	sb.WriteString("*" + strconv.Itoa(len(args)) + "\r\n")
	for _, a := range args {
		sb.WriteString("$" + strconv.Itoa(len(a)) + "\r\n" + a + "\r\n")
	}
	_ = c.c.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := io.WriteString(c.c, sb.String())
	return err
}

// ReadReply parses one RESP2 reply: simple/error string, integer, bulk, array.
func (c *Conn) ReadReply() (any, error) {
	line, err := c.readLine()
	if err != nil {
		return nil, err
	}
	if len(line) < 2 {
		return nil, fmt.Errorf("short reply line")
	}
	body := string(line[1 : len(line)-2]) // strip type byte + CRLF
	switch line[0] {
	case '+':
		return body, nil
	case '-':
		return nil, fmt.Errorf("%s", body)
	case ':':
		return strconv.ParseInt(body, 10, 64)
	case '$':
		n, err := strconv.Atoi(body)
		if err != nil {
			return nil, err
		}
		if n == -1 {
			return nil, nil // nil bulk
		}
		buf := make([]byte, n+2) // payload + CRLF
		if _, err := io.ReadFull(c.br, buf); err != nil {
			return nil, err
		}
		return string(buf[:n]), nil
	case '*':
		n, err := strconv.Atoi(body)
		if err != nil {
			return nil, err
		}
		if n == -1 {
			return nil, nil // nil array
		}
		out := make([]any, 0, n)
		for i := 0; i < n; i++ {
			item, err := c.ReadReply()
			if err != nil {
				return nil, err
			}
			out = append(out, item)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unknown reply type %q", line[0])
	}
}

func (c *Conn) readLine() ([]byte, error) {
	line, err := c.br.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	if len(line) < 2 || line[len(line)-2] != '\r' {
		return nil, fmt.Errorf("malformed reply line")
	}
	return line, nil
}

// Close closes the underlying connection (idempotent).
func (c *Conn) Close() {
	c.once.Do(func() { _ = c.c.Close() })
}
