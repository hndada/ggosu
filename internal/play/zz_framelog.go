package play

// TEMPORARY frame diagnostics; remove after the stutter investigation.

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"sync"
	"time"
)

var zzLastDump time.Time

var zzLog = struct {
	sync.Mutex
	on                bool
	lastDraw, lastUpd time.Time
	draws, updates    []time.Duration
	updCost, drawCost []time.Duration
	start             time.Time
	big               []string
}{on: os.Getenv("GOSU_FRAMELOG") != ""}

func zzUpdate(start time.Time, cost time.Duration) {
	if !zzLog.on {
		return
	}
	if zzLog.start.IsZero() {
		zzLog.start = start
	}
	if !zzLog.lastUpd.IsZero() {
		gap := start.Sub(zzLog.lastUpd)
		zzLog.updates = append(zzLog.updates, gap)
		if gap > 12*time.Millisecond {
			zzLog.big = append(zzLog.big, fmt.Sprintf("upd gap %v at %v", gap, start.Sub(zzLog.start)))
		}
	}
	zzLog.lastUpd = start
	zzLog.updCost = append(zzLog.updCost, cost)
	if cost > 4*time.Millisecond {
		zzLog.big = append(zzLog.big, fmt.Sprintf("upd cost %v at %v", cost, start.Sub(zzLog.start)))
	}
}

func zzDraw(start time.Time, cost time.Duration) {
	if !zzLog.on {
		return
	}
	if !zzLog.lastDraw.IsZero() {
		gap := start.Sub(zzLog.lastDraw)
		zzLog.draws = append(zzLog.draws, gap)
		if gap > 12*time.Millisecond {
			zzLog.big = append(zzLog.big, fmt.Sprintf("draw gap %v at %v", gap, start.Sub(zzLog.start)))
		}
	}
	zzLog.lastDraw = start
	if time.Since(zzLastDump) > 5*time.Second {
		zzLastDump = time.Now()
		zzDump()
	}
	zzLog.drawCost = append(zzLog.drawCost, cost)
	if cost > 4*time.Millisecond {
		zzLog.big = append(zzLog.big, fmt.Sprintf("draw cost %v at %v", cost, start.Sub(zzLog.start)))
	}
}

func zzDump() {
	if !zzLog.on {
		return
	}
	f, err := os.Create(os.Getenv("GOSU_FRAMELOG"))
	if err != nil {
		return
	}
	defer f.Close()
	pct := func(name string, v []time.Duration) {
		if len(v) == 0 {
			return
		}
		s := append([]time.Duration(nil), v...)
		sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
		q := func(p float64) time.Duration { return s[min(len(s)-1, int(p*float64(len(s))))] }
		fmt.Fprintf(f, "%-9s n=%d p50=%v p90=%v p99=%v p99.9=%v max=%v\n", name, len(s), q(.5), q(.9), q(.99), q(.999), s[len(s)-1])
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Fprintf(f, "gc=%d pauseTotal=%v heap=%dMB alloc=%dMB numGoroutine=%d\n", m.NumGC, time.Duration(m.PauseTotalNs), m.HeapAlloc>>20, m.TotalAlloc>>20, runtime.NumGoroutine())
	pct("updGap", zzLog.updates)
	pct("updCost", zzLog.updCost)
	pct("drawGap", zzLog.draws)
	pct("drawCost", zzLog.drawCost)
	for i, b := range zzLog.big {
		if i >= 300 {
			break
		}
		fmt.Fprintln(f, b)
	}
}
