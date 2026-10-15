package ojn

import "testing"

func TestDecodeTextPicksEncoding(t *testing.T) {
	for _, c := range []struct {
		raw  []byte
		want string
	}{
		{[]byte{0xC0, 0xE5, 0xBF, 0xAC, 0xC1, 0xD6, 0}, "장연주"},                      // EUC-KR
		{append([]byte{'[', 0xD4, 0xAD, 0xB4, 0xB4, ']'}, "Canon"...), "[原创]Canon"}, // GBK
		{[]byte{0xD6, 0xDC, 0xBD, 0xDC, 0xC2, 0xD7}, "周杰伦"},                         // GBK
		{append([]byte{0xB2, 0xF6, 0xA5, 0xBE, 0xAF, 0x53}, " Mozart"...), "莫札特 Mozart"}, // Big5, also valid GBK with kana
		{[]byte{0xA9, 0x55, 0xA7, 0xA3, 0xA8, 0xAE}, "垃圾車"},                         // Big5, also valid GBK as Cyrillic
		{[]byte{0xA5, 0xBF, 0xB4, 0xE4, 0xAA, 0x42, 0xA4, 0xCD}, "正港朋友"},             // Big5, also valid GBK with lone kana
		{[]byte("Secret"), "Secret"},
	} {
		if got := decodeText(c.raw)[0]; got != c.want {
			t.Errorf("decodeText(% x) = %q, want %q", c.raw, got, c.want)
		}
	}
}
