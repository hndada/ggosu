package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ggosu/internal/difficulty"
)

func TestWritePageEmbedsNotes(t *testing.T) {
	trials, err := compare(difficulty.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	c, r := trials[1].Chart, trials[1].Report
	c.Metadata.Title = "</script>"
	path := filepath.Join(t.TempDir(), "strain.html")
	if err := writePage(path, "notice", c, r); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	start := strings.Index(s, "const D = ")
	end := strings.Index(s[start:], ";\n")
	if start < 0 || end < 0 || strings.Contains(s, "/*DATA*/") {
		t.Fatal("page data was not embedded")
	}
	var p struct {
		Notes []struct{ Cost difficulty.Cost } `json:"notes"`
	}
	if err := json.Unmarshal([]byte(strings.ReplaceAll(s[start+len("const D = "):start+end], `<\/`, "</")), &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Notes) != len(c.Notes) || strings.Count(s, "</script>") != 1 {
		t.Fatal("every note must be embedded without closing the script early")
	}
	var sum float64
	for _, n := range p.Notes {
		sum += n.Cost.Total()
	}
	if diff := sum - r.Strain; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("page notes sum to %v, report strain %v", sum, r.Strain)
	}
}
