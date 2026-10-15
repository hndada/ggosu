package web

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/piano"
	"ggosu/internal/score"
)

func TestWebScoreRouteUsesCanonicalCatalogueAndPersistentStore(t *testing.T) {
	h, id, root := fixture(t)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/chart/"+id, nil))
	c, err := chart.Read(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	f, err := piano.Auto(c, piano.Rules)
	if err != nil {
		t.Fatal(err)
	}
	s, hit, err := piano.ReplayScore(c, f)
	if err != nil {
		t.Fatal(err)
	}
	r := piano.Record{Version: piano.RecordVersion, ID: "web-test", Date: time.Now().UTC(), Metadata: c.Metadata, Mode: "live", Completed: true, Rate: 1, Replay: f, Score: s, Accuracy: s.Accuracy(), HitError: hit}
	server := httptest.NewServer(h)
	defer server.Close()
	receipt, err := score.Submit(context.Background(), server.URL, score.Request{Player: "web", Record: r})
	if err != nil || receipt.ID != r.ID || !receipt.Verified {
		t.Fatal(receipt, err)
	}
	files, err := os.ReadDir(filepath.Join(root, "scores"))
	if err != nil || len(files) != 1 {
		t.Fatal("web score was not stored", files, err)
	}
}
