package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

type wrappedResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *wrappedResponseWriter) WriteHeader(statusCode int) {
	w.ResponseWriter.WriteHeader(statusCode)
	w.statusCode = statusCode
}

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		wrw := &wrappedResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(wrw, r)
		slog.Info(
			"Request completed",
			"Method", r.Method,
			"Path", r.URL.Path,
			"StatusCode", wrw.statusCode,
			"Duration", time.Since(start),
		)
	})
}
