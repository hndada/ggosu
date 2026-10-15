package archive

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/text/unicode/norm"
)

// Server answers pack and chart searches and serves stored files.
type Server struct {
	store Store
	mu    sync.RWMutex
	packs []packEntry
	byKey map[string]int // source/id → packs index
	chart []chartEntry
	byID  map[string]int   // chart identity → chart index
	inPak map[string][]int // source/id → chart indexes, also for event packs
}

type packEntry struct {
	Item
	file string // absolute path when stored
	size int64
	text string // normalized search text
}

type chartEntry struct {
	Chart
	text string
}

// PackHit is a pack in a response. Download is set when the archive holds the file.
type PackHit struct {
	Item
	FileName string `json:"file_name,omitempty"`
	Download string `json:"download,omitempty"`
	FileSize int64  `json:"file_size,omitempty"`
	Charts   int    `json:"charts"` // charts known to be in this pack
}

// PackLink is a pack a chart appears in, with its download when stored.
type PackLink struct {
	PackRef
	Download string `json:"download,omitempty"`
	FileSize int64  `json:"file_size,omitempty"`
}

// ChartHit is a merged chart with every pack that contains it.
type ChartHit struct {
	Chart
	Packs []PackLink `json:"packs"`
}

func NewServer(st Store) (*Server, error) {
	s := &Server{store: st}
	return s, s.Reload()
}

// Reload rereads every index, download log, chart record and scan.
func (s *Server) Reload() error {
	var packs []packEntry
	byKey := map[string]int{}
	for _, src := range Sources {
		list, err := s.store.ReadIndex(src)
		if err != nil {
			return err
		}
		results, err := s.store.Results(src)
		if err != nil {
			return err
		}
		for _, it := range list {
			e := packEntry{Item: it, text: normalize(it.Title + " " + it.Artist + " " + it.Creator + " " + it.Status)}
			if r, ok := results[it.ID]; ok && r.File != "" {
				if dir, err := s.store.Dir(src, it.ID); err == nil {
					if p := filepath.Join(dir, r.File); exists(p) {
						e.file, e.size = p, r.Size
					}
				}
			}
			byKey[src+"/"+it.ID] = len(packs)
			packs = append(packs, e)
		}
	}
	merged, err := s.store.AllCharts()
	if err != nil {
		return err
	}
	charts := make([]chartEntry, len(merged))
	byID, inPak := map[string]int{}, map[string][]int{}
	for i, c := range merged {
		var b strings.Builder
		for _, v := range []string{c.Title, c.Artist, c.Creator, c.Difficulty, c.Level} {
			b.WriteString(v + " ")
		}
		for _, t := range c.Tables {
			b.WriteString(t + " ")
		}
		for _, p := range c.Packs {
			b.WriteString(p.Title + " ")
			inPak[p.Source+"/"+p.ID] = append(inPak[p.Source+"/"+p.ID], i)
		}
		charts[i] = chartEntry{Chart: c, text: normalize(b.String())}
		for _, id := range c.IDs {
			byID[id] = i
		}
	}
	s.mu.Lock()
	s.packs, s.byKey, s.chart, s.byID, s.inPak = packs, byKey, charts, byID, inPak
	s.mu.Unlock()
	return nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/packs", s.searchPacks)
	mux.HandleFunc("GET /api/packs/{source}/{id}", s.pack)
	mux.HandleFunc("GET /api/charts", s.searchCharts)
	mux.HandleFunc("GET /api/charts/{key}", s.chartByID)
	mux.HandleFunc("GET /api/md5/{md5}", s.md5)
	mux.HandleFunc("GET /api/stats", s.stats)
	mux.HandleFunc("GET /d/{source}/{id}", s.download)
	return mux
}

type page struct {
	offset, limit int
}

func readPage(q url.Values) page {
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit, err := strconv.Atoi(q.Get("limit"))
	if err != nil || limit <= 0 || limit > 100 {
		limit = 50
	}
	return page{max(offset, 0), limit}
}

func (p page) slice(idx []int) []int {
	if p.offset >= len(idx) {
		return nil
	}
	return idx[p.offset:min(p.offset+p.limit, len(idx))]
}

