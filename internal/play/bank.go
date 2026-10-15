// bank decodes chart audio assets and owns the game's audio players.
package play

import (
	"bytes"
	"embed"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/mp3"
	"github.com/hajimehoshi/ebiten/v2/audio/vorbis"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"

	"ggosu/internal/chart"
	_ "ggosu/internal/chart/ojn"
	"ggosu/internal/settings"
)

// Recorded effects (CC0, see sounds/SOURCES.txt); other effect kinds are synthesised cues.
//
//go:embed sounds/*.ogg
var sounds embed.FS

const sampleRate = 48000
const maxPCM = 512 << 20

var errPCM = errors.New("decoded audio exceeds memory limit")

const nativeBuffer = 20 * time.Millisecond

// Oto's browser worklet consumes 2048 stereo frames per main-thread refill
// (42.7ms at 48kHz). A 20ms player buffer cannot supply even one full refill.
// Leave room for two refills and browser scheduling jitter.
const webBuffer = 100 * time.Millisecond

func playerBuffer() time.Duration {
	if runtime.GOOS == "js" {
		return webBuffer
	}
	return nativeBuffer
}

var defaultHit = regexp.MustCompile(`^(normal|soft|drum)-hit(normal|whistle|finish|clap)[0-9]*\.wav$`)

func fallback(path string) []byte {
	match := defaultHit.FindStringSubmatch(filepath.Base(path))
	if match == nil {
		return nil
	}
	freq := 660.0
	switch match[2] {
	case "whistle":
		freq = 1320
	case "finish":
		freq = 330
	case "clap":
		freq = 990
	}
	frames := sampleRate / 12
	pcm := make([]byte, frames*4)
	for i := 0; i < frames; i++ {
		seconds := float64(i) / sampleRate
		value := uint16(int16(math.Sin(2*math.Pi*freq*seconds) * math.Exp(-50*seconds) * 6000))
		binary.LittleEndian.PutUint16(pcm[i*4:], value)
		binary.LittleEndian.PutUint16(pcm[i*4+2:], value)
	}
	return pcm
}

type voice struct {
	player *audio.Player
	pcm    []byte
	offset time.Duration
	volume float64
	effect bool
	music  bool
	fixed  bool // Volume is already final; a volume change does not rescale it.
}
type music struct {
	at     time.Duration
	sound  chart.Sound
	player *audio.Player
	pcm    []byte
}
type bank struct {
	ctx          *audio.Context
	pcm          map[string][]byte
	voices       []voice
	rate         float64
	paused       bool
	warnings     []string
	effects      map[string][]byte
	music        []music
	next         int
	preserve     bool
	stretched    map[stretchKey][]byte
	stretchBytes int
	audio        *settings.Audio
	loop         pauseLoop
}

type stretchKey struct {
	pcm    *byte
	length int
}

func loadBank(c *chart.Chart) (*bank, error) {
	return loadBankAssets(c, func(a chart.Asset) ([]byte, error) {
		source := c.Source.Path
		if a.Source != "" {
			source = a.Source
		}
		path, err := chart.AssetPath(source, a.Path)
		if err != nil {
			return nil, err
		}
		return readLimited(path)
	})
}

