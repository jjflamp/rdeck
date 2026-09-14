package redisclient

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// SSHSession owns one multiplexed ssh.Client for a connection config.
// All redis TCP dials go through its DialContext. It handles auth
// (password / private key / agent), TOFU host-key verification, keepalives
// and lazy reconnection — the plan §4.1-③④ requirements.
type SSHSession struct {
	cfg            *Config
	knownHostsPath string

	mu     sync.Mutex
	client *ssh.Client
	stop   chan struct{}
}

func newSSHSession(cfg *Config, settingsDir string) *SSHSession {
	return &SSHSession{
		cfg:            cfg,
		knownHostsPath: filepath.Join(settingsDir, "known_hosts.json"),
		stop:           make(chan struct{}),
	}
}

// Client returns a live ssh.Client, (re)connecting if necessary.
func (s *SSHSession) Client(ctx context.Context) (*ssh.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		if _, _, err := s.client.SendRequest("keepalive@openssh.com", true, nil); err == nil {
			return s.client, nil
		}
		// Dead connection: drop and rebuild below.
		s.client.Close()
		s.client = nil
	}

	auth, err := s.authMethods()
	if err != nil {
		return nil, err
	}

	timeout := s.cfg.ConnectTimeout()
	clientCfg := &ssh.ClientConfig{
		User:            s.cfg.SSHUser,
		Auth:            auth,
		HostKeyCallback: s.hostKeyCallback(),
		Timeout:         timeout,
	}

	dialer := &net.Dialer{Timeout: timeout}
	netConn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", s.cfg.SSHHost, s.cfg.SSHPort))
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", s.cfg.SSHHost, err)
	}

	c, chans, reqs, err := ssh.NewClientConn(netConn, fmt.Sprintf("%s:%d", s.cfg.SSHHost, s.cfg.SSHPort), clientCfg)
	if err != nil {
		netConn.Close()
		return nil, fmt.Errorf("ssh handshake: %w", err)
	}
	s.client = ssh.NewClient(c, chans, reqs)
	s.startKeepalive()
	return s.client, nil
}

// DialContext implements the go-redis dialer hook: redis traffic is tunneled
// through the ssh session as forwarded TCP channels.
func (s *SSHSession) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	c, err := s.Client(ctx)
	if err != nil {
		return nil, err
	}
	conn, err := c.DialContext(ctx, network, addr)
	if err != nil {
		return nil, fmt.Errorf("ssh tunnel to %s: %w", addr, err)
	}
	return conn, nil
}

func (s *SSHSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stop != nil {
		close(s.stop)
		s.stop = nil
	}
	if s.client != nil {
		s.client.Close()
		s.client = nil
	}
}

func (s *SSHSession) startKeepalive() {
	stop := s.stop
	client := s.client
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				if _, _, err := client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
					return // next Client() call will rebuild
				}
			}
		}
	}()
}

// authMethods builds the auth list in preference order:
// explicit private key > agent > password.
func (s *SSHSession) authMethods() ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	if s.cfg.SSHPrivateKeyPath != "" {
		key, err := os.ReadFile(s.cfg.SSHPrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("read ssh private key: %w", err)
		}
		var signer ssh.Signer
		if s.cfg.SSHPassword != "" {
			// Reuse the password field as key passphrase when a key is set
			// (RESP.app semantics: ask_ssh_password prompts separately).
			signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(s.cfg.SSHPassword))
		} else {
			signer, err = ssh.ParsePrivateKey(key)
		}
		if err != nil {
			return nil, fmt.Errorf("parse ssh private key: %w", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	if s.cfg.SSHAgent {
		a, err := connectAgent(s.cfg.SSHAgentPath)
		if err != nil {
			return nil, fmt.Errorf("connect ssh-agent: %w", err)
		}
		methods = append(methods, ssh.PublicKeysCallback(a.Signers))
	}

	if s.cfg.SSHPassword != "" {
		methods = append(methods, ssh.Password(s.cfg.SSHPassword))
	}

	if len(methods) == 0 {
		if s.cfg.AskForSSHPassword {
			return nil, ErrSSHPasswordRequired
		}
		return nil, fmt.Errorf("no ssh auth method configured")
	}
	return methods, nil
}

func connectAgent(path string) (agent.Agent, error) {
	if path == "" {
		path = defaultAgentPath()
	}
	conn, err := net.Dial(defaultAgentNetwork(), path)
	if err != nil {
		return nil, err
	}
	return agent.NewClient(conn), nil
}

func defaultAgentPath() string {
	if runtime.GOOS == "windows" {
		return `\\.\pipe\openssh-ssh-agent`
	}
	return os.Getenv("SSH_AUTH_SOCK")
}

func defaultAgentNetwork() string {
	if runtime.GOOS == "windows" {
		// Named pipes are openable with net.Dial via the winpipe layer
		// in Go's runtime on Windows ("pipe" network is not standard;
		// os.OpenFile handles \\.\pipe paths, net.Dial accepts them too
		// through the same mechanism as of Go 1.22+).
		return "pipe"
	}
	return "unix"
}

// --- TOFU host-key verification (plan §4.1-③) -------------------------------

type knownHosts map[string]string // "host:port" -> base64(SHA256(hostkey))

func (s *SSHSession) hostKeyCallback() ssh.HostKeyCallback {
	return func(hostname string, _ net.Addr, key ssh.PublicKey) error {
		sum := sha256.Sum256(key.Marshal())
		fp := base64.RawStdEncoding.EncodeToString(sum[:])
		addrKey := net.JoinHostPort(hostname, fmt.Sprint(s.cfg.SSHPort))

		stored := s.loadKnownHosts()
		if existing, ok := stored[addrKey]; ok {
			if existing != fp {
				return fmt.Errorf(
					"ssh host key for %s has CHANGED (possible MITM). Stored fingerprint %s, got %s. "+
						"Remove the entry in %s if the change is legitimate",
					addrKey, existing, fp, s.knownHostsPath)
			}
			return nil
		}
		// First use: trust and record (TOFU).
		stored[addrKey] = fp
		return s.saveKnownHosts(stored)
	}
}

func (s *SSHSession) loadKnownHosts() knownHosts {
	data, err := os.ReadFile(s.knownHostsPath)
	if err != nil {
		return knownHosts{}
	}
	kh := knownHosts{}
	_ = json.Unmarshal(data, &kh)
	return kh
}

func (s *SSHSession) saveKnownHosts(kh knownHosts) error {
	data, err := json.MarshalIndent(kh, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.knownHostsPath), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.knownHostsPath, data, 0o600)
}
