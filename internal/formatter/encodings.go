package formatter

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/fxamacker/cbor/v2"
	"github.com/vmihailenco/msgpack/v5"
)

// CBORFormatter — decode CBOR to pretty JSON, encode JSON back to CBOR.
type CBORFormatter struct{}

func (CBORFormatter) Name() string        { return "cbor" }
func (CBORFormatter) Description() string { return "CBOR ↔ JSON" }
func (CBORFormatter) Decode(data []byte) Result {
	var v interface{}
	if err := cbor.Unmarshal(data, &v); err != nil {
		return Result{Error: "invalid CBOR: " + err.Error()}
	}
	out, err := json.MarshalIndent(normalizeForJSON(v), "", "  ")
	if err != nil {
		return Result{Error: err.Error()}
	}
	return Result{Output: string(out), Format: "json"}
}

// normalizeForJSON converts CBOR-style map[interface{}]interface{} values
// into JSON-serializable shapes.
func normalizeForJSON(v interface{}) interface{} {
	switch t := v.(type) {
	case map[interface{}]interface{}:
		m := make(map[string]interface{}, len(t))
		for k, val := range t {
			m[fmt.Sprint(k)] = normalizeForJSON(val)
		}
		return m
	case []interface{}:
		for i := range t {
			t[i] = normalizeForJSON(t[i])
		}
		return t
	default:
		return v
	}
}
func (CBORFormatter) Encode(input string) ([]byte, error) {
	var v interface{}
	if err := json.Unmarshal([]byte(input), &v); err != nil {
		return nil, err
	}
	return cbor.Marshal(v)
}

// MsgpackFormatter — decode msgpack to pretty JSON, encode JSON back.
type MsgpackFormatter struct{}

func (MsgpackFormatter) Name() string        { return "msgpack" }
func (MsgpackFormatter) Description() string { return "MsgPack ↔ JSON" }
func (MsgpackFormatter) Decode(data []byte) Result {
	var v interface{}
	if err := msgpack.Unmarshal(data, &v); err != nil {
		return Result{Error: "invalid MsgPack: " + err.Error()}
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return Result{Error: err.Error()}
	}
	return Result{Output: string(out), Format: "json"}
}
func (MsgpackFormatter) Encode(input string) ([]byte, error) {
	var v interface{}
	if err := json.Unmarshal([]byte(input), &v); err != nil {
		return nil, err
	}
	return msgpack.Marshal(v)
}

// PHPFormatter — PHP serialize() ↔ JSON (covers string/int/float/bool/nil/
// arrays/objects; enough for phpserialize.py's common cases).
type PHPFormatter struct{}

func (PHPFormatter) Name() string        { return "php" }
func (PHPFormatter) Description() string { return "PHP serialize ↔ JSON" }

func (PHPFormatter) Decode(data []byte) Result {
	v, rest, err := phpUnserialize(string(data))
	if err != nil {
		return Result{Error: "invalid PHP serialize: " + err.Error()}
	}
	_ = rest
	out, err := json.MarshalIndent(phpToNative(v), "", "  ")
	if err != nil {
		return Result{Error: err.Error()}
	}
	return Result{Output: string(out), Format: "json"}
}

func (PHPFormatter) Encode(input string) ([]byte, error) {
	var v interface{}
	if err := json.Unmarshal([]byte(input), &v); err != nil {
		return nil, err
	}
	return []byte(phpSerialize(nativeToPHP(v))), nil
}

// --- php serialize parser -----------------------------------------------------

