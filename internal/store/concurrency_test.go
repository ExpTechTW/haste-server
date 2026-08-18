package store

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/YuYu1015/haste-server/internal/corpus"
)

// TestWriteConcurrency measures where the write path actually spends its time
// and how it behaves as callers pile on, so the need for admission control can
// be argued from numbers rather than intuition.
func TestWriteConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("timing measurement; slow under -short")
	}

	content := corpus.Log(1) // a full 4000-character paste

	// Split the cost: compression runs before the transaction and is therefore
	// unbounded in parallelism, while the insert is serialised by the writer.
	{
		st := newTestStore(t, nil)
		codec := st.opts.Codec

		start := time.Now()
		const n = 200
		for i := 0; i < n; i++ {
			codec.Compress([]byte(content))
		}
		perCompress := time.Since(start) / n

		start = time.Now()
		for i := 0; i < n; i++ {
			if _, err := st.Create(context.Background(), content, "", "", NoExpiry); err != nil {
				t.Fatal(err)
			}
		}
		perCreate := time.Since(start) / n

		t.Logf("sequential: zstd-19 %v/paste | full Create %v/paste (insert ≈ %v) | ceiling ≈ %.0f writes/s",
			perCompress.Round(time.Microsecond),
			perCreate.Round(time.Microsecond),
			(perCreate - perCompress).Round(time.Microsecond),
			float64(time.Second)/float64(perCreate))
	}

	gates := []struct {
		name  string
		tweak func(*Options)
	}{
		{"unbounded", nil},
		{"bounded", func(o *Options) {
			o.WriteConcurrency = runtime.NumCPU()
			o.WriteQueue = 512
		}},
	}

	for _, gate := range gates {
		for _, clients := range []int{1, 8, 64, 512} {
			t.Run(fmt.Sprintf("%s/clients=%d", gate.name, clients), func(t *testing.T) {
				measure(t, gate.name, gate.tweak, clients)
			})
		}
	}
}

func measure(t *testing.T, label string, tweak func(*Options), clients int) {
	t.Helper()

	st := newTestStore(t, tweak)
	ctx := context.Background()
	content := corpus.Log(2)

	const perClient = 8
	latencies := make([]time.Duration, 0, clients*perClient)
	var mu sync.Mutex
	var shed int

	var wg sync.WaitGroup
	start := time.Now()
	for c := 0; c < clients; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perClient; i++ {
				began := time.Now()
				_, err := st.Create(ctx, content, "", "", NoExpiry)
				took := time.Since(began)

				mu.Lock()
				switch {
				case errors.Is(err, ErrBusy):
					shed++
				case err != nil:
					t.Errorf("create: %v", err)
				default:
					latencies = append(latencies, took)
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	if len(latencies) == 0 {
		t.Fatal("every write was shed")
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	t.Logf("%-9s %4d clients: %6.0f writes/s | p50 %9v | p99 %9v | worst %9v | shed %d",
		label,
		clients,
		float64(len(latencies))/elapsed.Seconds(),
		latencies[len(latencies)/2].Round(time.Microsecond),
		latencies[len(latencies)*99/100].Round(time.Microsecond),
		latencies[len(latencies)-1].Round(time.Microsecond),
		shed)
}

// The queue has to have an end: past it, callers are refused rather than left
// holding a request buffer in a line that is no longer worth joining.
func TestWriteQueueShedsWhenFull(t *testing.T) {
	st := newTestStore(t, func(o *Options) {
		o.WriteConcurrency = 1
		o.WriteQueue = 4
	})
	ctx := context.Background()
	content := corpus.Log(3)

	const clients = 200
	var wg sync.WaitGroup
	var mu sync.Mutex
	var accepted, shed int

	for i := 0; i < clients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := st.Create(ctx, content, "", "", NoExpiry)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				accepted++
			case errors.Is(err, ErrBusy):
				shed++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if shed == 0 {
		t.Errorf("%d concurrent writes against a queue of 4 shed nothing", clients)
	}
	if accepted == 0 {
		t.Error("every write was shed; the gate is refusing work it could have done")
	}
	t.Logf("concurrency=1 queue=4: %d accepted, %d shed of %d", accepted, shed, clients)
}
