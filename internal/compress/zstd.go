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
// nearly always wins for short pastes, but a long paste can carry enough of its
// own context that the dictionary's frame header stops paying for itself.
func (c *Codec) Compress(src []byte) (blob []byte, codec int) {
	plain := gozstd.CompressLevel(nil, src, c.level)
	dicted := gozstd.CompressDict(nil, src, c.cdict)
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
