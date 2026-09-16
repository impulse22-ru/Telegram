package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"tgcloud/server/internal/log"
)

// requestLog — middleware логирования и сбора метрик. Оборачивает ResponseWriter
// для перехвата HTTP-кода ответа, логирует method, path, status, dur,
// а также увеличивает счётчик метрик по ключу "METHOD /path".
func (s *Server) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &logWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(lw, r)
		s.metrics.inc(r.Method+" "+r.URL.Path, strconv.Itoa(lw.status), time.Since(start))
		log.F("http", "method", r.Method, "path", r.URL.Path, "status", lw.status,
			"dur", time.Since(start).String())
	})
}

// logWriter — обёртка над http.ResponseWriter, запоминающая записанный HTTP-код
// статуса для целей логирования (стандартный WriteHeader не возвращает код).
type logWriter struct {
	http.ResponseWriter
	status int
}

// WriteHeader — перехватывает запись статус-кода и сохраняет его в поле status.
func (lw *logWriter) WriteHeader(code int) {
	lw.status = code
	lw.ResponseWriter.WriteHeader(code)
}

// corsMW — включение CORS для API-клиентов (возможность работать с веб/mobile-web).
func corsMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		// Preflight-запросы OPTIONS отвечают 204 No Content без вызова хендлера.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
