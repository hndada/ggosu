package menu

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
	"ggosu/internal/score"
	"ggosu/internal/server/scores"
)

func submissionPlay(t *testing.T, a *App) (piano.Record, string, *chart.Chart) {
	t.Helper()
	c := chart.New(4)
	c.Metadata.Title = "Silent submission fixture"
	c.Timing = []chart.Timing{{BPM: 120, Scroll: 1, Meter: 4}}
	c.Notes = []chart.Note{{At: time.Second, End: time.Second, Kind: "tap"}}
	registerChart(t, a, c)
	f, err := piano.Auto(c, piano.Rules)
	if err != nil {
		t.Fatal(err)
	}
	f.Mods.Visibility = "hidden"
	s, h, err := piano.ReplayScore(c, f)
	if err != nil {
		t.Fatal(err)
	}
	r := piano.Record{Version: piano.RecordVersion, ID: "0123456789abcdef0123456789abcdef", Date: time.Now().UTC(), Metadata: c.Metadata, Mode: "live", Completed: true, Rate: 1, Score: s, Accuracy: s.Accuracy(), Replay: f, HitError: h}
	path, err := a.replayPath(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := piano.SaveRecord(path, r); err != nil {
		t.Fatal(err)
	}
	return r, path, c
}

func submissionEvents(a *App) <-chan struct{} {
	done := make(chan struct{}, 8)
	a.onChange = func() {
		select {
		case done <- struct{}{}:
		default:
		}
	}
	return done
}

func waitDelivery(t *testing.T, a *App, id, state string, events <-chan struct{}) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		dir, err := a.submissionDir()
		if err != nil {
			t.Fatal(err)
		}
		j, err := readSubmission(filepath.Join(dir, id+".json"))
		if err == nil && j.Status.State == state {
			return
		}
		select {
		case <-events:
		case <-timer.C:
			t.Fatalf("delivery did not become %s: %+v %v", state, j, err)
		}
	}
}

func TestFinishPlayQueuesWithoutBlockingAndRetriesAfterRelaunch(t *testing.T) {
	a := &App{dataDir: filepath.Join(t.TempDir(), "records")}
	t.Cleanup(func() {
		if err := a.close(); err != nil {
			t.Error(err)
		}
	})
	r, path, c := submissionPlay(t, a)
	dir := t.TempDir()
	s, err := scores.New(dir, func(hash string) (*chart.Chart, bool) { return c, hash == r.Replay.ChartHash })
	if err != nil {
		t.Fatal(err)
	}
	var offline atomic.Bool
	offline.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if offline.Load() {
			http.Error(w, "offline", 503)
			return
		}
		s.ServeHTTP(w, req)
	}))
	defer server.Close()
	if _, err := a.SaveScoreServer(server.URL); err != nil {
		t.Fatal(err)
	}
	events := submissionEvents(a)
	a.mu.Lock()
	a.finishPlay(launch{resultPath: path, lanes: 4, selection: Selection{Mode: "live"}}, nil)
	a.mu.Unlock()
	if result := a.State().Result; result == nil || result.Score.Points != 1100000 || result.Submission == nil {
		t.Fatal("submission replaced local result", result)
	}
	waitDelivery(t, a, r.ID, "failed", events)
	if _, err := os.Stat(path); err != nil {
		t.Fatal("failed submission lost local record", err)
	}
	if err := a.close(); err != nil {
		t.Fatal(err)
	}
	offline.Store(false)
	after := &App{dataDir: a.dataDir}
	registerChart(t, after, c)
	events = submissionEvents(after)
	t.Cleanup(func() {
		if err := after.close(); err != nil {
			t.Error(err)
		}
	})
	if err := after.startSubmissions(); err != nil {
		t.Fatal(err)
	}
	waitDelivery(t, after, r.ID, "sent", events)
	past, err := after.Submission(r.ID)
	if err != nil || past.State != "sent" || past.Receipt == nil {
		t.Fatal("past result lost delivery status", past, err)
	}
	status, err := after.RetrySubmission(r.ID)
	if err != nil || status.State != "sent" {
		t.Fatal("accepted score was sent again", status, err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatal("server did not persist exactly one record", files, err)
	}
}

func TestCloseCancelsInFlightSubmissionAndKeepsPendingJob(t *testing.T) {
	a := &App{dataDir: filepath.Join(t.TempDir(), "records")}
	r, _, _ := submissionPlay(t, a)
	entered := make(chan struct{})
	stop := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var q score.Request
		if err := json.NewDecoder(req.Body).Decode(&q); err != nil {
			t.Error(err)
			return
		}
		close(entered)
		select {
		case <-req.Context().Done():
		case <-stop:
		}
	}))
	defer server.Close()
	defer close(stop)
	if _, err := a.SaveScoreServer(server.URL); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	status := a.queueScore(r)
	a.mu.Unlock()
	if status.State != "pending" {
		t.Fatal(status)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("submission never started")
	}
	start := time.Now()
	if err := a.close(); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("close waited for HTTP timeout")
	}
	dir, err := a.submissionDir()
	if err != nil {
		t.Fatal(err)
	}
	j, err := readSubmission(filepath.Join(dir, r.ID+".json"))
	if err != nil || j.Status.State != "pending" {
		t.Fatal("canceled request lost the outbox", j, err)
	}
}

func TestSubmissionExcludesAutoReplayAndDisabledServer(t *testing.T) {
	a := &App{dataDir: filepath.Join(t.TempDir(), "records")}
	r, _, _ := submissionPlay(t, a)
	if _, err := a.SaveScoreServer(""); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	status := a.queueScore(r)
	a.mu.Unlock()
	if status.State != "disabled" {
		t.Fatal(status)
	}
	for _, mode := range []string{"auto", "replay"} {
		r.Mode = mode
		a.mu.Lock()
		status := a.queueScore(r)
		a.mu.Unlock()
		if status != nil {
			t.Fatal("queued", mode)
		}
	}
	if a.submitDone != nil {
		t.Fatal("excluded plays started a worker")
	}
}

func TestFinishedScoreArrivingDuringShutdownRemainsQueued(t *testing.T) {
	a := &App{dataDir: filepath.Join(t.TempDir(), "records")}
	r, _, _ := submissionPlay(t, a)
	if err := a.close(); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	status := a.queueScore(r)
	a.mu.Unlock()
	if status == nil || status.State != "pending" || a.submitDone != nil {
		t.Fatal("shutdown launched work or dropped a play", status)
	}
	dir, err := a.submissionDir()
	if err != nil {
		t.Fatal(err)
	}
	j, err := readSubmission(filepath.Join(dir, r.ID+".json"))
	if err != nil || j.Status.State != "pending" {
		t.Fatal(j, err)
	}
}
