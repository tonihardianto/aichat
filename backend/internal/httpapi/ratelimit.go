package httpapi

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"chat/backend/internal/auth"
	"chat/backend/internal/observability"
)

type clientLimit struct {
	tokens     float64
	lastRefill time.Time
}

type RateLimiter struct {
	mu       sync.Mutex
	limits   map[string]*clientLimit
	rate     float64 // tokens per second
	capacity float64 // max tokens
}

func NewRateLimiter(ratePerMin int, capacity int) *RateLimiter {
	return &RateLimiter{
		limits:   make(map[string]*clientLimit),
		rate:     float64(ratePerMin) / 60.0,
		capacity: float64(capacity),
	}
}

func (rl *RateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cl, exists := rl.limits[key]
	if !exists {
		rl.limits[key] = &clientLimit{
			tokens:     rl.capacity - 1,
			lastRefill: now,
		}
		return true
	}

	elapsed := now.Sub(cl.lastRefill).Seconds()
	cl.tokens += elapsed * rl.rate
	if cl.tokens > rl.capacity {
		cl.tokens = rl.capacity
	}
	cl.lastRefill = now

	if cl.tokens >= 1.0 {
		cl.tokens -= 1.0
		return true
	}

	return false
}

func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		return strings.TrimSpace(xrip)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func IPRateLimitMiddleware(rl *RateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := getClientIP(r)
		if !rl.allow(ip) {
			observability.WriteError(w, http.StatusTooManyRequests, "rate_limited", "Terlalu banyak permintaan. Silakan tunggu beberapa saat.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func UserRateLimitMiddleware(rl *RateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := getClientIP(r)
		if u := auth.GetUser(r.Context()); u != nil {
			key = u.ID
		}
		if !rl.allow(key) {
			observability.WriteError(w, http.StatusTooManyRequests, "rate_limited", "Batas request tercapai. Silakan coba sesaat lagi.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func GuestRateLimitMiddleware(rl *RateLimiter, keyer interface{ GuestRateKey(string) string }, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.allow(keyer.GuestRateKey(getClientIP(r))) {
			observability.WriteError(w, http.StatusTooManyRequests, "rate_limited", "Batas chat tamu tercapai. Silakan coba lagi nanti.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
