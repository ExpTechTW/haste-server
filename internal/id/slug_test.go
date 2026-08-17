package id

import (
	"strings"
	"testing"
)

func newTestGenerator(t *testing.T) *Generator {
	t.Helper()
	return NewGenerator([]byte("test-secret-value"), []string{"api", "raw"})
}

// The whole point of deriving codes from a counter is that collisions are
// structurally impossible. This asserts it across three tier boundaries.
func TestCodesAreUniqueAcrossTiers(t *testing.T) {
	g := newTestGenerator(t)
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
	g := newTestGenerator(t)

	cases := []struct {
		seq  uint64
		want int
	}{
		{1, 1},
		{62, 1},
		{63, 2},        // first code that cannot fit in one character
		{3906, 2},      // 62 + 62^2
		{3907, 3},
		{242234, 3},    // 3906 + 62^3
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

// Cycle walking must permute each tier onto itself, not merely avoid repeats:
// the first 62 sequence numbers have to cover the alphabet exactly once.
func TestFirstTierIsAPermutationOfTheAlphabet(t *testing.T) {
	g := newTestGenerator(t)

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

// Codes must be shuffled, not sequential — otherwise every paste is one
// increment away from being guessed.
func TestCodesAreNotSequential(t *testing.T) {
	g := newTestGenerator(t)

	var sequential int
	for seq := uint64(1); seq < 200; seq++ {
		a, _ := g.Code(seq)
		b, _ := g.Code(seq + 1)
		if len(a) == len(b) && strings.IndexByte(Alphabet, b[len(b)-1])-strings.IndexByte(Alphabet, a[len(a)-1]) == 1 {
			sequential++
		}
	}
	// A real permutation lands adjacent by chance roughly 1/62 of the time.
	if sequential > 20 {
		t.Errorf("%d/199 consecutive pairs were adjacent codes; permutation looks broken", sequential)
	}
}

func TestDifferentSecretsProduceDifferentCodes(t *testing.T) {
	a := NewGenerator([]byte("secret-a"), nil)
	b := NewGenerator([]byte("secret-b"), nil)

	var same int
	for seq := uint64(1); seq <= 500; seq++ {
		ca, _ := a.Code(seq)
		cb, _ := b.Code(seq)
		if ca == cb {
			same++
		}
	}
	if same > 30 {
		t.Errorf("%d/500 codes matched across secrets; the key is not being mixed in", same)
	}
}

func TestReservedAndValid(t *testing.T) {
	g := newTestGenerator(t)

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
	g := newTestGenerator(t)
	full, err := Capacity(MaxLen)
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

func sortedString(s string) string {
	b := []byte(s)
	for i := 1; i < len(b); i++ {
		for j := i; j > 0 && b[j] < b[j-1]; j-- {
			b[j], b[j-1] = b[j-1], b[j]
		}
	}
	return string(b)
}
