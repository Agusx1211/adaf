package webserver

import (
	"bufio"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/agusx1211/adaf/internal/debug"
)

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func authMiddleware(token string, next http.Handler) http.Handler {
	token = strings.TrimSpace(token)
	if token == "" {
		return next
	}

	expected := []byte(token)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || isPublicRequest(r) {
			next.ServeHTTP(w, r)
			return
		}

		received := bearerToken(r.Header.Get("Authorization"))
		if received == "" {
			received = strings.TrimSpace(r.URL.Query().Get("token"))
		}

		actual := []byte(received)
		if len(actual) == len(expected) && subtle.ConstantTimeCompare(expected, actual) == 1 {
			next.ServeHTTP(w, r)
			return
		}

		writeError(w, http.StatusUnauthorized, "unauthorized")
	})
}

func isPublicRequest(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}

	if r.URL.Path == "/" {
		return true
	}
	return strings.HasPrefix(r.URL.Path, "/static/")
}

func bearerToken(value string) string {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(fields[1])
}

type ipRateLimiter struct {
	mu         sync.Mutex
	tokens     float64
	lastRefill time.Time
	lastSeen   time.Time
}

// rateLimitMiddleware limits requests per IP using a token bucket.
func rateLimitMiddleware(rps float64, next http.Handler) http.Handler {
	if rps <= 0 {
		return next
	}

	burst := rps * 2
	if burst < 10 {
		burst = 10
	}

	var (
		limiters    sync.Map
		cleanupOnce sync.Once
	)

	cleanup := func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			cutoff := time.Now().Add(-5 * time.Minute)
			limiters.Range(func(key, value any) bool {
				limiter, ok := value.(*ipRateLimiter)
				if !ok || limiter == nil {
					limiters.Delete(key)
					return true
				}

				limiter.mu.Lock()
				stale := limiter.lastSeen.Before(cutoff)
				limiter.mu.Unlock()
				if stale {
					limiters.Delete(key)
				}

				return true
			})
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanupOnce.Do(func() {
			go cleanup()
		})

		ip := remoteIPFromAddr(r.RemoteAddr)
		now := time.Now()

		actual, _ := limiters.LoadOrStore(ip, &ipRateLimiter{
			tokens:     burst,
			lastRefill: now,
			lastSeen:   now,
		})

		limiter := actual.(*ipRateLimiter)
		limiter.mu.Lock()
		elapsed := now.Sub(limiter.lastRefill).Seconds()
		if elapsed > 0 {
			limiter.tokens += elapsed * rps
			if limiter.tokens > burst {
				limiter.tokens = burst
			}
		}
		limiter.lastRefill = now
		limiter.lastSeen = now

		allowed := limiter.tokens >= 1
		if allowed {
			limiter.tokens--
		}
		limiter.mu.Unlock()

		if !allowed {
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func remoteIPFromAddr(remoteAddr string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err == nil && host != "" {
		return host
	}

	raw := strings.TrimSpace(remoteAddr)
	if raw == "" {
		return "unknown"
	}
	return raw
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(recorder, r)

		debug.LogKV("webserver", "http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}
