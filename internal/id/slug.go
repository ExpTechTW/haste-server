// Package id turns a monotonic counter into the shortest possible share code.
//
// The counter guarantees uniqueness, so codes never collide and never need a
// retry loop against the database. Codes are laid out in tiers: the first 62
// pastes get 1-character codes, the next 3 844 get 2 characters, and so on, so
// length only ever grows when the shorter space is genuinely exhausted.
//
// Handing out the raw counter would make every code guessable, so within each
// tier the counter is run through a keyed Feistel permutation with cycle
// walking. That is a bijection over exactly the tier's code space: still no
// collisions, still minimal length, but consecutive pastes land far apart.
package id

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math/bits"
	"strings"
)

// Alphabet is base62 in ASCII order, so codes sort the same way bytes do.
const Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

const (
	base = uint64(62)
	// MaxLen caps the tier ladder. 62^10 is ~8.4e17 codes, still inside uint64.
	MaxLen = 10
	rounds = 4
)

var (
	// offset[l] is how many codes exist below length l, so tier l covers the
	// zero-based counter range [offset[l], offset[l+1]).
	offset [MaxLen + 2]uint64
	// size[l] is 62^l, the number of codes of exactly length l.
	size [MaxLen + 1]uint64

	// ErrExhausted means the counter outgrew MaxLen characters.
	ErrExhausted = errors.New("id: code space exhausted")
)

func init() {
	n := uint64(1)
	for l := 1; l <= MaxLen; l++ {
		n *= base
		size[l] = n
		offset[l+1] = offset[l] + n
	}
}

// Generator maps sequence numbers to share codes. It is safe for concurrent use.
type Generator struct {
	key      []byte
	reserved map[string]struct{}
}

// NewGenerator keys the permutation with secret. Codes matching any reserved
// word (compared case-insensitively) are refused, so share codes can never
// shadow a real route such as /api or /raw.
func NewGenerator(secret []byte, reserved []string) *Generator {
	g := &Generator{key: secret, reserved: make(map[string]struct{}, len(reserved))}
	for _, r := range reserved {
		g.reserved[strings.ToLower(r)] = struct{}{}
	}
	return g
}

// Code returns the share code for a 1-based sequence number.
func (g *Generator) Code(seq uint64) (string, error) {
	if seq == 0 {
		return "", errors.New("id: sequence numbers start at 1")
	}
	m := seq - 1
	l := 1
	for l <= MaxLen && m >= offset[l+1] {
		l++
	}
	if l > MaxLen {
		return "", ErrExhausted
	}
	return encode(g.permute(m-offset[l], size[l], l), l), nil
}

// IsReserved reports whether a code would shadow a fixed route.
func (g *Generator) IsReserved(code string) bool {
	_, ok := g.reserved[strings.ToLower(code)]
	return ok
}

// Valid reports whether code could ever have been issued. Cheap input
// validation that keeps malformed lookups away from the database.
func Valid(code string) bool {
	if len(code) == 0 || len(code) > MaxLen {
		return false
	}
	for i := 0; i < len(code); i++ {
		if !strings.ContainsRune(Alphabet, rune(code[i])) {
			return false
		}
	}
	return true
}

// permute shuffles x within [0, n) using a Feistel network over the smallest
// even bit width that covers n, walking the cycle whenever a round lands
// outside the range. Cycle walking keeps the result a permutation of exactly
// [0, n), which is what makes collisions structurally impossible.
func (g *Generator) permute(x, n uint64, tier int) uint64 {
	half := halfWidth(n)
	mask := uint64(1)<<half - 1
	for {
		x = g.feistel(x, half, mask, tier)
		if x < n {
			return x
		}
	}
}

// halfWidth returns half of the smallest even bit width w with 2^w >= n.
func halfWidth(n uint64) uint {
	b := uint(bits.Len64(n - 1))
	if b == 0 {
		b = 1
	}
	return (b + 1) / 2
}

func (g *Generator) feistel(x uint64, half uint, mask uint64, tier int) uint64 {
	l, r := (x>>half)&mask, x&mask
	for round := 0; round < rounds; round++ {
		l, r = r, l^g.round(round, tier, r, mask)
	}
	return l<<half | r
}

// round is the Feistel round function. Mixing the tier in gives every code
// length an independent permutation.
func (g *Generator) round(round, tier int, v, mask uint64) uint64 {
	var buf [10]byte
	buf[0] = byte(round)
	buf[1] = byte(tier)
	binary.BigEndian.PutUint64(buf[2:], v)

	mac := hmac.New(sha256.New, g.key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	return binary.BigEndian.Uint64(sum[:8]) & mask
}

func encode(v uint64, l int) string {
	buf := make([]byte, l)
	for i := l - 1; i >= 0; i-- {
		buf[i] = Alphabet[v%base]
		v /= base
	}
	return string(buf)
}

// Capacity reports how many codes fit in codes of at most l characters.
func Capacity(l int) (uint64, error) {
	if l < 1 || l > MaxLen {
		return 0, fmt.Errorf("id: length %d out of range 1..%d", l, MaxLen)
	}
	return offset[l+1], nil
}
