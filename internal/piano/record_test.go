package piano

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"ggosu/internal/chart"
)

func recordFixture(t *testing.T) (*chart.Chart, Record) {
	t.Helper()
	c := chart.New(4)
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: time.Second, End: time.Second, Kind: "tap"}, {At: 2 * time.Second, End: 3 * time.Second, Kind: "hold", Lane: 1}}
	f, err := Auto(c, Rules)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSession(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range f.Events {
		if err := s.Input(e.At, int(e.Key), e.Down); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Advance(f.Duration); err != nil {
		t.Fatal(err)
	}
	r := Record{Version: RecordVersion, ID: "test", Date: time.Now().UTC(), Metadata: c.Metadata, Mode: "auto", Completed: true, Rate: 1, Score: s.Score, Accuracy: s.Score.Accuracy(), Replay: f}
	return c, r
}

func TestPitchRoundTripAndLegacyDefault(t *testing.T) {
	_, r := recordFixture(t)
	if !r.PitchEnabled() {
		t.Fatal("legacy pitch default changed")
	}
	for _, pitch := range []bool{false, true} {
		r.Pitch = &pitch
		path := filepath.Join(t.TempDir(), "pitch.json")
		if err := SaveRecord(path, r); err != nil {
			t.Fatal(err)
		}
		saved, err := ReadRecord(path)
		if err != nil {
			t.Fatal(err)
		}
		if saved.PitchEnabled() != pitch || saved.Pitch == nil || !reflect.DeepEqual(saved.Replay, r.Replay) {
			t.Fatal("pitch or original replay coordinates changed")
		}
	}
}

func TestLegacyResultRemainsReadableAndReplayUsesNewScore(t *testing.T) {
	c, r := recordFixture(t)
	r.Version = 1
	r.Score = Score{Kool: 3, Points: 3000, Total: 3, Combo: 3, MaxCombo: 3}
	r.Accuracy = 1
	path := filepath.Join(t.TempDir(), "old.json")
	if err := SaveRecord(path, r); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	old, err := ReadRecord(path)
	if err != nil || old.Score.Points != 3000 || old.Version != 1 {
		t.Fatalf("legacy result: %+v, %v", old, err)
	}
	s, err := NewSession(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range old.Replay.Events {
		if err := s.Input(e.At, int(e.Key), e.Down); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Advance(old.Replay.Duration); err != nil {
		t.Fatal(err)
	}
	if s.Score.Points != MaxScore {
		t.Fatalf("legacy inputs did not use current score: %+v", s.Score)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("rewrote old record")
	}
}

func TestStoredPermutationRoundTrip(t *testing.T) {
	c, r := recordFixture(t)
	m, err := chart.NewMod(c, "random", 4294967295)
	if err != nil {
		t.Fatal(err)
	}
	r.Replay.Mods = m
	path := filepath.Join(t.TempDir(), "mod.json")
	if err := SaveRecord(path, r); err != nil {
		t.Fatal(err)
	}
	got, err := ReadRecord(path)
	if err != nil || !reflect.DeepEqual(got.Replay.Mods, m) {
		t.Fatalf("lost stored mapping: %+v, %v", got.Replay.Mods, err)
	}
	if err := got.Replay.Match(c, Rules); err != nil {
		t.Fatal(err)
	}
}

func TestSaveAndReplay(t *testing.T) {
	c, r := recordFixture(t)
	path := filepath.Join(t.TempDir(), "result.json")
	if err := SaveRecord(path, r); err != nil {
		t.Fatal(err)
	}
	got, err := ReadRecord(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, r) {
		t.Fatalf("result changed: %+v", got)
	}
	if err := got.Replay.Match(c, Rules); err != nil {
		t.Fatal(err)
	}
	s, _ := NewSession(c)
	for _, e := range got.Replay.Events {
		if err := s.Input(e.At, int(e.Key), e.Down); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Advance(got.Replay.Duration); err != nil {
		t.Fatal(err)
	}
	if s.Score != r.Score {
		t.Fatalf("replay score differs: %+v", s.Score)
	}
}

func TestInvalidResultCannotReplaceRecord(t *testing.T) {
	_, r := recordFixture(t)
	path := filepath.Join(t.TempDir(), "result.json")
	if err := SaveRecord(path, r); err != nil {
		t.Fatal(err)
	}
	r.Score.Points++
	if SaveRecord(path, r) == nil {
		t.Fatal("saved inconsistent score")
	}
	if _, err := ReadRecord(path); err != nil {
		t.Fatalf("damaged previous result: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"version":1`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRecord(path); err == nil {
		t.Fatal("accepted truncated result")
	}
}

func TestPauseEventsSurviveSavedRecord(t *testing.T) {
	_, r := recordFixture(t)
	r.Replay.Pauses = []Pause{
		{At: time.Second, Elapsed: time.Second, Action: "pause", Reason: "focus", Rate: 1},
		{At: time.Second, Elapsed: 2 * time.Second, Action: "resume-request", Reason: "manual", Rate: 1},
		{At: time.Second, Elapsed: 3 * time.Second, Action: "resume-cancel", Reason: "escape", Rate: 1},
		{At: time.Second, Elapsed: 4 * time.Second, Action: "resume-request", Reason: "manual", Rate: 1},
		{At: time.Second, Elapsed: 7 * time.Second, Action: "resume", Reason: "countdown-complete", Rate: 1},
	}
	path := filepath.Join(t.TempDir(), "paused.json")
	if err := SaveRecord(path, r); err != nil {
		t.Fatal(err)
	}
	got, err := ReadRecord(path)
	if err != nil || !reflect.DeepEqual(got.Replay.Pauses, r.Replay.Pauses) {
		t.Fatal("lost pause metadata", err)
	}
}
