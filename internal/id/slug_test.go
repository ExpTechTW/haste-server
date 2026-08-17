package id

import (
	"strings"
	"testing"
)

func newTestGenerator(t *testing.T, minLen int) *Generator {
	t.Helper()
	return NewGenerator([]byte("test-secret-value"), minLen, []string{"api", "raw"})
}

// The whole point of deriving codes from a counter is that collisions are
// structurally impossible. This asserts it across three tier boundaries.
func TestCodesAreUniqueAcrossTiers(t *testing.T) {
	g := newTestGenerator(t, 1)
	const n = 300_000

	seen := make(map[string]uint64, n)
	for seq := uint64(1); seq <= n; seq++ {
		code, err := g.Code(seq)
		if err != nil {
			t.Fatalf("Code(%d): %v", seq, err)
		}
		if prev, dup := seen[code]; dup {
			t.Fatalf("collision: seq %d and %d both produced %q", prev, seq, code)
		}
		seen[code] = seq
	}
}

func TestCodeLengthGrowsOnlyWhenTierFills(t *testing.T) {
	g := newTestGenerator(t, 1)

	cases := []struct {
		seq  uint64
		want int
	}{
		{1, 1},
		{62, 1},
		{63, 2},   // first code that cannot fit in one character
		{3906, 2}, // 62 + 62^2
		{3907, 3},
		{242234, 3}, // 3906 + 62^3
		{242235, 4},
	}
	for _, tc := range cases {
		code, err := g.Code(tc.seq)
		if err != nil {
			t.Fatalf("Code(%d): %v", tc.seq, err)
		}
		if len(code) != tc.want {
			t.Errorf("Code(%d) = %q (len %d), want len %d", tc.seq, code, len(code), tc.want)
		}
	}
}

// The minimum length is what stops anyone from walking the URL space by hand.
func TestMinimumLengthIsHonoured(t *testing.T) {
	g := newTestGenerator(t, DefaultMinLen)

	for _, seq := range []uint64{1, 2, 63, 1_000, 1_000_000} {
		code, err := g.Code(seq)
		if err != nil {
			t.Fatalf("Code(%d): %v", seq, err)
		}
		if len(code) != DefaultMinLen {
			t.Errorf("Code(%d) = %q (len %d), want len %d", seq, code, len(code), DefaultMinLen)
		}
	}

	// The tier above only opens once the minimum-length tier is full.
	if code, err := g.Code(tiers[DefaultMinLen].size); err != nil || len(code) != DefaultMinLen {
		t.Errorf("last code of the first tier = %q (%v), want %d chars", code, err, DefaultMinLen)
	}
	if code, err := g.Code(tiers[DefaultMinLen].size + 1); err != nil || len(code) != DefaultMinLen+1 {
		t.Errorf("first code past the tier = %q (%v), want %d chars", code, err, DefaultMinLen+1)
	}
}

// Cycle walking must permute each tier onto itself, not merely avoid repeats:
// the first 62 sequence numbers have to cover the alphabet exactly once.
func TestFirstTierIsAPermutationOfTheAlphabet(t *testing.T) {
	g := newTestGenerator(t, 1)

	var got []byte
	for seq := uint64(1); seq <= 62; seq++ {
		code, err := g.Code(seq)
		if err != nil {
			t.Fatalf("Code(%d): %v", seq, err)
		}
		got = append(got, code[0])
	}
	if want, have := sortedString(Alphabet), sortedString(string(got)); want != have {
		t.Errorf("tier 1 covered %q, want the full alphabet %q", have, want)
	}
}

// Consecutive pastes must land far apart, or the address bar becomes a browser
// for other people's pastes.
func TestConsecutiveCodesShareNoPrefix(t *testing.T) {
	g := newTestGenerator(t, DefaultMinLen)

	var neighbours int
	for seq := uint64(1); seq < 500; seq++ {
		a, _ := g.Code(seq)
		b, _ := g.Code(seq + 1)
		if commonPrefix(a, b) >= 3 {
			neighbours++
		}
	}
	if neighbours > 0 {
		t.Errorf("%d consecutive pairs shared a 3-character prefix; the permutation is leaking order", neighbours)
	}
}

