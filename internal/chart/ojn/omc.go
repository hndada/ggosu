package ojn

// OMC permutes 17 blocks according to the payload length, then inverts bytes
// using a key carried across WAV samples. Ogg samples are stored unchanged.
// Format table: https://open2jam.wordpress.com/the-ojm-documentation/
var order = [17][17]byte{
	{16, 14, 2, 9, 4, 0, 7, 1, 6, 8, 15, 10, 5, 12, 3, 13, 11},
	{7, 2, 10, 11, 3, 5, 13, 8, 4, 0, 12, 6, 15, 14, 16, 1, 9},
	{12, 13, 3, 0, 6, 9, 10, 1, 7, 8, 16, 2, 11, 14, 4, 15, 5},
	{8, 3, 4, 13, 6, 5, 11, 16, 2, 12, 7, 9, 10, 15, 14, 0, 1},
	{15, 2, 12, 13, 0, 4, 1, 5, 7, 3, 9, 16, 6, 11, 10, 8, 14},
	{0, 4, 11, 16, 15, 13, 12, 6, 5, 7, 1, 2, 3, 8, 9, 10, 14},
	{3, 16, 8, 7, 6, 9, 14, 13, 0, 10, 11, 4, 5, 12, 2, 1, 15},
	{4, 14, 16, 15, 5, 8, 7, 11, 0, 1, 6, 2, 12, 9, 3, 10, 13},
	{6, 13, 14, 7, 16, 10, 11, 0, 1, 12, 15, 2, 3, 8, 9, 4, 5},
	{10, 12, 0, 8, 9, 13, 3, 4, 5, 16, 14, 15, 1, 2, 11, 6, 7},
	{5, 6, 12, 4, 13, 15, 7, 14, 8, 1, 9, 2, 16, 10, 11, 0, 3},
	{11, 15, 4, 14, 3, 1, 0, 2, 13, 12, 6, 7, 5, 16, 9, 8, 10},
	{3, 2, 1, 0, 4, 12, 13, 11, 16, 5, 6, 15, 14, 7, 9, 10, 8},
	{9, 10, 0, 7, 8, 6, 16, 3, 4, 1, 2, 5, 11, 14, 15, 13, 12},
	{10, 6, 9, 12, 11, 16, 7, 8, 0, 15, 3, 1, 2, 5, 13, 14, 4},
	{13, 0, 1, 14, 2, 3, 8, 11, 7, 12, 9, 5, 10, 15, 4, 6, 16},
	{1, 14, 2, 3, 13, 11, 7, 0, 8, 12, 9, 6, 15, 16, 5, 10, 4},
}

type scramble struct {
	key byte
	bit uint
}

func (s *scramble) decode(data []byte) {
	encoded := append([]byte(nil), data...)
	size := len(data) / 17
	for from, to := range order[len(data)%17] {
		copy(data[int(to)*size:], encoded[from*size:(from+1)*size])
	}
	for i, b := range data {
		if s.key<<s.bit&0x80 != 0 {
			data[i] = ^b
		}
		s.bit++
		if s.bit == 8 {
			s.bit = 0
			s.key = b
		}
	}
}
