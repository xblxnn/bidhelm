package runtime

import (
	"runtime"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"
)

var processStarted = time.Now()
var durationBounds = [...]float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 5}

const (
	resultBid = iota
	resultNoBid
	resultInvalid
	resultNotReady
	resultError
	resultCount
)

var resultNames = [...]string{"bid", "no_bid", "invalid", "not_ready", "server_error"}

// Fixed-size aggregates only. No request values, source IDs, or campaign IDs are retained.
type trafficMetrics struct {
	started    atomic.Uint64
	active     atomic.Int64
	outcomes   [resultCount]atomic.Uint64
	durationNS atomic.Uint64
	buckets    [len(durationBounds) + 1]atomic.Uint64
}

func (m *trafficMetrics) finish(start time.Time, outcome int) {
	elapsed := time.Since(start)
	m.durationNS.Add(uint64(elapsed))
	i := 0
	for i < len(durationBounds) && elapsed.Seconds() > durationBounds[i] {
		i++
	}
	m.buckets[i].Add(1)
	m.outcomes[outcome].Add(1)
	m.active.Add(-1)
}

type durationBucket struct {
	LE    string `json:"le_seconds"`
	Count uint64 `json:"count"`
}
type metricsSnapshot struct {
	Uptime    float64           `json:"uptime_seconds"`
	Started   uint64            `json:"requests_started_total"`
	Completed uint64            `json:"requests_completed_total"`
	Active    int64             `json:"requests_in_flight"`
	Outcomes  map[string]uint64 `json:"outcomes_total"`
	Duration  struct {
		Sum     float64          `json:"sum_seconds"`
		Count   uint64           `json:"count"`
		Buckets []durationBucket `json:"buckets"`
	} `json:"request_duration"`
	Go struct {
		Goroutines int     `json:"goroutines"`
		GOMAXPROCS int     `json:"gomaxprocs"`
		HeapAlloc  uint64  `json:"heap_alloc_bytes"`
		HeapInuse  uint64  `json:"heap_inuse_bytes"`
		Sys        uint64  `json:"sys_bytes"`
		AllocTotal uint64  `json:"alloc_bytes_total"`
		GCCycles   uint32  `json:"gc_cycles_total"`
		GCPause    float64 `json:"gc_pause_seconds_total"`
	} `json:"go"`
	CPU    *cpuMetrics `json:"cpu,omitempty"`
	Config Status      `json:"config"`
}
type cpuMetrics struct {
	User   float64 `json:"user_seconds_total"`
	System float64 `json:"system_seconds_total"`
}

func (e *Engine) Metrics() metricsSnapshot {
	m := &e.metrics
	out := metricsSnapshot{Uptime: time.Since(processStarted).Seconds(), Started: m.started.Load(), Active: m.active.Load(), Outcomes: map[string]uint64{}, Config: e.Status()}
	for i, name := range resultNames {
		n := m.outcomes[i].Load()
		out.Outcomes[name] = n
		out.Completed += n
	}
	out.Duration.Sum = float64(m.durationNS.Load()) / 1e9
	var count uint64
	for i := range m.buckets {
		count += m.buckets[i].Load()
		le := "+Inf"
		if i < len(durationBounds) {
			le = strconv.FormatFloat(durationBounds[i], 'g', -1, 64)
		}
		out.Duration.Buckets = append(out.Duration.Buckets, durationBucket{LE: le, Count: count})
	}
	out.Duration.Count = count
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	out.Go.Goroutines = runtime.NumGoroutine()
	out.Go.GOMAXPROCS = runtime.GOMAXPROCS(0)
	out.Go.HeapAlloc = mem.HeapAlloc
	out.Go.HeapInuse = mem.HeapInuse
	out.Go.Sys = mem.Sys
	out.Go.AllocTotal = mem.TotalAlloc
	out.Go.GCCycles = mem.NumGC
	out.Go.GCPause = float64(mem.PauseTotalNs) / 1e9
	var usage syscall.Rusage
	if syscall.Getrusage(syscall.RUSAGE_SELF, &usage) == nil {
		out.CPU = &cpuMetrics{User: float64(usage.Utime.Sec) + float64(usage.Utime.Usec)/1e6, System: float64(usage.Stime.Sec) + float64(usage.Stime.Usec)/1e6}
	}
	return out
}
