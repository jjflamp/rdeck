// Package serverstats backs the Server panel: INFO snapshots, slow log,
// client list and pub/sub — counterpart of RESP.app's ServerStats::Model.
package serverstats

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

// InfoSnapshot is one INFO sample (a subset of sections, rendered verbatim).
type InfoSnapshot struct {
	Sections map[string]map[string]string `json:"sections"`
}

// CollectInfo fetches and parses the requested INFO sections
// (all sections when none are given).
func CollectInfo(ctx context.Context, cl redis.Cmdable, sections ...string) (*InfoSnapshot, error) {
	cmd := cl.Info(ctx)
	if len(sections) > 0 {
		cmd = cl.Info(ctx, sections...)
	}
	raw, err := cmd.Result()
	if err != nil {
		return nil, err
	}
	snap := &InfoSnapshot{Sections: map[string]map[string]string{}}
	cur := ""
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			cur = strings.TrimSpace(strings.TrimPrefix(line, "#"))
			snap.Sections[cur] = map[string]string{}
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok || cur == "" {
			continue
		}
		snap.Sections[cur][k] = v
	}
	return snap, nil
}

// SlowLogEntry is one SLOWLOG GET row.
type SlowLogEntry struct {
	ID        int64    `json:"id"`
	Timestamp int64    `json:"timestamp"` // unix seconds
	DurationUS int64   `json:"duration_us"`
	Command   string   `json:"command"`
	Client    string   `json:"client"`
}

// slowDoer matches clients with raw Do (Cmdable lacks it).
type slowDoer interface {
	Do(ctx context.Context, args ...interface{}) *redis.Cmd
}

// SlowLog fetches the last count entries.
func SlowLog(ctx context.Context, cl slowDoer, count int64) ([]SlowLogEntry, error) {
	raw, err := cl.Do(ctx, "SLOWLOG", "GET", count).Slice()
	if err != nil {
		// miniredis and some proxies may not support SLOWLOG
		return nil, fmt.Errorf("SLOWLOG failed: %w", err)
	}
	var out []SlowLogEntry
	for _, item := range raw {
		arr, ok := item.([]any)
		if !ok || len(arr) < 4 {
			continue
		}
		e := SlowLogEntry{}
		e.ID, _ = arr[0].(int64)
		e.Timestamp, _ = arr[1].(int64)
		e.DurationUS, _ = arr[2].(int64)
		if cmdArr, ok := arr[3].([]any); ok {
			parts := make([]string, 0, len(cmdArr))
			for _, c := range cmdArr {
				parts = append(parts, fmt.Sprint(c))
			}
			e.Command = strings.Join(parts, " ")
		}
		if len(arr) >= 6 {
			ip, _ := arr[4].(string)
			name, _ := arr[5].(string)
			e.Client = ip + " " + name
		}
		out = append(out, e)
	}
	return out, nil
}

// ClientRow is one CLIENT LIST entry.
type ClientRow struct {
	ID      string `json:"id"`
	Addr    string `json:"addr"`
	Name    string `json:"name"`
	Age     string `json:"age"`
	Idle    string `json:"idle"`
	DB      string `json:"db"`
	Cmd     string `json:"cmd"`
}

// ClientList parses CLIENT LIST output.
func ClientList(ctx context.Context, cl redis.Cmdable) ([]ClientRow, error) {
	raw, err := cl.ClientList(ctx).Result()
	if err != nil {
		return nil, err
	}
	var out []ClientRow
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line == "" {
			continue
		}
		row := ClientRow{}
		for _, kv := range strings.Fields(line) {
			k, v, _ := strings.Cut(kv, "=")
			switch k {
			case "id":
				row.ID = v
			case "addr":
				row.Addr = v
			case "name":
				row.Name = v
			case "age":
				row.Age = v
			case "idle":
				row.Idle = v
			case "db":
				row.DB = v
			case "cmd":
				row.Cmd = v
			}
		}
		out = append(out, row)
	}
	return out, nil
}

// DBSizes returns keyspace entries as db -> keys (for charts).
func DBSizes(snap *InfoSnapshot) map[string]int64 {
	out := map[string]int64{}
	if ks, ok := snap.Sections["Keyspace"]; ok {
		for k, v := range ks {
			idx := strings.TrimPrefix(k, "db")
			if n, err := strconv.ParseInt(strings.Split(v, ",")[0][5:], 10, 64); err == nil {
				out["db"+idx] = n
			}
		}
	}
	return out
}