// LoadAssets permits browser fetch data and native files to use the same decoder.
// read supplies the encoded asset; it is called once per distinct path/bank.
func loadBankAssets(c *chart.Chart, read func(chart.Asset) ([]byte, error)) (*bank, error) {
	if read == nil {
		return nil, fmt.Errorf("sound: missing asset reader")
	}
	b := &bank{pcm: map[string][]byte{}, rate: 1}
	containers := map[string]map[int]chart.Sample{}
	failed := map[string]bool{}
	cache := map[string][]byte{}
	used := 0
	workers := min(4, runtime.GOMAXPROCS(0))
	if runtime.GOOS == "js" {
		workers = 1
	}
	jobs := make([]decodeJob, 0, workers)
	queued := map[string]int{}
	flush := func() {
		// Each job gets a share of the remaining retained-PCM budget. Larger
		// samples retry sequentially after the other results are accounted for.
		decodeBatch(jobs, maxPCM-used)
		save := func(j *decodeJob) {
			if j.err != nil {
				b.warnings = append(b.warnings, j.ids[0]+": "+j.err.Error())
				return
			}
			used += len(j.pcm)
			cache[j.key] = j.pcm
			for _, id := range j.ids {
				b.pcm[id] = j.pcm
			}
		}
		for i := range jobs {
			j := &jobs[i]
			if !errors.Is(j.err, errPCM) {
				save(j)
			}
		}
		for i := range jobs {
			j := &jobs[i]
			if errors.Is(j.err, errPCM) {
				j.pcm, j.err = decode(j.data, j.ext, maxPCM-used)
				save(j)
			}
		}
		clear(jobs)
		jobs = jobs[:0]
		clear(queued)
	}
	for id, a := range c.Assets {
		if a.Kind != "audio" && a.Kind != "bank" {
			continue
		}
		path := a.Path
		// Medley parts come from different folders that may reuse a file name.
		file := a.Source + "|" + path
		key := file
		if a.Kind == "bank" {
			key += fmt.Sprintf(":%d", a.Sample)
		}
		if pcm, ok := cache[key]; ok {
			b.pcm[id] = pcm
			continue
		}
		if i, ok := queued[key]; ok {
			jobs[i].ids = append(jobs[i].ids, id)
			continue
		}
		var encoded []byte
		ext := strings.ToLower(filepath.Ext(path))
		if a.Kind == "bank" {
			if failed[file] {
				continue
			}
			samples, ok := containers[file]
			if !ok {
				data, err := read(a)
				if err != nil {
					b.warnings = append(b.warnings, err.Error())
					failed[file] = true
					continue
				}
				samples, err = chart.ReadBank(path, data)
				if err != nil {
					b.warnings = append(b.warnings, path+": "+err.Error())
					if len(samples) == 0 {
						failed[file] = true
						continue
					}
				}
				containers[file] = samples
			}
			sample, ok := samples[a.Sample]
			if !ok {
				if a.Sample <= 0 || a.Sample > 2000 {
					b.warnings = append(b.warnings, fmt.Sprintf("bank sample %d missing", a.Sample))
				}
				continue
			}
			if len(sample.Data) == 0 {
				continue
			}
			encoded, ext = sample.Data, sample.Extension
		} else {
			var err error
			encoded, err = read(a)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					if pcm := fallback(a.Path); pcm != nil {
						if len(pcm) > maxPCM-used {
							b.warnings = append(b.warnings, id+": "+errPCM.Error())
							continue
						}
						used += len(pcm)
						cache[key], b.pcm[id] = pcm, pcm
						continue
					}
				}
				b.warnings = append(b.warnings, err.Error())
				continue
			}
		}
		queued[key] = len(jobs)
		jobs = append(jobs, decodeJob{ids: []string{id}, key: key, data: encoded, ext: ext})
		if len(jobs) == workers {
			flush()
		}
	}
	if len(jobs) > 0 {
		flush()
	}

	return b, nil
}

type decodeJob struct {
	ids       []string
	key, ext  string
	data, pcm []byte
	err       error
}

// All workers finish before their assets are installed or loading returns.
// Decoders own their readers; audio players stay on the game's goroutine.
func decodeBatch(jobs []decodeJob, limit int) {
	if len(jobs) == 1 {
		jobs[0].pcm, jobs[0].err = decode(jobs[0].data, jobs[0].ext, limit)
		return
	}
	var wg sync.WaitGroup
	for i := range jobs {
		wg.Go(func() {
			j := &jobs[i]
			j.pcm, j.err = decode(j.data, j.ext, limit/len(jobs))
		})
	}
	wg.Wait()
}

func readLimited(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	b, readErr := io.ReadAll(io.LimitReader(f, 256<<20+1))
	closeErr := f.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(b) > 256<<20 {
		return nil, fmt.Errorf("audio asset too large: %s", path)
	}
	return b, nil
}

