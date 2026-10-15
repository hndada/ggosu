package main

import (
	"slices"
	"time"
)

const sampleCount = 4096

type latency struct {
	n      uint64
	total  time.Duration
	max    time.Duration
	values [sampleCount]time.Duration
}

func (l *latency) add(d time.Duration) {
	if d < 0 {
		d = 0
	}
	l.values[l.n%sampleCount] = d
	l.n++
	l.total += d
	if d > l.max {
		l.max = d
	}
}

type distribution struct {
	Count   uint64  `json:"count"`
	Samples int     `json:"percentile_samples"`
	MeanUS  float64 `json:"mean_us"`
	P50US   float64 `json:"p50_us"`
	P95US   float64 `json:"p95_us"`
	P99US   float64 `json:"p99_us"`
	MaxUS   float64 `json:"max_us"`
}

func (l latency) summary() distribution {
	if l.n == 0 {
		return distribution{}
	}
	n := min(int(l.n), sampleCount)
	values := l.values[:n]
	slices.Sort(values)
	percentile := func(p int) float64 {
		return float64(values[(n*p+99)/100-1]) / float64(time.Microsecond)
	}
	return distribution{
		Count: l.n, Samples: n,
		MeanUS: float64(l.total) / float64(l.n) / float64(time.Microsecond),
		P50US:  percentile(50), P95US: percentile(95), P99US: percentile(99),
		MaxUS: float64(l.max) / float64(time.Microsecond),
	}
}
