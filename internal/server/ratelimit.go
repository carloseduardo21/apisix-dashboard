package server

import (
	"math"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/carloseduardo21/apisix-dashboard/internal/audit"
	"github.com/carloseduardo21/apisix-dashboard/internal/store"
)

// Limits of the per-client limiter state. When more clients are tracked,
// full buckets are evicted; past maxTrackedClients new clients are not
// tracked (the global password hashing limit still protects the process).
const (
	sweepThreshold    = 4096
	maxTrackedClients = 65536
)

// limiter is a token bucket per client key: up to burst attempts at once,
// refilled at rate per second.
type limiter struct {
	mu      sync.Mutex
	rate    float64
	burst   float64
	buckets map[string]*bucket
	now     func() time.Time
}

type bucket struct {
	tokens    float64
	last      time.Time
	throttled bool // currently rejecting; used to audit only the first rejection
}

func newLimiter(perMinute int) *limiter {
	if perMinute < 1 {
		perMinute = 20
	}
	return &limiter{
		rate:    float64(perMinute) / 60,
		burst:   float64(perMinute),
		buckets: map[string]*bucket{},
		now:     time.Now,
	}
}

// allow takes one token for key. When none is left it returns false, the
// time until the next token, and whether this is the first rejection since
// the client was last allowed.
func (l *limiter) allow(key string) (ok bool, retry time.Duration, first bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b, found := l.buckets[key]
	if !found {
		if len(l.buckets) >= sweepThreshold {
			l.sweep(now)
		}
		if len(l.buckets) >= maxTrackedClients {
			return true, 0, false
		}
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens = math.Min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		b.throttled = false
		return true, 0, false
	}
	first = !b.throttled
	b.throttled = true
	retry = time.Duration((1 - b.tokens) / l.rate * float64(time.Second))
	return false, retry, first
}

// sweep drops buckets that have refilled completely (clients that stopped).
func (l *limiter) sweep(now time.Time) {
	for k, b := range l.buckets {
		if b.tokens+now.Sub(b.last).Seconds()*l.rate >= l.burst {
			delete(l.buckets, k)
		}
	}
}

// clientKey groups IPv6 clients by /64, the smallest block usually assigned
// to one host, so rotating addresses inside it does not reset the limit.
func clientKey(ip string) string {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}
	if a.Is6() && !a.Is4In6() {
		p, _ := a.Prefix(64)
		return p.String()
	}
	return a.Unmap().String()
}

// throttle limits authentication attempts per client IP (PCI DSS 8.3.4
// complement: the account lockout alone lets anyone lock any account and
// does not slow down password spraying across accounts).
func (s *Server) throttle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := s.clientIP(r)
		ok, retry, first := s.limiter.allow(clientKey(ip))
		if !ok {
			secs := int(math.Ceil(retry.Seconds()))
			if first {
				s.Audit.Record(r.Context(), store.AuditEvent{
					Actor: "-", IP: ip, Action: "auth.throttled", Outcome: audit.Failure,
					Detail: "too many authentication attempts on " + r.URL.Path,
				})
			}
			w.Header().Set("Retry-After", strconv.Itoa(secs))
			writeError(w, http.StatusTooManyRequests, "auth.too_many_requests",
				"too many attempts; try again in {seconds} seconds", "seconds", secs)
			return
		}
		next(w, r)
	}
}
