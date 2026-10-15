// Package parse holds number and record helpers shared by the source format readers.
package parse

import (
	"encoding/csv"
	"strconv"
	"strings"
	"time"

	"ggosu/internal/chart"
)

// Numbers retains the first conversion failure while reading one record.
type Numbers struct{ Err error }

func (n *Numbers) Float(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil && n.Err == nil {
		n.Err = err
	}
	return v
}
func (n *Numbers) Int(s string) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil && n.Err == nil {
		n.Err = err
	}
	return v
}

func (n *Numbers) Millis(s string) time.Duration {
	at, err := chart.Seconds(n.Float(s) / 1000)
	if err != nil && n.Err == nil {
		n.Err = err
	}
	return at
}

// Fields splits one comma-separated record, honouring quotes.
func Fields(s string) ([]string, error) {
	r := csv.NewReader(strings.NewReader(s))
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = -1
	return r.Read()
}

// Asset registers path under id with a slash-separated path and returns id.
func Asset(c *chart.Chart, id, path, kind string) string {
	c.Assets[id] = chart.Asset{Path: strings.ReplaceAll(path, "\\", "/"), Kind: kind}
	return id
}
