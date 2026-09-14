// Package console implements the Redis CLI session: command tokenizing
// (port of RedisClient::Command::splitCommandString), execution over the
// dedicated session, human-readable reply rendering, and MONITOR streaming.
package console

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"rdeck/internal/resp"
)

type Session struct {
	conn *resp.Conn
	db   uint
	mu   sync.Mutex // serializes command mode usage
}

func New(ctx context.Context, conn *resp.Conn, db uint) *Session {
	return &Session{conn: conn, db: db}
}

// Execute tokenizes and runs one command line, returning rendered output.
func (s *Session) Execute(ctx context.Context, line string) (string, error) {
	args, err := SplitCommandString(line)
	if err != nil {
		return "", err
	}
	if len(args) == 0 {
		return "", nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	reply, err := s.conn.Cmd(ctx, args...)
	if err != nil {
		// redis errors come back as err: "(error) ..." style
		return "", fmt.Errorf("(error) %s", err.Error())
	}

	// SELECT issued in console changes this session's db only.
	if strings.EqualFold(args[0], "SELECT") && len(args) == 2 {
		s.db = parseUint(args[1])
	}
	return RenderReply(reply, 0), nil
}

// StartMonitor switches the session to MONITOR mode and returns a stream of
// raw monitor lines. Cancel ctx or call Close to stop.
func (s *Session) StartMonitor(ctx context.Context) (<-chan string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.conn.Cmd(ctx, "MONITOR"); err != nil {
		return nil, err
	}
	out := make(chan string, 64)
	go func() {
		defer close(out)
		for {
			reply, err := s.conn.ReadReply()
			if err != nil {
				return
			}
			select {
			case out <- fmt.Sprint(reply):
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// Subscribe enters pub/sub on this session and streams payloads of
// [kind, channel, payload] tuples.
func (s *Session) Subscribe(ctx context.Context, channels ...string) (<-chan []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.conn.Cmd(ctx, append([]string{"SUBSCRIBE"}, channels...)...); err != nil {
		return nil, err
	}
	out := make(chan []string, 64)
	go func() {
		defer close(out)
		for {
			reply, err := s.conn.ReadReply()
			if err != nil {
				return
			}
			if arr, ok := reply.([]any); ok && len(arr) >= 3 {
				msg := make([]string, 0, 3)
				for _, it := range arr[:3] {
					msg = append(msg, fmt.Sprint(it))
				}
				select {
				case out <- msg:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

func (s *Session) Close() { s.conn.Close() }

// SplitCommandString tokenizes a command line honoring single/double quotes,
// backtick quoting and backslash escapes — port of the original
// splitCommandString (plan §3.1-⑤).
func SplitCommandString(input string) ([]string, error) {
	var args []string
	var cur strings.Builder
	i := 0
	n := len(input)
	started := false

	endQuote := func(q byte) error {
		for i < n {
			c := input[i]
			i++
			switch c {
			case '\\':
				if i < n {
					cur.WriteByte(input[i])
					i++
				}
			case q:
				return nil
			default:
				cur.WriteByte(c)
			}
		}
		return fmt.Errorf("unbalanced quotes in command")
	}

	for i < n {
		c := input[i]
		i++
		switch c {
		case ' ', '\t', '\n', '\r':
			if started {
				args = append(args, cur.String())
				cur.Reset()
				started = false
			}
		case '\'', '"', '`':
			q := c
			if !started {
				started = true
			}
			if err := endQuote(q); err != nil {
				return nil, err
			}
		default:
			started = true
			cur.WriteByte(c)
		}
	}
	if started {
		args = append(args, cur.String())
	}
	return args, nil
}

// RenderReply converts a RESP value into console text, capping array depth.
func RenderReply(reply any, depth int) string {
	switch v := reply.(type) {
	case nil:
		return "(nil)"
	case string:
		return v
	case int64:
		return fmt.Sprintf("(integer) %d", v)
	case []any:
		if len(v) == 0 {
			return "(empty array)"
		}
		var sb strings.Builder
		pad := strings.Repeat("  ", depth)
		for i, item := range v {
			fmt.Fprintf(&sb, "%s%d) %s\n", pad, i+1, RenderReply(item, depth+1))
		}
		return strings.TrimRight(sb.String(), "\n")
	default:
		return fmt.Sprint(v)
	}
}

func parseUint(s string) uint {
	var n uint
	for _, c := range s {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + uint(c-'0')
	}
	return n
}
