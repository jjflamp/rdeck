package redisclient

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"
)

type Mode string

const (
	ModeStandalone Mode = "standalone"
	ModeCluster    Mode = "cluster"
	ModeSentinel   Mode = "sentinel"
)

// DBInfo describes one database of the keyspace (from INFO keyspace).
type DBInfo struct {
	Index uint   `json:"index"`
	Keys  int64  `json:"keys"`
	Label string `json:"label,omitempty"`
}

// Connection is the per-connection facade: owns the transport (direct/SSH),
// detects the deployment mode, and hands out per-db clients
// (plan §2.3-① and §3.1-③).
type Connection struct {
	cfg         Config
	mode        Mode
	version     string
	settingsDir string

	mu      sync.Mutex
	ssh     *SSHSession            // non-nil when tunneling
	cluster *redis.ClusterClient
	dbs     map[uint]*redis.Client // standalone: (connID, dbIndex) -> client
	dbInfos []DBInfo
	master  string // resolved master addr for sentinel mode
}

// Open dials the config, authenticates and probes the deployment mode.
// settingsDir is needed for the SSH known-hosts store.
func Open(ctx context.Context, cfg Config, settingsDir string) (*Connection, error) {
	c := &Connection{cfg: cfg, mode: ModeStandalone, settingsDir: settingsDir, dbs: map[uint]*redis.Client{}}

	base := c.newClient(ctx, 0, cfg.Addr())
	if err := base.Ping(ctx).Err(); err != nil {
		base.Close()
		return nil, fmt.Errorf("connect %s: %w", cfg.Addr(), err)
	}

	info, err := base.Info(ctx, "server", "replication", "cluster", "keyspace").Result()
	if err != nil {
		// Some proxies/miniredis reject multi-section INFO — fall back.
		if info, err = base.Info(ctx).Result(); err != nil {
			base.Close()
			return nil, fmt.Errorf("INFO failed: %w", err)
		}
	}
	parsed := parseInfo(info)
	c.version = parsed["redis_version"]

	switch {
	case parsed["cluster_enabled"] == "1":
		base.Close()
		c.mode = ModeCluster
		if err := c.openCluster(ctx); err != nil {
			return nil, err
		}
	case parsed["role"] == "sentinel":
		base.Close()
		c.mode = ModeSentinel
		if err := c.openSentinelMaster(ctx, cfg.Addr()); err != nil {
			return nil, err
		}
	default:
		c.dbs[0] = base
		c.dbInfos = c.fillDBRange(ctx, base, parseKeyspace(parsed, info))
	}
	return c, nil
}

// fillDBRange expands the keyspace-derived db list to the full configured
// range (db0..N-1, empty dbs included with count 0) — matching the original
// RDM sidebar which lists all databases.
func (c *Connection) fillDBRange(ctx context.Context, base *redis.Client, present []DBInfo) []DBInfo {
	total := 16
	if v, err := base.Do(ctx, "CONFIG", "GET", "databases").Slice(); err == nil && len(v) == 2 {
		if n, cerr := strconv.Atoi(fmt.Sprint(v[1])); cerr == nil && n > 0 && n <= 1024 {
			total = n
		}
	}
	byIdx := make(map[uint]DBInfo, len(present))
	for _, d := range present {
		byIdx[d.Index] = d
	}
	out := make([]DBInfo, 0, total)
	for i := 0; i < total; i++ {
		d, ok := byIdx[uint(i)]
		if !ok {
			d = DBInfo{Index: uint(i), Keys: 0}
		}
		out = append(out, d)
	}
	return out
}

func (c *Connection) newClient(ctx context.Context, db uint, addr string) *redis.Client {
	d := &dialer{cfg: &c.cfg}
	if c.cfg.UseSSHTunnel {
		if c.ssh == nil {
			c.ssh = newSSHSession(&c.cfg, c.settingsDir)
		}
		d.ssh = c.ssh
	}
	return redis.NewClient(&redis.Options{
		Addr:         addr,
		DB:           int(db),
		Username:     c.cfg.Username,
		Password:     c.cfg.Auth,
		Dialer:       d.DialContext,
		DialTimeout:  c.cfg.ConnectTimeout(),
		ReadTimeout:  c.cfg.ExecuteTimeout(),
		WriteTimeout: c.cfg.ExecuteTimeout(),
		MaxRetries:   1,
		Protocol:     2, // RESP2, plan §4.1-⑤
	})
}

func (c *Connection) openCluster(ctx context.Context) error {
	d := &dialer{cfg: &c.cfg}
	if c.cfg.UseSSHTunnel {
		c.ssh = newSSHSession(&c.cfg, c.settingsDir)
		d.ssh = c.ssh
	}
	c.cluster = redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:        []string{c.cfg.Addr()},
		Username:     c.cfg.Username,
		Password:     c.cfg.Auth,
		Dialer:       d.DialContext,
		DialTimeout:  c.cfg.ConnectTimeout(),
		ReadTimeout:  c.cfg.ExecuteTimeout(),
		WriteTimeout: c.cfg.ExecuteTimeout(),
		Protocol:     2,
	})
	if err := c.cluster.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("cluster connect: %w", err)
	}
	c.dbInfos = []DBInfo{{Index: 0, Label: "cluster db0"}}
	return nil
}

