package play

import (
	"testing"
	"time"
)

func TestRetryRequiresHoldAndRelease(t *testing.T) {
	g := pauseGame(t, 1)
	for _, at := range []time.Duration{time.Second, time.Second + retryHold - time.Nanosecond} {
		if retried, err := g.retry(at, true); err != nil || retried || g.retries != 0 {
			t.Fatalf("retry before hold delay: %v %v", retried, err)
		}
	}
	if retried, err := g.retry(time.Second+retryHold, true); err != nil || !retried || g.retries != 1 {
		t.Fatalf("retry after hold delay: %v %v", retried, err)
	}
	if retried, err := g.retry(2*time.Second, true); err != nil || retried || g.retries != 1 {
		t.Fatalf("held key retried twice: %v %v", retried, err)
	}
	if _, err := g.retry(3*time.Second, false); err != nil {
		t.Fatal(err)
	}
	if _, err := g.retry(4*time.Second, true); err != nil {
		t.Fatal(err)
	}
	if retried, err := g.retry(4*time.Second+retryHold, true); err != nil || !retried || g.retries != 2 {
		t.Fatalf("released key could not retry again: %v %v", retried, err)
	}
}

func TestRetryReleaseAndMultiplayerCancelHold(t *testing.T) {
	g := pauseGame(t, 1)
	if _, err := g.retry(time.Second, true); err != nil {
		t.Fatal(err)
	}
	if _, err := g.retry(time.Second+retryHold/2, false); err != nil {
		t.Fatal(err)
	}
	if g.retryHeld || g.retryLatched {
		t.Fatal("release retained retry state")
	}
	g.network = &network{}
	if _, err := g.retry(2*time.Second, true); err != nil {
		t.Fatal(err)
	}
	if retried, err := g.retry(3*time.Second, true); err != nil || retried || g.retryHeld || g.retries != 0 {
		t.Fatalf("personal retry affected multiplayer: %v %v", retried, err)
	}
}
