package httpapi

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// sweepEvery bounds the bookkeeping cost of expiring idle buckets: the map is
// only walked once per this many admission checks.
const sweepEvery = 1024

// idleTTL is how long a silent client keeps its bucket. Longer than any
// realistic burst, short enough that the map tracks active clients only.
const idleTTL = 10 * time.Minute

type bucket struct {
	limiter *rate.Limiter
	seen    time.Time
}

// ipLimiter throttles paste creation per client address. Reads are unmetered:
// they are served from the page cache and cost far less than a write.
type ipLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	calls   int

	rps   rate.Limit
	burst int
}

// newIPLimiter returns nil when rps is zero, which callers treat as "no limit".
func newIPLimiter(rps float64, burst int) *ipLimiter {
	if rps <= 0 {
		return nil
	}
	if burst < 1 {
		burst = 1
	}
	return &ipLimiter{
		buckets: make(map[string]*bucket),
		rps:     rate.Limit(rps),
		burst:   burst,
	}
}

func (l *ipLimiter) allow(ip string) bool {
	if l == nil {
		return true
	}
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	l.calls++
	if l.calls%sweepEvery == 0 {
		for k, b := range l.buckets {
			if now.Sub(b.seen) > idleTTL {
				delete(l.buckets, k)
			}
		}
	}

	b, ok := l.buckets[ip]
	if !ok {
		b = &bucket{limiter: rate.NewLimiter(l.rps, l.burst)}
		l.buckets[ip] = b
	}
	b.seen = now
	return b.limiter.Allow()
}

// clientIP identifies the caller. Proxy headers are trivially spoofable, so
// they are only consulted when the operator has said a proxy is in front.
func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			// Left-most entry is the original client; the rest are hops.
			if first, _, ok := strings.Cut(fwd, ","); ok {
				return strings.TrimSpace(first)
			}
			return strings.TrimSpace(fwd)
		}
		if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
			return real
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