// openSentinelMaster resolves the monitored master via the sentinel and
// reconnects to it as a standalone instance (mirrors the original
// sentinelConnectToMaster flow).
func (c *Connection) openSentinelMaster(ctx context.Context, sentinelAddr string) error {
	sc := c.newClient(ctx, 0, sentinelAddr)
	defer sc.Close()

	masters, err := sc.Do(ctx, "SENTINEL", "masters").Slice()
	if err != nil || len(masters) == 0 {
		return fmt.Errorf("SENTINEL masters failed: %w", err)
	}
	first, ok := masters[0].([]interface{})
	if !ok {
		return fmt.Errorf("unexpected SENTINEL masters reply shape")
	}
	kv := flatPairs(first)
	name := kv["name"]
	if name == "" {
		return fmt.Errorf("sentinel master has no name")
	}

	reply, err := sc.Do(ctx, "SENTINEL", "get-master-addr-by-name", name).Slice()
	if err != nil || len(reply) != 2 {
		return fmt.Errorf("get-master-addr-by-name failed: %w", err)
	}
	host, _ := reply[0].(string)
	port, _ := reply[1].(string)
	if host == "" || port == "" {
		return fmt.Errorf("sentinel returned empty master address")
	}

	master := fmt.Sprintf("%s:%s", host, port)
	mc := c.newClient(ctx, 0, master)
	if err := mc.Ping(ctx).Err(); err != nil {
		mc.Close()
		return fmt.Errorf("connect master %s: %w", master, err)
	}
	c.dbs[0] = mc
	c.master = master
	c.dbInfos = c.fillDBRange(ctx, mc, parseKeyspace(nil, mustInfo(ctx, mc)))
	return nil
}

// Client returns the cmdable for a db index. Cluster mode only exposes db0.
func (c *Connection) Client(db uint) (redis.Cmdable, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cluster != nil {
		if db != 0 {
			return nil, fmt.Errorf("cluster mode only supports db0")
		}
		return c.cluster, nil
	}
	if cl, ok := c.dbs[db]; ok {
		return cl, nil
	}
	// Lazily create clients for dbs not present in the keyspace listing.
	cl := c.newClient(context.Background(), db, c.addr())
	c.dbs[db] = cl
	return cl, nil
}

func (c *Connection) addr() string {
	if c.master != "" {
		return c.master
	}
	return c.cfg.Addr()
}

func (c *Connection) Databases() []DBInfo { return c.dbInfos }

func (c *Connection) Mode() Mode      { return c.mode }
func (c *Connection) Version() string { return c.version }
func (c *Connection) Config() *Config { return &c.cfg }

// DBSize returns the number of keys in db (used for db tree labels).
func (c *Connection) DBSize(ctx context.Context, db uint) (int64, error) {
	cl, err := c.Client(db)
	if err != nil {
		return 0, err
	}
	return cl.DBSize(ctx).Result()
}

func (c *Connection) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, cl := range c.dbs {
		cl.Close()
	}
	c.dbs = map[uint]*redis.Client{}
	if c.cluster != nil {
		c.cluster.Close()
		c.cluster = nil
	}
	if c.ssh != nil {
		c.ssh.Close()
		c.ssh = nil
	}
}

// --- INFO helpers -----------------------------------------------------------

// parseInfo flattens an INFO payload into a map (last value wins per key).
func parseInfo(info string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, ":"); ok {
			m[k] = v
		}
	}
	return m
}

// parseKeyspace extracts db list from the INFO keyspace section lines
// ("db0:keys=1,expires=0,...").
func parseKeyspace(flat map[string]string, info string) []DBInfo {
	var dbs []DBInfo
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "db") || !strings.Contains(line, "keys=") {
			continue
		}
		idxStr, rest, _ := strings.Cut(strings.TrimPrefix(line, "db"), ":")
		idx, err := strconv.ParseUint(idxStr, 10, 32)
		if err != nil {
			continue
		}
		var keys int64
		for _, kv := range strings.Split(rest, ",") {
			if k, v, ok := strings.Cut(kv, "="); ok && k == "keys" {
				keys, _ = strconv.ParseInt(v, 10, 64)
			}
		}
		dbs = append(dbs, DBInfo{Index: uint(idx), Keys: keys})
	}
	if dbs == nil {
		dbs = []DBInfo{{Index: 0}}
	}
	return dbs
}

func flatPairs(items []interface{}) map[string]string {
	m := map[string]string{}
	for i := 0; i+1 < len(items); i += 2 {
		k, _ := items[i].(string)
		v, _ := items[i+1].(string)
		m[k] = v
	}
	return m
}

func mustInfo(ctx context.Context, cl *redis.Client) string {
	s, err := cl.Info(ctx, "keyspace").Result()
	if err != nil {
		return ""
	}
	return s
}
