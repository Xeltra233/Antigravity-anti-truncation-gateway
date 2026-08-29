package metrics

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

type Metrics struct {
	RequestsTotal           atomic.Uint64
	SyntheticHits           atomic.Uint64
	SyntheticRepairs        atomic.Uint64
	SyntheticConflicts      atomic.Uint64
	SyntheticRetries        atomic.Uint64
	ActiveRequests          atomic.Int64
	OverloadRejectionsTotal atomic.Uint64
	KeyAuthFailuresTotal    atomic.Uint64
}

var Default = &Metrics{}

func (m *Metrics) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, "# HELP gateway_requests_total Total number of gateway requests\n")
		fmt.Fprintf(w, "# TYPE gateway_requests_total counter\n")
		fmt.Fprintf(w, "gateway_requests_total %d\n\n", m.RequestsTotal.Load())

		fmt.Fprintf(w, "# HELP synthetic_tool_hit_total Total requests that used synthetic transport tool\n")
		fmt.Fprintf(w, "# TYPE synthetic_tool_hit_total counter\n")
		fmt.Fprintf(w, "synthetic_tool_hit_total %d\n\n", m.SyntheticHits.Load())

		fmt.Fprintf(w, "# HELP synthetic_repair_total Total repaired synthetic arguments\n")
		fmt.Fprintf(w, "# TYPE synthetic_repair_total counter\n")
		fmt.Fprintf(w, "synthetic_repair_total %d\n\n", m.SyntheticRepairs.Load())

		fmt.Fprintf(w, "# HELP synthetic_content_conflict_total Total responses with discarded standard content\n")
		fmt.Fprintf(w, "# TYPE synthetic_content_conflict_total counter\n")
		fmt.Fprintf(w, "synthetic_content_conflict_total %d\n\n", m.SyntheticConflicts.Load())

		fmt.Fprintf(w, "# HELP synthetic_retry_total Total recovery retries performed\n")
		fmt.Fprintf(w, "# TYPE synthetic_retry_total counter\n")
		fmt.Fprintf(w, "synthetic_retry_total %d\n\n", m.SyntheticRetries.Load())

		fmt.Fprintf(w, "# HELP gateway_active_requests Current active in-flight requests\n")
		fmt.Fprintf(w, "# TYPE gateway_active_requests gauge\n")
		fmt.Fprintf(w, "gateway_active_requests %d\n\n", m.ActiveRequests.Load())

		fmt.Fprintf(w, "# HELP gateway_overload_rejections_total Total requests rejected due to concurrency limits\n")
		fmt.Fprintf(w, "# TYPE gateway_overload_rejections_total counter\n")
		fmt.Fprintf(w, "gateway_overload_rejections_total %d\n\n", m.OverloadRejectionsTotal.Load())

		fmt.Fprintf(w, "# HELP gateway_key_auth_failures_total Total key auth failures\n")
		fmt.Fprintf(w, "# TYPE gateway_key_auth_failures_total counter\n")
		fmt.Fprintf(w, "gateway_key_auth_failures_total %d\n", m.KeyAuthFailuresTotal.Load())
	}
}
