package web

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"ggosu/internal/download"
)

// Browser installation writes this server's songs, so only direct localhost
// browsers can install. Public deployments can search but cannot mutate songs.
func localInstall(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	u, err := url.Parse("http://" + r.Host)
	if err != nil || ip == nil || !ip.IsLoopback() {
		return false
	}
	localHost := u.Hostname() == "localhost"
	if p := net.ParseIP(u.Hostname()); p != nil {
		localHost = p.IsLoopback()
	}
	return localHost && r.Header.Get("Forwarded") == "" && r.Header.Get("X-Forwarded-For") == ""
}

func downloadRoutes(mux *http.ServeMux, library *download.Library) {
	respond := func(w http.ResponseWriter, value any) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(value); err != nil {
			return
		}
	}
	allowed := func(w http.ResponseWriter, r *http.Request) bool {
		if !localInstall(r) || r.Header.Get("X-Gosu-Install") != "1" || (r.Header.Get("Sec-Fetch-Site") != "" && r.Header.Get("Sec-Fetch-Site") != "same-origin") {
			http.Error(w, "브라우저 설치는 localhost 웹 서버에서만 사용할 수 있습니다", http.StatusForbidden)
			return false
		}
		return true
	}
	mux.HandleFunc("GET /api/download/settings", func(w http.ResponseWriter, r *http.Request) {
		respond(w, download.Info{Archive: library.Archive(), CanInstall: localInstall(r), CanConfigure: false})
	})
	mux.HandleFunc("GET /api/download/search", func(w http.ResponseWriter, r *http.Request) {
		params := r.URL.Query()
		keys, err1 := strconv.Atoi(params.Get("keys"))
		offset, err2 := strconv.Atoi(params.Get("offset"))
		if params.Get("keys") == "" {
			err1 = nil
		}
		if params.Get("offset") == "" {
			err2 = nil
		}
		if err1 != nil || err2 != nil {
			http.Error(w, "검색 조건을 확인해 주세요", 400)
			return
		}
		page, err := library.Search(r.Context(), download.Query{Catalog: params.Get("catalog"), Text: params.Get("q"), Keys: keys, Offset: offset})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		respond(w, page)
	})
	mux.HandleFunc("GET /api/download/state", func(w http.ResponseWriter, r *http.Request) {
		if !localInstall(r) {
			http.Error(w, "local installation required", http.StatusForbidden)
			return
		}
		respond(w, library.State())
	})
	mux.HandleFunc("POST /api/download/start", func(w http.ResponseWriter, r *http.Request) {
		if !allowed(w, r) {
			return
		}
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
		d.DisallowUnknownFields()
		var q struct {
			Key string `json:"key"`
		}
		if err := d.Decode(&q); err != nil {
			http.Error(w, "잘못된 설치 요청", 400)
			return
		}
		var extra any
		if err := d.Decode(&extra); err != io.EOF {
			http.Error(w, "설치 요청은 하나여야 합니다", 400)
			return
		}
		state, err := library.Start(q.Key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		respond(w, state)
	})
	mux.HandleFunc("POST /api/download/cancel", func(w http.ResponseWriter, r *http.Request) {
		if !allowed(w, r) {
			return
		}
		library.Cancel()
		respond(w, library.State())
	})
}
