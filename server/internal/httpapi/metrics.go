package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// simple metrics в Prometheus text format (этап 5 — мониторинг).
// metrics — потокобезопасный счётчик HTTP-запросов по ключу "METHOD /path" (MAP),
// защищённый mutex'ом. Хранит время старта для расчёта uptime и суммарную длительность.
type metrics struct {
	mu      sync.Mutex
	started time.Time
	reqs    map[string]int64   // path -> count
	status  map[string]int64   // status -> count
	durSum  map[string]float64 // path -> суммарное время (сек)
}

// newMetrics — конструктор metrics; стартует отсчёт uptime.
func newMetrics() *metrics {
	return &metrics{
		started: time.Now(),
		reqs:    make(map[string]int64),
		status:  make(map[string]int64),
		durSum:  make(map[string]float64),
	}
}

// inc — увеличивает счётчик запросов для пути (вызывается из requestLog).
func (m *metrics) inc(path, status string, d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reqs[path]++
	m.status[status]++
	m.durSum[path] += d.Seconds()
}

// snapshot — потокобезопасно снимает срез метрик: uptime в секундах и строки
// Prometheus-меток http_requests_total по каждому пути, плюс статусы и длительности.
func (m *metrics) snapshot() (uptime int64, lines []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	uptime = int64(time.Since(m.started).Seconds())
	for p, c := range m.reqs {
		lines = append(lines, fmt.Sprintf("http_requests_total{path=%q} %d", p, c))
	}
	for st, c := range m.status {
		lines = append(lines, fmt.Sprintf("http_responses_by_status{status=%q} %d", st, c))
	}
	for p, s := range m.durSum {
		c := m.reqs[p]
		avg := "0"
		if c > 0 {
			avg = fmt.Sprintf("%.4f", s/float64(c))
		}
		lines = append(lines, fmt.Sprintf("http_request_duration_seconds_sum{path=%q} %.4f", p, s))
		lines = append(lines, fmt.Sprintf("http_request_duration_seconds_avg{path=%q} %s", p, avg))
	}
	return uptime, lines
}

// handleMetrics — GET /metrics. Отдаёт метрики в Prometheus text format
// (без авторизации — эндпоинт для систем мониторинга).
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	uptime, lines := s.metrics.snapshot()
	var b strings.Builder
	b.WriteString("# HELP http_requests_total Total HTTP requests by path.\n")
	b.WriteString("# TYPE http_requests_total counter\n")
	for _, l := range lines {
		b.WriteString(l)
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "uptime_seconds %d\n", uptime)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Write([]byte(b.String()))
}