// A codec stream exposes source-rate stereo PCM; resampling uses the cached
// sinc weights rather than rebuilding the filter for every output frame.
type pcmStream interface {
	io.Reader
	SampleRate() int
	Length() int64
}

func decode(data []byte, ext string, limit int) ([]byte, error) {
	if limit < 0 {
		return nil, errPCM
	}
	r := bytes.NewReader(data)
	var stream pcmStream
	var err error
	switch ext {
	case ".wav":
		stream, err = wav.DecodeWithoutResampling(r)
	case ".ogg":
		stream, err = vorbis.DecodeWithoutResampling(r)
	case ".mp3":
		stream, err = mp3.DecodeWithoutResampling(r)
	default:
		return nil, fmt.Errorf("unsupported audio %s", ext)
	}
	if err != nil {
		return nil, err
	}
	rate, length := stream.SampleRate(), stream.Length()
	if rate <= 0 {
		return nil, fmt.Errorf("sound: invalid sample rate")
	}
	if length > 0 && length/4*int64(sampleRate)/int64(rate) > int64(limit/4) {
		return nil, errPCM
	}
	if length < 0 || length > maxPCM {
		// Keep large/unknown sources bounded without storing all source PCM.
		r := audio.ResampleReader(stream, length, rate, sampleRate)
		pcm, err := io.ReadAll(io.LimitReader(r, int64(limit)+1))
		if err != nil {
			return nil, err
		}
		if len(pcm) > limit {
			return nil, errPCM
		}
		return pcm, nil
	}
	var buf bytes.Buffer
	if length > 0 {
		buf.Grow(int(length))
	}
	// Hide ReaderFrom so the codec sees a bounded scratch buffer.
	_, err = io.CopyBuffer(struct{ io.Writer }{&buf}, io.LimitReader(stream, maxPCM+1), make([]byte, 32<<10))
	if err != nil {
		return nil, err
	}
	if buf.Len() > maxPCM {
		return nil, errPCM
	}
	return resamplePCM(buf.Bytes(), rate, sampleRate, limit)
}

func (b *bank) newPlayer(pcm []byte, offset time.Duration, volume float64) (*audio.Player, error) {
	p, err := b.player(pcm, offset, volume, b.rate)
	if err == nil && !b.paused {
		p.Play()
	}
	return p, err
}
func (b *bank) player(pcm []byte, offset time.Duration, volume, rate float64) (*audio.Player, error) {
	if b.ctx == nil {
		b.ctx = audio.CurrentContext()
		if b.ctx == nil {
			b.ctx = audio.NewContext(sampleRate)
		}
	}
	playRate := rate
	if b.preserve && rate != 1 && len(pcm) > 0 {
		data, err := b.stretch(pcm, rate)
		if err != nil {
			return nil, err
		}
		pcm, playRate = data, 1
	}
	stream := audio.ResampleReader(bytes.NewReader(pcm), int64(len(pcm)), int(math.Round(sampleRate*playRate)), sampleRate)
	p, err := b.ctx.NewPlayer(stream)
	if err != nil {
		return nil, err
	}
	p.SetVolume(volume)
	p.SetBufferSize(playerBuffer())
	if offset > 0 {
		if err := p.SetPosition(time.Duration(float64(offset) / rate)); err != nil {
			p.PauseAndStopReading()
			return nil, err
		}
	}
	return p, nil
}

func (b *bank) stretch(pcm []byte, rate float64) ([]byte, error) {
	if len(pcm) == 0 {
		return pcm, nil
	}
	key := stretchKey{&pcm[0], len(pcm)}
	if data, ok := b.stretched[key]; ok {
		return data, nil
	}
	data, err := stretchPCM(pcm, rate)
	if err != nil {
		return nil, err
	}
	if b.stretchBytes+len(data) > maxPCM {
		b.stretched = nil
		b.stretchBytes = 0
	}
	if b.stretched == nil {
		b.stretched = make(map[stretchKey][]byte)
	}
	b.stretched[key] = data
	b.stretchBytes += len(data)
	return data, nil
}

