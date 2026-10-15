package menu

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strconv"

	"ggosu/internal/chart"
	"ggosu/internal/chart/osu"
)

func (a *App) EditChart(s Selection) (*chart.Chart, error) {
	e, err := a.songs.Lookup(s)
	if err != nil {
		return nil, err
	}
	cs, err := chart.Load(e.Path)
	if err != nil {
		return nil, err
	}
	if s.Index < 0 || s.Index >= len(cs) {
		return nil, fmt.Errorf("채보가 변경됐습니다. 목록을 새로 읽어 주세요")
	}
	return cs[s.Index], nil
}

func (a *App) SaveChart(s Selection, draft *chart.Chart) (string, error) {
	c, err := a.EditChart(s)
	if err != nil {
		return "", err
	}
	return chart.SaveEdit(c.Source.Path, c, draft)
}

// Export is a written copy in another format and the content that format could not hold.
type Export struct {
	Path    string   `json:"path"`
	Dropped []string `json:"dropped"`
}

// ExportChart writes the draft as a new osu!mania .osu beside the source.
func (a *App) ExportChart(s Selection, draft *chart.Chart) (Export, error) {
	c, err := a.EditChart(s)
	if err != nil {
		return Export{}, err
	}
	path, dropped, err := osu.SaveEdit(c.Source.Path, c, draft)
	return Export{path, dropped}, err
}

// serveAudio supplies audio without exposing arbitrary files or a Wails binding.
func (a *App) serveAudio(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || (r.URL.Path != "/editor/audio" && r.URL.Path != "/preview/audio" && r.URL.Path != "/preview/background") {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query()
	i, err := strconv.Atoi(q.Get("index"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	c, err := a.EditChart(Selection{ChartID: q.Get("chart"), Folder: q.Get("folder"), Index: i})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if r.URL.Path == "/preview/audio" {
		serveAudio(w, r, c, q.Get("id"))
		return
	}
	// The song select shows the chart's background image; every other route serves audio only.
	if r.URL.Path == "/preview/background" {
		name, mod, img, err := chart.OpenImage(c, q.Get("id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer img.Close()
		http.ServeContent(w, r, name, mod, img)
		return
	}
	asset, ok := c.Assets[q.Get("id")]
	if !ok || asset.Kind != "audio" {
		http.NotFound(w, r)
		return
	}
	path, err := chart.AssetPath(c.Source.Path, asset.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 256<<20 {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, path)
}

// serveAudio answers a preview request with an audio asset or bank sample of c.
func serveAudio(w http.ResponseWriter, r *http.Request, c *chart.Chart, id string) {
	name, mod, a, err := chart.OpenAudio(c, id)
	if errors.Is(err, chart.ErrSilentAudio) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if errors.Is(err, fs.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	defer a.Close()
	http.ServeContent(w, r, name, mod, a)
}
