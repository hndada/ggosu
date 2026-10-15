//go:build !js

package multi

import (
	"context"
	"ggosu/internal/multi"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ggosu/internal/clock"
)

func TestNativeClientClockAndClose(t *testing.T) {
	s, err := NewServer(Config{})
	if err != nil {
		t.Fatal(err)
	}
	h := httptest.NewServer(s)
	t.Cleanup(func() { _ = s.Close(); h.Close() })
	counter, err := clock.New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, err := multi.Dial(ctx, "ws"+strings.TrimPrefix(h.URL, "http")+"/ws", counter.Now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	var estimate multi.Clock
	for range 3 {
		if err := c.Send(multi.Request{Type: "clock"}); err != nil {
			t.Fatal(err)
		}
		for {
			select {
			case <-ctx.Done():
				t.Fatal("clock response timed out")
			case err := <-c.Errors():
				t.Fatal(err)
			case e := <-c.Events():
				if e.Type != "clock" {
					continue
				}
				if e.Clock == nil {
					t.Fatal("clock response omitted timestamp quartet")
				}
				if err := estimate.Observe(*e.Clock, e.At); err != nil {
					t.Fatal(err)
				}
			}
			break
		}
	}
	if _, ok := estimate.Estimate(counter.Now()); !ok {
		t.Fatal("native client could not synchronize with the server counter")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.Send(multi.Request{Type: "clock"}); err == nil {
		t.Fatal("closed client accepted a new request")
	}
}
