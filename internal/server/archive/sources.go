package archive

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Endpoints are variables so tests can point them at local servers.
var (
	osuDirectSearch  = "https://osu.direct/api/v2/search"
	osuDirectFile    = "https://osu.direct/api/d/"
	nerinyanFile     = "https://api.nerinyan.moe/d/"
	osuToken         = "https://osu.ppy.sh/oauth/token"
	osuSearch        = "https://osu.ppy.sh/api/v2/beatmapsets/search"
	etternaPacks     = "https://api.etternaonline.com/api/packs"
	smoPacks         = "https://stepmaniaonline.net/api/packs"
	smoSite          = "https://stepmaniaonline.net"
	bmsTableList     = "https://script.google.com/macros/s/AKfycbzaQbcI9UZDcDlSHHl2NHilhmePrNrwxRdOFkmIXsfnbfksKKmAB3V65WZ8jPWU-7E/exec?table=tablelist"
	bmsSearchAPI     = "https://api.bmssearch.net/v1"
	osuDirectMaxSkip = 10000
)

// Options tunes index runs.
type Options struct {
	BMSTags []string // DARKSABUN tag2 values to include
	Charts  bool     // also crawl per-chart listings (Etterna songs, BMS Search); slower
}

// Catalog is what one index run learns: downloadable packs and the charts they hold.
type Catalog struct {
	Items  []Item
	Charts []ChartRecord
}

// Index builds the pack list and chart records of one source.
func Index(ctx context.Context, c *Client, source string, opt Options) (Catalog, error) {
	switch source {
	case SourceOsu:
		if id, secret := os.Getenv("OSU_CLIENT_ID"), os.Getenv("OSU_CLIENT_SECRET"); id != "" && secret != "" {
			return indexOsuOfficial(ctx, c, id, secret)
		}
		return indexOsuDirect(ctx, c)
	case SourceEtterna:
		return indexEtterna(ctx, c, opt.Charts)
	case SourceSMO:
		return indexSMO(ctx, c)
	case SourceBMS:
		return indexBMS(ctx, c, opt.BMSTags, opt.Charts)
	}
	return Catalog{}, fmt.Errorf("unknown source %q", source)
}

type osuSet struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Creator     string `json:"creator"`
	Status      string `json:"status"`
	LastUpdated string `json:"last_updated"`
	PlayCount   int    `json:"play_count"`
	Beatmaps    []struct {
		Mode       string  `json:"mode"`
		CS         float64 `json:"cs"`
		Version    string  `json:"version"`
		Checksum   string  `json:"checksum"`
		Difficulty float64 `json:"difficulty_rating"`
		PlayCount  int     `json:"playcount"`
	} `json:"beatmaps"`
}

// add appends the set as a pack and each mania difficulty as a chart keyed by its file md5.
func (s osuSet) add(cat *Catalog) {
	id := strconv.Itoa(s.ID)
	tier := osuTier(s.Status)
	it := Item{
		Source: SourceOsu, ID: id, Title: s.Title, Artist: s.Artist, Creator: s.Creator,
		Status: s.Status, Tier: tier, Plays: s.PlayCount, Updated: s.LastUpdated,
		Links: []string{osuDirectFile + id, nerinyanFile + id},
	}
	keys := map[int]bool{}
	for _, b := range s.Beatmaps {
		if b.Mode != "mania" {
			continue
		}
		keys[int(b.CS)] = true
		it.Levels = append(it.Levels, b.Version)
		if b.Checksum == "" {
			continue
		}
		cat.Charts = append(cat.Charts, ChartRecord{
			IDs: []string{"md5:" + strings.ToLower(b.Checksum)}, Format: "osu",
			Title: s.Title, Artist: s.Artist, Creator: s.Creator, Difficulty: b.Version,
			Level: fmt.Sprintf("%.2f", b.Difficulty), Lanes: int(b.CS), Tier: tier, Plays: b.PlayCount,
			Packs: []PackRef{{Source: SourceOsu, ID: id, Title: s.Artist + " - " + s.Title}},
		})
	}
	it.Keys = sortedKeys(keys)
	cat.Items = append(cat.Items, it)
}

