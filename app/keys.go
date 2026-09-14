// Value-editor bindings: OpenKey / pagination / row editing / formatters /
// compression. All value payloads cross the IPC boundary base64-encoded
// (binary-safe transport rule, plan §3.1-①).
package app

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"rdeck/internal/formatter"
	"rdeck/internal/keymodel"
	"rdeck/internal/redisclient"
)

func (a *App) clientFor(connID string, db uint) (keymodel.Client, error) {
	conn, err := a.conns.Get(connID)
	if err != nil {
		return nil, err
	}
	cl, err := conn.Client(db)
	if err != nil {
		return nil, err
	}
	return cl.(keymodel.Client), nil
}

// OpenKey returns the key header (type/ttl/rows/size).
func (a *App) OpenKey(connID string, db uint, key string, keyType string) (*keymodel.KeyMeta, error) {
	cl, err := a.clientFor(connID, db)
	if err != nil {
		return nil, err
	}
	loader, err := keymodel.NewLoader(keyType, key, int64(a.settings.Get().ValueSizeLimit))
	if err != nil {
		return nil, err
	}
	return loader.Meta(a.ctx, cl)
}

// LoadKeyRows fetches one page of rows.
func (a *App) LoadKeyRows(connID string, db uint, key, keyType, cursor, match string, limit int64) (*keymodel.RowsPage, error) {
	cl, err := a.clientFor(connID, db)
	if err != nil {
		return nil, err
	}
	loader, err := keymodel.NewLoader(keyType, key, int64(a.settings.Get().ValueSizeLimit))
	if err != nil {
		return nil, err
	}
	return loader.LoadPage(a.ctx, cl, cursor, match, limit)
}

// EditRow applies one row mutation (see keymodel.EditRequest for semantics).
func (a *App) EditRow(connID string, db uint, key string, req keymodel.EditRequest) error {
	cl, err := a.clientFor(connID, db)
	if err != nil {
		return err
	}
	return keymodel.EditRow(a.ctx, cl, key, req)
}

// SetKeyTTL updates expiry (ttl <= 0 removes it).
func (a *App) SetKeyTTL(connID string, db uint, key string, ttl int64) error {
	cl, err := a.clientFor(connID, db)
	if err != nil {
		return err
	}
	return keymodel.SetTTL(a.ctx, cl, key, ttl)
}

// RenameKey renames a key within its db.
func (a *App) RenameKey(connID string, db uint, from, to string) error {
	cl, err := a.clientFor(connID, db)
	if err != nil {
		return err
	}
	return keymodel.RenameKey(a.ctx, cl, from, to)
}

// DeleteKey removes a key.
func (a *App) DeleteKey(connID string, db uint, key string) error {
	cl, err := a.clientFor(connID, db)
	if err != nil {
		return err
	}
	return keymodel.DeleteKey(a.ctx, cl, key)
}

// CreateKey writes a new string key (other types are created implicitly by
// adding their first row to a non-existent key — HSET/SADD/... semantics).
func (a *App) CreateKey(connID string, db uint, key, valueB64 string, ttl int64) error {
	cl, err := a.clientFor(connID, db)
	if err != nil {
		return err
	}
	if err := keymodel.SetString(a.ctx, cl, key, valueB64); err != nil {
		return err
	}
	if ttl > 0 {
		return keymodel.SetTTL(a.ctx, cl, key, ttl)
	}
	return nil
}

// SaveStringValue writes back the (full) string value.
func (a *App) SaveStringValue(connID string, db uint, key, valueB64 string) error {
	cl, err := a.clientFor(connID, db)
	if err != nil {
		return err
	}
	return keymodel.SetString(a.ctx, cl, key, valueB64)
}

// GetFullValue loads a truncated string value in full (the "load complete
// value" button for keys exceeding valueSizeLimit).
func (a *App) GetFullValue(connID string, db uint, key string) (string, error) {
	cl, err := a.clientFor(connID, db)
	if err != nil {
		return "", err
	}
	raw, err := cl.Get(a.ctx, key).Bytes()
	if err != nil {
		return "", err
	}
	return encodeB64(raw), nil
}

// --- Formatters -------------------------------------------------------------

type FormatterInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ReadOnly    bool   `json:"read_only"`
}

// ListFormatters exposes the registry plus any Extension Server formatters
// (prefixed "ext:<id>") for the editor dropdown.
func (a *App) ListFormatters() []FormatterInfo {
	var out []FormatterInfo
	for _, f := range formatter.Registry() {
		out = append(out, FormatterInfo{Name: f.Name(), Description: f.Description()})
	}
	if url := a.settings.Get().ExtServerURL; url != "" {
		ctx, cancel := context.WithTimeout(a.ctx, 3*time.Second)
		defer cancel()
		if list, err := formatter.NewExtClient(url).List(ctx); err == nil {
			for _, ef := range list {
				out = append(out, FormatterInfo{
					Name:        "ext:" + ef.ID,
					Description: ef.Name + "（扩展）",
				})
			}
		}
	}
	return out
}

