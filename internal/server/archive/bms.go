package archive

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type bmsTable struct {
	Name   string `json:"name"`
	Symbol string `json:"symbol"`
	Tag1   string `json:"tag1"`
	Tag2   string `json:"tag2"`
	URL    string `json:"url"`
}

type bmsEntry struct {
	MD5     string          `json:"md5"`
	Level   json.RawMessage `json:"level"`
	Title   string          `json:"title"`
	Artist  string          `json:"artist"`
	URL     string          `json:"url"`
	URLDiff string          `json:"url_diff"`
}

var bmstableMeta = regexp.MustCompile(`(?is)<meta[^>]*name\s*=\s*["']?bmstable["']?[^>]*>`)
var metaContent = regexp.MustCompile(`(?is)content\s*=\s*["']([^"']+)["']`)

// diffSuffix drops a trailing chart name such as " [ANOTHER]" so a song item keeps its song title.
var diffSuffix = regexp.MustCompile(`\s*[\[(（【][^\[\]()（）【】]*[\])）】]\s*$`)

// indexBMS reads every DARKSABUN table with one of tags. Entries that share a song link become one
// song item and a sabun link its own item; every entry is also a chart record keyed by its md5.
// With charts, BMS Search adds its songs, event names and per-chart md5 list.
func indexBMS(ctx context.Context, c *Client, tags []string, charts bool) (Catalog, error) {
	var list []bmsTable
	if err := c.JSON(ctx, bmsTableList, nil, &list); err != nil {
		return Catalog{}, err
	}
	want := map[string]bool{}
	for _, t := range tags {
		want[strings.ToLower(t)] = true
	}
	songs := map[string]*Item{}
	byLink := map[string]string{} // song link → song item ID
	var order []string
	add := func(kind, link string, e bmsEntry, table, tier, parent string) *Item {
		// Many songs point their sabun at one shared uploader page, so a diff is keyed by its song too.
		key := link
		if parent != "" {
			key = parent + " " + link
		}
		id := bmsID(kind, key)
		it := songs[id]
		if it == nil {
			title := e.Title
			if kind == "song" {
				title = strings.TrimSpace(diffSuffix.ReplaceAllString(title, ""))
				byLink[link] = id
			}
			it = &Item{Source: SourceBMS, ID: id, Kind: kind, Parent: parent, Title: title, Artist: e.Artist, Links: []string{link}}
			songs[id] = it
			order = append(order, id)
		}
		it.Charts = appendUnique(it.Charts, strings.ToLower(e.MD5))
		it.Tables = appendUnique(it.Tables, table)
		it.Tier = maxTier(it.Tier, tier)
		return it
	}
	var cat Catalog
	missing := 0
	for _, t := range list {
		if len(want) > 0 && !want[strings.ToLower(t.Tag2)] {
			continue
		}
		symbol, entries, err := readBMSTable(ctx, c, t.URL)
		if err != nil {
			c.Log("bms table %s: %v", t.Name, err)
			continue
		}
		if symbol == "" {
			symbol = t.Symbol
		}
		tier := tableTier(t)
		for _, e := range entries {
			if e.MD5 == "" {
				continue
			}
			level := symbol + strings.Trim(string(e.Level), `"`)
			rec := ChartRecord{
				IDs: []string{"md5:" + strings.ToLower(e.MD5)}, Format: "bms",
				Title: e.Title, Artist: e.Artist, Level: level, Tables: []string{level}, Packs: []PackRef{}, Tier: tier,
			}
			if song := strings.TrimSpace(e.URL); song != "" {
				it := add("song", song, e, level, tier, "")
				rec.Packs = append(rec.Packs, PackRef{Source: SourceBMS, ID: it.ID, Title: it.Title})
				if d := strings.TrimSpace(e.URLDiff); d != "" && d != song {
					diff := add("diff", d, e, level, tier, it.ID)
					rec.Packs = append(rec.Packs, PackRef{Source: SourceBMS, ID: diff.ID, Title: diff.Title})
				}
			} else {
				missing++
			}
			cat.Charts = append(cat.Charts, rec)
		}
		c.Log("bms table %s: %d charts", t.Name, len(entries))
	}
	if missing > 0 {
		c.Log("bms: %d table charts have no song link", missing)
	}
	for _, id := range order {
		cat.Items = append(cat.Items, *songs[id])
	}
	// Songs listed by more tables come first so a limited fetch gets the most played charts.
	sort.SliceStable(cat.Items, func(i, j int) bool { return len(cat.Items[i].Tables) > len(cat.Items[j].Tables) })
	if charts {
		if err := bmsSearch(ctx, c, &cat, byLink); err != nil {
			return Catalog{}, err
		}
	}
	return cat, nil
}

type bmsSearchSong struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	Exhibition *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"exhibition"`
	Downloads    []struct{ URL string } `json:"downloads"`
	RelatedLinks []struct{ URL string } `json:"relatedLinks"`
}

type bmsSearchPattern struct {
	BMS *struct {
		ID string `json:"id"`
	} `json:"bms"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	Difficulty string `json:"difficulty"`
	PlayLevel  any    `json:"playlevel"`
	LaneType   string `json:"laneType"`
	TotalNotes int    `json:"totalNotes"`
	File       struct {
		MD5 string `json:"hashMd5"`
	} `json:"file"`
}

