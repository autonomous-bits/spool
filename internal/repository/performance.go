package repository

import (
	"runtime"
	"sort"
	"sync"
	"time"
)

// PerformancePhase reports one accumulated opt-in diagnostic phase.
type PerformancePhase struct {
	Name           string        `json:"name"`
	Duration       time.Duration `json:"-"`
	DurationNanos  int64         `json:"durationNanos"`
	AllocBytes     uint64        `json:"allocBytes"`
	AllocObjects   uint64        `json:"allocObjects"`
	HeapAllocBytes uint64        `json:"heapAllocBytes"`
}

// PerformanceRecorder collects opt-in lifecycle diagnostics for tests and benchmarks.
type PerformanceRecorder struct {
	mu     sync.Mutex
	phases map[string]PerformancePhase
}

// NewPerformanceRecorder creates an empty recorder.
func NewPerformanceRecorder() *PerformanceRecorder {
	return &PerformanceRecorder{phases: make(map[string]PerformancePhase)}
}

// Measure records duration and Go runtime allocation deltas until the returned function is called.
func (r *PerformanceRecorder) Measure(name string) func() {
	if r == nil {
		return func() {}
	}
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	started := time.Now()
	return func() {
		var after runtime.MemStats
		runtime.ReadMemStats(&after)
		r.mu.Lock()
		defer r.mu.Unlock()
		phase := r.phases[name]
		phase.Name = name
		phase.Duration += time.Since(started)
		phase.DurationNanos = phase.Duration.Nanoseconds()
		phase.AllocBytes += after.TotalAlloc - before.TotalAlloc
		phase.AllocObjects += after.Mallocs - before.Mallocs
		if after.HeapAlloc > phase.HeapAllocBytes {
			phase.HeapAllocBytes = after.HeapAlloc
		}
		r.phases[name] = phase
	}
}

// Phases returns the recorded phases in a stable order.
func (r *PerformanceRecorder) Phases() []PerformancePhase {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	phases := make([]PerformancePhase, 0, len(r.phases))
	for _, phase := range r.phases {
		phases = append(phases, phase)
	}
	sort.Slice(phases, func(i, j int) bool { return phases[i].Name < phases[j].Name })
	return phases
}