// DecodeValue runs a formatter over raw value bytes (b64 in/out for data).
func (a *App) DecodeValue(formatterName string, dataB64 string) formatter.Result {
	raw, err := decodeB64(dataB64)
	if err != nil {
		return formatter.Result{Error: err.Error()}
	}
	if id, ok := strings.CutPrefix(formatterName, "ext:"); ok {
		url := a.settings.Get().ExtServerURL
		if url == "" {
			return formatter.Result{Error: "extension server not configured"}
		}
		ctx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
		defer cancel()
		r, err := formatter.NewExtClient(url).Decode(ctx, id, raw)
		if err != nil {
			return formatter.Result{Error: err.Error()}
		}
		return r
	}
	f, err := formatter.Get(formatterName)
	if err != nil {
		return formatter.Result{Error: err.Error()}
	}
	return f.Decode(raw)
}

// EncodeValue converts edited text back to raw bytes (b64).
func (a *App) EncodeValue(formatterName string, input string) (string, error) {
	if id, ok := strings.CutPrefix(formatterName, "ext:"); ok {
		url := a.settings.Get().ExtServerURL
		if url == "" {
			return "", fmt.Errorf("extension server not configured")
		}
		ctx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
		defer cancel()
		raw, err := formatter.NewExtClient(url).Encode(ctx, id, input)
		if err != nil {
			return "", err
		}
		return encodeB64(raw), nil
	}
	f, err := formatter.Get(formatterName)
	if err != nil {
		return "", err
	}
	raw, err := f.Encode(input)
	if err != nil {
		return "", err
	}
	return encodeB64(raw), nil
}

// IsBinaryValue reports whether the payload looks non-textual.
func (a *App) IsBinaryValue(dataB64 string) (bool, error) {
	raw, err := decodeB64(dataB64)
	if err != nil {
		return false, err
	}
	return formatter.IsBinary(raw), nil
}

// --- Compression ------------------------------------------------------------

// DetectCompression returns the guessed algorithm ("" = none detected).
func (a *App) DetectCompression(dataB64 string) (string, error) {
	raw, err := decodeB64(dataB64)
	if err != nil {
		return "", err
	}
	return formatter.DetectCompression(raw), nil
}

// DecompressValue decompresses with the given (or detected) algorithm.
func (a *App) DecompressValue(alg string, dataB64 string) (string, error) {
	raw, err := decodeB64(dataB64)
	if err != nil {
		return "", err
	}
	if alg == "" {
		if alg = formatter.DetectCompression(raw); alg == "" {
			return "", fmt.Errorf("cannot detect compression (brotli/lz4_raw have no magic — pick manually)")
		}
	}
	out, err := formatter.Decompress(alg, raw)
	if err != nil {
		return "", err
	}
	return encodeB64(out), nil
}

// CompressValue re-compresses edited content before save.
func (a *App) CompressValue(alg string, dataB64 string) (string, error) {
	raw, err := decodeB64(dataB64)
	if err != nil {
		return "", err
	}
	out, err := formatter.Compress(alg, raw)
	if err != nil {
		return "", err
	}
	return encodeB64(out), nil
}

// ListCompressionAlgs for the manual picker (no-magic algorithms included).
func (a *App) ListCompressionAlgs() []string {
	return []string{
		"", formatter.AlgGzip, formatter.AlgZlib, formatter.AlgLZ4,
		formatter.AlgLZ4Raw, formatter.AlgZstd, formatter.AlgSnappy,
		formatter.AlgSnappyFramed, formatter.AlgBrotli,
		formatter.AlgMagentoSessionGzip, formatter.AlgMagentoSessionLZ4,
		formatter.AlgMagentoSessionSnappy, formatter.AlgMagentoCacheGzip,
		formatter.AlgMagentoCacheLZ4, formatter.AlgMagentoCacheZstd,
		formatter.AlgMagentoCacheSnappy,
	}
}

// ProbeExtServer validates an extension server URL by listing formatters
// (used by the settings dialog before saving).
func (a *App) ProbeExtServer(url string) ([]formatter.ExtFormatterInfo, error) {
	if url == "" {
		return nil, fmt.Errorf("URL 为空")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 3*time.Second)
	defer cancel()
	return formatter.NewExtClient(url).List(ctx)
}

// --- b64 helpers (single choke point for the transport rule) -----------------

func decodeB64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

func encodeB64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

var _ = redisclient.ModeStandalone // keep import stable for future use
