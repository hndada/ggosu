// Command difficulty compares controlled finger patterns or analyzes a chart.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"ggosu/internal/chart"
	_ "ggosu/internal/chart/bms"
	_ "ggosu/internal/chart/ojn"
	_ "ggosu/internal/chart/osu"
	_ "ggosu/internal/chart/sm"
	"ggosu/internal/difficulty"
)

type output struct {
	Notice string  `json:"notice"`
	Cases  []trial `json:"cases"`
}

type trial struct {
	Name   string             `json:"name"`
	Chart  *chart.Chart       `json:"chart,omitempty"`
	Region *measurement       `json:"region,omitempty"`
	Report *difficulty.Report `json:"report"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	o := difficulty.DefaultOptions()
	input := flag.String("input", "", "chart file; omitted: controlled chord/trill and hold experiments")
	suite := flag.String("suite", "basic", "generated experiment suite: basic, chords, recovery or holds")
	index := flag.Int("index", 0, "difficulty index within the chart file")
	hands := flag.String("hands", "", "explicit hand ID for each lane, e.g. 0,0,1,1")
	profile := flag.String("profile", "", "JSON difficulty.Profile; basic suite uses two fingers, other suites use six")
	charts := flag.String("charts", "", "export the exact analyzed charts as numbered .ggc files")
	out := flag.String("out", "", "write full state, cost, window and profile report as JSON")
	html := flag.String("html", "", "write a per-note strain page for -input; without -hands or -profile, uses the game's hands")
	asJSON := flag.Bool("json", false, "print JSON instead of a summary table")
	flag.DurationVar(&o.Window, "window", o.Window, "fixed strain-window width")
	flag.DurationVar(&o.TapHold, "tap-hold", o.TapHold, "maximum dwell before returning from a tap")
	flag.Float64Var(&o.Weight, "weight", o.Weight, "descending window weight decay, 0 <= value < 1")
	flag.Float64Var(&o.SpeedPower, "power", o.SpeedPower, "time-compression exponent")
	flag.Float64Var(&o.Rate, "rate", o.Rate, "playback rate; source timestamps remain unchanged")
	flag.DurationVar(&o.Recovery, "recovery", o.Recovery, "released-finger load decay time constant")
	flag.Float64Var(&o.HoldRecovery, "hold-recovery", o.HoldRecovery, "held-finger recovery relative to released, 0..1")
	flag.Float64Var(&o.LoadLimit, "load-limit", o.LoadLimit, "bounded workload memory capacity")
	flag.Float64Var(&o.RepeatGain, "repeat-gain", o.RepeatGain, "maximum extra repeat multiplier; 0 disables repeat cost")
	flag.Float64Var(&o.InteractionLoad, "interaction-load", o.InteractionLoad, "interaction contribution to workload memory, 0..1; immediate costs remain unchanged")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if *suite != "basic" && *suite != "chords" && *suite != "recovery" && *suite != "holds" {
		return fmt.Errorf("unknown experiment suite %q", *suite)
	}
	if *html != "" && *input == "" {
		return fmt.Errorf("-html requires -input")
	}
	if *input != "" && *suite != "basic" {
		return fmt.Errorf("-suite selects generated patterns and cannot be used with -input")
	}
	result := output{Notice: "POC coefficients are illustrative and uncalibrated. Results verify model behavior, not human difficulty."}
	if *input == "" {
		if *hands != "" {
			return fmt.Errorf("-hands requires -input; use -profile to customize generated comparisons")
		}
		p := difficulty.NewProfile([]int{0, 0})
		if *suite == "chords" || *suite == "recovery" || *suite == "holds" {
			p = difficulty.NewProfile([]int{0, 0, 0, 1, 1, 1})
		}
		if *profile != "" {
			var err error
			p, err = readProfile(*profile)
			if err != nil {
				return err
			}
		}
		var cases []trial
		var err error
		if *suite == "holds" {
			cases, err = compareHolds(o, p)
		} else if *suite == "recovery" {
			cases, err = compareRecovery(o, p)
		} else if *suite == "chords" {
			cases, err = compareChords(o, p)
		} else {
			cases, err = compareProfile(o, p)
		}
		if err != nil {
			return err
		}
		result.Cases = cases
	} else {
		if *hands != "" && *profile != "" {
			return fmt.Errorf("choose -hands or -profile")
		}
		cs, err := chart.Load(*input)
		if err != nil {
			return err
		}
		if *index < 0 || *index >= len(cs) {
			return fmt.Errorf("chart index %d out of range", *index)
		}
		var p difficulty.Profile
		if *profile != "" {
			p, err = readProfile(*profile)
		} else if *hands == "" && *html != "" {
			p = difficulty.NewProfile(difficulty.Hands(cs[*index].Lanes))
		} else {
			var ids []int
			ids, err = parseHands(*hands)
			p = difficulty.NewProfile(ids)
		}
		if err != nil {
			return err
		}
		r, err := difficulty.Analyze(cs[*index], p, o)
		if err != nil {
			return err
		}
		result.Cases = []trial{{Name: cs[*index].Metadata.Title + " / " + cs[*index].Metadata.Difficulty, Chart: cs[*index], Report: r}}
		if *html != "" {
			if err := writePage(*html, result.Notice, cs[*index], r); err != nil {
				return err
			}
		}
	}
	if *charts != "" {
		if err := exportCharts(*charts, result.Cases); err != nil {
			return err
		}
	}
	if *out != "" {
		// Exclusive creation avoids overwriting a source chart or an existing
		// report. A new output path is cheap and keeps experiments reviewable.
		f, err := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return fmt.Errorf("create report: %w", err)
		}
		writeErr := writeJSON(f, result)
		if err := errors.Join(writeErr, f.Close()); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
	}
	if *asJSON {
		return writeJSON(os.Stdout, result)
	}
	return summary(os.Stdout, result)
}

func exportCharts(dir string, cases []trial) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create chart directory: %w", err)
	}
	for i, c := range cases {
		path := filepath.Join(dir, fmt.Sprintf("%02d.ggc", i+1))
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return fmt.Errorf("create chart %s: %w", path, err)
		}
		if err := errors.Join(chart.Write(f, c.Chart), f.Close()); err != nil {
			return fmt.Errorf("write chart %s: %w", path, err)
		}
	}
	return nil
}

func parseHands(s string) ([]int, error) {
	if s == "" {
		return nil, fmt.Errorf("chart analysis requires an explicit -hands mapping or -profile")
	}
	parts := strings.Split(s, ",")
	ids := make([]int, len(parts))
	for i, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 0 {
			return nil, fmt.Errorf("invalid hand ID at lane %d", i)
		}
		ids[i] = n
	}
	return ids, nil
}

func readProfile(path string) (p difficulty.Profile, err error) {
	f, err := os.Open(path)
	if err != nil {
		return p, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	d := json.NewDecoder(io.LimitReader(f, 1<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return p, fmt.Errorf("read profile: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return p, fmt.Errorf("profile must contain exactly one JSON object")
	}
	return p, nil
}

func writeJSON(w io.Writer, result output) error {
	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	return e.Encode(result)
}

func summary(w io.Writer, result output) error {
	if _, err := fmt.Fprintln(w, result.Notice); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "CASE\tNOTES/s\tMOVEMENT\tINTERACTION\tREPEAT\tSTRAIN\tDIFFICULTY"); err != nil {
		return err
	}
	for _, c := range result.Cases {
		var movement, interaction, repeat float64
		for _, f := range c.Report.Fingers {
			movement += f.Movement
			interaction += f.Interaction
			repeat += f.Repeat
		}
		d, density := c.Report.Difficulty, "-"
		if c.Region != nil {
			movement, interaction, d = c.Region.Movement, c.Region.Interaction, c.Region.Difficulty
			repeat = c.Region.Repeat
			density = fmt.Sprintf("%.2f", c.Region.NotesPerSecond)
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%.4f\t%.4f\t%.4f\t%.4f\t%.4f\n", c.Name, density, movement, interaction, repeat, movement+interaction+repeat, d); err != nil {
			return err
		}
	}
	return tw.Flush()
}
