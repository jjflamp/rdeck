package formatter

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// PickleFormatter decodes Python pickle protocols 0-2 basic types (read-only,
// the original pickle.py was read-only too). Opcodes that need runtime
// support (REDUCE/BUILD/NEWOBJ for custom classes) return an error and the
// UI degrades to hex — the documented P6 scope (plan §2.5-2).
type PickleFormatter struct{}

func (PickleFormatter) Name() string        { return "pickle" }
func (PickleFormatter) Description() string { return "Python pickle（协议≤2，只读）" }

func (PickleFormatter) Decode(data []byte) Result {
	v, err := pickleUnmarshal(data)
	if err != nil {
		return Result{Error: "pickle: " + err.Error()}
	}
	out, jerr := json.MarshalIndent(pickleNormalize(v), "", "  ")
	if jerr != nil {
		return Result{Error: "pickle render: " + jerr.Error()}
	}
	return Result{Output: string(out), Format: "json", ReadOnly: true}
}

func (PickleFormatter) Encode(string) ([]byte, error) {
	return nil, fmt.Errorf("pickle is read-only")
}

type pickleStack struct {
	items []interface{}
	marks []int // stack positions of MARK
	memo  map[int]interface{}
}

func (s *pickleStack) push(v interface{}) { s.items = append(s.items, v) }

func (s *pickleStack) pop() (interface{}, error) {
	if len(s.items) == 0 {
		return nil, fmt.Errorf("stack underflow")
	}
	v := s.items[len(s.items)-1]
	s.items = s.items[:len(s.items)-1]
	return v, nil
}

func (s *pickleStack) markPos() int {
	if len(s.marks) == 0 {
		return 0
	}
	return s.marks[len(s.marks)-1]
}

// popSince pops everything above the last MARK as a slice.
func (s *pickleStack) popSince() []interface{} {
	pos := s.markPos()
	items := append([]interface{}{}, s.items[pos:]...)
	s.items = s.items[:pos]
	if len(s.marks) > 0 {
		s.marks = s.marks[:len(s.marks)-1]
	}
	return items
}