func (b *bank) soundPCM(s chart.Sound) ([]byte, bool) {
	pcm, ok := b.pcm[s.Asset]
	if !ok {
		return nil, false
	}
	if s.From > 0 || s.Length > 0 || s.FadeIn > 0 || s.FadeOut > 0 {
		if pcm = clipPCM(pcm, s); len(pcm) == 0 {
			return nil, false
		}
	}
	if s.Pan != 0 {
		pcm = append([]byte(nil), pcm...)
		left, right := 1.0, 1.0
		if s.Pan > 0 {
			left = 1 - s.Pan
		} else {
			right = 1 + s.Pan
		}
		for i := 0; i+3 < len(pcm); i += 4 {
			binary.LittleEndian.PutUint16(pcm[i:], uint16(int16(float64(int16(binary.LittleEndian.Uint16(pcm[i:])))*left)))
			binary.LittleEndian.PutUint16(pcm[i+2:], uint16(int16(float64(int16(binary.LittleEndian.Uint16(pcm[i+2:])))*right)))
		}
	}
	return pcm, true
}

// clipPCM cuts a medley clip out of 16-bit stereo PCM and applies its linear fades.
func clipPCM(pcm []byte, s chart.Sound) []byte {
	frame := func(d time.Duration) int { return int(d * sampleRate / time.Second) }
	frames := len(pcm) / 4
	from := min(frames, frame(s.From))
	to := frames
	if s.Length > 0 {
		to = min(frames, from+frame(s.Length))
	}
	if from >= to {
		return nil
	}
	out := append([]byte(nil), pcm[from*4:to*4]...)
	n, in, fadeOut := to-from, frame(s.FadeIn), frame(s.FadeOut)
	for i := range n {
		g := 1.0
		if i < in {
			g = float64(i) / float64(in)
		}
		if left := n - i; left < fadeOut {
			g = min(g, float64(left)/float64(fadeOut))
		}
		if g == 1 {
			continue
		}
		for c := i * 4; c < i*4+4; c += 2 {
			binary.LittleEndian.PutUint16(out[c:], uint16(int16(float64(int16(binary.LittleEndian.Uint16(out[c:])))*g)))
		}
	}
	return out
}

// SetAudio updates playing, paused and primed players without seeking or restarting.
func (b *bank) SetAudio(a settings.Audio) {
	b.audio = &a
	for _, v := range b.voices {
		if !v.fixed {
			v.player.SetVolume(v.volume * b.gain(v.music, v.effect))
		}
	}
	for _, m := range b.music {
		if m.player != nil {
			m.player.SetVolume(m.sound.Volume * b.gain(true, false))
		}
	}
	if b.loop.player != nil {
		b.loop.player.SetVolume(b.loop.volume * b.gain(false, true))
	}
}

func (b *bank) gain(music, effect bool) float64 {
	if b.audio == nil {
		return 1
	}
	a := b.audio
	v := a.Keys
	if effect {
		v = a.Effects
	} else if music {
		v = a.Music
	}
	return float64(a.Master) * float64(v) / 10000
}

func (b *bank) Play(s chart.Sound) error {
	return b.play(s, false)
}

func (b *bank) play(s chart.Sound, music bool) error {
	pcm, ok := b.soundPCM(s)
	if !ok {
		return nil
	}
	b.prune()
	if len(b.voices) >= 256 {
		return fmt.Errorf("too many simultaneous audio voices")
	}
	p, err := b.newPlayer(pcm, 0, s.Volume*b.gain(music, false))
	if err != nil {
		return err
	}
	b.voices = append(b.voices, voice{player: p, pcm: pcm, volume: s.Volume, music: music})
	return nil
}

// PrepareMusic retains the schedule and primes the first players without
// starting playback. Only 32 upcoming players are prepared at a time.
func (b *bank) PrepareMusic(events []chart.Audio) error {
	if len(b.music) != 0 {
		return fmt.Errorf("music schedule already prepared")
	}
	b.next = 0
	for _, e := range events {
		b.music = append(b.music, music{at: e.At, sound: e.Sound})
	}
	if len(events) == 0 {
		return nil
	}
	return b.PrimeMusic(events[0].At)
}