func phpUnserialize(s string) (interface{}, string, error) {
	read := func(n int) (string, string, error) {
		if len(s) < n {
			return "", "", fmt.Errorf("unexpected end of data")
		}
		return s[:n], s[n:], nil
	}
	expect := func(prefix string) (string, error) {
		if !strings.HasPrefix(s, prefix) {
			return "", fmt.Errorf("expected %q", prefix)
		}
		return s[len(prefix):], nil
	}

	switch {
	case strings.HasPrefix(s, "s:"):
		rest, err := expect("s:")
		if err != nil {
			return nil, s, err
		}
		rest, s = splitOnce(rest, ":")
		n, err := strconv.Atoi(rest)
		if err != nil {
			return nil, s, err
		}
		s, err = expect(`"`)
		if err != nil {
			return nil, s, err
		}
		val, rest, err := read(n)
		if err != nil {
			return nil, s, err
		}
		s = rest
		s, err = expect(`";`)
		if err != nil {
			return nil, s, err
		}
		return val, s, nil

	case strings.HasPrefix(s, "i:"):
		rest, _ := expect("i:")
		num, rest2, _ := splitOnce2(rest, ";")
		n, err := strconv.Atoi(num)
		if err != nil {
			return nil, rest, err
		}
		return n, rest2, nil

	case strings.HasPrefix(s, "d:"):
		rest, _ := expect("d:")
		num, rest2, _ := splitOnce2(rest, ";")
		f, err := strconv.ParseFloat(num, 64)
		if err != nil {
			if num == "INF" {
				f = math.Inf(1)
			} else if num == "-INF" {
				f = math.Inf(-1)
			} else {
				return nil, rest, err
			}
		}
		return f, rest2, nil

	case strings.HasPrefix(s, "b:"):
		rest, _ := expect("b:")
		v, rest2, _ := splitOnce2(rest, ";")
		return v == "1", rest2, nil

	case strings.HasPrefix(s, "N;"):
		return nil, s[2:], nil

	case strings.HasPrefix(s, "a:"):
		rest, _ := expect("a:")
		countStr, body, _ := splitOnce2(rest, ":{")
		n, err := strconv.Atoi(countStr)
		if err != nil {
			return nil, body, err
		}
		s = body // advance past "a:N:{" — parses below read from here
		m := make(map[string]interface{}, n)
		for i := 0; i < n; i++ {
			k, rest2, err := phpUnserialize(s)
			if err != nil {
				return nil, s, err
			}
			v, rest3, err := phpUnserialize(rest2)
			if err != nil {
				return nil, rest2, err
			}
			ks, ok := k.(string)
			if !ok {
				ks = fmt.Sprint(k)
			}
			m[ks] = v
			s = rest3
		}
		if !strings.HasPrefix(s, "}") {
			return nil, s, fmt.Errorf("expected } after array")
		}
		return m, s[1:], nil

	default:
		return nil, s, fmt.Errorf("unsupported php type tag")
	}
}

func splitOnce(s, sep string) (string, string) { a, b, _ := strings.Cut(s, sep); return a, b }
func splitOnce2(s, sep string) (string, string, bool) {
	a, b, ok := strings.Cut(s, sep)
	return a, b, ok
}

func phpToNative(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		// Arrays with sequential 0..n integer keys become JSON arrays.
		seq := true
		for i := 0; i < len(t); i++ {
			if _, ok := t[strconv.Itoa(i)]; !ok {
				seq = false
				break
			}
		}
		if seq {
			arr := make([]interface{}, len(t))
			for i := 0; i < len(t); i++ {
				arr[i] = phpToNative(t[strconv.Itoa(i)])
			}
			return arr
		}
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			out[k] = phpToNative(val)
		}
		return out
	default:
		return v
	}
}

func nativeToPHP(v interface{}) interface{} {
	switch t := v.(type) {
	case []interface{}:
		m := make(map[string]interface{}, len(t))
		for i, item := range t {
			m[strconv.Itoa(i)] = nativeToPHP(item)
		}
		return m
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			out[k] = nativeToPHP(val)
		}
		return out
	default:
		return v
	}
}

// phpSerialize implements PHP serialize() for basic types.
func phpSerialize(v interface{}) string {
	var sb strings.Builder
	phpSerializeTo(&sb, v)
	return sb.String()
}

func phpSerializeTo(sb *strings.Builder, v interface{}) {
	switch t := v.(type) {
	case nil:
		sb.WriteString("N;")
	case bool:
		if t {
			sb.WriteString("b:1;")
		} else {
			sb.WriteString("b:0;")
		}
	case int:
		fmt.Fprintf(sb, "i:%d;", t)
	case int64:
		fmt.Fprintf(sb, "i:%d;", t)
	case float64:
		if t == math.Trunc(t) && math.Abs(t) < 1e15 {
			fmt.Fprintf(sb, "d:%d;", int64(t))
		} else if math.IsInf(t, 1) {
			sb.WriteString("d:INF;")
		} else if math.IsInf(t, -1) {
			sb.WriteString("d:-INF;")
		} else {
			fmt.Fprintf(sb, "d:%s;", strconv.FormatFloat(t, 'G', 17, 64))
		}
	case string:
		fmt.Fprintf(sb, "s:%d:\"%s\";", len(t), t)
	case []byte:
		fmt.Fprintf(sb, "s:%d:\"%s\";", len(t), t)
	case []interface{}:
		fmt.Fprintf(sb, "a:%d:{", len(t))
		for i, item := range t {
			phpSerializeTo(sb, i)
			phpSerializeTo(sb, item)
		}
		sb.WriteString("}")
	case map[string]interface{}:
		fmt.Fprintf(sb, "a:%d:{", len(t))
		for k, item := range t {
			phpSerializeTo(sb, k)
			phpSerializeTo(sb, item)
		}
		sb.WriteString("}")
	default:
		fmt.Fprintf(sb, "s:%d:\"%v\";", len(fmt.Sprint(t)), t)
	}
}
