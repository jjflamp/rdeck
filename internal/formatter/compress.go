// Compression detection/decompression — the Go counterpart of RESP.app's
// qcompress. Magento session/cache wrapper formats are deferred (plan MVP
// cut line).
package formatter

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"

	"github.com/andybalholm/brotli"
	"github.com/golang/snappy"
	"github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
)

// Compression algorithm identifiers (aligned with qcompress naming).
const (
	AlgGzip         = "gzip"
	AlgZlib         = "gzip_php" // PHP gzcompress = zlib stream (78 xx magic)
	AlgLZ4          = "lz4"
	AlgLZ4Raw       = "lz4_raw"
	AlgZstd         = "zstd"
	AlgSnappy       = "snappy"
	AlgSnappyFramed = "snappy_framed"
	AlgBrotli       = "brotli"

	// Magento wrapper formats (ported from qcompress.cpp — plan §4.2).
	// Session lib: https://github.com/colinmollenhour/php-redis-session-abstract
	// Cache lib:   https://github.com/colinmollenhour/Cm_Cache_Backend_Redis
	AlgMagentoSessionGzip   = "magento-session-gzip"
	AlgMagentoSessionLZ4    = "magento-session-lz4"
	AlgMagentoSessionSnappy = "magento-session-snappy"
	AlgMagentoCacheGzip     = "magento-cache-gzip"
	AlgMagentoCacheLZ4      = "magento-cache-lz4"
	AlgMagentoCacheZstd     = "magento-cache-zstd"
	AlgMagentoCacheSnappy   = "magento-cache-snappy"
)

// magentoPrefix returns the literal wrapper prefix (qcompress.cpp magentoPrefix).
func magentoPrefix(alg string) string {
	var id string
	switch alg {
	case AlgMagentoSessionGzip, AlgMagentoCacheGzip:
		id = "gz"
	case AlgMagentoSessionLZ4, AlgMagentoCacheLZ4:
		id = "l4"
	case AlgMagentoCacheZstd:
		id = "zs"
	case AlgMagentoSessionSnappy, AlgMagentoCacheSnappy:
		id = "sn"
	default:
		return ""
	}
	if isMagentoCache(alg) {
		return id + ":" // cache: "<id>:"
	}
	return ":" + id + ":" // session: ":<id>:"
}

func isMagentoCache(alg string) bool {
	switch alg {
	case AlgMagentoCacheGzip, AlgMagentoCacheLZ4, AlgMagentoCacheZstd, AlgMagentoCacheSnappy:
		return true
	}
	return false
}

func isMagento(alg string) bool { return magentoPrefix(alg) != "" }

// magentoInner maps a magento wrapper to its inner algorithm (qcompress.cpp).
func magentoInner(alg string) string {
	switch alg {
	case AlgMagentoSessionGzip, AlgMagentoCacheGzip:
		return AlgZlib // ZLIB_PHP_WINDOW_BIT = 15 (gzcompress)
	case AlgMagentoSessionLZ4, AlgMagentoCacheLZ4:
		return AlgLZ4Raw
	case AlgMagentoCacheZstd:
		return AlgZstd
	case AlgMagentoSessionSnappy, AlgMagentoCacheSnappy:
		return AlgSnappy
	}
	return ""
}

// magentoDetect checks wrapper prefixes (tried before plain magic bytes).
func magentoDetect(data []byte) string {
	if len(data) < 4 {
		return ""
	}
	for _, alg := range []string{
		AlgMagentoSessionGzip, AlgMagentoSessionLZ4, AlgMagentoSessionSnappy,
		AlgMagentoCacheGzip, AlgMagentoCacheLZ4, AlgMagentoCacheZstd, AlgMagentoCacheSnappy,
	} {
		if p := magentoPrefix(alg); bytes.HasPrefix(data, []byte(p)) {
			return alg
		}
	}
	return ""
}

// DetectCompression guesses the algorithm from magic bytes (plan §2.1 压缩行).
// Returns "" when nothing matches — brotli and raw lz4 have no magic and
// must be selected manually (original behavior, remembered per session).
func DetectCompression(data []byte) string {
	if alg := magentoDetect(data); alg != "" {
		return alg
	}
	switch {
	case bytes.HasPrefix(data, []byte{0x1f, 0x8b}):
		return AlgGzip
	case bytes.HasPrefix(data, []byte{0x28, 0xb5, 0x2f, 0xfd}):
		return AlgZstd
	case bytes.HasPrefix(data, []byte{0x04, 0x22, 0x4d, 0x18}):
		return AlgLZ4
	case bytes.HasPrefix(data, []byte{0xff, 0x06, 0x00, 0x00, 's', 'N', 'a', 'P', 'p', 'Y'}):
		return AlgSnappyFramed
	case len(data) > 1 && data[0] == 0x78 && // zlib headers: 78 01 / 78 9c / 78 da
		(data[1] == 0x01 || data[1] == 0x9c || data[1] == 0xda):
		return AlgZlib
	default:
		return ""
	}
}

