package formatter

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

// python3 -c "import pickle; print(pickle.dumps({'a':1,'b':['x',(2,3)],'c':True}, protocol=2).hex())"
func TestPickleProtocol2(t *testing.T) {
	// {'a': 1, 'b': ['x', (2, 3)], 'c': True} pickled with protocol 2
	// (generated via python3 pickle.dumps)
	hex := "80027d71002858010000006171014b0158010000006271025d71032858010000007871044b024b0386710565580100000063710688752e"
	data := mustHex(hex)

	r := PickleFormatter{}.Decode(data)
	if r.Error != "" {
		t.Fatalf("decode: %s", r.Error)
	}
	if !r.ReadOnly {
		t.Fatal("pickle must be read-only")
	}
	for _, want := range []string{`"a": 1`, `"x"`, `"b"`, `"c": true`} {
		if !strings.Contains(r.Output, want) {
			t.Fatalf("missing %s in:\n%s", want, r.Output)
		}
	}
	pf := PickleFormatter{}
	if _, err := pf.Encode("x"); err == nil {
		t.Fatal("encode must fail (read-only)")
	}
}

func TestPickleUnsupportedClass(t *testing.T) {
	// REDUCE opcode with a global: should degrade with a clear error.
	data := mustHex("8002635f5f6d61696e5f5f0a466f6f0a942e") // proto2 GLOBAL REDUCE STOP
	r := PickleFormatter{}.Decode(data)
	if r.Error == "" {
		t.Fatal("expected unsupported error")
	}
	if !strings.Contains(r.Error, "unsupported") {
		t.Fatalf("unexpected error: %s", r.Error)
	}
}

// Magento wrappers (ported from qcompress.cpp): prefix + inner codec.
func TestMagentoRoundTrip(t *testing.T) {
	payload := bytes.Repeat([]byte("magento session data "), 50)

	for _, alg := range []string{
		AlgMagentoSessionGzip, AlgMagentoSessionLZ4, AlgMagentoSessionSnappy,
		AlgMagentoCacheGzip, AlgMagentoCacheLZ4, AlgMagentoCacheZstd, AlgMagentoCacheSnappy,
	} {
		compressed, err := Compress(alg, payload)
		if err != nil {
			t.Fatalf("%s compress: %v", alg, err)
		}
		if !bytes.HasPrefix(compressed, []byte(magentoPrefix(alg))) {
			t.Fatalf("%s missing prefix %q", alg, magentoPrefix(alg))
		}
		// Detection must identify the wrapper before plain magic bytes.
		if got := DetectCompression(compressed); got != alg {
			t.Fatalf("%s detected as %q", alg, got)
		}
		out, err := Decompress(alg, compressed)
		if err != nil || !bytes.Equal(out, payload) {
			t.Fatalf("%s round trip: %v", alg, err)
		}
		// Empty-alg path also auto-detects the wrapper.
		out2, err := Decompress("", compressed)
		if err != nil || !bytes.Equal(out2, payload) {
			t.Fatalf("%s auto-detect decompress: %v", alg, err)
		}
	}
}

func mustHex(h string) []byte {
	out, err := hex.DecodeString(h)
	if err != nil {
		panic(err)
	}
	return out
}