// indexOsuDirect pages osu.direct per status. Its offset stops at 10,000, so a large status is
// read from both ends; the middle of a status with more than 20,000 sets needs the official API.
func indexOsuDirect(ctx context.Context, c *Client) (Catalog, error) {
	seen := map[int]bool{}
	var cat Catalog
	for _, status := range []int{1, 2, 3, 4, 0, -1, -2} {
		for _, order := range []string{"asc", "desc"} {
			capped, fresh := true, 0
			for off := 0; off < osuDirectMaxSkip; off += 100 {
				q := url.Values{"mode": {"3"}, "amount": {"100"}, "offset": {strconv.Itoa(off)},
					"status": {strconv.Itoa(status)}, "sort": {"submitted_date:" + order}}
				var page []osuSet
				if err := c.JSON(ctx, osuDirectSearch+"?"+q.Encode(), nil, &page); err != nil {
					return Catalog{}, err
				}
				pageNew := 0
				for _, s := range page {
					if !seen[s.ID] {
						seen[s.ID] = true
						s.add(&cat)
						pageNew++
					}
				}
				fresh += pageNew
				if len(page) < 100 || (order == "desc" && pageNew == 0) {
					capped = false
					break
				}
			}
			c.Log("osu status %d %s: %d new sets", status, order, fresh)
			if !capped {
				break
			}
			if order == "desc" {
				c.Log("osu status %d: more than %d sets, middle skipped (set OSU_CLIENT_ID/OSU_CLIENT_SECRET)", status, 2*osuDirectMaxSkip)
			}
		}
	}
	return cat, nil
}

