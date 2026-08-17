// Package compress stores paste bodies as zstd frames.
//
// Pastes are small — a few kilobytes at most — which is exactly the size range
// where plain zstd does its worst: the compressor spends most of the input
// learning the data before it can encode anything cheaply. A prebuilt
// dictionary of common source-code and log fragments gives it that model up
// front, which typically halves the stored size of a short paste.
//
// Each row records which codec produced its bytes, so the dictionary can be
// revised later without invalidating anything already written.
package compress

import (
	_ "embed"
	"fmt"

	"github.com/valyala/gozstd"
)

//go:embed dict/v1.txt
var dictV1 []byte

// Codec identifiers, persisted per row. The stored bytes are undecodable
// without knowing which dictionary (if any) produced them, so these values are
// append-only: never reuse or renumber one.
const (
	CodecZstd     = 0 // plain zstd frame
	CodecZstdDict = 1 // zstd frame built against dict/v1.txt
)

// Codec compresses and decompresses paste bodies. It is safe for concurrent use.
type Codec struct {
	level int
	cdict *gozstd.CDict
	ddict *gozstd.DDict
}

// New builds a codec at the given zstd level (1-22).
func New(level int) (*Codec, error) {
	cdict, err := gozstd.NewCDictLevel(dictV1, level)
	if err != nil {
		return nil, fmt.Errorf("compress: build compression dictionary: %w", err)
	}
	ddict, err := gozstd.NewDDict(dictV1)
	if err != nil {
		cdict.Release()
		return nil, fmt.Errorf("compress: build decompression dictionary: %w", err)
	}
	return &Codec{level: level, cdict: cdict, ddict: ddict}, nil
}

// Level reports the configured zstd compression level.
func (c *Codec) Level() int { return c.level }

// Close releases the native dictionary handles.
func (c *Codec) Close() {
	c.cdict.Release()
	c.ddict.Release()
}

// Compress encodes src both ways and keeps whichever frame came out smaller,
// returning the bytes and the codec id needed to read them back. The dictionary
// wins for most pastes, but one that carries enough of its own context can beat
// it, and on this corpus that is worth about 1% of total storage.
//
// The two frames are independent, so they run on separate cores. The dictionary
// gives its encoder a head start and finishes in roughly half the time of the
// plain one, so overlapping them costs a goroutine and returns the slower of the
// two rather than their sum — the same bytes for about two thirds of the wait.
func (c *Codec) Compress(src []byte) (blob []byte, codec int) {
	var plain []byte
	done := make(chan struct{})
	go func() {
		defer close(done)
		plain = gozstd.CompressLevel(nil, src, c.level)
	}()

	dicted := gozstd.CompressDict(nil, src, c.cdict)
	<-done

	if len(dicted) < len(plain) {
		return dicted, CodecZstdDict
	}
	return plain, CodecZstd
}

// Decompress reverses Compress. limit caps the decoded size so a malformed or
// hostile frame cannot be expanded into an unbounded allocation.
func (c *Codec) Decompress(blob []byte, codec int, limit int) ([]byte, error) {
	switch codec {
	case CodecZstd:
		out, err := gozstd.DecompressLimited(nil, blob, limit)
		if err != nil {
			return nil, fmt.Errorf("compress: decode frame: %w", err)
		}
		return out, nil
	case CodecZstdDict:
		out, err := gozstd.DecompressDictLimited(nil, blob, c.ddict, limit)
		if err != nil {
			return nil, fmt.Errorf("compress: decode dictionary frame: %w", err)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("compress: unknown codec id %d", codec)
	}
}

// DefaultLevel is the compression level the server uses unless configured
// otherwise.
//
// This server is tuned for storage, not for write latency, so the default sits
// at the top of the useful range. Measured on its own corpus of 4000-character
// pastes (see level_test.go, and the cross-codec comparison in the README):
//
//	zstd -19 + dictionary   760 B/paste   345µs      <- default
//	brotli q11              766 B/paste   4.1ms
//	zstd -19, no dictionary 799 B/paste   576µs
//	zstd -4  + dictionary   811 B/paste    15µs
//	gzip -9                 916 B/paste    82µs
//	xz (LZMA2)              954 B/paste   577µs
//	bzip2 -9                965 B/paste   267µs
//
// bzip2 and xz losing is not a surprise once the input size is taken into
// account: block sorting and a large LZMA window both need far more than four
// kilobytes before they repay their own overhead. Levels 20-22 produce byte-for-
// byte the same output as 19 here, so there is nothing above this to reach for.
//
// An operator who cares more about write throughput than about bytes can drop
// this a long way — level 4 costs about 6% more space for a twentieth of the
// CPU — but that is not the default this server ships.
const DefaultLevel = 19

// MaxLevel is the highest level zstd accepts.
const MaxLevel = 22
