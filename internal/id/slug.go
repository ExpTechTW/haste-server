// Package id turns a monotonic counter into a short, unguessable share code.
//
// The counter guarantees uniqueness, so codes never collide and never need a
// retry loop against the database. Codes start at a configured minimum length
// and only grow when that space is genuinely exhausted.
//
// Handing out the raw counter would make every code one increment away from
// being guessed in the address bar, so within each length tier the counter runs
// through a keyed Feistel permutation with cycle walking. That is a bijection
// onto exactly that tier's code space: still no collisions, still minimal
// length, but the result is indistinguishable from a random base62 hash.
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
	// DefaultMinLen reads as a short hash and puts 2.2e14 codes between any two
	// live pastes, which is what makes walking the URL space pointless.
	DefaultMinLen = 8
	rounds        = 4
)

// tier describes one code length: how many codes it holds, and the Feistel
// block that covers them. Both are fixed per length, so they are derived once
// here rather than recomputed on every code.
type tier struct {
	size uint64 // 62^length
	half uint   // half the block width, in bits
	mask uint64 // 1<<half - 1
}

var tiers [MaxLen + 1]tier

// alphabetSet answers "is this byte a code character" in one indexed load,
// rather than by scanning the alphabet for each character of every lookup.
var alphabetSet [256]bool

// ErrExhausted means the counter outgrew MaxLen characters.
var ErrExhausted = errors.New("id: code space exhausted")

func init() {
	n := uint64(1)
	for l := 1; l <= MaxLen; l++ {
		n *= base
		// Half of the smallest even bit width w with 2^w >= n, which is what
		// makes the Feistel network a permutation of a superset of the tier.
		half := (uint(bits.Len64(n-1)) + 1) / 2
		tiers[l] = tier{size: n, half: half, mask: 1<<half - 1}
	}
	for i := 0; i < len(Alphabet); i++ {
		alphabetSet[Alphabet[i]] = true
	}
}

// Generator maps sequence numbers to share codes. It is safe for concurrent use.
type Generator struct {
	key      []byte
	minLen   int
	reserved map[string]struct{}
}

// NewGenerator keys the permutation with secret and issues codes of at least
// minLen characters. Codes matching any reserved word (compared
// case-insensitively) are refused, so a share code can never shadow a route.
// minLen is clamped into range; callers validate it for a useful error message.
func NewGenerator(secret []byte, minLen int, reserved []string) *Generator {
	switch {
	case minLen < 1:
		minLen = 1
	case minLen > MaxLen:
		minLen = MaxLen
	}

	g := &Generator{key: secret, minLen: minLen, reserved: make(map[string]struct{}, len(reserved))}
	for _, r := range reserved {
		g.reserved[strings.ToLower(r)] = struct{}{}
	}
	return g
}

// MinLen reports the shortest code this generator will issue.
func (g *Generator) MinLen() int { return g.minLen }

// Code returns the share code for a 1-based sequence number.
func (g *Generator) Code(seq uint64) (string, error) {
	if seq == 0 {
		return "", errors.New("id: sequence numbers start at 1")
	}

	// Walk the tiers, consuming each one's capacity until the sequence number
	// falls inside the current length.
	idx := seq - 1
	for length := g.minLen; length <= MaxLen; length++ {
		t := tiers[length]
		if idx < t.size {
			return encode(g.permute(idx, t, length), length), nil
		}
		idx -= t.size
	}
	return "", ErrExhausted
}

// IsReserved reports whether a code would shadow a fixed route.
func (g *Generator) IsReserved(code string) bool {
	_, ok := g.reserved[strings.ToLower(code)]
	return ok
}

// Capacity reports how many codes exist between minLen and MaxLen characters.
func Capacity(minLen int) (uint64, error) {
	if minLen < 1 || minLen > MaxLen {
		return 0, fmt.Errorf("id: minimum length %d out of range 1..%d", minLen, MaxLen)
	}
	var total uint64
	for length := minLen; length <= MaxLen; length++ {
		total += tiers[length].size
	}
	return total, nil
}

// Valid reports whether code could ever have been issued. Cheap input
// validation that keeps malformed lookups away from the database. It checks
// shape only, not the current minimum length, so codes issued under an older
// setting stay resolvable.
func Valid(code string) bool {
	if len(code) == 0 || len(code) > MaxLen {
		return false
	}
	for i := 0; i < len(code); i++ {
		if !alphabetSet[code[i]] {
			return false
		}
	}
	return true
}

// permute shuffles x within the tier using a Feistel network over the smallest
// even bit width that covers it, walking the cycle whenever a round lands
// outside the range. Cycle walking keeps the result a permutation of exactly
// [0, size), which is what makes collisions structurally impossible.
func (g *Generator) permute(x uint64, t tier, length int) uint64 {
	for {
		x = g.feistel(x, t, length)
		if x < t.size {
			return x
		}
	}
}

func (g *Generator) feistel(x uint64, t tier, length int) uint64 {
	l, r := (x>>t.half)&t.mask, x&t.mask
	for round := 0; round < rounds; round++ {
		l, r = r, l^g.round(round, length, r, t.mask)
	}
	return l<<t.half | r
}

// round is the Feistel round function. Mixing the code length in gives every
// tier an independent permutation.
func (g *Generator) round(round, length int, v, mask uint64) uint64 {
	var buf [10]byte
	buf[0] = byte(round)
	buf[1] = byte(length)
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
