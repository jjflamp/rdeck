// Package secrets stores connection passwords in the OS keychain
// (plan §3.2: default keyring, JSON keeps no secrets). Falls back to
// in-memory operation when the platform keyring is unavailable — callers
// detect failure and keep plaintext in JSON as a graceful degradation.
package secrets

import (
	"fmt"

	"github.com/zalando/go-keyring"
)

const service = "rdeck"

// legacyService holds secrets written before the project rename; read as a
// fallback so users don't have to re-enter passwords.
const legacyService = "resp-go"

// Provider abstracts the keyring for testability.
type Provider interface {
	Set(field, value string) error
	Get(field string) (string, error)
	WipeAll()
}

type keyringProvider struct{ connID string }

func (k keyringProvider) Set(field, value string) error {
	return keyring.Set(service, k.connID+":"+field, value)
}

func (k keyringProvider) Get(field string) (string, error) {
	v, err := keyring.Get(service, k.connID+":"+field)
	if err == nil {
		return v, nil
	}
	// Fall back to the pre-rename service and migrate the value forward.
	if legacy, lerr := keyring.Get(legacyService, k.connID+":"+field); lerr == nil && legacy != "" {
		_ = keyring.Set(service, k.connID+":"+field, legacy)
		_ = keyring.Delete(legacyService, k.connID+":"+field)
		return legacy, nil
	}
	return v, err
}

func (k keyringProvider) WipeAll() {
	for _, f := range []string{"auth", "ssh_password"} {
		_ = keyring.Delete(service, k.connID+":"+f)
	}
}

// Fields that live in the keyring.
var Fields = []string{"auth", "ssh_password"}

// ProviderFactory builds a provider for a connection id.
type ProviderFactory func(connID string) Provider

// For returns a provider bound to a connection id.
func For(connID string) Provider { return keyringProvider{connID} }

// ForMemory returns a factory bound to one shared memory store (tests).
func ForMemory() ProviderFactory {
	m := NewMemory()
	return func(string) Provider { return m }
}

// Memory is an in-memory provider (tests / keyring-less environments).
type Memory struct{ data map[string]string }

func NewMemory() *Memory { return &Memory{data: map[string]string{}} }

func (m *Memory) Set(field, value string) error {
	m.data[field] = value
	return nil
}

func (m *Memory) Get(field string) (string, error) {
	v, ok := m.data[field]
	if !ok {
		return "", fmt.Errorf("secret not found")
	}
	return v, nil
}

func (m *Memory) WipeAll() { m.data = map[string]string{} }
