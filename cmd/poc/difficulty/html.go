package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"ggosu/internal/chart"
	"ggosu/internal/difficulty"
)

//go:embed strain.html
var strainPage string

// page is the per-note view of one report. Chart times stay in source time;
// part times are in played time (source / rate), as the model computed them.
type page struct {
	Title      string             `json:"title"`
	Notice     string             `json:"notice"`
	Model      string             `json:"model"`
	Lanes      int                `json:"lanes"`
	Profile    difficulty.Profile `json:"profile"`
	Options    difficulty.Options `json:"options"`
	Strain     float64            `json:"strain"`
	Difficulty float64            `json:"difficulty"`
	Level      float64            `json:"level"` // Song select value: round(Difficulty) / 10.
	Notes      []pageNote         `json:"notes"`
	Windows    []pageWindow       `json:"windows"`
}

type pageNote struct {
	At    float64           `json:"at"` // ms, source time.
	End   float64           `json:"end"`
	Lane  int               `json:"lane"`
	Kind  string            `json:"kind"`
	Cost  difficulty.Cost   `json:"cost"`
	Parts []difficulty.Part `json:"parts,omitempty"`
}

type pageWindow struct {
	At       float64 `json:"at"` // ms, played time.
	End      float64 `json:"end"`
	Total    float64 `json:"total"`
	Rank     int     `json:"rank"`
	Weight   float64 `json:"weight"`
	Weighted float64 `json:"weighted"`
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

func writePage(path, notice string, c *chart.Chart, r *difficulty.Report) error {
	p := page{Title: c.Metadata.Title + " / " + c.Metadata.Difficulty, Notice: notice, Model: r.Model, Lanes: c.Lanes,
		Profile: r.Profile, Options: r.Options, Strain: r.Strain, Difficulty: r.Difficulty}
	p.Level = float64(int64(r.Difficulty+0.5)) / 10
	for i, n := range r.Attribute(len(c.Notes)) {
		src := c.Notes[i]
		p.Notes = append(p.Notes, pageNote{At: ms(src.At), End: ms(src.End), Lane: src.Lane, Kind: src.Kind, Cost: n.Cost, Parts: n.Parts})
	}
	for _, w := range r.Windows {
		p.Windows = append(p.Windows, pageWindow{ms(w.At), ms(w.End), w.Total, w.Rank, w.Weight, w.Weighted})
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	// The data sits inside a <script> element; "</" would end it early.
	html := strings.Replace(strainPage, "/*DATA*/null", strings.ReplaceAll(string(data), "</", `<\/`), 1)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create page: %w", err)
	}
	_, writeErr := f.WriteString(html)
	if err := errors.Join(writeErr, f.Close()); err != nil {
		return fmt.Errorf("write page: %w", err)
	}
	return nil
}