func indexOsuOfficial(ctx context.Context, c *Client, id, secret string) (Catalog, error) {
	form := url.Values{"client_id": {id}, "client_secret": {secret}, "grant_type": {"client_credentials"}, "scope": {"public"}}
	resp, err := c.HTTP.PostForm(osuToken, form)
	if err != nil {
		return Catalog{}, err
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := decodeJSON(resp, &tok); err != nil {
		return Catalog{}, fmt.Errorf("osu token: %w", err)
	}
	header := http.Header{"Authorization": {"Bearer " + tok.AccessToken}}
	var cat Catalog
	cursor := ""
	for {
		q := url.Values{"m": {"3"}, "s": {"any"}, "nsfw": {"true"}, "sort": {"ranked_asc"}}
		if cursor != "" {
			q.Set("cursor_string", cursor)
		}
		var page struct {
			Beatmapsets []osuSet `json:"beatmapsets"`
			Cursor      *string  `json:"cursor_string"`
		}
		if err := c.JSON(ctx, osuSearch+"?"+q.Encode(), header, &page); err != nil {
			return Catalog{}, err
		}
		for _, s := range page.Beatmapsets {
			s.add(&cat)
		}
		if len(cat.Items)%5000 < len(page.Beatmapsets) {
			c.Log("osu official: %d sets", len(cat.Items))
		}
		if page.Cursor == nil || *page.Cursor == "" || len(page.Beatmapsets) == 0 {
			return cat, nil
		}
		cursor = *page.Cursor
	}
}

func indexEtterna(ctx context.Context, c *Client, charts bool) (Catalog, error) {
	var cat Catalog
	for page := 1; ; page++ {
		var res struct {
			Data []struct {
				ID        int    `json:"id"`
				Name      string `json:"name"`
				Size      string `json:"size"`
				Overall   string `json:"overall"`
				PlayCount int    `json:"play_count"`
				Download  string `json:"download"`
				Tags      []struct {
					Type string `json:"type"`
					Name string `json:"name"`
				} `json:"tags"`
			} `json:"data"`
			Meta struct {
				LastPage int `json:"last_page"`
			} `json:"meta"`
		}
		if err := c.JSON(ctx, fmt.Sprintf("%s?page=%d&limit=100", etternaPacks, page), nil, &res); err != nil {
			return Catalog{}, err
		}
		for _, p := range res.Data {
			// EtternaOnline hosts only packs it accepted for its leaderboards.
			it := Item{Source: SourceEtterna, ID: strconv.Itoa(p.ID), Title: p.Name, Size: parseSize(p.Size),
				Tier: TierOfficial, Plays: p.PlayCount}
			if u, err := url.Parse(p.Download); err == nil && p.Download != "" {
				it.Links = []string{u.String()}
			}
			if f, err := strconv.ParseFloat(p.Overall, 64); err == nil {
				it.Levels = []string{fmt.Sprintf("MSD %.1f", f)}
			}
			keys := map[int]bool{}
			for _, t := range p.Tags {
				switch t.Type {
				case "keyCount":
					if k, err := strconv.Atoi(strings.TrimSuffix(strings.ToLower(t.Name), "k")); err == nil {
						keys[k] = true
					}
				case "skillset":
					it.Levels = append(it.Levels, t.Name)
				}
			}
			it.Keys = sortedKeys(keys)
			cat.Items = append(cat.Items, it)
		}
		if page >= res.Meta.LastPage || len(res.Data) == 0 {
			break
		}
	}
	if charts {
		recs, err := etternaCharts(ctx, c, cat.Items)
		if err != nil {
			return Catalog{}, err
		}
		cat.Charts = recs
	}
	return cat, nil
}

// etternaCharts lists every pack's songs. EtternaOnline already merges a song found in several
// packs, so each difficulty becomes one record that names all of its packs.
func etternaCharts(ctx context.Context, c *Client, packs []Item) ([]ChartRecord, error) {
	seen := map[int]bool{}
	var recs []ChartRecord
	for n, p := range packs {
		for page := 1; ; page++ {
			var res struct {
				Data []struct {
					ID           int    `json:"id"`
					Name         string `json:"name"`
					Subtitle     string `json:"subtitle"`
					Artist       string `json:"artist"`
					Author       string `json:"author"`
					PlayCount    int    `json:"play_count"`
					Difficulties []struct {
						ID   int    `json:"id"`
						Name string `json:"name"`
						MSD  string `json:"msd"`
						Keys int    `json:"keys"`
					} `json:"difficulties"`
					Packs []struct {
						ID   int    `json:"id"`
						Name string `json:"name"`
					} `json:"packs"`
				} `json:"data"`
				Meta struct {
					LastPage int `json:"last_page"`
				} `json:"meta"`
			}
			if err := c.JSON(ctx, fmt.Sprintf("%s/%s/songs?page=%d&limit=100", etternaPacks, p.ID, page), nil, &res); err != nil {
				return nil, err
			}
			for _, s := range res.Data {
				if seen[s.ID] {
					continue
				}
				seen[s.ID] = true
				for _, d := range s.Difficulties {
					r := ChartRecord{
						IDs: []string{"eo:" + strconv.Itoa(d.ID)}, Format: "sm",
						Title: s.Name, Artist: s.Artist, Creator: s.Author, Difficulty: d.Name, Lanes: d.Keys,
						Tier: TierOfficial, Plays: s.PlayCount,
					}
					if f, err := strconv.ParseFloat(d.MSD, 64); err == nil {
						r.Level = fmt.Sprintf("MSD %.1f", f)
					}
					for _, sp := range s.Packs {
						pid := strconv.Itoa(sp.ID)
						r.Packs = append(r.Packs, PackRef{Source: SourceEtterna, ID: pid, Title: sp.Name})
						r.IDs = append(r.IDs, packKey(SourceEtterna, pid, s.Name, d.Name))
					}
					recs = append(recs, r)
				}
			}
			if page >= res.Meta.LastPage || len(res.Data) == 0 {
				break
			}
		}
		if (n+1)%100 == 0 {
			c.Log("etterna songs: %d/%d packs, %d charts", n+1, len(packs), len(recs))
		}
	}
	return recs, nil
}

func indexSMO(ctx context.Context, c *Client) (Catalog, error) {
	text, err := c.Text(ctx, smoPacks)
	if err != nil {
		return Catalog{}, err
	}
	items, err := parseSMO(text)
	return Catalog{Items: items}, err
}

// parseSMO reads the CSV pack list: ID, Pack Name, Song Count, Size, Sync, PackType, Substyle, Min Version.
func parseSMO(text string) ([]Item, error) {
	r := csv.NewReader(strings.NewReader(text))
	r.LazyQuotes, r.TrimLeadingSpace, r.FieldsPerRecord = true, true, -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("smo pack list: %w", err)
	}
	var items []Item
	for i, row := range rows {
		if i == 0 || len(row) < 4 {
			continue
		}
		id, err := strconv.Atoi(row[0])
		if err != nil {
			continue
		}
		size, _ := strconv.ParseInt(row[3], 10, 64)
		it := Item{Source: SourceSMO, ID: row[0], Title: row[1], Size: size, Tier: TierUnvetted, Links: []string{
			fmt.Sprintf("%s/download/pack/%d/", smoSite, id),
			fmt.Sprintf("%s/download/mirror/%d/", smoSite, id),
		}}
		if len(row) > 5 && row[5] != "None" {
			it.Status = row[5]
		}
		items = append(items, it)
	}
	return items, nil
}

var sizeRE = regexp.MustCompile(`^([\d.]+)\s*([KMGT]?B)$`)

func parseSize(s string) int64 {
	m := sizeRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0
	}
	f, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0
	}
	mul := map[string]float64{"B": 1, "KB": 1e3, "MB": 1e6, "GB": 1e9, "TB": 1e12}[m[2]]
	return int64(f * mul)
}

func sortedKeys(m map[int]bool) []int {
	var out []int
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
