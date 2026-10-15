package ojn

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"testing"
)

// Fixed vectors exercise block permutation, the unpermuted remainder and the
// byte inversion key carried from one WAV payload into the next.
func TestOMC(t *testing.T) {
	decode := func(s string) []byte {
		t.Helper()
		b, err := hex.DecodeString(s)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	encoded := [][]byte{decode("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f2021222324252627"), decode("32333435363738393a3b3c3d3e3f404142434445464748494a4b4c4d4e4f5051525354555657")}
	plain := [][]byte{decode("efeee1e0e3e2fffe16171819080906f804050a0b12ecebea1a1b0ef00cf220de0203dd23242526d8"), decode("3a3bc13f34cabbba3cc240414ab442bc50ae46474e4fb3b236c83839adac32cc4849abaa5657")}
	var bank bytes.Buffer
	bank.Write(make([]byte, 20))
	for i, data := range encoded {
		if i == 1 {
			bank.Write(make([]byte, 56))
		} // An empty slot must not advance the key.
		h := make([]byte, 56)
		binary.LittleEndian.PutUint16(h[32:], 1)
		binary.LittleEndian.PutUint16(h[34:], 1)
		binary.LittleEndian.PutUint32(h[36:], 48000)
		binary.LittleEndian.PutUint32(h[40:], 96000)
		binary.LittleEndian.PutUint16(h[44:], 2)
		binary.LittleEndian.PutUint16(h[46:], 16)
		binary.LittleEndian.PutUint32(h[52:], uint32(len(data)))
		bank.Write(h)
		bank.Write(data)
	}
	oggStart := bank.Len()
	h := make([]byte, 36)
	binary.LittleEndian.PutUint32(h[32:], 8)
	bank.Write(h)
	bank.WriteString("OggSdata")
	data := bank.Bytes()
	copy(data, "OMC\x00")
	binary.LittleEndian.PutUint32(data[8:], 20)
	binary.LittleEndian.PutUint32(data[12:], uint32(oggStart))
	binary.LittleEndian.PutUint32(data[16:], uint32(len(data)))
	original := append([]byte(nil), data...)
	samples, err := (Format{}).ReadBank(data)
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range []int{1, 3} {
		if !bytes.Equal(samples[id].Data[44:], plain[i]) {
			t.Fatalf("sample %d: got %x, want %x", id, samples[id].Data[44:], plain[i])
		}
	}
	if !bytes.Equal(samples[1001].Data, []byte("OggSdata")) {
		t.Fatal("OMC altered Ogg data")
	}
	if _, ok := samples[2]; ok {
		t.Fatal("empty slot contains audio")
	}
	if !bytes.Equal(data, original) {
		t.Fatal("decoder changed the source bank")
	}
	copy(data, "OJM\x00")
	samples, err = (Format{}).ReadBank(data)
	if err != nil || !bytes.Equal(samples[1].Data[44:], encoded[0]) {
		t.Fatal("unencrypted OJM was unscrambled", err)
	}
}
