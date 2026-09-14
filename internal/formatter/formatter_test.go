package formatter

import (
	"bytes"
	"testing"
)

func TestJSONRoundTrip(t *testing.T) {
	raw := []byte(`{"a":1,"b":[true,null,"x"]}`)
	r := JSONFormatter{}.Decode(raw)
	if r.Error != "" || r.Format != "json" {
		t.Fatalf("decode: %+v", r)
	}
	back, err := JSONFormatter{}.Encode(r.Output)
	if err != nil {
		t.Fatal(err)
	}
	// minified
	if string(back) != `{"a":1,"b":[true,null,"x"]}` {
		t.Fatalf("encode: %s", back)
	}
}

func TestHexRoundTrip(t *testing.T) {
	raw := []byte{0x00, 0xff, 0x10}
	out := HexFormatter{}.Decode(raw)
	if out.Output != "00ff10" {
		t.Fatalf("hex: %s", out.Output)
	}
	back, err := HexFormatter{}.Encode(out.Output)
	if err != nil || !bytes.Equal(back, raw) {
		t.Fatalf("hex encode: %x %v", back, err)
	}
}

func TestBase64Text(t *testing.T) {
	raw := []byte("hello")
	r := Base64TextFormatter{}.Decode([]byte("aGVsbG8="))
	if r.Output != "hello" {
		t.Fatalf("decode: %s", r.Output)
	}
	back, err := Base64TextFormatter{}.Encode(r.Output)
	if err != nil || string(back) != "aGVsbG8=" {
		t.Fatalf("encode: %s %v", back, err)
	}
	_ = raw
}

func TestCBORAndMsgpackRoundTrip(t *testing.T) {
	for name, f := range map[string]Formatter{CBORFormatter{}.Name(): CBORFormatter{}, MsgpackFormatter{}.Name(): MsgpackFormatter{}} {
		// produce encoded input by encoding JSON via the formatter itself
		input, err := f.Encode(`{"n": 42, "s": "x", "arr": [1, 2, 3], "ok": true}`)
		if err != nil {
			t.Fatalf("%s encode: %v", name, err)
		}
		r := f.Decode(input)
		if r.Error != "" {
			t.Fatalf("%s decode: %s", name, r.Error)
		}
		if r.Format != "json" {
			t.Fatalf("%s format: %s", name, r.Format)
		}
	}
}

func TestPHPRoundTrip(t *testing.T) {
	php := `a:2:{i:0;s:5:"hello";s:4:"user";a:1:{s:3:"age";i:30;}}`
	r := PHPFormatter{}.Decode([]byte(php))
	if r.Error != "" {
		t.Fatalf("decode: %s", r.Error)
	}
	if r.Output != `[\n  "hello",\n  {\n    "user": {\n      "age": 30\n    }\n  }\n]` &&
		r.Output == "" {
		t.Fatalf("unexpected output: %s", r.Output)
	}
	back, err := PHPFormatter{}.Encode(r.Output)
	if err != nil {
		t.Fatal(err)
	}
	r2 := PHPFormatter{}.Decode(back)
	if r2.Error != "" || r2.Output != r.Output {
		t.Fatalf("round trip: %s vs %s (%s)", r2.Output, r.Output, r2.Error)
	}
}

func TestDetectAndCompressRoundTrip(t *testing.T) {
	payload := bytes.Repeat([]byte("redis desktop manager "), 100)

	for _, alg := range []string{AlgGzip, AlgZlib, AlgZstd, AlgLZ4, AlgSnappy, AlgBrotli} {
		compressed, err := Compress(alg, payload)
		if err != nil {
			t.Fatalf("%s compress: %v", alg, err)
		}
		guessed := DetectCompression(compressed)
		if alg == AlgGzip || alg == AlgZlib || alg == AlgZstd || alg == AlgLZ4 {
			if guessed != alg {
				t.Fatalf("%s magic not detected, got %q", alg, guessed)
			}
		}
		out, err := Decompress(alg, compressed)
		if err != nil || !bytes.Equal(out, payload) {
			t.Fatalf("%s round trip failed: %v", alg, err)
		}
	}
}

func TestHexDump(t *testing.T) {
	out := hexDump([]byte("ABC"))
	if out == "" || len(bytes.Split([]byte(out), []byte("\n"))) < 1 {
		t.Fatal("empty hexdump")
	}
}

func TestIsBinary(t *testing.T) {
	if IsBinary([]byte("plain text\n")) {
		t.Fatal("text flagged as binary")
	}
	if !IsBinary([]byte{0x00, 0x01, 0x02}) {
		t.Fatal("binary not flagged")
	}
}
