package multi

import (
	"testing"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/multi"
	"ggosu/internal/piano"
)

func canonical(t *testing.T) (*chart.Chart, multi.Selection, piano.Replay) {
	t.Helper()
	c := chart.New(4)
	c.Metadata.Title = "Silent verification fixture"
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: time.Second, End: time.Second, Kind: "tap"}, {At: 2 * time.Second, End: 3 * time.Second, Kind: "hold", Lane: 1}}
	v, err := multi.Describe(c, chart.Mod{}, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	f, err := piano.Auto(c, piano.Rules)
	if err != nil {
		t.Fatal(err)
	}
	f.Duration = time.Duration(v.DurationNS)
	return c, v, f
}

func TestVerifiedMatchRejectsForgeryAndRetainsState(t *testing.T) {
	c, v, replay := canonical(t)
	v.Mods.Visibility, replay.Mods.Visibility = "hidden", "hidden"
	f := newFixture(t, Config{Charts: []*chart.Chart{c}})
	host, guest := f.peer("host"), f.peer("guest")
	host.ok(multi.Request{Type: "create", Name: "verified", Capacity: 2})
	guest.ok(multi.Request{Type: "join", RoomID: host.room.ID})
	bad := v
	bad.Judgements++
	host.reject(multi.Request{Type: "select", Selection: &bad})
	host.ok(multi.Request{Type: "select", Selection: &v})
	// Mutating the caller's chart after NewServer cannot change its catalogue.
	c.Notes[0].Lane = 3
	f.ready(host)
	f.ready(guest)
	host.ok(multi.Request{Type: "start"})
	f.arm(host)
	f.arm(guest)
	host.ok(multi.Request{Type: "clock"})
	r := *host.room
	if !r.VerifyResults {
		t.Fatal("verification mode was not announced")
	}
	f.clock.Store(r.StartAt.Add(time.Duration(v.DurationNS - v.BeginNS)).UnixNano())
	// A perfect report is structurally valid; the empty replay says all MISS.
	f.s.mu.Lock()
	serverChart := f.s.charts[v.ChartHash]
	f.s.mu.Unlock()
	perfect, _, err := piano.ReplayScore(serverChart, replay)
	if err != nil {
		t.Fatal(err)
	}
	empty := replay
	empty.Events = nil
	q := multi.Request{Type: "finish", MatchID: r.MatchID, Sequence: 1, PositionNS: v.DurationNS, Score: &perfect, Replay: &empty}
	host.reject(q)
	host.ok(multi.Request{Type: "clock"})
	if host.room.Members[0].Score.Total != 0 || host.room.Members[0].Verified || host.room.Members[0].Status != "playing" {
		t.Fatalf("forgery changed state: %+v", host.room.Members[0])
	}
	q.Replay = nil
	host.reject(q)
	wrong := replay
	wrong.Offsets.GlobalMS = 1
	q.Replay = &wrong
	host.reject(q)
	wrong = replay
	wrong.Duration--
	q.Replay = &wrong
	host.reject(q)
	wrong = replay
	wrong.Mods.Visibility = "cover"
	q.Replay = &wrong
	host.reject(q)
	q.Replay = &replay
	host.ok(q)
	if !host.room.Members[0].Verified || host.room.Members[0].Score != perfect {
		t.Fatal("valid result was not verified")
	}
	// A proof larger than the ordinary 16 KiB request limit is accepted through
	// the real socket path. Repeated transitions before the notes earn no hits.
	for i := 0; i < 3000; i++ {
		empty.Events = append(empty.Events, piano.Event{Key: 2, Down: i%2 == 0})
	}
	miss, _, err := piano.ReplayScore(serverChart, empty)
	if err != nil {
		t.Fatal(err)
	}
	q.Score, q.Replay = &miss, &empty
	guest.ok(q)
	if guest.room.Phase != "finished" || !guest.room.Members[1].Verified {
		t.Fatal("valid large proof was lost")
	}
	host.ok(multi.Request{Type: "reset"})
	if host.room.Members[0].Verified || !host.room.VerifyResults {
		t.Fatal("verification state did not reset")
	}
}

func TestCanonicalCatalogueLimits(t *testing.T) {
	if _, err := NewServer(Config{Charts: []*chart.Chart{}}); err == nil {
		t.Fatal("empty verification catalogue fell back to provisional")
	}
	if _, err := NewServer(Config{Charts: []*chart.Chart{nil}}); err == nil {
		t.Fatal("accepted nil chart")
	}
	c, v, _ := canonical(t)
	s, err := NewServer(Config{Charts: []*chart.Chart{c}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, change := range []func(*multi.Selection){
		func(v *multi.Selection) { v.DurationNS++ },
		func(v *multi.Selection) { v.BeginNS++ },
		func(v *multi.Selection) { v.Lanes++ },
		func(v *multi.Selection) { v.ChartHash = selection().ChartHash },
	} {
		bad := v
		change(&bad)
		if err := s.checkSelection(bad); err == nil {
			t.Fatalf("accepted forged selection: %+v", bad)
		}
	}
}
