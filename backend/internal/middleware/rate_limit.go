package middleware

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultRateLimitRequests = 180
	defaultRateLimitWindow   = time.Minute
)

type rateLimitVisitor struct {
	windowStarted time.Time
	requests      int
	lastSeen      time.Time
}

type RateLimiter struct {
	mutex    sync.Mutex
	visitors map[string]*rateLimitVisitor

	limit  int
	window time.Duration

	lastCleanup time.Time
}

func NewRateLimiter(
	limit int,
	window time.Duration,
) *RateLimiter {
	if limit <= 0 {
		limit = defaultRateLimitRequests
	}

	if window <= 0 {
		window = defaultRateLimitWindow
	}

	return &RateLimiter{
		visitors:    make(map[string]*rateLimitVisitor),
		limit:       limit,
		window:      window,
		lastCleanup: time.Now(),
	}
}

func NewDefaultRateLimiter() *RateLimiter {
	return NewRateLimiter(
		defaultRateLimitRequests,
		defaultRateLimitWindow,
	)
}

func (l *RateLimiter) Middleware(
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(
		func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			ipAddress := rateLimitIPAddress(r)
			now := time.Now()

			allowed, remaining, resetAt :=
				l.allow(ipAddress, now)

			w.Header().Set(
				"X-RateLimit-Limit",
				strconv.Itoa(l.limit),
			)
			w.Header().Set(
				"X-RateLimit-Remaining",
				strconv.Itoa(remaining),
			)
			w.Header().Set(
				"X-RateLimit-Reset",
				strconv.FormatInt(
					resetAt.Unix(),
					10,
				),
			)

			if !allowed {
				retryAfter := int(
					time.Until(resetAt).Seconds(),
				)

				if retryAfter < 1 {
					retryAfter = 1
				}

				w.Header().Set(
					"Retry-After",
					strconv.Itoa(retryAfter),
				)

				writeRateLimitExceeded(w, r)
				return
			}

			next.ServeHTTP(w, r)
		},
	)
}

func (l *RateLimiter) allow(
	ipAddress string,
	now time.Time,
) (
	bool,
	int,
	time.Time,
) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.cleanupExpiredVisitors(now)

	visitor, exists := l.visitors[ipAddress]

	if !exists ||
		now.Sub(visitor.windowStarted) >= l.window {
		visitor = &rateLimitVisitor{
			windowStarted: now,
			requests:      0,
			lastSeen:      now,
		}

		l.visitors[ipAddress] = visitor
	}

	visitor.lastSeen = now

	resetAt := visitor.windowStarted.Add(
		l.window,
	)

	if visitor.requests >= l.limit {
		return false, 0, resetAt
	}

	visitor.requests++

	remaining := l.limit - visitor.requests

	return true, remaining, resetAt
}

func (l *RateLimiter) cleanupExpiredVisitors(
	now time.Time,
) {
	if now.Sub(l.lastCleanup) < 5*time.Minute {
		return
	}

	for ipAddress, visitor := range l.visitors {
		if now.Sub(visitor.lastSeen) >
			2*l.window {
			delete(l.visitors, ipAddress)
		}
	}

	l.lastCleanup = now
}

func rateLimitIPAddress(
	r *http.Request,
) string {
	address := strings.TrimSpace(r.RemoteAddr)

	host, _, err := net.SplitHostPort(address)
	if err == nil && host != "" {
		return host
	}

	if parsed := net.ParseIP(address); parsed != nil {
		return parsed.String()
	}

	return "unknown"
}

func writeRateLimitExceeded(
	w http.ResponseWriter,
	r *http.Request,
) {
	w.Header().Set(
		"Content-Type",
		"application/json",
	)
	w.Header().Set(
		"Cache-Control",
		"no-store",
	)
	w.Header().Set(
		"X-Content-Type-Options",
		"nosniff",
	)

	w.WriteHeader(http.StatusTooManyRequests)

	_ = json.NewEncoder(w).Encode(
		map[string]any{
			"error": map[string]any{
				"code":    "rate_limit_exceeded",
				"message": "Too many requests. Try again shortly.",
				"request_id": GetRequestID(
					r.Context(),
				),
			},
		},
	)
}