func (b *bank) PrimeMusic(at time.Duration) error {
	for i := b.next; i < min(b.next+32, len(b.music)); i++ {
		m := &b.music[i]
		if m.at > at+250*time.Millisecond {
			break
		}
		if m.player != nil {
			continue
		}
		pcm, ok := b.soundPCM(m.sound)
		if !ok || pcmEnded(pcm, at-m.at) {
			continue
		}
		p, err := b.player(pcm, 0, m.sound.Volume*b.gain(true, false), b.rate)
		if err != nil {
			return err
		}
		m.player, m.pcm = p, pcm
	}
	return nil
}

// PlayMusic starts a primed player. Late game frames seek to the current song
// position rather than shifting the whole BGM behind the shared timeline.
// This compensates software scheduling only, not physical output latency.
func (b *bank) PlayMusic(s chart.Sound, at time.Duration) error {
	if len(b.music) == 0 {
		return b.play(s, true)
	}
	if b.next >= len(b.music) || b.music[b.next].sound != s {
		return fmt.Errorf("prepared music order changed")
	}
	m := &b.music[b.next]
	b.next++
	pcm, ok := b.soundPCM(s)
	if !ok {
		return nil
	}
	late := max(time.Duration(0), at-m.at)
	if pcmEnded(pcm, late) {
		// A section starting mid-song skips sounds that already ended without decoding players.
		if m.player != nil {
			m.player.PauseAndStopReading()
			m.player = nil
		}
		return nil
	}
	p := m.player
	if p == nil {
		var err error
		p, err = b.player(pcm, 0, s.Volume*b.gain(true, false), b.rate)
		if err != nil {
			return err
		}
	}
	m.player = nil
	if late > 0 {
		if err := p.SetPosition(time.Duration(float64(late) / b.rate)); err != nil {
			p.PauseAndStopReading()
			return err
		}
	}
	b.prune()
	if len(b.voices) >= 256 {
		p.PauseAndStopReading()
		return fmt.Errorf("too many simultaneous audio voices")
	}
	if !b.paused {
		p.Play()
	}
	b.voices = append(b.voices, voice{player: p, pcm: pcm, offset: late, volume: s.Volume, music: true})
	return nil
}

// pcmEnded reports whether a sound started this long ago has finished.
func pcmEnded(pcm []byte, late time.Duration) bool {
	return late >= time.Duration(len(pcm)/4)*time.Second/sampleRate
}

func (b *bank) Pause(paused bool) {
	b.paused = paused
	for _, v := range b.voices {
		if v.effect {
			continue
		}
		if paused {
			v.player.Pause()
		} else {
			v.player.Play()
		}
	}
}

// Playing includes the tail of BGM and key sounds after the final judgement.
func (b *bank) Playing() bool {
	for _, v := range b.voices {
		if v.player.IsPlaying() {
			return true
		}
	}
	return false
}

func (b *bank) SetRate(rate float64) error {
	return b.change(rate, b.preserve)
}

// SetPitch selects whether rate changes also change pitch. The default is true.
func (b *bank) SetPitch(pitch bool) error {
	return b.change(b.rate, !pitch)
}

func (b *bank) change(rate float64, preserve bool) error {
	if math.IsNaN(rate) || math.IsInf(rate, 0) || rate < .5 || rate > 2 {
		return fmt.Errorf("audio rate must be 0.5..2.0")
	}
	if rate == b.rate && preserve == b.preserve {
		return nil
	}
	old := b.rate
	b.rate = rate
	b.preserve = preserve
	b.stretched, b.stretchBytes = nil, 0
	for i := b.next; i < len(b.music); i++ {
		m := &b.music[i]
		if m.player != nil {
			m.player.PauseAndStopReading()
			m.player = nil
		}
	}
	// Prepare before gameplay starts, rather than stretching the first BGM in
	// the frame that starts it. Reused asset buffers share this cache.
	if preserve && rate != 1 {
		for _, pcm := range b.pcm {
			if _, err := b.stretch(pcm, rate); err != nil {
				return err
			}
		}
	}
	active := b.voices[:0]
	for _, v := range b.voices {
		if v.effect {
			active = append(active, v)
			continue
		}
		if !v.player.IsPlaying() && !b.paused {
			v.player.PauseAndStopReading()
			continue
		}
		position := v.offset + time.Duration(float64(v.player.Position())*old)
		v.player.PauseAndStopReading()
		p, err := b.newPlayer(v.pcm, position, v.volume*b.gain(v.music, false))
		if err != nil {
			return err
		}
		v.player = p
		v.offset = position
		active = append(active, v)
	}
	b.voices = active
	return nil
}

