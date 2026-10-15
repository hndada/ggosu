package play

import (
	"testing"

	"ggosu/internal/chart"
)

func TestChatOverlayNeverMovesOrCoversNoteField(t *testing.T) {
	for lanes := 1; lanes <= 18; lanes++ {
		c := chart.New(lanes)
		p := &view{field: newField(c, c, "")}
		before := p.field.layout
		bounds := p.ChatBounds()
		if float32(bounds.Min.X) <= before.left+before.width || bounds.Max.X != Width {
			t.Fatalf("%d-key field covered by chat: %v", lanes, bounds)
		}
		if p.field.layout.left != before.left || p.field.layout.width != before.width {
			t.Fatal("opening chat reflowed the note field")
		}
	}
}
