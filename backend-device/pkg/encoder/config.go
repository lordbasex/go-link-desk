// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package encoder

// Config describes the video stream.
type Config struct {
	Width, Height int
	FPS           int
	BitrateKbps   int
	// MinQuantizer and MaxQuantizer bound the quantizer (0-63, lower is
	// better quality); CPUUsed is the libvpx speed preset (-16..16: a
	// negative one is fixed, a positive one lets libvpx pick by the time
	// left). Zero keeps the streaming defaults (4, 56 and -6); the video
	// quality lab sets them to compare other tunings.
	MinQuantizer, MaxQuantizer int
	CPUUsed                    int
	// Threads is libvpx's thread count; zero keeps the streaming default (2).
	// HD sizes need more (go-link HD, experiment T-31).
	Threads int
	// Screen turns on libvpx's screen content mode (a desktop, not a
	// camera or a game).
	Screen bool
	// GOPFrames is H264's keyframe interval (0: two seconds); a viewer
	// switched onto a stream starts at its next keyframe.
	GOPFrames int
	// H.264 tuning, zero values are the streaming defaults: Constrained
	// High profile, the low-latency rate control, no quantizer cap.
	// Baseline and NoLowLatency go back to the plainer settings; MaxQP
	// caps the quantizer (1-51, lower keeps text sharper but costs bits).
	Baseline     bool
	NoLowLatency bool
	MaxQP        int
}

// timebase is the frames' clock (Hz), the C side's TIMEBASE.
const timebase = 90000

// Encoder defaults, used when Config leaves the tuning fields at zero.
const (
	DefaultMinQuantizer = 4
	DefaultMaxQuantizer = 56
	// DefaultCPUUsed is a fixed speed 6, WebRTC's. libvpx's own choice
	// (positive values) runs to its fastest settings on a busy screen,
	// which there are slower and far past the bitrate: build/vbench, 2
	// cores of a 1.8 GHz Xeon, scrolling text at 1080p: 22 fps and 45
	// Mbps for 8 asked with 8; 45 fps and 9 Mbps with -6. The overshoot
	// was what the congestion control took for a full network, dropping
	// the picture to 640x400 on a LAN.
	DefaultCPUUsed = -6
)

// FrameSize returns the byte length of a packed I420 frame.
func FrameSize(w, h int) int {
	cw, ch := (w+1)/2, (h+1)/2
	return w*h + 2*cw*ch
}

// OpusSampleRate is the only rate WebRTC uses for Opus.
const OpusSampleRate = 48000

// OpusFrameSamples is one 20 ms frame per channel at 48 kHz.
const OpusFrameSamples = OpusSampleRate / 50