// bmsSearch pages every BMS Search song and chart. A song's event becomes a searchable pack, and a
// song whose download link no table uses becomes a new item.
func bmsSearch(ctx context.Context, c *Client, cat *Catalog, byLink map[string]string) error {
	type song struct {
		packs []PackRef
	}
	known := map[string]song{}
	for off := 0; ; off += 100 {
		var page []bmsSearchSong
		if err := c.JSON(ctx, fmt.Sprintf("%s/bmses/search?limit=100&offset=%d", bmsSearchAPI, off), nil, &page); err != nil {
			return err
		}
		for _, b := range page {
			var s song
			var links []string
			for _, d := range b.Downloads {
				links = appendUnique(links, strings.TrimSpace(d.URL))
			}
			for _, d := range b.RelatedLinks {
				links = appendUnique(links, strings.TrimSpace(d.URL))
			}
			itemID := ""
			for _, l := range links {
				if id, ok := byLink[l]; ok {
					itemID = id
					break
				}
			}
			title := b.Title
			if itemID == "" && len(links) > 0 {
				itemID = bmsID("query", b.ID)
				it := Item{Source: SourceBMS, ID: itemID, Kind: "song", Tier: TierCandidate, Title: b.Title, Artist: b.Artist, Links: links}
				if b.Exhibition != nil {
					it.Status = b.Exhibition.Name
				}
				cat.Items = append(cat.Items, it)
			}
			if itemID != "" {
				s.packs = append(s.packs, PackRef{Source: SourceBMS, ID: itemID, Title: title})
			}
			if b.Exhibition != nil && b.Exhibition.ID != "" {
				s.packs = append(s.packs, PackRef{Source: "event", ID: b.Exhibition.ID, Title: b.Exhibition.Name})
			}
			known[b.ID] = s
		}
		if len(page) < 100 {
			break
		}
		if off%5000 == 0 {
			c.Log("bms search: %d songs", len(known))
		}
	}
	n := 0
	for off := 0; ; off += 100 {
		var page []bmsSearchPattern
		if err := c.JSON(ctx, fmt.Sprintf("%s/patterns/search?limit=100&offset=%d", bmsSearchAPI, off), nil, &page); err != nil {
			return err
		}
		for _, p := range page {
			if p.BMS == nil || p.File.MD5 == "" {
				continue
			}
			s, ok := known[p.BMS.ID]
			if !ok {
				continue
			}
			level := ""
			if p.PlayLevel != nil {
				level = fmt.Sprint(p.PlayLevel)
			}
			cat.Charts = append(cat.Charts, ChartRecord{
				IDs: []string{"md5:" + strings.ToLower(p.File.MD5)}, Format: "bms",
				Title: p.Title, Artist: p.Artist, Difficulty: p.Difficulty, Level: level,
				Lanes: laneCount(p.LaneType), Notes: p.TotalNotes, Packs: s.packs, Tier: TierCandidate,
			})
			n++
		}
		if len(page) < 100 {
			break
		}
		if off%20000 == 0 {
			c.Log("bms search: %d charts", n)
		}
	}
	c.Log("bms search: %d songs, %d charts", len(known), n)
	return nil
}

var laneTypeRE = regexp.MustCompile(`^([A-Z])_(\d+)K$`)

// laneCount turns a BMS Search lane type into playable columns: beatmania layouts add one scratch
// per side, pop'n (P) layouts have none.
func laneCount(t string) int {
	m := laneTypeRE.FindStringSubmatch(t)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[2])
	if m[1] == "B" {
		if n >= 10 {
			return n + 2
		}
		return n + 1
	}
	return n
}

// candidateTable marks tables that list charts still being voted on, set aside, or picked from
// a main table rather than ranked by it.
var candidateTable = regexp.MustCompile(`(?i)vot|reject|sub\b|recommend|投票|隔離|お題|月別`)

// tableTier ranks a DARKSABUN table: main tables of the community "General" list are official,
// their vote, sub and recommend lists and every personal or event table are candidates.
func tableTier(t bmsTable) string {
	if !strings.EqualFold(t.Tag2, "General") || candidateTable.MatchString(t.Name) {
		return TierCandidate
	}
	return TierOfficial
}

// readBMSTable follows page → bmstable meta → header → data_url.
func readBMSTable(ctx context.Context, c *Client, page string) (string, []bmsEntry, error) {
	header := page
	if !strings.HasSuffix(strings.ToLower(strings.SplitN(page, "?", 2)[0]), ".json") {
		html, err := c.Text(ctx, page)
		if err != nil {
			return "", nil, err
		}
		tag := bmstableMeta.FindString(html)
		m := metaContent.FindStringSubmatch(tag)
		if m == nil {
			return "", nil, fmt.Errorf("no bmstable meta")
		}
		if header, err = resolveRef(page, m[1]); err != nil {
			return "", nil, err
		}
	}
	var h struct {
		Symbol  string `json:"symbol"`
		DataURL string `json:"data_url"`
	}
	if err := c.JSON(ctx, header, nil, &h); err != nil {
		return "", nil, err
	}
	data, err := resolveRef(header, h.DataURL)
	if err != nil {
		return "", nil, err
	}
	var entries []bmsEntry
	if err := c.JSON(ctx, data, nil, &entries); err != nil {
		return "", nil, err
	}
	return h.Symbol, entries, nil
}

func resolveRef(base, ref string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	r, err := url.Parse(strings.TrimSpace(ref))
	if err != nil {
		return "", err
	}
	return b.ResolveReference(r).String(), nil
}

func bmsID(kind, link string) string {
	sum := sha1.Sum([]byte(link))
	return kind[:1] + hex.EncodeToString(sum[:8])
}

func appendUnique(list []string, v string) []string {
	if v == "" {
		return list
	}
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

func decodeJSON(resp *http.Response, v any) error {
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s", resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(v)
}
