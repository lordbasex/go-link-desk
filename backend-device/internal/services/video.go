// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"errors"
	"slices"
	"sync"
	"time"
	"unsafe"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"

	screencap "github.com/lordbasex/go-link-desk/backend-device/pkg/capture"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/encoder"
)

// Frame is one picture for the encoders: the test card's packed I420, or a
// screen capture's NV12 pixel buffer (valid only while VideoFrame runs).
type Frame struct {
	I420          []byte
	PixelBuffer   unsafe.Pointer
	Width, Height int
	PTS           time.Duration // on the source's clock; frames come when the picture changes
}

// Preset is the viewer's choice between quality and speed.
type Preset string

const (
	PresetSpeed    Preset = "speed"
	PresetBalanced Preset = "balanced" // the default
	PresetQuality  Preset = "quality"
)

// presetLimits is what each preset allows at most: the picture's width
// (0: the display's own pixels), frames per second and bitrate. The
// congestion control and the viewer's own window lower them.
var presetLimits = map[Preset]struct {
	maxWidth, fps, kbps int
}{
	PresetSpeed:    {1280, 60, 4000},
	PresetBalanced: {1920, 60, 10000},
	PresetQuality:  {0, 60, 20000},
}

// ParsePreset reads a preset name; anything else is Balanced.
func ParsePreset(s string) Preset {
	if _, ok := presetLimits[Preset(s)]; ok {
		return Preset(s)
	}
	return PresetBalanced
}

// minVideoSide keeps pictures readable however small the viewer's window.
const minVideoSide = 320

// bandwidthLadder is what a short bandwidth allows: under each estimate,
// a narrower picture and fewer frames per second. Fewer, sharper frames
// read better than many blurry ones, and a smaller picture makes smaller
// frames, which arrive whole on a bad network. The bitrate alone is not
// enough: some encoders (VideoToolbox on Intel Macs) send far more than
// asked for on a busy screen. Rungs go down at once; back up one at a
// time, only when the estimate is a third above them and the last change
// is ladderClimbWait old, so the picture does not flap: every new size is
// a new stream for the viewer's decoder, which some (iPadOS) do not take
// many times in a row.
var bandwidthLadder = []struct {
	underKbps, maxWidth, fps int
}{
	{400, 640, 15},
	{800, 960, 20},
	{1500, 1280, 30},
	{2500, 0, 30}, // the preset's width, at 30
}

// ladderClimbWait is how long a rung holds before the picture grows again.
const ladderClimbWait = 10 * time.Second

// ladderRung is the rung for an estimate, starting from the current one
// (len(bandwidthLadder): no limit), changed since ago. est 0 (no estimate
// yet) keeps it.
func ladderRung(est, current int, since time.Duration) int {
	if est <= 0 {
		return current
	}
	for i, r := range bandwidthLadder {
		if est < r.underKbps {
			if i < current {
				return i // down at once
			}
			break
		}
	}
	// up one rung, with room to spare and after a while
	if since >= ladderClimbWait && current < len(bandwidthLadder) && est >= bandwidthLadder[current].underKbps*4/3 {
		current++
	}
	return current
}

// minKbps is the least an encoder is asked for.
const minKbps = 300

// VideoWant is what the viewer would like: the size it shows the picture
// at (device pixels), the preset, and whether to send the display's
// native pixels (Retina) instead of its size in points.
type VideoWant struct {
	Width, Height int
	Preset        Preset
	Retina        bool
	// CursorInVideo draws the pointer into the picture (games, pointer
	// lock) instead of sending its shape and position for the viewer to
	// draw.
	CursorInVideo bool
}

