package web

import (
	"fmt"
	"io"
	"sort"
	"sync"
	"time"
)

// metrics is a tiny hand-rolled Prometheus exposition. No dependency: the app
// exposes four counters and one duration summary, which is not worth a library.
type metrics struct {
	mu       sync.Mutex
	requests map[string]int64   // "path|status" -> count
	toolRuns map[string]int64   // "tool|outcome" -> count
	durSum   map[string]float64 // path -> seconds
	durCount map[string]int64
}

func newMetrics() *metrics {
	return &metrics{
		requests: make(map[string]int64),
		toolRuns: make(map[string]int64),
		durSum:   make(map[string]float64),
		durCount: make(map[string]int64),
	}
}

func (m *metrics) observe(path string, status int, d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests[fmt.Sprintf("%s|%d", path, status)]++
	m.durSum[path] += d.Seconds()
	m.durCount[path]++
}

func (m *metrics) toolRun(name, outcome string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolRuns[name+"|"+outcome]++
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (m *metrics) write(w io.Writer) {
	m.mu.Lock()
	defer m.mu.Unlock()

	fmt.Fprintln(w, "# HELP tools_http_requests_total HTTP requests by path template and status.")
	fmt.Fprintln(w, "# TYPE tools_http_requests_total counter")
	for _, k := range sortedKeys(m.requests) {
		path, status, _ := cut(k, '|')
		fmt.Fprintf(w, "tools_http_requests_total{path=%q,status=%q} %d\n", path, status, m.requests[k])
	}

	fmt.Fprintln(w, "# HELP tools_tool_runs_total Tool executions by tool and outcome.")
	fmt.Fprintln(w, "# TYPE tools_tool_runs_total counter")
	for _, k := range sortedKeys(m.toolRuns) {
		name, outcome, _ := cut(k, '|')
		fmt.Fprintf(w, "tools_tool_runs_total{tool=%q,outcome=%q} %d\n", name, outcome, m.toolRuns[k])
	}

	fmt.Fprintln(w, "# HELP tools_request_duration_seconds Request duration by path template.")
	fmt.Fprintln(w, "# TYPE tools_request_duration_seconds summary")
	for _, path := range sortedKeys(m.durSum) {
		fmt.Fprintf(w, "tools_request_duration_seconds_sum{path=%q} %g\n", path, m.durSum[path])
		fmt.Fprintf(w, "tools_request_duration_seconds_count{path=%q} %d\n", path, m.durCount[path])
	}
}

func cut(s string, sep byte) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			return s[:i], s[i+1:], true
		}
	}
	return s, "", false
}