func pickleUnmarshal(data []byte) (interface{}, error) {
	st := &pickleStack{memo: map[int]interface{}{}}
	r := strings.NewReader(string(data))

	readN := func(n int) ([]byte, error) {
		b := make([]byte, n)
		_, err := r.Read(b)
		if err != nil {
			return nil, fmt.Errorf("unexpected end of data")
		}
		return b, nil
	}

	for {
		op, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("missing STOP")
		}
		switch op {
		case 0x80: // PROTO
			if _, err := r.ReadByte(); err != nil {
				return nil, err
			}
		case 0x95: // FRAME
			if _, err := readN(8); err != nil {
				return nil, err
			}
		case '.': // STOP
			if len(st.items) != 1 {
				return nil, fmt.Errorf("stack has %d items at STOP", len(st.items))
			}
			return st.items[0], nil

		case '(': // MARK
			st.marks = append(st.marks, len(st.items))
		case 'N': // NONE
			st.push(nil)
		case 'O', 0x88: // NEWTRUE
			st.push(true)
		case 'o', 0x89: // NEWFALSE
			st.push(false)
		case 'I': // INT
			line, err := readLine(r)
			if err != nil {
				return nil, err
			}
			if line == "01" {
				st.push(true)
				break
			}
			if line == "00" {
				st.push(false)
				break
			}
			var n int
			if _, err := fmt.Sscanf(line, "%d", &n); err != nil {
				return nil, fmt.Errorf("bad INT %q", line)
			}
			st.push(n)
		case 'L': // LONG (decimal + "L")
			line, err := readLine(r)
			if err != nil {
				return nil, err
			}
			var n int64
			fmt.Sscanf(strings.TrimSuffix(line, "L"), "%d", &n)
			st.push(n)
		case 'J': // BININT
			b, err := readN(4)
			if err != nil {
				return nil, err
			}
			st.push(int(int32(binary.LittleEndian.Uint32(b))))
		case 'K': // BININT1
			b, err := readN(1)
			if err != nil {
				return nil, err
			}
			st.push(int(b[0]))
		case 'M': // BININT2
			b, err := readN(2)
			if err != nil {
				return nil, err
			}
			st.push(int(binary.LittleEndian.Uint16(b)))
		case 'F': // FLOAT (ascii)
			line, err := readLine(r)
			if err != nil {
				return nil, err
			}
			var f float64
			fmt.Sscanf(line, "%g", &f)
			st.push(f)
		case 'G': // BINFLOAT
			b, err := readN(8)
			if err != nil {
				return nil, err
			}
			st.push(math.Float64frombits(binary.BigEndian.Uint64(b)))
		case 'S': // STRING (quoted, with python escapes)
			line, err := readLine(r)
			if err != nil {
				return nil, err
			}
			s, err := unquotePickleString(line)
			if err != nil {
				return nil, err
			}
			st.push(s)
		case 'T': // BINSTRING (4-byte len)
			b, err := readN(4)
			if err != nil {
				return nil, err
			}
			s, err := readN(int(binary.LittleEndian.Uint32(b)))
			if err != nil {
				return nil, err
			}
			st.push(string(s))
		case 'U': // SHORT_BINSTRING (1-byte len)
			n, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			s, err := readN(int(n))
			if err != nil {
				return nil, err
			}
			st.push(string(s))
		case 'V': // UNICODE (raw-unicode-escape line)
			line, err := readLine(r)
			if err != nil {
				return nil, err
			}
			st.push(line)
		case 'X': // BINUNICODE (4-byte len)
			b, err := readN(4)
			if err != nil {
				return nil, err
			}
			s, err := readN(int(binary.LittleEndian.Uint32(b)))
			if err != nil {
				return nil, err
			}
			st.push(string(s))
		case ')': // EMPTY_TUPLE
			st.push([]interface{}{})
		case 0x85: // TUPLE1
			v, err := st.pop()
			if err != nil {
				return nil, err
			}
			st.push([]interface{}{v})
		case 0x86: // TUPLE2
			b, err := st.pop()
			if err != nil {
				return nil, err
			}
			a, err := st.pop()
			if err != nil {
				return nil, err
			}
			st.push([]interface{}{a, b})
		case 0x87: // TUPLE3
			c, err := st.pop()
			if err != nil {
				return nil, err
			}
			b, err := st.pop()
			if err != nil {
				return nil, err
			}
			a, err := st.pop()
			if err != nil {
				return nil, err
			}
			st.push([]interface{}{a, b, c})
		case 't': // TUPLE
			st.push(st.popSince())
		case 'l': // LIST
			st.push([]interface{}{})
		case ']': // EMPTY_LIST
			st.push([]interface{}{})
		case 'a': // APPEND
			v, err := st.pop()
			if err != nil {
				return nil, err
			}
			lst, err := st.pop()
			if err != nil {
				return nil, err
			}
			list, ok := lst.([]interface{})
			if !ok {
				return nil, fmt.Errorf("APPEND on non-list")
			}
			st.push(append(list, v))
		case 'e': // APPENDS
			items := st.popSince()
			lst, err := st.pop()
			if err != nil {
				return nil, err
			}
			list, ok := lst.([]interface{})
			if !ok {
				return nil, fmt.Errorf("APPENDS on non-list")
			}
			st.push(append(list, items...))
		case '}': // EMPTY_DICT
			st.push(map[string]interface{}{})
		case 'd': // DICT
			st.push(map[string]interface{}{})
		case 's': // SETITEM
			v, err := st.pop()
			if err != nil {
				return nil, err
			}
			k, err := st.pop()
			if err != nil {
				return nil, err
			}
			d, err := st.pop()
			if err != nil {
				return nil, err
			}
			dict, ok := d.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("SETITEM on non-dict")
			}
			dict[fmt.Sprint(k)] = v
			st.push(dict)
		case 'u': // SETITEMS
			items := st.popSince()
			d, err := st.pop()
			if err != nil {
				return nil, err
			}
			dict, ok := d.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("SETITEMS on non-dict")
			}
			for i := 0; i+1 < len(items); i += 2 {
				dict[fmt.Sprint(items[i])] = items[i+1]
			}
			st.push(dict)
		case 'p': // PUT
			line, err := readLine(r)
			if err != nil {
				return nil, err
			}
			var idx int
			fmt.Sscanf(line, "%d", &idx)
			if len(st.items) > 0 {
				st.memo[idx] = st.items[len(st.items)-1]
			}
		case 'q': // BINPUT
			b, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			if len(st.items) > 0 {
				st.memo[int(b)] = st.items[len(st.items)-1]
			}
		case 'r': // MEMOIZE
			if len(st.items) == 0 {
				return nil, fmt.Errorf("MEMOIZE on empty stack")
			}
			st.memo[len(st.memo)] = st.items[len(st.items)-1]
		case 'g': // GET
			line, err := readLine(r)
			if err != nil {
				return nil, err
			}
			var idx int
			fmt.Sscanf(line, "%d", &idx)
			v, ok := st.memo[idx]
			if !ok {
				return nil, fmt.Errorf("memo %d not found", idx)
			}
			st.push(v)
		case 'h': // BINGET
			b, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			v, ok := st.memo[int(b)]
			if !ok {
				return nil, fmt.Errorf("memo %d not found", b)
			}
			st.push(v)
		case 'j': // LONG_BINGET
			b, err := readN(4)
			if err != nil {
				return nil, err
			}
			idx := int(binary.LittleEndian.Uint32(b))
			v, ok := st.memo[idx]
			if !ok {
				return nil, fmt.Errorf("memo %d not found", idx)
			}
			st.push(v)

		case 'R', 'b': // REDUCE / BUILD — need class support
			return nil, fmt.Errorf("unsupported opcode %q (class instance)", string(rune(op)))

		default:
			return nil, fmt.Errorf("unsupported opcode 0x%02x", op)
		}
	}
}

