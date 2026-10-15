package ui

import (
	"testing"

	"golang.org/x/image/font"
)

func TestBundledFontsKeepKoreanAndCounterDigitsReadable(t *testing.T) {
	tx, err := NewText(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{Small, Regular, Label, Large} {
		for _, r := range "곡선택환경설정가힣" {
			if _, ok := tx.Faces[size].GlyphAdvance(r); !ok {
				t.Fatalf("size %d is missing %c", size, r)
			}
		}
	}
	f := tx.Faces[CounterLarge]
	want, ok := f.GlyphAdvance('0')
	if !ok {
		t.Fatal("counter is missing zero")
	}
	for _, r := range "123456789" {
		if advance, ok := f.GlyphAdvance(r); !ok || advance != want {
			t.Fatalf("counter digit %c changes column width: %v != %v", r, advance, want)
		}
	}
	for _, s := range []string{"0000000", "1111111", "0123456", "1100000"} {
		if w := tx.CounterWidth(s, 25); w > 168 || w < 100 {
			t.Fatalf("score %s does not fit its HUD rail: %v", s, w)
		}
	}
	if font.MeasureString(f, "KOOL") == 0 || font.MeasureString(f, "MISS") == 0 {
		t.Fatal("judgement font is empty")
	}
}
