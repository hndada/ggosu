package play

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/font"

	"ggosu/internal/ui"
)

// chat holds the drawer's display data; chatState retains the room history.
type chat struct {
	Open, Focused bool
	ReadOnly      bool
	Unread        int
	Messages      []chatLine
	Draft, Status string
	Room          string
	Ticker, Popup *chatAlert
}

// chatLine is one message as osu!lazer's ChatLine shows it; Time is empty within the same minute.
type chatLine struct {
	Time, Name, Text string
	Colour           color.NRGBA
}

// osu!lazer ChatLine.default_username_colours, picked by sender.
var chatNameColours = []color.NRGBA{
	{0x58, 0x8c, 0x7e, 255}, {0xb2, 0xa3, 0x67, 255}, {0xc9, 0x8f, 0x65, 255}, {0xbc, 0x51, 0x51, 255},
	{0x5c, 0x8b, 0xd6, 255}, {0x7f, 0x6a, 0xb7, 255}, {0xa3, 0x68, 0xad, 255}, {0xaa, 0x68, 0x80, 255},
	{0x6f, 0xad, 0x9b, 255}, {0xf2, 0xe3, 0x94, 255}, {0xf2, 0xae, 0x72, 255}, {0xf9, 0x8f, 0x8a, 255},
	{0x7d, 0xae, 0xf4, 255}, {0xa6, 0x91, 0xf2, 255}, {0xc8, 0x94, 0xd3, 255}, {0xd8, 0x95, 0xb0, 255},
	{0x53, 0xc4, 0xa1, 255}, {0xea, 0xce, 0x5c, 255}, {0xea, 0x8c, 0x47, 255}, {0xfc, 0x4f, 0x4f, 255},
	{0x3d, 0x94, 0xea, 255}, {0x77, 0x60, 0xea, 255}, {0xaf, 0x52, 0xc6, 255}, {0xe2, 0x56, 0x96, 255},
	{0x67, 0x7c, 0x66, 255}, {0x9b, 0x87, 0x32, 255}, {0x8c, 0x51, 0x29, 255}, {0x8c, 0x30, 0x30, 255},
	{0x1f, 0x5d, 0x91, 255}, {0x43, 0x35, 0xa5, 255}, {0x81, 0x2a, 0x96, 255}, {0x99, 0x28, 0x61, 255},
}

// chatNameColour hashes the sender like the lobby chat (scenes/lobby/chat.js) so both agree.
func chatNameColour(id string) color.NRGBA {
	var h uint32
	for _, r := range id {
		h = h*31 + uint32(r)
	}
	return chatNameColours[h%32]
}

// The drawer covers HUD space, never the note field, even at 18 keys.
func (p *view) ChatBounds() image.Rectangle {
	right := int(math.Ceil(float64(p.field.layout.left+p.field.layout.width))) + 12
	return image.Rect(Width-min(360, Width-right), 0, Width, Height)
}
func (p *view) ChatToggleBounds(open bool) image.Rectangle {
	if open {
		return image.Rect(Width-62, 12, Width, 42)
	}
	return image.Rect(Width-62, 502, Width, 534)
}