// captureSize picks the picture's size for a display: the display's
// aspect, no larger than the display (its points, or its pixels with
// Retina or the Quality preset), the preset's width and the viewer's
// window, and even.
func captureSize(d screencap.Display, want VideoWant) (w, h int) {
	srcW, srcH := d.PointsWidth, d.PointsHeight
	if want.Retina || want.Preset == PresetQuality || srcW <= 0 {
		srcW, srcH = d.Width, d.Height
	}
	if srcW <= 0 || srcH <= 0 {
		return 1280, 720
	}
	w = srcW
	if limit := presetLimits[ParsePreset(string(want.Preset))].maxWidth; limit > 0 && w > limit {
		w = limit
	}
	// The viewer's window, in its own aspect: fit the display into it.
	// Not with Quality: it sends the display's pixels as they are, so
	// text is never resampled on the way (shrinking it by a few percent
	// is what blurs it most).
	if want.Width > 0 && want.Height > 0 && want.Preset != PresetQuality {
		fit := min(want.Width, want.Height*srcW/srcH)
		// A window a little smaller than the picture (a border, a toolbar)
		// gets the picture as it is: shrinking text by a few pixels blurs
		// all of it and saves nothing.
		if fit*10 < w*9 {
			w = max(fit, minVideoSide)
		}
	}
	h = w * srcH / srcW
	if h < minVideoSide && srcH >= minVideoSide {
		h = minVideoSide
		w = h * srcW / srcH
	}
	return w &^ 1, h &^ 1
}

// Video codecs a viewer's browser can take (the knock's "codecs").
const (
	CodecH264High = "h264-high" // Constrained High (H.264 profile 640c)
	CodecH264     = "h264"      // Constrained Baseline
	CodecVP8      = "vp8"
)

// pickCodec chooses the session's codec: H.264 from the system's encoder
// when there is one and the browser takes it (High when it can), else
// VP8.
func pickCodec(browser []string, hasH264 bool) string {
	switch {
	case hasH264 && slices.Contains(browser, CodecH264High):
		return CodecH264High
	case hasH264 && slices.Contains(browser, CodecH264):
		return CodecH264
	default:
		return CodecVP8
	}
}

// The H.264 lines the host offers. Pion's defaults have Baseline and
// High; Constrained High is added (payload types 114 and 115 are free).
const (
	h264HighFmtp     = "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=640c1f"
	h264BaselineFmtp = "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f"
)

// trackCodec is the track's capability for a codec.
func trackCodec(codec string) webrtc.RTPCodecCapability {
	switch codec {
	case CodecH264High:
		return webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000, SDPFmtpLine: h264HighFmtp}
	case CodecH264:
		return webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000, SDPFmtpLine: h264BaselineFmtp}
	default:
		return webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000}
	}
}

// videoEncoder is one viewer's encoder.
type videoEncoder interface {
	// encode takes a frame; the encoded frame goes to the viewer's track.
	encode(f Frame, key bool) error
	setBitrate(kbps int)
	size() (w, h int)
	close()
}

// sampleWriter writes encoded frames to a track, timed by their
// presentation times: frames come only when the picture changes, so each
// one lasts until the next.
type sampleWriter struct {
	track *webrtc.TrackLocalStaticSample

	mu     sync.Mutex // the encoder's thread writes, the stats ticker counts
	last   time.Duration
	have   bool
	frames int // since the last count (for stream_stats)
	bytes  int
}

// write sends one frame. A sample's duration moves the next one's RTP
// time, so each frame carries the time since the previous.
func (w *sampleWriter) write(data []byte, pts time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	dur := time.Second / 60
	if w.have && pts > w.last {
		dur = pts - w.last
	}
	w.last, w.have = pts, true
	if err := w.track.WriteSample(media.Sample{Data: data, Duration: dur}); err != nil && !errors.Is(err, errClosedPipe) {
		return
	}
	w.frames++
	w.bytes += len(data)
}

// counts reads and resets the frames and bytes sent.
func (w *sampleWriter) counts() (frames, bytes int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	frames, bytes = w.frames, w.bytes
	w.frames, w.bytes = 0, 0
	return
}

