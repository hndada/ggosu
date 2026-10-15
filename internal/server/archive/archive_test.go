package archive

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ggosu/internal/chart"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		in   string
		kind linkKind
		out  string
	}{
		{"https://drive.google.com/file/d/AbC_1-x/view?usp=sharing", linkDirect, "https://drive.usercontent.google.com/download?id=AbC_1-x&export=download&confirm=t"},
		{"https://drive.google.com/open?id=XyZ", linkDirect, "https://drive.usercontent.google.com/download?id=XyZ&export=download&confirm=t"},
		{"https://drive.google.com/drive/folders/abc?usp=sharing", linkPage, "https://drive.google.com/embeddedfolderview?id=abc"},
		{"https://www.dropbox.com/s/k/a.zip?dl=0", linkDirect, "https://www.dropbox.com/s/k/a.zip?dl=1"},
		{"https://mega.nz/#!abc", linkUnsupported, "mega.nz"},
		{"https://example.com/songs/a.RAR", linkDirect, "https://example.com/songs/a.RAR"},
		{"https://web.archive.org/web/2015/http://x.jp/a.zip", linkDirect, "https://web.archive.org/web/2015id_/http://x.jp/a.zip"},
		{"http://manbow.nothing.sh/event/event.cgi?action=More_def&num=1", linkPage, "http://manbow.nothing.sh/event/event.cgi?action=More_def&num=1"},
		{"ftp://x/a.zip", linkUnsupported, "not a web link"},
	}
	for _, c := range cases {
		k, out := classify(c.in)
		if k != c.kind || out != c.out {
			t.Errorf("classify(%q) = %v %q, want %v %q", c.in, k, out, c.kind, c.out)
		}
	}
}

func TestPageLinks(t *testing.T) {
	html := `<a href="/files/b.zip">b</a> <a href="https://mega.nz/#!x">m</a>
	<A HREF="https://www.dropbox.com/s/q/c.zip?dl=1&amp;x=1">https://www.dropbox.com/s/q/c.zip?dl=1&amp;x=1</A>
	<a href="https://twitter.com/x">t</a>`
	got := pageLinks("http://host/event/page.cgi", html)
	want := []string{"http://host/files/b.zip", "https://mega.nz/#!x", "https://www.dropbox.com/s/q/c.zip?dl=1&x=1"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("pageLinks = %q, want %q", got, want)
	}
}

func TestResolveIndexPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/song" {
			fmt.Fprint(w, `<a href="a_ogg.zip">ogg</a> <a href="a_wav.zip">wav</a>`)
			return
		}
		fmt.Fprint(w, `<a href="a.zip">a</a> <a href="b.zip">b</a> <a href="c.zip">c</a> <a href="d.zip">d</a>`)
	}))
	defer srv.Close()
	got, err := Resolve(context.Background(), testClient(), srv.URL+"/song")
	if err != nil || fmt.Sprint(got) != fmt.Sprintf("[%[1]s/a_ogg.zip %[1]s/a_wav.zip]", srv.URL) {
		t.Fatalf("song page = %q %v", got, err)
	}
	if got, err := Resolve(context.Background(), testClient(), srv.URL+"/"); err == nil {
		t.Fatalf("index page resolved to %q", got)
	}
}

func TestParseSMOAndSize(t *testing.T) {
	items, err := parseSMO("ID, Pack Name, Song Count, Size, Sync, PackType, Substyle, Min Version\n5299, \"0  OmegaSteps 1\", 22, 102035436, n/a, None, None, Stepmania 3.9\n8, \"Pad, Pack\", 1, 10, n/a, itg, None, x\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "5299" || items[0].Size != 102035436 || items[0].Status != "" || items[1].Title != "Pad, Pack" || items[1].Status != "itg" {
		t.Fatalf("items = %+v", items)
	}
	if got := parseSize("609.6 MB"); got != 609600000 {
		t.Fatalf("parseSize = %d", got)
	}
}

