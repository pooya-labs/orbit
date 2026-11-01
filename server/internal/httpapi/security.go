package httpapi

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pooya16pg-spec/orbit/server/internal/auth"
)

type TokenVerifier interface {
	Verify(context.Context, string) (auth.Principal, error)
}

type principalContextKey struct{}

func principalFromContext(ctx context.Context) (auth.Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(auth.Principal)
	return principal, ok
}

func withAuthorization(environment string, verifier TokenVerifier, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/meta" || !strings.HasPrefix(r.URL.Path, "/api/v1/") {
			next.ServeHTTP(w, r)
			return
		}

		if verifier == nil {
			if environment == "production" || environment == "staging" {
				writeError(w, r, http.StatusServiceUnavailable, "authentication_unavailable", "API authentication is not configured")
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		headerValues := r.Header.Values("Authorization")
		if len(headerValues) != 1 {
			unauthorized(w, r)
			return
		}
		parts := strings.Fields(headerValues[0])
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			unauthorized(w, r)
			return
		}

		principal, err := verifier.Verify(r.Context(), parts[1])
		if err != nil {
			unauthorized(w, r)
			return
		}

		requiredRole := "viewer"
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			requiredRole = "operator"
		}
		if strings.HasPrefix(r.URL.Path, "/api/v1/admin/") {
			requiredRole = "administrator"
		}
		if !principal.HasRole(requiredRole) {
			writeError(w, r, http.StatusForbidden, "insufficient_role", "The authenticated principal is not permitted to access this resource")
			return
		}

		ctx := context.WithValue(r.Context(), principalContextKey{}, principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func unauthorized(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="orbit", error="invalid_token"`)
	writeError(w, r, http.StatusUnauthorized, "authentication_required", "A valid ORBIT API access token is required")
}

func withBodyLimit(maxBytes int64, next http.Handler) http.Handler {
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		}
		next.ServeHTTP(w, r)
	})
}

type rateWindow struct {
	start time.Time
	count int
}

type requestRateLimiter struct {
	mu      sync.Mutex
	limit   int
	windows map[string]rateWindow
}

func newRequestRateLimiter(limit int) *requestRateLimiter {
	if limit <= 0 {
		limit = 120
	}
	return &requestRateLimiter{limit: limit, windows: make(map[string]rateWindow)}
}

func (l *requestRateLimiter) allow(keys []string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.windows) >= 10000 {
		for key, window := range l.windows {
			if now.Sub(window.start) >= time.Minute {
				delete(l.windows, key)
			}
		}
	}

	seen := make(map[string]struct{}, len(keys))
	missing := 0
	for _, key := range keys {
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		window, exists := l.windows[key]
		if !exists {
			missing++
			continue
		}
		if now.Sub(window.start) < time.Minute && window.count >= l.limit {
			return false
		}
	}
	if len(l.windows)+missing > 10000 {
		return false
	}

	for key := range seen {
		window := l.windows[key]
		if window.start.IsZero() || now.Sub(window.start) >= time.Minute {
			window = rateWindow{start: now}
		}
		window.count++
		l.windows[key] = window
	}
	return true
}

func requestIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || host == "" {
		return r.RemoteAddr
	}
	return host
}

func withIPRateLimit(limit int, next http.Handler) http.Handler {
	limiter := newRequestRateLimiter(limit)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/") || r.URL.Path == "/api/v1/meta" {
			next.ServeHTTP(w, r)
			return
		}
		if !limiter.allow([]string{"ip:" + requestIP(r)}, time.Now()) {
			w.Header().Set("Retry-After", "60")
			writeError(w, r, http.StatusTooManyRequests, "rate_limit_exceeded", "The API request limit has been reached; retry shortly")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withSubjectRateLimit(limit int, next http.Handler) http.Handler {
	limiter := newRequestRateLimiter(limit)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/") || r.URL.Path == "/api/v1/meta" {
			next.ServeHTTP(w, r)
			return
		}
		principal, ok := principalFromContext(r.Context())
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		key := "subject:" + principal.Issuer + "|" + principal.Subject
		if !limiter.allow([]string{key}, time.Now()) {
			w.Header().Set("Retry-After", "60")
			writeError(w, r, http.StatusTooManyRequests, "rate_limit_exceeded", "The authenticated subject request limit has been reached; retry shortly")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}
