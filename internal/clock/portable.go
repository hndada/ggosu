//go:build !windows

package clock

import "time"

type Counter struct{ origin time.Time }

func New() (*Counter, error)          { return &Counter{origin: time.Now()}, nil }
func (*Counter) Frequency() int64     { return int64(time.Second) }
func (c *Counter) Now() time.Duration { return time.Since(c.origin) }
