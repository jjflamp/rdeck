// Package formatter replaces RESP.app's three formatter layers (QML built-ins
// + embedded Python + Extension Server) with pure-Go implementations.
// Result mirrors the original base.py protocol [error, output, read_only,
// decode_format].
package formatter

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Result of a decode operation.
type Result struct {
	Output   string `json:"output"`
	Format   string `json:"format"`             // plain_text | json | hex | binary
	ReadOnly bool   `json:"read_only"`
	Error    string `json:"error,omitempty"`
}

// Formatter decodes raw redis values to displayable text and encodes back.
type Formatter interface {
	Name() string
	Description() string
	Decode(data []byte) Result
	Encode(input string) ([]byte, error)
}

// Registry returns all built-in formatters in display order.
func Registry() []Formatter {
	return []Formatter{
		HexFormatter{},
		HexDumpFormatter{},
		JSONFormatter{},
		Base64TextFormatter{},
		Base64JSONFormatter{},
		BinaryFormatter{},
		CBORFormatter{},
		MsgpackFormatter{},
		PHPFormatter{},
		PickleFormatter{},
	}
}

// Get returns a formatter by name (case-insensitive).
func Get(name string) (Formatter, error) {
	for _, f := range Registry() {
		if strings.EqualFold(f.Name(), name) {
			return f, nil
		}
	}
	return nil, fmt.Errorf("unknown formatter %q", name)
}

// IsBinary reports whether data likely contains non-text bytes (mirrors
// qmlUtils::isBinaryString).
func IsBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	if !utf8.Valid(data) {
		return true
	}
	// control chars other than \t \n \r
	for _, b := range data {
		if b < 0x20 && b != '\t' && b != '\n' && b != '\r' {
			return true
		}
	}
	return false
}

// --- built-ins ---------------------------------------------------------------

// HexFormatter — editable hex (original "HEX").
type HexFormatter struct{}

func (HexFormatter) Name() string        { return "hex" }
func (HexFormatter) Description() string { return "HEX 编码" }
func (HexFormatter) Decode(data []byte) Result {
	return Result{Output: strings.ToLower(hex.EncodeToString(data)), Format: "hex"}
}
func (HexFormatter) Encode(input string) ([]byte, error) {
	return hex.DecodeString(strings.TrimSpace(input))
}

// HexDumpFormatter — read-only hex table (original "HEX TABLE" / hexy.js).
type HexDumpFormatter struct{}

func (HexDumpFormatter) Name() string        { return "hexdump" }
func (HexDumpFormatter) Description() string { return "HEX TABLE（只读）" }
func (HexDumpFormatter) Decode(data []byte) Result {
	return Result{Output: hexDump(data), Format: "plain_text", ReadOnly: true}
}
func (HexDumpFormatter) Encode(string) ([]byte, error) {
	return nil, fmt.Errorf("hexdump is read-only")
}

// JSONFormatter — pretty-print valid JSON; minify on save.
type JSONFormatter struct{}

func (JSONFormatter) Name() string        { return "json" }
func (JSONFormatter) Description() string { return "JSON" }
func (JSONFormatter) Decode(data []byte) Result {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return Result{Error: "not valid JSON: " + err.Error()}
	}
	pretty, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return Result{Error: err.Error()}
	}
	return Result{Output: string(pretty), Format: "json"}
}
func (JSONFormatter) Encode(input string) ([]byte, error) {
	var v interface{}
	if err := json.Unmarshal([]byte(input), &v); err != nil {
		return nil, err
	}
	return json.Marshal(v) // minified, like the original minifyJSON
}

// Base64TextFormatter — decode base64 payload as UTF-8 text.
type Base64TextFormatter struct{}

func (Base64TextFormatter) Name() string        { return "base64-text" }
func (Base64TextFormatter) Description() string { return "BASE64 → Text" }
func (Base64TextFormatter) Decode(data []byte) Result {
	raw, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return Result{Error: "invalid base64: " + err.Error()}
	}
	return Result{Output: string(raw), Format: "plain_text"}
}
func (Base64TextFormatter) Encode(input string) ([]byte, error) {
	return []byte(base64.StdEncoding.EncodeToString([]byte(input))), nil
}

// Base64JSONFormatter — decode base64 payload as JSON.
type Base64JSONFormatter struct{}

func (Base64JSONFormatter) Name() string        { return "base64-json" }
func (Base64JSONFormatter) Description() string { return "BASE64 → JSON" }
func (Base64JSONFormatter) Decode(data []byte) Result {
	raw, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return Result{Error: "invalid base64: " + err.Error()}
	}
	return JSONFormatter{}.Decode(raw)
}
func (Base64JSONFormatter) Encode(input string) ([]byte, error) {
	minified, err := JSONFormatter{}.Encode(input)
	if err != nil {
		return nil, err
	}
	return []byte(base64.StdEncoding.EncodeToString(minified)), nil
}

// BinaryFormatter — bit string (original binary.py, read-only).
type BinaryFormatter struct{}

func (BinaryFormatter) Name() string        { return "binary" }
func (BinaryFormatter) Description() string { return "二进制位串（只读）" }
func (BinaryFormatter) Decode(data []byte) Result {
	var sb strings.Builder
	for _, b := range data {
		sb.WriteString(fmt.Sprintf("%08b", b))
	}
	return Result{Output: sb.String(), Format: "binary", ReadOnly: true}
}
func (BinaryFormatter) Encode(string) ([]byte, error) {
	return nil, fmt.Errorf("binary is read-only")
}

// hexDump renders classic hexdump: offset, hex bytes, printable ASCII.
func hexDump(data []byte) string {
	var sb strings.Builder
	for off := 0; off < len(data); off += 16 {
		end := min(off+16, len(data))
		chunk := data[off:end]

		fmt.Fprintf(&sb, "%08x  ", off)
		for i := 0; i < 16; i++ {
			if i < len(chunk) {
				fmt.Fprintf(&sb, "%02x ", chunk[i])
			} else {
				sb.WriteString("   ")
			}
			if i == 7 {
				sb.WriteByte(' ')
			}
		}
		sb.WriteString(" |")
		for _, b := range chunk {
			if b >= 0x20 && b < 0x7f {
				sb.WriteByte(b)
			} else {
				sb.WriteByte('.')
			}
		}
		sb.WriteString("|\n")
	}
	return sb.String()
}
