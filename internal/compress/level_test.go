package compress

import (
	"testing"
	"time"

	"github.com/YuYu1015/haste-server/internal/corpus"
)

// TestLevelTradeoff measures what each zstd level actually buys on the inputs
// this server stores, so the default can be chosen from numbers rather than
// from "19 sounds thorough".
//
// It reports rather than asserts; the assertions that matter live in
// TestDefaultLevelMaximisesCompression pins the intent behind the default: this
// server trades CPU for bytes, so the default has to be at the point where more
// compression is genuinely unavailable rather than merely expensive.
func TestDefaultLevelMaximisesCompression(t *testing.T) {
	if testing.Short() {
		t.Skip("timing measurement; slow under -short")
	}

	pastes := corpus.Mixed()

	measure := func(level int) (stored int, per time.Duration) {
		codec, err := New(level)
		if err != nil {
			t.Fatalf("New(%d): %v", level, err)
		}
		defer codec.Close()

		start := time.Now()
		for _, src := range pastes {
			frame, _ := codec.Compress(src)
			stored += len(frame)
		}
		return stored, time.Since(start) / time.Duration(len(pastes))
	}

	stored, per := measure(DefaultLevel)
	bestStored, _ := measure(MaxLevel)
	cheapStored, cheapPer := measure(4)

	t.Logf("level %d: %d B/paste at %v | level %d: %d B/paste | level 4: %d B/paste at %v",
		DefaultLevel, stored/len(pastes), per.Round(time.Microsecond),
		MaxLevel, bestStored/len(pastes),
		cheapStored/len(pastes), cheapPer.Round(time.Microsecond))

	// Nothing above the default may still be on the table.
	if excess := float64(stored)/float64(bestStored) - 1; excess > 0.005 {
		t.Errorf("level %d stores %.2f%% more than level %d; the default is not at the top of the range",
			DefaultLevel, 100*excess, MaxLevel)
	}
	// And the cost of choosing it has to remain a real trade rather than a
	// mistake: a cheap level must be meaningfully worse at storing bytes.
	if gain := float64(cheapStored)/float64(stored) - 1; gain < 0.02 {
		t.Errorf("level 4 is only %.1f%% larger than level %d; the expensive default is no longer earning its CPU",
			100*gain, DefaultLevel)
	}
}
