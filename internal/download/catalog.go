// Package download searches song catalogues and installs chart packages.
package download

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const DefaultArchive = "http://127.0.0.1:8091"
const DefaultDirect = "https://osu.direct"
const pageSize = 20

var validID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type Query struct {
	Catalog string `json:"catalog"` // osu or archive
	Text    string `json:"text"`
	Keys    int    `json:"keys"` // zero means every key count
	Offset  int    `json:"offset"`
}

type Pack struct {
	Key       string   `json:"key"`
	Source    string   `json:"source"`
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Artist    string   `json:"artist"`
	Creator   string   `json:"creator"`
	Keys      []int    `json:"keys"`
	Levels    []string `json:"levels"`
	Status    string   `json:"status"`
	Size      int64    `json:"size"`
	Available bool     `json:"available"`
	Reason    string   `json:"reason,omitempty"`
	Installed bool     `json:"installed"`
}

type Page struct {
	Items  []Pack `json:"items"`
	Offset int    `json:"offset"`
	More   bool   `json:"more"`
	Total  int    `json:"total"` // -1 when the remote catalogue does not return a total
}

type offer struct {
	Pack
	url string
}

func ValidateServer(server string) error {
	u, err := url.Parse(server)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.TrimSpace(server) != server {
		return fmt.Errorf("아카이브 서버는 http:// 또는 https:// 주소여야 합니다")
	}
	return nil
}

func (l *Library) Search(ctx context.Context, q Query) (Page, error) {
	if (q.Catalog != "osu" && q.Catalog != "archive") || q.Keys < 0 || q.Keys > 18 || q.Offset < 0 || q.Offset > 10000 || !utf8.ValidString(q.Text) || len(q.Text) > 512 {
		return Page{}, fmt.Errorf("검색 조건을 확인해 주세요")
	}
	l.mu.Lock()
	base, direct, closed := l.archive, l.direct, l.closed
	l.mu.Unlock()
	if closed {
		return Page{}, fmt.Errorf("곡 다운로드가 종료됐습니다")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var offers []offer
	page := Page{Items: []Pack{}, Offset: q.Offset, Total: -1}
	if q.Catalog == "osu" {
		params := url.Values{"mode": {"3"}, "amount": {strconv.Itoa(pageSize)}, "offset": {strconv.Itoa(q.Offset)}, "query": {q.Text}}
		var sets []struct {
			ID       int    `json:"id"`
			Title    string `json:"title"`
			Artist   string `json:"artist"`
			Creator  string `json:"creator"`
			Status   string `json:"status"`
			Beatmaps []struct {
				Mode    string  `json:"mode"`
				CS      float64 `json:"cs"`
				Version string  `json:"version"`
			} `json:"beatmaps"`
		}
		if err := l.getJSON(ctx, direct+"/api/v2/search?"+params.Encode(), &sets); err != nil {
			return page, err
		}
		page.More = len(sets) >= pageSize
		for _, set := range sets {
			if set.ID <= 0 {
				continue
			}
			p := Pack{Source: "osu", ID: strconv.Itoa(set.ID), Title: set.Title, Artist: set.Artist, Creator: set.Creator, Status: set.Status, Available: true}
			keys := map[int]bool{}
			for _, b := range set.Beatmaps {
				if b.Mode != "mania" || b.CS < 1 || b.CS > 18 || b.CS != float64(int(b.CS)) {
					continue
				}
				keys[int(b.CS)] = true
				p.Levels = append(p.Levels, b.Version)
			}
			if len(keys) == 0 || q.Keys != 0 && !keys[q.Keys] {
				continue
			}
			for k := range keys {
				p.Keys = append(p.Keys, k)
			}
			sort.Ints(p.Keys)
			p.Key = "osu:osu:" + p.ID
			offers = append(offers, offer{p, direct + "/api/d/" + p.ID})
		}
	} else {
		params := url.Values{"q": {q.Text}, "offset": {strconv.Itoa(q.Offset)}, "limit": {strconv.Itoa(pageSize)}}
		if q.Keys != 0 {
			params.Set("keys", strconv.Itoa(q.Keys))
		}
		var remote struct {
			Total int `json:"total"`
			Items []struct {
				Pack
				Download string `json:"download"`
				FileName string `json:"file_name"`
				FileSize int64  `json:"file_size"`
				Kind     string `json:"kind"`
			} `json:"items"`
		}
		if err := l.getJSON(ctx, base+"/api/packs?"+params.Encode(), &remote); err != nil {
			return page, err
		}
		page.Total, page.More = remote.Total, q.Offset+len(remote.Items) < remote.Total
		for _, item := range remote.Items {
			p := item.Pack
			if !sourceID(p.Source, p.ID) {
				continue
			}
			p.Key, p.Size = "archive:"+p.Source+":"+p.ID, item.FileSize
			p.Available = item.Download == "/d/"+p.Source+"/"+p.ID
			if !p.Available {
				p.Reason = "아카이브에 파일이 아직 보관되지 않았습니다"
			}
			if item.Kind == "diff" {
				p.Available, p.Reason = false, "차분은 원곡과 함께 설치해야 합니다"
			}
			if item.FileName != "" && !zipExtension(item.FileName) {
				p.Available, p.Reason = false, "현재 ZIP·OSZ·SMZIP 묶음만 설치할 수 있습니다"
			}
			offers = append(offers, offer{p, base + "/d/" + p.Source + "/" + p.ID})
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return page, fmt.Errorf("곡 다운로드가 종료됐습니다")
	}
	if q.Catalog == "archive" && base != l.archive {
		return page, fmt.Errorf("아카이브 주소가 바뀌었습니다. 다시 검색해 주세요")
	}
	if len(l.offers) > 1000 {
		l.offers = map[string]offer{}
	}
	for _, o := range offers {
		o.Installed = l.installed(o.Source, o.ID)
		l.offers[o.Key] = o
		page.Items = append(page.Items, o.Pack)
	}
	return page, nil
}

func sourceID(source, id string) bool {
	return validID.MatchString(id) && (source == "osu" || source == "smo" || source == "etterna" || source == "bms")
}

func zipExtension(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".zip", ".osz", ".smzip":
		return true
	}
	return false
}

func (l *Library) getJSON(ctx context.Context, raw string, value any) error {
	resp, err := l.get(ctx, raw)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(value); err != nil {
		return fmt.Errorf("곡 검색 응답을 읽지 못했습니다: %w", err)
	}
	return nil
}

func (l *Library) get(ctx context.Context, raw string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ggosu/1.0 (song catalogue and downloads)")
	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("다운로드 서버 연결: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		if err := resp.Body.Close(); err != nil {
			return nil, fmt.Errorf("HTTP %d; 응답 종료: %w", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("다운로드 서버 HTTP %d", resp.StatusCode)
	}
	return resp, nil
}
