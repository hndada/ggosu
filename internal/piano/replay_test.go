package piano

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"

	"ggosu/internal/clock"
)

func TestRoundTripAndReplayWithDifferentPlayback(t *testing.T) {
	f := NewReplay()
	f.Duration = 3 * time.Second
	f.Events = []Event{{At: time.Second, Key: 'D', Down: true},
		{At: time.Second, Key: 'D'}, {At: 2 * time.Second, Key: 'F', Down: true},
		{At: 3 * time.Second, Key: 'F'}}
	var buf bytes.Buffer
	if err := WriteReplay(&buf, f); err != nil {
		t.Fatal(err)
	}
	loaded, err := ReadReplay(&buf)
	if err != nil || !reflect.DeepEqual(loaded, f) {
		t.Fatalf("round trip: %+v, %v", loaded, err)
	}
	p, err := NewPlayer(loaded)
	if err != nil {
		t.Fatal(err)
	}
	c := clock.NewTimeline(0)
	if err := c.SetRate(0, 2); err != nil {
		t.Fatal(err)
	}
	var all []Event
	consume := func(at time.Duration, want int) {
		t.Helper()
		before := len(all)
		all, err = p.AppendDue(all, c.Position(at))
		if err != nil || len(all)-before != want {
			t.Fatalf("at %s: added %d, want %d, err %v", at, len(all)-before, want, err)
		}
	}
	consume(499*time.Millisecond, 0)
	consume(500*time.Millisecond, 2) // Equal-time down/up retain order.
	if err := c.SetRate(500*time.Millisecond, 0); err != nil {
		t.Fatal(err)
	}
	consume(10*time.Second, 0)
	if err := c.SetRate(10*time.Second, 0.5); err != nil {
		t.Fatal(err)
	}
	consume(12*time.Second, 1)
	consume(14*time.Second, 1)
	consume(20*time.Second, 0)
	if !p.Done() || !reflect.DeepEqual(all, f.Events) {
		t.Fatal("playback changed event times or order")
	}
	if _, err := p.AppendDue(nil, 0); err == nil {
		t.Fatal("accepted a backwards seek")
	}
	p.Reset()
	all, err = p.AppendDue(nil, f.Duration)
	if err != nil || !reflect.DeepEqual(all, f.Events) {
		t.Fatal("reset did not reproduce all events")
	}
}

func TestInvalidFiles(t *testing.T) {
	for _, data := range []string{
		`{"version":3,"key_encoding":"windows-vk","duration_ns":0,"events":[]}`,
		`{"version":2,"key_encoding":"windows-vk","duration_ns":0,"events":[]}`,
		`{"version":2,"key_encoding":"windows-vk","time_base":"wall","duration_ns":0,"events":[]}`,
		`{"version":1,"key_encoding":"windows-vk","duration_ns":-1,"events":[]}`,
		`{"version":1,"key_encoding":"windows-vk","duration_ns":2,"events":[{"at_ns":-1}]}`,
		`{"version":1,"key_encoding":"windows-vk","duration_ns":2,"events":[{"at_ns":3}]}`,
		`{"version":1,"key_encoding":"windows-vk","duration_ns":2,"events":[{"at_ns":2},{"at_ns":1}]}`,
		`{"version":1,"key_encoding":"windows-vk","duration_ns":0,"typo":1}`,
		`{"version":1,"key_encoding":"windows-vk","duration_ns":0} {}`,
	} {
		if _, err := ReadReplay(strings.NewReader(data)); err == nil {
			t.Errorf("accepted %s", data)
		}
	}
}

func TestReadLegacyTimeBase(t *testing.T) {
	f, err := ReadReplay(strings.NewReader(`{"version":1,"key_encoding":"windows-vk","duration_ns":12000000000,"events":[{"at_ns":10000000000,"key":68,"down":true}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if f.Version != ReplayVersion || f.TimeBase != TimeBase || f.Duration != 12*time.Second ||
		len(f.Events) != 1 || f.Events[0].At != 10*time.Second {
		t.Fatalf("legacy upgrade changed the time coordinates: %+v", f)
	}
	var buf bytes.Buffer
	if err := WriteReplay(&buf, f); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"time_base": "game-1x"`) {
		t.Fatal("new format did not declare its time base")
	}
}
