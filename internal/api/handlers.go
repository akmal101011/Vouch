package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/slyt3/Vouch/internal/assert"
	"github.com/slyt3/Vouch/internal/core"
	"github.com/slyt3/Vouch/internal/pool"
)

type Handlers struct {
	Core *core.Engine
}

func NewHandlers(engine *core.Engine) *Handlers {
	return &Handlers{Core: engine}
}

func (h *Handlers) HandleRekey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	adminToken := os.Getenv("VOUCH_ADMIN_TOKEN")
	if adminToken != "" {
		if r.Header.Get("X-Admin-Token") != adminToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}
	oldPubKey, newPubKey, err := h.Core.Worker.GetSigner().RotateKey(".vouch_key")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, err := fmt.Fprintf(w, "Key rotated\nOld: %s\nNew: %s", oldPubKey, newPubKey); err != nil {
		log.Printf("Failed to write response: %v", err)
	}
}

func (h *Handlers) HandleStats(w http.ResponseWriter, r *http.Request) {
	metrics := pool.GetMetrics()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(metrics); err != nil {
		log.Printf("Failed to encode JSON: %v", err)
	}
}

func (h *Handlers) HandlePrometheus(w http.ResponseWriter, r *http.Request) {
	poolMetrics := pool.GetMetrics()
	proc, drop := h.Core.Worker.Stats()
	tasks := 0
	const maxActiveTasks = 10000
	h.Core.ActiveTasks.Range(func(_, _ interface{}) bool {
		if tasks >= maxActiveTasks {
			return false
		}
		tasks++
		return true
	})
	if err := assert.Check(tasks <= maxActiveTasks, "active tasks exceeded cap: %d", tasks); err != nil {
		log.Printf("[WARN] active tasks exceeded cap: %d", tasks)
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP vouch_pool_event_hits_total Total hits on the event pool\n")
	fmt.Fprintf(w, "# TYPE vouch_pool_event_hits_total counter\n")
	fmt.Fprintf(w, "vouch_pool_event_hits_total %d\n", poolMetrics.EventHits)

	fmt.Fprintf(w, "# HELP vouch_pool_event_misses_total Total misses (allocations) in the event pool\n")
	fmt.Fprintf(w, "# TYPE vouch_pool_event_misses_total counter\n")
	fmt.Fprintf(w, "vouch_pool_event_misses_total %d\n", poolMetrics.EventMisses)

	fmt.Fprintf(w, "# HELP vouch_ledger_events_processed_total Total events successfully written to the ledger\n")
	fmt.Fprintf(w, "# TYPE vouch_ledger_events_processed_total counter\n")
	fmt.Fprintf(w, "vouch_ledger_events_processed_total %d\n", proc)

	fmt.Fprintf(w, "# HELP vouch_ledger_events_dropped_total Total events dropped due to backpressure\n")
	fmt.Fprintf(w, "# TYPE vouch_ledger_events_dropped_total counter\n")
	fmt.Fprintf(w, "vouch_ledger_events_dropped_total %d\n", drop)

	fmt.Fprintf(w, "# HELP vouch_engine_active_tasks_total Number of currently active causal tasks\n")
	fmt.Fprintf(w, "# TYPE vouch_engine_active_tasks_total gauge\n")
	fmt.Fprintf(w, "vouch_engine_active_tasks_total %d\n", tasks)
}