func readLine(r *strings.Reader) (string, error) {
	var sb strings.Builder
	for {
		b, err := r.ReadByte()
		if err != nil {
			if sb.Len() > 0 {
				break
			}
			return "", fmt.Errorf("unexpected end of data")
		}
		if b == '\n' {
			break
		}
		sb.WriteByte(b)
	}
	return strings.TrimRight(sb.String(), "\r"), nil
}

// unquotePickleString decodes python repr-style 'xxx' with \xNN escapes.
func unquotePickleString(s string) (string, error) {
	if len(s) < 2 || s[0] != '\'' || s[len(s)-1] != '\'' {
		return s, nil
	}
	body := s[1 : len(s)-1]
	var sb strings.Builder
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c != '\\' {
			sb.WriteByte(c)
			continue
		}
		i++
		if i >= len(body) {
			break
		}
		switch body[i] {
		case 'x':
			if i+2 < len(body) {
				var v int
				fmt.Sscanf(body[i+1:i+3], "%02x", &v)
				sb.WriteByte(byte(v))
				i += 2
			}
		case 'n':
			sb.WriteByte('\n')
		case 't':
			sb.WriteByte('\t')
		case 'r':
			sb.WriteByte('\r')
		case '\\':
			sb.WriteByte('\\')
		case '\'':
			sb.WriteByte('\'')
		default:
			sb.WriteByte(body[i])
		}
	}
	return sb.String(), nil
}

// pickleNormalize converts tuples/lists for JSON output (dict keys stay strings).
func pickleNormalize(v interface{}) interface{} {
	switch t := v.(type) {
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, item := range t {
			out[i] = pickleNormalize(item)
		}
		return out
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			out[k] = pickleNormalize(val)
		}
		return out
	default:
		return v
	}
}
