// Package settings persists global app settings (fonts, limits, locale...)
// analogous to RESP.app's QSettings-backed options.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Settings struct {
	// Value editor
	ValueSizeLimit  int    `json:"value_size_limit"`  // max bytes loaded for a single value
	AppFont         string `json:"app_font,omitempty"`
	AppFontSize     int    `json:"app_font_size,omitempty"`
	ValueEditorFont string `json:"value_editor_font,omitempty"`

	// Scanning
	ScanLimit int `json:"scan_limit"` // key-space scan limit per database

	// Extension server (external data formatters, server_spec.yaml API)
	ExtServerURL string `json:"ext_server_url,omitempty"`

	// Misc
	Locale           string `json:"locale,omitempty"` // "system" or e.g. "zh_CN"
	DarkMode         bool   `json:"dark_mode"`
	UseSystemProxy   bool   `json:"use_system_proxy"`
}

const (
	defaultValueSizeLimit = 150000
	defaultScanLimit      = 10000
)

func Defaults() Settings {
	return Settings{
		ValueSizeLimit: defaultValueSizeLimit,
		ScanLimit:      defaultScanLimit,
		Locale:         "system",
	}
}

type Manager struct {
	path string
	data Settings
}

func NewManager(settingsDir string) (*Manager, error) {
	if err := os.MkdirAll(settingsDir, 0o700); err != nil {
		return nil, err
	}
	m := &Manager{path: filepath.Join(settingsDir, "settings.json"), data: Defaults()}
	if data, err := os.ReadFile(m.path); err == nil {
		_ = json.Unmarshal(data, &m.data) // keep defaults for missing fields
	}
	return m, nil
}

func (m *Manager) Get() Settings { return m.data }

func (m *Manager) Set(s Settings) error {
	m.data = s
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.path)
}