func TestIndexBMS(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("/list", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"name":"T","symbol":"x","tag2":"General","url":"%[1]s/t/table.html"},{"name":"P","tag2":"Personal","url":"%[1]s/p.html"}]`, srv.URL)
	})
	mux.HandleFunc("/t/table.html", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><head><meta name="bmstable" content="head.json"></head></html>`)
	})
	mux.HandleFunc("/t/head.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "\xef\xbb\xbf"+`{"name":"T","symbol":"sl","data_url":"data/score.json"}`)
	})
	mux.HandleFunc("/t/data/score.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[
			{"md5":"AA","level":"3","title":"Song [HYPER]","artist":"a","url":"http://e/1","url_diff":""},
			{"md5":"bb","level":12,"title":"Song [ANOTHER]","artist":"a","url":"http://e/1","url_diff":"http://e/1.bms"},
			{"md5":"dd","level":"5","title":"Other","url":"http://e/2","url_diff":"http://e/1.bms"},
			{"md5":"cc","level":"1","title":"Lonely","url":""}]`)
	})
	mux.HandleFunc("/bs/bmses/search", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("offset") != "0" {
			fmt.Fprint(w, `[]`)
			return
		}
		fmt.Fprint(w, `[
			{"id":"b1","title":"Other","artist":"o","exhibition":{"id":"ev","name":"BOF Test"},"downloads":[{"url":"http://e/2"}]},
			{"id":"b2","title":"Fresh","artist":"f","exhibition":{"id":"ev","name":"BOF Test"},"downloads":[{"url":"http://f/x.zip"}]}]`)
	})
	mux.HandleFunc("/bs/patterns/search", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("offset") != "0" {
			fmt.Fprint(w, `[]`)
			return
		}
		fmt.Fprint(w, `[
			{"bms":{"id":"b1"},"title":"Other [7KEYS]","playlevel":5,"laneType":"B_7K","totalNotes":900,"file":{"hashMd5":"DD"}},
			{"bms":{"id":"b2"},"title":"Fresh [HARD]","playlevel":9,"laneType":"B_14K","file":{"hashMd5":"ee"}},
			{"bms":null,"title":"Orphan","file":{"hashMd5":"ff"}}]`)
	})
	defer swap(&bmsTableList, srv.URL+"/list")()
	defer swap(&bmsSearchAPI, srv.URL+"/bs")()
	cat, err := Index(context.Background(), testClient(), SourceBMS, Options{BMSTags: []string{"General"}, Charts: true})
	if err != nil {
		t.Fatal(err)
	}
	items := cat.Items
	if len(items) != 5 || items[4].Title != "Fresh" || items[4].Status != "BOF Test" || items[4].Links[0] != "http://f/x.zip" {
		t.Fatalf("items = %+v", items)
	}
	if len(cat.Charts) != 6 {
		t.Fatalf("chart records = %+v", cat.Charts)
	}
	// The table entry and the BMS Search pattern for md5 dd are one chart in the table song,
	// its sabun and the event.
	var dd *Chart
	merged := MergeCharts(cat.Charts)
	for i := range merged {
		if merged[i].Key == "md5:dd" {
			dd = &merged[i]
		}
	}
	if len(merged) != 5 || dd == nil || dd.Lanes != 8 || dd.Notes != 900 || len(dd.Packs) != 3 || dd.Packs[2].Title != "BOF Test" {
		t.Fatalf("merged = %+v", merged)
	}
	// The General table is a main table, so its songs and charts are official; BMS Search alone is a candidate.
	if items[0].Tier != TierOfficial || items[4].Tier != TierCandidate || dd.Tier != TierOfficial {
		t.Fatalf("tiers: song %q, search-only song %q, merged chart %q", items[0].Tier, items[4].Tier, dd.Tier)
	}
	song, diff, other := items[0], items[1], items[3]
	if song.Kind != "song" || song.Title != "Song" || fmt.Sprint(song.Charts) != "[aa bb]" || fmt.Sprint(song.Tables) != "[sl3 sl12]" {
		t.Fatalf("song = %+v", song)
	}
	if diff.Kind != "diff" || diff.Parent != song.ID || diff.Links[0] != "http://e/1.bms" || fmt.Sprint(diff.Charts) != "[bb]" {
		t.Fatalf("diff = %+v", diff)
	}
	if other.Kind != "diff" || other.Parent != items[2].ID || other.ID == diff.ID || fmt.Sprint(other.Charts) != "[dd]" {
		t.Fatalf("shared sabun link merged across songs: %+v", other)
	}
}

func TestFetchAndServe(t *testing.T) {
	zip := "PK\x03\x04" + strings.Repeat("z", 5000)
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	resumed := false
	mux.HandleFunc("/ok.zip", func(w http.ResponseWriter, r *http.Request) {
		resumed = resumed || r.Header.Get("Range") == "bytes=1000-"
		w.Header().Set("Content-Disposition", `attachment; filename="Song A.zip"`)
		http.ServeContent(w, r, "", time.Time{}, strings.NewReader(zip))
	})
	mux.HandleFunc("/error", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html>quota exceeded</html>")
	})
	mux.HandleFunc("/big.zip", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(zip))
	})
	st := Store{Root: t.TempDir()}
	items := []Item{
		{Source: SourceSMO, ID: "1", Title: "Song A", Artist: "Ｂｅｅ", Links: []string{srv.URL + "/error", srv.URL + "/ok.zip"}},
		{Source: SourceSMO, ID: "2", Title: "Broken", Links: []string{srv.URL + "/error"}},
		{Source: SourceSMO, ID: "3", Title: "Huge", Links: []string{srv.URL + "/big.zip"}},
	}
	if err := st.WriteIndex(SourceSMO, items); err != nil {
		t.Fatal(err)
	}
	// A partial file from an interrupted run must be resumed, not restarted.
	dir, _ := st.Dir(SourceSMO, "1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".part"), []byte(zip[:1000]), 0o644); err != nil {
		t.Fatal(err)
	}
	c := testClient()
	plan, err := Fetch(context.Background(), c, st, SourceSMO, FetchOptions{DryRun: true})
	if err != nil || plan.Done != 3 || plan.Unknown != 3 || resumed {
		t.Fatalf("dry run = %+v %v", plan, err)
	}
	if _, err := os.Stat(filepath.Join(st.Root, "state")); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote state: %v", err)
	}
	stats, err := Fetch(context.Background(), c, st, SourceSMO, FetchOptions{MaxSize: 4000})
	if err != nil {
		t.Fatal(err)
	}
	// MaxSize 4000 rejects every 5004-byte file; rerun with room for them.
	if stats.Done != 0 {
		t.Fatalf("stats with small max size = %+v", stats)
	}
	stats, err = Fetch(context.Background(), c, st, SourceSMO, FetchOptions{Retry: true})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Done != 2 || stats.Failed != 1 {
		t.Fatalf("stats = %+v", stats)
	}
	b, err := os.ReadFile(filepath.Join(dir, "Song A.zip"))
	if err != nil || string(b) != zip || !resumed {
		t.Fatalf("stored file = %d bytes, resumed %v, %v", len(b), resumed, err)
	}
	again, err := Fetch(context.Background(), c, st, SourceSMO, FetchOptions{})
	if err != nil || again.Done != 0 || again.Failed != 0 || again.Skipped != 1 {
		t.Fatalf("second run = %+v %v", again, err)
	}

	s, err := NewServer(st)
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(s.Handler())
	defer api.Close()
	var res struct {
		Total int       `json:"total"`
		Items []PackHit `json:"items"`
	}
	getJSON(t, api.URL+"/api/packs?q=song+bee&stored=1", &res)
	if res.Total != 1 || res.Items[0].ID != "1" || res.Items[0].Download != "/d/smo/1" {
		t.Fatalf("search = %+v", res)
	}
	resp, err := http.Get(api.URL + "/d/smo/1")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != zip || !strings.Contains(resp.Header.Get("Content-Disposition"), "Song%20A.zip") {
		t.Fatalf("download = %d bytes, %q", len(body), resp.Header.Get("Content-Disposition"))
	}
	if resp, err := http.Get(api.URL + "/d/smo/2"); err != nil || resp.StatusCode != 404 {
		t.Fatalf("missing file status = %v %v", resp.StatusCode, err)
	}
}

// TestScanMergesPacks stores the same chart in an Etterna pack (renamed, retitled) and an SMO pack.
// The scan must yield one chart that lists both packs, linked to EtternaOnline metadata by pack key,
// and a search for either pack name must find it.
func TestScanMergesPacks(t *testing.T) {
	st := Store{Root: t.TempDir()}
	sm, err := os.ReadFile(filepath.Join("..", "..", "..", "songs", "demo", "demo.sm"))
	if err != nil {
		t.Fatal(err)
	}
	retitled := strings.Replace(string(sm), "#TITLE:", "#TITLE:Renamed ", 1)
	if retitled == string(sm) {
		t.Fatal("demo.sm has no #TITLE")
	}
	put := func(source, id, title, entry, body string) {
		t.Helper()
		if err := st.WriteIndex(source, []Item{{Source: source, ID: id, Title: title, Links: []string{"x"}}}); err != nil {
			t.Fatal(err)
		}
		dir, _ := st.Dir(source, id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(dir, "pack.zip"))
		if err != nil {
			t.Fatal(err)
		}
		zw := zip.NewWriter(f)
		w, err := zw.Create(entry)
		if err == nil {
			_, err = w.Write([]byte(body))
		}
		if err := errors.Join(err, zw.Close(), f.Close()); err != nil {
			t.Fatal(err)
		}
		if err := st.Record(source, Result{ID: id, File: "pack.zip", SHA256: id}); err != nil {
			t.Fatal(err)
		}
	}
	put(SourceEtterna, "7", "Keyboard Pack", "Keyboard Pack/Song/song.sm", string(sm))
	put(SourceSMO, "9", "Pad Pack Vol.1", "Pad Pack/Other/renamed.sm", retitled)
	charts, err := chart.Decode("demo.sm", strings.NewReader(string(sm)))
	if err != nil {
		t.Fatal(err)
	}
	m := charts[0].Metadata
	// EtternaOnline metadata for the same chart, known before any download.
	if err := st.WriteCharts(SourceEtterna, []ChartRecord{{
		IDs: []string{"eo:1", packKey(SourceEtterna, "7", m.Title, m.Difficulty)}, Title: m.Title, Difficulty: m.Difficulty, Level: "MSD 20.0",
		Packs: []PackRef{{Source: SourceEtterna, ID: "7", Title: "Keyboard Pack"}, {Source: SourceEtterna, ID: "8", Title: "Other EO Pack"}},
	}}); err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{SourceEtterna, SourceSMO} {
		s, err := Scan(context.Background(), st, src, ScanOptions{}, t.Logf)
		if err != nil || s.Packs != 1 || s.Charts != len(charts) {
			t.Fatalf("scan %s = %+v %v", src, s, err)
		}
	}
	if s, err := Scan(context.Background(), st, SourceSMO, ScanOptions{}, t.Logf); err != nil || s.Skipped != 1 {
		t.Fatalf("rescan of unchanged pack = %+v %v", s, err)
	}
	all, err := st.AllCharts()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != len(charts) {
		t.Fatalf("merged %d charts, want %d: %+v", len(all), len(charts), all)
	}
	c := all[0]
	if !strings.HasPrefix(c.Key, "fp:") || len(c.Packs) != 3 || c.Level != "MSD 20.0" && c.Level != m.Level || !c.Scanned {
		t.Fatalf("chart = %+v", c)
	}

	s, err := NewServer(st)
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(s.Handler())
	defer api.Close()
	for _, q := range []string{"pad+pack", "keyboard", "other+eo"} {
		var res struct {
			Total int        `json:"total"`
			Items []ChartHit `json:"items"`
		}
		getJSON(t, api.URL+"/api/charts?q="+q, &res)
		if res.Total < 1 || res.Items[0].Key != c.Key || len(res.Items[0].Packs) != 3 {
			t.Fatalf("chart search %q = %+v", q, res)
		}
	}
	var pack struct {
		Pack   PackHit    `json:"pack"`
		Charts []ChartHit `json:"charts"`
	}
	getJSON(t, api.URL+"/api/packs/smo/9", &pack)
	if pack.Pack.Charts != len(charts) || len(pack.Charts) != len(charts) || pack.Charts[0].Packs[0].Path == "" {
		t.Fatalf("pack = %+v", pack)
	}
	var byID ChartHit
	getJSON(t, api.URL+"/api/charts/eo:1", &byID)
	if byID.Key != c.Key {
		t.Fatalf("lookup by eo id = %+v", byID)
	}
}

// Two different songs titled alike in one pack share a pack key; it must not merge them,
// while a unique pack key still links metadata to its scan.
func TestMergeIgnoresAmbiguousPackKey(t *testing.T) {
	same := packKey(SourceEtterna, "1", "MAX 300", "Hard")
	solo := packKey(SourceEtterna, "1", "Solo", "Hard")
	p := []PackRef{{Source: SourceEtterna, ID: "1", Title: "P"}}
	got := MergeCharts([]ChartRecord{
		{IDs: []string{"eo:1", same}, Title: "MAX 300", Packs: p},
		{IDs: []string{"eo:2", same}, Title: "Max 300 (remix)", Packs: p},
		{IDs: []string{"eo:3", solo}, Title: "Solo", Packs: p},
		{IDs: []string{"fp:x", solo}, Title: "Solo", Packs: p, Scanned: true},
	})
	if len(got) != 3 {
		t.Fatalf("merged = %+v", got)
	}
	for _, c := range got {
		for _, id := range c.IDs {
			if id == same {
				t.Fatalf("ambiguous pack key kept in %+v", c)
			}
		}
		if c.Title == "Solo" && (c.Key != "fp:x" || !c.Scanned) {
			t.Fatalf("unique pack key did not link scan: %+v", c)
		}
	}
}

func TestTierRules(t *testing.T) {
	for status, want := range map[string]string{
		"ranked": TierOfficial, "approved": TierOfficial, "loved": TierOfficial,
		"qualified": TierCandidate, "pending": TierCandidate, "wip": TierUnvetted, "graveyard": TierUnvetted,
	} {
		if got := osuTier(status); got != want {
			t.Errorf("osuTier(%q) = %q, want %q", status, got, want)
		}
	}
	for _, c := range []struct {
		name, tag, want string
	}{
		{"Satellite", "General", TierOfficial},
		{"発狂BMS難易度表", "General", TierOfficial},
		{"Satellite Sub", "General", TierCandidate},
		{"Stella (Voting)", "General", TierCandidate},
		{"Satellite Recommend", "General", TierCandidate},
		{"第三期Overjoy隔離", "General", TierCandidate},
		{"Luminous (only rejected)", "General", TierCandidate},
		{"Someone's table", "Personal", TierCandidate},
	} {
		if got := tableTier(bmsTable{Name: c.name, Tag2: c.tag}); got != c.want {
			t.Errorf("tableTier(%q, %q) = %q, want %q", c.name, c.tag, got, c.want)
		}
	}
	if !atLeast(TierOfficial, TierCandidate) || atLeast(TierUnvetted, TierCandidate) || !atLeast("", "") || atLeast("", TierCandidate) {
		t.Error("atLeast ordering")
	}
	if _, err := ParseTier("공인"); err == nil {
		t.Error("ParseTier accepted an unknown tier")
	}
}

// -tier filters first, then -top keeps the most played of what is left, most played first.
func TestFetchTierAndTop(t *testing.T) {
	st := Store{Root: t.TempDir()}
	items := []Item{
		{Source: SourceEtterna, ID: "1", Title: "low", Tier: TierOfficial, Plays: 10, Size: 1, Links: []string{"x"}},
		{Source: SourceEtterna, ID: "2", Title: "high", Tier: TierOfficial, Plays: 900, Size: 2, Links: []string{"x"}},
		{Source: SourceEtterna, ID: "3", Title: "mid", Tier: TierOfficial, Plays: 500, Size: 4, Links: []string{"x"}},
		{Source: SourceEtterna, ID: "4", Title: "loud but unvetted", Tier: TierUnvetted, Plays: 9999, Size: 8, Links: []string{"x"}},
	}
	if err := st.WriteIndex(SourceEtterna, items); err != nil {
		t.Fatal(err)
	}
	plan := func(o FetchOptions) FetchStats {
		t.Helper()
		o.DryRun = true
		s, err := Fetch(context.Background(), testClient(), st, SourceEtterna, o)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	if s := plan(FetchOptions{Tier: TierOfficial, Top: 2}); s.Done != 2 || s.Bytes != 6 {
		t.Fatalf("official top 2 = %+v, want high+mid (6 bytes)", s)
	}
	if s := plan(FetchOptions{Top: 1}); s.Done != 1 || s.Bytes != 8 {
		t.Fatalf("top 1 without tier = %+v, want the unvetted pack", s)
	}
	if s := plan(FetchOptions{Tier: TierCandidate}); s.Done != 3 {
		t.Fatalf("candidate or better = %+v", s)
	}
}

// A chart takes the highest tier and play count of its sightings, and the API filters and sorts by them.
func TestChartTierSearch(t *testing.T) {
	st := Store{Root: t.TempDir()}
	p := func(src, id string) []PackRef { return []PackRef{{Source: src, ID: id, Title: "pack " + id}} }
	if err := st.WriteCharts(SourceBMS, []ChartRecord{
		{IDs: []string{"md5:a"}, Title: "Alpha", Tier: TierCandidate, Packs: p("event", "e1")},
		{IDs: []string{"md5:a"}, Title: "Alpha", Tier: TierOfficial, Tables: []string{"sl3"}, Packs: p(SourceBMS, "s1")},
		{IDs: []string{"md5:b"}, Title: "Beta", Tier: TierCandidate, Packs: p("event", "e1")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.WriteCharts(SourceOsu, []ChartRecord{
		{IDs: []string{"md5:c"}, Title: "Gamma", Tier: TierUnvetted, Plays: 50, Packs: p(SourceOsu, "9")},
		{IDs: []string{"md5:d"}, Title: "Delta", Tier: TierOfficial, Plays: 700, Packs: p(SourceOsu, "8")},
	}); err != nil {
		t.Fatal(err)
	}
	s, err := NewServer(st)
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(s.Handler())
	defer api.Close()
	titles := func(q string) string {
		t.Helper()
		var res struct {
			Items []ChartHit `json:"items"`
		}
		getJSON(t, api.URL+"/api/charts?"+q, &res)
		var out []string
		for _, c := range res.Items {
			out = append(out, c.Title+":"+c.Tier)
		}
		return strings.Join(out, " ")
	}
	if got := titles("tier=official"); got != "Delta:official Alpha:official" {
		t.Fatalf("official = %q", got)
	}
	if got := titles("tier=candidate&q=pack+e1"); got != "Alpha:official Beta:candidate" {
		t.Fatalf("candidate in event pack = %q", got)
	}
	if got := titles("sort=plays&limit=2"); got != "Delta:official Gamma:unvetted" {
		t.Fatalf("sort by plays = %q", got)
	}
	if got := titles("sort=packs&limit=1"); got != "Alpha:official" {
		t.Fatalf("sort by packs = %q", got)
	}
	if resp, err := http.Get(api.URL + "/api/charts?tier=best"); err != nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad tier status = %v %v", resp.StatusCode, err)
	}
}

// Fetch stops before a download could push the disk below the reserve, counting an item of
// unknown size as the per-file limit.
func TestFetchReserve(t *testing.T) {
	body := "PK\x03\x04" + strings.Repeat("z", 96)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer srv.Close()
	st := Store{Root: t.TempDir()}
	var items []Item
	for _, id := range []string{"1", "2", "3"} {
		items = append(items, Item{Source: SourceSMO, ID: id, Title: id, Links: []string{srv.URL + "/" + id + ".zip"}})
	}
	if err := st.WriteIndex(SourceSMO, items); err != nil {
		t.Fatal(err)
	}
	// The fake disk shrinks by each stored file; with 1000 free, 100 per file and 750 reserved,
	// two files fit (1000→900→800) and the third would leave 700 < 750.
	used := int64(0)
	var asked string
	old := freeSpace
	freeSpace = func(dir string) (int64, error) {
		asked = dir
		return 1000 - used, nil
	}
	defer func() { freeSpace = old }()
	c := testClient()
	c.Log = func(f string, a ...any) {
		if strings.Contains(f, "bytes)") {
			used += 100
		}
	}
	stats, err := Fetch(context.Background(), c, st, SourceSMO, FetchOptions{Reserve: 750, MaxSize: 100})
	if err != nil || stats.Done != 2 {
		t.Fatalf("stats = %+v %v", stats, err)
	}
	if asked != filepath.Join(st.Root, "files", SourceSMO) {
		t.Fatalf("free space asked for %q", asked)
	}
	// A rerun must not get around the reserve the way a per-run budget would.
	stats, err = Fetch(context.Background(), c, st, SourceSMO, FetchOptions{Reserve: 750, MaxSize: 100})
	if err != nil || stats.Done != 0 {
		t.Fatalf("rerun = %+v %v", stats, err)
	}
}

func testClient() *Client {
	c := NewClient("test", 0)
	c.Retries = 0
	return c
}

func swap(p *string, v string) func() {
	old := *p
	*p = v
	return func() { *p = old }
}

func getJSON(t *testing.T, u string, v any) {
	t.Helper()
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}
