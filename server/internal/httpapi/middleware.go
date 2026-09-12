package httpapi

import (
	"net/http"
	"time"

	"tgcloud/server/internal/log"
)

func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &logWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(lw, r)
		log.F("http", "method", r.Method, "path", r.URL.Path, "status", lw.status,
			"dur", time.Since(start).String())
	})
}

type logWriter struct {
	http.ResponseWriter
	status int
}

func (lw *logWriter) WriteHeader(code int) {
	lw.status = code
	lw.ResponseWriter.WriteHeader(code)
}