// codecLabel is how stream_stats names a codec.
func codecLabel(codec string) string {
	if codec == CodecVP8 {
		return "VP8"
	}
	return "H.264"
}

// h264Encoder is the system's H.264 encoder. It takes the capture's pixel
// buffers as they are; the test card's I420 frames are copied in.
type h264Encoder struct {
	enc  *encoder.H264
	w, h int
}

func newH264Encoder(w, h, fps, kbps int, baseline bool, out *sampleWriter) (*h264Encoder, error) {
	e := &h264Encoder{w: w, h: h}
	enc, err := encoder.NewH264(encoder.Config{Width: w, Height: h, FPS: fps, BitrateKbps: kbps, GOPFrames: fps * 10, Baseline: baseline}, out.write)
	if err != nil {
		return nil, err
	}
	e.enc = enc
	return e, nil
}

func (e *h264Encoder) encode(f Frame, key bool) error {
	if f.PixelBuffer != nil {
		return e.enc.WritePixelBuffer(f.PixelBuffer, f.PTS, key)
	}
	return e.enc.WriteKey(f.I420, key)
}

func (e *h264Encoder) setBitrate(kbps int) { e.enc.SetBitrate(kbps) }
func (e *h264Encoder) size() (int, int)    { return e.w, e.h }
func (e *h264Encoder) close()              { e.enc.Close() }

// vp8Encoder is libvpx: the fallback for browsers without H.264 and
// systems without a system encoder. Its bitrate changes start it again
// (with a keyframe), so they are kept rare.
type vp8Encoder struct {
	enc       *encoder.VP8
	w, h, fps int
	kbps      int
	setAt     time.Time
	i420      []byte
	out       *sampleWriter
}

func newVP8Encoder(w, h, fps, kbps int, out *sampleWriter) (*vp8Encoder, error) {
	enc, err := encoder.NewVP8(encoder.Config{Width: w, Height: h, FPS: fps, BitrateKbps: kbps, Threads: vp8Threads(w, h)})
	if err != nil {
		return nil, err
	}
	return &vp8Encoder{enc: enc, w: w, h: h, fps: fps, kbps: kbps, setAt: time.Now(), out: out}, nil
}

// vp8Threads gives large pictures more of libvpx's threads.
func vp8Threads(w, h int) int {
	switch px := w * h; {
	case px >= 1920*1080:
		return 4
	case px >= 1280*720:
		return 3
	default:
		return 2
	}
}

func (e *vp8Encoder) encode(f Frame, key bool) error {
	frame := f.I420
	if f.PixelBuffer != nil {
		if cap(e.i420) < encoder.FrameSize(e.w, e.h) {
			e.i420 = make([]byte, encoder.FrameSize(e.w, e.h))
		}
		e.i420 = e.i420[:encoder.FrameSize(e.w, e.h)]
		if err := screencap.ToI420(f.PixelBuffer, e.i420, e.w, e.h); err != nil {
			return err
		}
		frame = e.i420
	}
	data, _, err := e.enc.Encode(frame, key)
	if err != nil || len(data) == 0 {
		return err
	}
	e.out.write(data, f.PTS)
	return nil
}

// setBitrate restarts the encoder only for a change of a fifth or more,
// and not more than every two seconds.
func (e *vp8Encoder) setBitrate(kbps int) {
	if kbps <= 0 || time.Since(e.setAt) < 2*time.Second || (kbps*5 > e.kbps*4 && kbps*5 < e.kbps*6) {
		return
	}
	enc, err := encoder.NewVP8(encoder.Config{Width: e.w, Height: e.h, FPS: e.fps, BitrateKbps: kbps, Threads: vp8Threads(e.w, e.h)})
	if err != nil {
		return
	}
	e.enc.Close()
	e.enc, e.kbps, e.setAt = enc, kbps, time.Now()
}

func (e *vp8Encoder) size() (int, int) { return e.w, e.h }
func (e *vp8Encoder) close()           { e.enc.Close() }