func (s *Server) searchPacks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	words := strings.Fields(normalize(q.Get("q")))
	keys := parseInts(q.Get("keys"))
	source, status, kind := q.Get("source"), q.Get("status"), q.Get("kind")
	stored := q.Get("stored") == "1"
	tier, err := ParseTier(q.Get("tier"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	pg := readPage(q)
	s.mu.RLock()
	defer s.mu.RUnlock()
	var idx []int
	for i, e := range s.packs {
		if source != "" && e.Source != source || kind != "" && e.Kind != kind ||
			status != "" && !strings.EqualFold(e.Status, status) || !atLeast(e.Tier, tier) ||
			len(keys) > 0 && !anyKey(e.Keys, keys) || stored && e.file == "" || !containsAll(e.text, words) {
			continue
		}
		idx = append(idx, i)
	}
	switch q.Get("sort") {
	case "":
	case "plays":
		sort.SliceStable(idx, func(a, b int) bool { return s.packs[idx[a]].Plays > s.packs[idx[b]].Plays })
	default:
		http.Error(w, "sort must be plays", http.StatusBadRequest)
		return
	}
	hits := []PackHit{}
	for _, i := range pg.slice(idx) {
		hits = append(hits, s.packHit(s.packs[i]))
	}
	writeJSON(w, map[string]any{"total": len(idx), "offset": pg.offset, "items": hits})
}

// pack returns a pack with every chart known to be in it; event packs have no item but still list charts.
func (s *Server) pack(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("source") + "/" + r.PathValue("id")
	s.mu.RLock()
	defer s.mu.RUnlock()
	i, ok := s.byKey[key]
	charts := s.inPak[key]
	if !ok && len(charts) == 0 {
		http.NotFound(w, r)
		return
	}
	out := map[string]any{"charts": s.chartHits(charts)}
	if ok {
		out["pack"] = s.packHit(s.packs[i])
	}
	writeJSON(w, out)
}

func (s *Server) searchCharts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	words := strings.Fields(normalize(q.Get("q")))
	lanes := parseInts(q.Get("lanes"))
	formatName := q.Get("format")
	stored := q.Get("stored") == "1"
	tier, err := ParseTier(q.Get("tier"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	pg := readPage(q)
	s.mu.RLock()
	defer s.mu.RUnlock()
	var idx []int
	for i, e := range s.chart {
		if formatName != "" && e.Format != formatName || len(lanes) > 0 && !anyKey([]int{e.Lanes}, lanes) ||
			!atLeast(e.Tier, tier) || stored && !s.anyStored(e.Packs) || !containsAll(e.text, words) {
			continue
		}
		idx = append(idx, i)
	}
	switch q.Get("sort") {
	case "":
	case "plays":
		sort.SliceStable(idx, func(a, b int) bool { return s.chart[idx[a]].Plays > s.chart[idx[b]].Plays })
	case "packs":
		// Charts that more packs and tables chose come first.
		n := func(i int) int { return len(s.chart[i].Packs) + len(s.chart[i].Tables) }
		sort.SliceStable(idx, func(a, b int) bool { return n(idx[a]) > n(idx[b]) })
	default:
		http.Error(w, "sort must be plays or packs", http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"total": len(idx), "offset": pg.offset, "items": s.chartHits(pg.slice(idx))})
}

func (s *Server) chartByID(w http.ResponseWriter, r *http.Request) {
	s.lookup(w, r, r.PathValue("key"))
}

func (s *Server) md5(w http.ResponseWriter, r *http.Request) {
	s.lookup(w, r, "md5:"+strings.ToLower(r.PathValue("md5")))
}

func (s *Server) lookup(w http.ResponseWriter, r *http.Request, id string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	i, ok := s.byID[id]
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, s.chartHits([]int{i})[0])
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	type count struct {
		Packs  int   `json:"packs"`
		Stored int   `json:"stored"`
		Bytes  int64 `json:"bytes"`
	}
	out := map[string]any{}
	s.mu.RLock()
	for _, e := range s.packs {
		c, _ := out[e.Source].(*count)
		if c == nil {
			c = &count{}
			out[e.Source] = c
		}
		c.Packs++
		if e.file != "" {
			c.Stored++
			c.Bytes += e.size
		}
	}
	charts := map[string]int{"total": len(s.chart)}
	for _, c := range s.chart {
		if c.Scanned {
			charts["scanned"]++
		}
		charts[tierName(c.Tier)]++
	}
	out["charts"] = charts
	s.mu.RUnlock()
	writeJSON(w, out)
}

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	i, ok := s.byKey[r.PathValue("source")+"/"+r.PathValue("id")]
	var p string
	if ok {
		p = s.packs[i].file
	}
	s.mu.RUnlock()
	if p == "" {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.Error(w, "stat failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(filepath.Base(p)))
	http.ServeContent(w, r, "", st.ModTime(), f)
}

func (s *Server) packHit(e packEntry) PackHit {
	h := PackHit{Item: e.Item, Charts: len(s.inPak[e.Source+"/"+e.ID])}
	if e.file != "" {
		h.FileName = filepath.Base(e.file)
		h.Download = "/d/" + e.Source + "/" + e.ID
		h.FileSize = e.size
	}
	return h
}

func (s *Server) chartHits(idx []int) []ChartHit {
	out := make([]ChartHit, 0, len(idx))
	for _, i := range idx {
		c := s.chart[i].Chart
		h := ChartHit{Chart: c, Packs: make([]PackLink, 0, len(c.Packs))}
		for _, p := range c.Packs {
			l := PackLink{PackRef: p}
			if j, ok := s.byKey[p.Source+"/"+p.ID]; ok && s.packs[j].file != "" {
				l.Download = "/d/" + p.Source + "/" + p.ID
				l.FileSize = s.packs[j].size
			}
			h.Packs = append(h.Packs, l)
		}
		out = append(out, h)
	}
	return out
}

func (s *Server) anyStored(packs []PackRef) bool {
	for _, p := range packs {
		if j, ok := s.byKey[p.Source+"/"+p.ID]; ok && s.packs[j].file != "" {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return
	}
}

func normalize(s string) string { return strings.ToLower(norm.NFKC.String(s)) }

func matchQuery(it Item, q string) bool {
	return containsAll(normalize(it.Title+" "+it.Artist+" "+it.Creator), strings.Fields(normalize(q)))
}

func containsAll(text string, words []string) bool {
	for _, w := range words {
		if !strings.Contains(text, w) {
			return false
		}
	}
	return true
}

func parseInts(s string) []int {
	var out []int
	for _, f := range strings.Split(s, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(f)); err == nil {
			out = append(out, n)
		}
	}
	return out
}