// A hash-looking code should mix letters and digits rather than reading as a
// number. Base62 gives that for free, but only if the encoding is really used.
func TestCodesLookLikeHashes(t *testing.T) {
	g := newTestGenerator(t, DefaultMinLen)

	var allDigits, hasLetter, hasDigit int
	const n = 2000
	for seq := uint64(1); seq <= n; seq++ {
		code, err := g.Code(seq)
		if err != nil {
			t.Fatal(err)
		}
		if strings.IndexFunc(code, func(r rune) bool { return r < '0' || r > '9' }) < 0 {
			allDigits++
		}
		if strings.ContainsAny(code, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz") {
			hasLetter++
		}
		if strings.ContainsAny(code, "0123456789") {
			hasDigit++
		}
	}
	// A code that reads as a bare number is the thing being ruled out.
	if allDigits > 0 {
		t.Errorf("%d/%d codes were all digits", allDigits, n)
	}
	// For a uniform base62 code of DefaultMinLen characters, a letter appears
	// with probability 1-(10/62)^8, which rounds to certainty, and a digit with
	// probability 1-(52/62)^8 ~= 0.755. Anything far off means the encoder is
	// not really spreading across the alphabet.
	if hasLetter < n*99/100 {
		t.Errorf("letters in %d/%d codes, expected effectively all", hasLetter, n)
	}
	if hasDigit < n*68/100 || hasDigit > n*83/100 {
		t.Errorf("digits in %d/%d codes, expected ~75%% for uniform base62", hasDigit, n)
	}
}

func TestDifferentSecretsProduceDifferentCodes(t *testing.T) {
	a := NewGenerator([]byte("secret-a"), DefaultMinLen, nil)
	b := NewGenerator([]byte("secret-b"), DefaultMinLen, nil)

	for seq := uint64(1); seq <= 500; seq++ {
		ca, _ := a.Code(seq)
		cb, _ := b.Code(seq)
		if ca == cb {
			t.Fatalf("seq %d produced %q under both secrets; the key is not being mixed in", seq, ca)
		}
	}
}

func TestReservedAndValid(t *testing.T) {
	g := newTestGenerator(t, 1)

	if !g.IsReserved("API") {
		t.Error("IsReserved should be case-insensitive")
	}
	if g.IsReserved("aPi2") {
		t.Error("only exact reserved words should be refused")
	}

	for _, ok := range []string{"a", "Zz09", "aaaaaaaaaa"} {
		if !Valid(ok) {
			t.Errorf("Valid(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "a-b", "a b", "héllo", "aaaaaaaaaaa", "../etc"} {
		if Valid(bad) {
			t.Errorf("Valid(%q) = true, want false", bad)
		}
	}
}

func TestExhaustion(t *testing.T) {
	g := newTestGenerator(t, 1)
	full, err := Capacity(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.Code(full); err != nil {
		t.Errorf("last code in range should succeed: %v", err)
	}
	if _, err := g.Code(full + 1); err == nil {
		t.Error("Code past the last tier should report exhaustion")
	}
}

func TestMinLenIsClamped(t *testing.T) {
	if got := NewGenerator(nil, 0, nil).MinLen(); got != 1 {
		t.Errorf("minLen 0 clamped to %d, want 1", got)
	}
	if got := NewGenerator(nil, 99, nil).MinLen(); got != MaxLen {
		t.Errorf("minLen 99 clamped to %d, want %d", got, MaxLen)
	}
}

func commonPrefix(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

func sortedString(s string) string {
	b := []byte(s)
	for i := 1; i < len(b); i++ {
		for j := i; j > 0 && b[j] < b[j-1]; j-- {
			b[j], b[j-1] = b[j-1], b[j]
		}
	}
	return string(b)
}