// osu!lazer StandAloneChatDisplay: black at 80%, rows of 45 px time | 5+90+5 px name | text with
// every other row tinted, and a 30 px message box at the bottom.
func (p *view) drawChat(dst *ebiten.Image) {
	c := p.state.Chat
	if c == nil {
		return
	}
	h, button := p.hud, p.ChatToggleBounds(c.Open)
	defer func() {
		if c.Popup != nil {
			p.drawChatAlert(dst, c.Popup, 64)
		}
		if !c.Open && c.Ticker != nil {
			p.drawChatAlert(dst, c.Ticker, 550)
		}
	}()
	if !c.Open {
		rect(dst, float32(button.Min.X), float32(button.Min.Y), float32(button.Dx()), float32(button.Dy()), color.NRGBA{0, 0, 0, 204})
		label := "Chat"
		if c.Unread > 0 {
			label = fmt.Sprintf("Chat %d", c.Unread)
		}
		h.Left(dst, ui.Fit(label, h.Faces[small], button.Dx()-6), small, float32(button.Min.X+3), float32(button.Min.Y+22), ink)
		return
	}
	b := p.ChatBounds()
	x, w := float32(b.Min.X), float32(b.Dx())
	fill(dst, x, 0, w, Height, color.NRGBA{0, 0, 0, 204}, false)
	h.Left(dst, "방 채팅", regular, x+12, 30, ink)
	h.Left(dst, "접기 F8", small, float32(button.Min.X+5), 30, accent)
	h.Left(dst, ui.Fit(c.Room, h.Faces[small], b.Dx()-24), small, x+12, 51, muted)
	const lineHeight, timeWidth, nameWidth = 19, 40, 80
	left := x + 3
	textWidth := b.Dx() - 3 - 10 - timeWidth - nameWidth
	if !slices.Equal(p.chatSource, c.Messages) {
		p.chatSource = append(p.chatSource[:0], c.Messages...)
		p.chatLines = p.chatLines[:0]
		for _, m := range c.Messages {
			p.chatLines = append(p.chatLines, wrapChat(m.Text, h.Faces[small], textWidth))
		}
	}
	box := float32(Height - 20 - 30)
	bottom := box - 8
	if c.Unread > 0 {
		bottom -= lineHeight
		h.Left(dst, fmt.Sprintf("New %d / wheel to latest", c.Unread), small, x+12, bottom+lineHeight-2, accent)
	}
	// Newest at the bottom; rows above the header are dropped.
	for i := len(c.Messages) - 1; i >= 0; i-- {
		m, text := c.Messages[i], p.chatLines[i]
		rowH := float32(len(text)*lineHeight + 4)
		top := bottom - rowH
		if top < 64 {
			break
		}
		if i%2 == 1 {
			fill(dst, left, top, w-13, rowH, color.NRGBA{59, 50, 52, 51}, true)
		}
		base := top + 2 + 13
		if m.Time != "" {
			h.Left(dst, m.Time, small, left+2, base, muted)
		}
		h.Right(dst, ui.Fit(m.Name, h.Faces[small], nameWidth-10), small, left+timeWidth+nameWidth-5, base, m.Colour)
		for j, part := range text {
			h.Left(dst, part, small, left+timeWidth+nameWidth, base+float32(j*lineHeight), ink)
		}
		bottom = top
	}
	if len(c.Messages) == 0 {
		h.Left(dst, "아직 대화가 없어요.", small, x+12, 86, muted)
	}
	if c.ReadOnly {
		h.Left(dst, "No room connected", small, x+12, box+20, muted)
		h.Left(dst, ui.Fit(c.Status, h.Faces[small], b.Dx()-24), small, x+12, Height-6, muted)
		return
	}
	bg := color.NRGBA{29, 36, 49, 255}
	if c.Focused {
		bg = color.NRGBA{39, 48, 61, 255}
	}
	rect(dst, x, box, w, 30, bg)
	draft, tone := c.Draft, ink
	if draft == "" {
		draft, tone = "방에 메시지 보내기", muted
	}
	input := wrapChat(draft, h.Faces[small], b.Dx()-20)
	h.Left(dst, input[len(input)-1], small, x+10, box+20, tone)
	status := c.Status
	if status == "" && p.phase == Ready {
		status = "F8 접기 / Enter 전송"
	}
	h.Left(dst, ui.Fit(status, h.Faces[small], b.Dx()-24), small, x+12, Height-6, muted)
}

func (p *view) drawChatAlert(dst *ebiten.Image, a *chatAlert, y float32) {
	b := p.ChatBounds()
	x, w := float32(b.Min.X+8), float32(b.Dx()-16)
	fill(dst, x, y, w, 78, color.NRGBA{0, 0, 0, 230}, false)
	name := a.Message.Name
	if a.Count > 1 {
		name = fmt.Sprintf("새 메시지 %d · %s", a.Count, name)
	}
	p.hud.Left(dst, ui.Fit(name, p.hud.Faces[small], int(w)-20), small, x+10, y+20, accent)
	parts := wrapChat(strings.Join(strings.Fields(a.Message.Text), " "), p.hud.Faces[small], int(w)-20)
	for i, line := range parts[:min(2, len(parts))] {
		p.hud.Left(dst, line, small, x+10, y+42+float32(i*19), ink)
	}
}
func wrapChat(text string, face font.Face, width int) []string {
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		var line string
		for _, r := range paragraph {
			next := line + string(r)
			if line != "" && font.MeasureString(face, next).Ceil() > width {
				lines = append(lines, line)
				line = string(r)
			} else {
				line = next
			}
		}
		lines = append(lines, line)
	}
	return lines
}
