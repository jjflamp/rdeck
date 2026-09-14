// Extension Server REST client — wire-compatible with RESP.app's
// data-formatters API (docs/server_spec.yaml):
//
//	GET  /data-formatters              → [{id, name, read-only}]
//	POST /data-formatters/{id}/decode  {data: base64}   → text output
//	POST /data-formatters/{id}/encode  {data: base64}   → raw bytes
package formatter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ExtClient struct {
	BaseURL string
	client  *http.Client
}

func NewExtClient(baseURL string) *ExtClient {
	return &ExtClient{
		BaseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 5 * time.Second},
	}
}

// ExtFormatterInfo mirrors the spec's DataFormatter schema.
type ExtFormatterInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ReadOnly bool   `json:"read-only"`
}

func (c *ExtClient) List(ctx context.Context) ([]ExtFormatterInfo, error) {
	var out []ExtFormatterInfo
	if err := c.do(ctx, http.MethodGet, "/data-formatters", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Decode sends raw value bytes; the response body is the display text.
func (c *ExtClient) Decode(ctx context.Context, id string, data []byte) (Result, error) {
	var out string
	err := c.do(ctx, http.MethodPost, "/data-formatters/"+id+"/decode",
		map[string]string{"data": base64.StdEncoding.EncodeToString(data)}, &out)
	if err != nil {
		return Result{}, err
	}
	return Result{Output: out, Format: "plain_text"}, nil
}

// Encode sends edited text; the response body is the raw value bytes.
func (c *ExtClient) Encode(ctx context.Context, id, input string) ([]byte, error) {
	var out []byte
	err := c.do(ctx, http.MethodPost, "/data-formatters/"+id+"/encode",
		map[string]string{"data": base64.StdEncoding.EncodeToString([]byte(input))}, &out)
	return out, err
}

func (c *ExtClient) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("extension server: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		// spec: 400 body {error: string}
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return fmt.Errorf("extension server: %s", e.Error)
		}
		return fmt.Errorf("extension server: HTTP %d", resp.StatusCode)
	}
	switch o := out.(type) {
	case *string:
		*o = string(data)
	case *[]byte:
		*o = data
	default:
		if len(data) > 0 && out != nil {
			if err := json.Unmarshal(data, out); err != nil {
				return err
			}
		}
	}
	return nil
}
