package web

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
)

func TestValidateRecalculatesForgedScoreAndMarksMissingChart(t *testing.T) {
	h, id, _ := fixture(t)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest("GET", "/api/chart/"+id, nil))
	c, err := chart.Read(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	f, err := piano.Auto(c, piano.Rules)
	if err != nil {
		t.Fatal(err)
	}
	score, hit, err := piano.ReplayScore(c, f)
	if err != nil {
		t.Fatal(err)
	}
	r := piano.Record{Version: piano.RecordVersion, ID: "test", Date: time.Now().UTC(), Mode: "live", Completed: true, Rate: 1, Score: score, Accuracy: score.Accuracy(), Replay: f, HitError: hit, Verification: "replayed"}
	r.Replay.Events = nil
	request := func(r piano.Record) piano.Record {
		t.Helper()
		b, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", "/api/validate", bytes.NewReader(b)))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var saved piano.Record
		if err := json.Unmarshal(w.Body.Bytes(), &saved); err != nil {
			t.Fatal(err)
		}
		return saved
	}
	x := request(r)
	if x.Score.Points != 0 || x.Score.Miss != 1 || x.Verification != "repaired" {
		t.Fatalf("forgery passed: %+v", x)
	}
	r.Score.Points++
	x = request(r)
	if x.Score.Points != 0 || x.Verification != "repaired" {
		t.Fatal("could not repair an inconsistent score")
	}
	r.Score.Points--
	r.Replay.ChartHash = strings.Repeat("f", 64)
	x = request(r)
	if x.Verification != "unavailable" {
		t.Fatal("trusted stored verification flag")
	}
}
