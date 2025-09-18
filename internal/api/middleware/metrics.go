package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/rohit/cdc-pipeline/internal/metrics"
)

// Metrics returns a middleware that tracks HTTP metrics
func Metrics(m *metrics.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Increment in-flight requests
			m.HTTPRequestsInFlight.Inc()
			defer m.HTTPRequestsInFlight.Dec()

			// Wrap response writer to capture status code
			wrapped := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			// Process request
			next.ServeHTTP(wrapped, r)

			// Record metrics
			duration := time.Since(start).Seconds()
			statusCode := strconv.Itoa(wrapped.statusCode)

			m.HTTPRequestsTotal.WithLabelValues(
				r.Method,
				r.URL.Path,
				statusCode,
			).Inc()

			m.HTTPRequestDuration.WithLabelValues(
				r.Method,
				r.URL.Path,
			).Observe(duration)
		})
	}
}