// Decompress decodes data using the given algorithm.
func Decompress(alg string, data []byte) ([]byte, error) {
	if isMagento(alg) {
		p := magentoPrefix(alg)
		if !bytes.HasPrefix(data, []byte(p)) {
			return nil, fmt.Errorf("data does not carry %s prefix", alg)
		}
		return Decompress(magentoInner(alg), data[len(p):])
	}
	switch alg {
	case AlgGzip:
		r, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		return io.ReadAll(r)

	case AlgZlib:
		r, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		return io.ReadAll(r)

	case AlgZstd:
		r, err := zstd.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		out, err := io.ReadAll(r.IOReadCloser())
		if err != nil {
			return nil, err
		}
		return out, nil

	case AlgLZ4:
		return io.ReadAll(lz4.NewReader(bytes.NewReader(data)))

	case AlgLZ4Raw:
		return lz4RawDecompress(data)

	case AlgSnappy:
		return snappy.Decode(nil, data)

	case AlgSnappyFramed:
		return io.ReadAll(snappy.NewReader(bytes.NewReader(data)))

	case AlgBrotli:
		return io.ReadAll(brotli.NewReader(bytes.NewReader(data)))

	case "":
		// Auto-detect from magic bytes (magento/plain); error when unknown.
		if detected := DetectCompression(data); detected != "" {
			return Decompress(detected, data)
		}
		return nil, fmt.Errorf("cannot detect compression (brotli/lz4_raw have no magic — pick manually)")

	default:
		return nil, fmt.Errorf("unsupported compression algorithm %q", alg)
	}
}

// Compress encodes data using the given algorithm.
func Compress(alg string, data []byte) ([]byte, error) {
	if isMagento(alg) {
		inner, err := Compress(magentoInner(alg), data)
		if err != nil {
			return nil, err
		}
		return append([]byte(magentoPrefix(alg)), inner...), nil
	}
	switch alg {
	case AlgGzip:
		var buf bytes.Buffer
		w := gzip.NewWriter(&buf)
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil { // Close flushes; must precede buf.Bytes()
			return nil, err
		}
		return buf.Bytes(), nil

	case AlgZlib:
		var buf bytes.Buffer
		w := zlib.NewWriter(&buf)
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil

	case AlgZstd:
		var buf bytes.Buffer
		w, err := zstd.NewWriter(&buf)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil

	case AlgLZ4:
		var buf bytes.Buffer
		w := lz4.NewWriter(&buf)
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil

	case AlgLZ4Raw:
		return lz4RawCompress(data)

	case AlgSnappy:
		return snappy.Encode(nil, data), nil

	case AlgSnappyFramed:
		var buf bytes.Buffer
		w := snappy.NewBufferedWriter(&buf)
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil

	case AlgBrotli:
		var buf bytes.Buffer
		w := brotli.NewWriter(&buf)
		if _, err := w.Write(data); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil

	case "":
		return nil, fmt.Errorf("no compression algorithm selected")

	default:
		return nil, fmt.Errorf("unsupported compression algorithm %q", alg)
	}
}

// lz4RawCompress encodes with the raw (frame-less) LZ4 block format used by
// some ecosystems: 4-byte big-endian original length + block.
func lz4RawCompress(data []byte) ([]byte, error) {
	out := make([]byte, 4, 4+len(data))
	out[0] = byte(len(data) >> 24)
	out[1] = byte(len(data) >> 16)
	out[2] = byte(len(data) >> 8)
	out[3] = byte(len(data))

	dst := make([]byte, lz4.CompressBlockBound(len(data)))
	var c lz4.Compressor
	n, err := c.CompressBlock(data, dst)
	if err != nil {
		return nil, err
	}
	return append(out, dst[:n]...), nil
}

func lz4RawDecompress(data []byte) ([]byte, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("lz4 raw data too short")
	}
	orig := int(data[0])<<24 | int(data[1])<<16 | int(data[2])<<8 | int(data[3])
	dst := make([]byte, orig)
	n, err := lz4.UncompressBlock(data[4:], dst)
	if err != nil {
		return nil, err
	}
	return dst[:n], nil
}