func (b *bank) prune() {
	active := b.voices[:0]
	for _, v := range b.voices {
		if v.player.IsPlaying() || b.paused && !v.effect {
			active = append(active, v)
		} else {
			v.player.PauseAndStopReading()
		}
	}
	b.voices = active
}

func cuePCM(kind string) ([]byte, error) {
	freq := 0.0
	switch kind {
	case "start":
		freq = 880
	case "resume":
		freq = 660
	case "finish":
		freq = 1046
	case "mention":
		freq = 1320
	default:
		return nil, fmt.Errorf("unknown playback effect %q", kind)
	}
	frames := sampleRate / 12
	pcm := make([]byte, frames*4)
	for i := 0; i < frames; i++ {
		t := float64(i) / sampleRate
		v := uint16(int16(math.Sin(2*math.Pi*freq*t) * math.Exp(-55*t) * 3500))
		binary.LittleEndian.PutUint16(pcm[i*4:], v)
		binary.LittleEndian.PutUint16(pcm[i*4+2:], v)
	}
	return pcm, nil
}

// Effects have their own volume, pitch and pause policy.
func (b *bank) Effect(kind string, volume float64) error {
	if math.IsNaN(volume) || volume < 0 || volume > 1 {
		return fmt.Errorf("invalid effect volume")
	}
	if volume*b.gain(false, true) == 0 {
		return nil
	}
	pcm, err := b.effectPCM(kind)
	if err != nil {
		return err
	}
	return b.start(voice{pcm: pcm, volume: volume, effect: true}, volume*b.gain(false, true))
}

// Cue plays a fixed-pitch sound at its own volume percent, scaled only by master:
// ONGEKI's hit and guide sounds have volumes of their own beside the effects.
func (b *bank) Cue(kind string, volume int) error {
	gain := float64(volume) / 100
	if b.audio != nil {
		gain *= float64(b.audio.Master) / 100
	}
	if gain == 0 {
		return nil
	}
	pcm, err := b.effectPCM(kind)
	if err != nil {
		return err
	}
	return b.start(voice{pcm: pcm, volume: gain, effect: true, fixed: true}, gain)
}

func (b *bank) effectPCM(kind string) ([]byte, error) {
	if b.effects == nil {
		b.effects = make(map[string][]byte)
	}
	if pcm, ok := b.effects[kind]; ok {
		return pcm, nil
	}
	var pcm []byte
	var err error
	if data, readErr := sounds.ReadFile("sounds/" + kind + ".ogg"); readErr == nil {
		pcm, err = decode(data, ".ogg", maxPCM)
	} else {
		pcm, err = cuePCM(kind)
	}
	if err != nil {
		return nil, err
	}
	b.effects[kind] = pcm
	return pcm, nil
}

func (b *bank) start(v voice, gain float64) error {
	b.prune()
	if len(b.voices) >= 256 {
		return fmt.Errorf("too many simultaneous audio voices")
	}
	p, err := b.player(v.pcm, 0, gain, 1)
	if err != nil {
		return err
	}
	p.Play()
	v.player = p
	b.voices = append(b.voices, v)
	return nil
}
func (b *bank) Close() {
	for _, v := range b.voices {
		v.player.PauseAndStopReading()
	}
	b.voices = nil
	for _, m := range b.music {
		if m.player != nil {
			m.player.PauseAndStopReading()
		}
	}
	b.music = nil
	if b.loop.player != nil {
		b.loop.player.PauseAndStopReading()
	}
	b.loop = pauseLoop{}
}
