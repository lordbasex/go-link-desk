// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build lordos

package services

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/pion/webrtc/v4/pkg/media"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/lordshare"
)

// On LordOS frames arrive already encoded from the session, so they skip
// VideoFrame's encoder per viewer.

// EncodedWant is what the viewers need from an encoded source.
type EncodedWant struct {
	Viewers int
	Codec   string // the first viewer's (CodecH264 or CodecVP8), "" with none
	Kbps    int
	NeedKey bool
}

// EncodedWant reports the current viewers' needs.
func (s *StreamService) EncodedWant() EncodedWant {
	s.mu.Lock()
	defer s.mu.Unlock()
	var w EncodedWant
	for _, v := range s.viewers {
		w.Viewers++
		if w.Codec == "" || int(v.kbps.Load()) < w.Kbps {
			w.Kbps = int(v.kbps.Load())
		}
		if w.Codec == "" {
			w.Codec = v.codec
		}
		if v.keyframe.Load() {
			w.NeedKey = true
		}
	}
	return w
}

// EncodedFrame sends one encoded frame to every viewer on that codec.
func (s *StreamService) EncodedFrame(codec string, key bool, pts time.Duration, data []byte, w, h int) {
	for _, v := range s.viewerList() {
		if v.codec != codec || v.out == nil {
			continue
		}
		if key {
			v.keyframe.Store(false)
		}
		s.mu.Lock()
		s.sizes[v.id] = [2]int{w, h}
		s.mu.Unlock()
		v.out.write(data, pts)
	}
}

// EncodedAudio sends one Opus packet to the viewers that may hear it.
func (s *StreamService) EncodedAudio(pkt []byte, d time.Duration) {
	for _, t := range s.listeners() {
		_ = t.WriteSample(media.Sample{Data: pkt, Duration: d}) // a closed viewer is dropped elsewhere
	}
}

// EncodedResized tells the viewers the session's new size: pointer
// positions are relative to it.
func (s *StreamService) EncodedResized() {
	s.refreshDisplays()
	for _, v := range s.viewerList() {
		s.sendDisplays(v)
	}
}

// PathInfo describes the ICE pair each viewer is using.
func (s *StreamService) PathInfo() []string {
	var out []string
	for _, v := range s.viewerList() {
		st := v.pc.SCTP()
		if st == nil || st.Transport() == nil || st.Transport().ICETransport() == nil {
			continue
		}
		p, err := st.Transport().ICETransport().GetSelectedCandidatePair()
		if err != nil || p == nil {
			continue
		}
		out = append(out, fmt.Sprintf("local=%s/%s remote=%s/%s", p.Local.Typ, p.Local.Protocol, p.Remote.Typ, p.Remote.Protocol))
	}
	return out
}

// EncodedCodec picks a viewer's codec: the session's H.264 line is
// Constrained Baseline, so a viewer that takes High gets Baseline, and
// never the High the other platforms' encoder sends.
func EncodedCodec(browser []string) string {
	if slices.Contains(browser, CodecH264) {
		return CodecH264
	}
	return CodecVP8
}

// Encoded is what EncodedSource needs of the stream, which tests stand in
// for.
type Encoded interface {
	EncodedWant() EncodedWant
	EncodedFrame(codec string, key bool, pts time.Duration, data []byte, w, h int)
	EncodedAudio(pkt []byte, d time.Duration)
	Hearing() bool
	Notice(code string)
	EncodedResized()
}

// The encoder's limits.
const (
	encodedHz    = 30
	encodedPoll  = 50 * time.Millisecond
	keyEvery     = 500 * time.Millisecond
	audioDefault = 20 * time.Millisecond
)

// EncodedSource is a MediaSource for frames that arrive encoded through
// LordOS's screen-sharing API. The share lasts as long as a viewer does:
// it starts when the first one is in and stops when the last one leaves.
// It keeps the session's size in step and follows the viewers: PLI asks for
// a keyframe, the bandwidth estimate sets the rate, hearing gates the sound.
type EncodedSource struct {
	Share  *lordshare.Client
	Stream Encoded
	Log    *slog.Logger
	// End is called when the system takes the share away while a viewer
	// is in (the indicator's Stop, a lock, a revoked permission, another
	// door). Nothing can be shown any more: the session must end.
	End func(reason string)

	mu   sync.Mutex
	w, h int
	last uint32 // the last audio packet's first sample
}

// Run follows the viewers until ctx ends.
func (e *EncodedSource) Run(ctx context.Context, sink MediaSink) error {
	if e.Log == nil {
		e.Log = slog.Default()
	}
	over := make(chan lordshare.Reason, 4)
	e.Share.SetMedia(lordshare.Media{
		Video:   e.video,
		Audio:   e.audio,
		Size:    e.size,
		Started: func(st lordshare.State) { e.Log.Info("share started", "state", st.String()) },
		Refused: func(r lordshare.Reason) { signal(over, r) },
		Ended:   func(r lordshare.Reason) { signal(over, r) },
	})
	if cs, ok := sink.(CursorSink); ok {
		go followCursor(ctx, cs, sink)
	}
	e.drive(ctx, over, e.Share, time.NewTicker(encodedPoll).C)
	return ctx.Err()
}

func signal(c chan lordshare.Reason, r lordshare.Reason) {
	select {
	case c <- r:
	default:
	}
}

// shareDriver is the part of the client drive uses.
type shareDriver interface {
	Start(flags, codec, hz, kbps int)
	Stop()
	Keyframe()
	Rate(kbps int)
	Hear(on bool)
	Done() <-chan struct{}
}

func lordCodec(c string) int {
	switch c {
	case CodecH264:
		return lordshare.CodecH264
	case CodecVP8:
		return lordshare.CodecVP8
	}
	return 0
}

func (e *EncodedSource) drive(ctx context.Context, over <-chan lordshare.Reason, share shareDriver, tick <-chan time.Time) {
	var codec, kbps int
	var lastKey time.Time
	hear, gone := false, false // gone: the system said no until the viewers leave
	dead, closed := false, share.Done()
	stop := func() {
		if codec != 0 {
			share.Stop()
			codec, kbps = 0, 0
		}
	}
	end := func(r lordshare.Reason) {
		if r == lordshare.EndedByApp {
			return // ours
		}
		codec, kbps = 0, 0
		gone = true
		e.Log.Warn("the share is over", "reason", r.String())
		if e.Stream.EncodedWant().Viewers == 0 {
			return
		}
		e.Stream.Notice(shareNotice(r))
		if e.End != nil {
			e.End(r.String())
		}
	}
	for {
		select {
		case <-ctx.Done():
			stop()
			return
		case r := <-over:
			end(r)
		case <-tick:
			if !dead {
				select {
				case <-closed:
					dead = true
					codec, kbps = 0, 0
				default:
				}
			}
			want := e.Stream.EncodedWant()
			if want.Viewers == 0 {
				stop()
				gone, hear = false, false
				continue
			}
			if dead && !gone {
				end(lordshare.NoIndicator)
				continue
			}
			lc := lordCodec(want.Codec)
			switch {
			case gone || lc == 0:
			case lc != codec:
				stop()
				share.Hear(false)
				hear = false
				share.Start(lordshare.FlagAll, lc, encodedHz, want.Kbps)
				codec, kbps, lastKey = lc, want.Kbps, time.Time{}
			case want.Kbps != kbps:
				share.Rate(want.Kbps)
				kbps = want.Kbps
			}
			if codec == 0 {
				continue
			}
			if h := e.Stream.Hearing(); h != hear {
				share.Hear(h)
				hear = h
			}
			if want.NeedKey && time.Since(lastKey) >= keyEvery {
				share.Keyframe()
				lastKey = time.Now()
			}
		}
	}
}

// shareNotice is what a viewer is told when the system takes the share away.
func shareNotice(r lordshare.Reason) string {
	switch r {
	case lordshare.Denied, lordshare.Revoked, lordshare.NoPerson, lordshare.Restricted:
		return NoticeNoScreenPermission
	case lordshare.Stopped, lordshare.NoIndicator:
		return NoticeShareDenied
	}
	return NoticeCaptureFailed
}

func (e *EncodedSource) video(v lordshare.Video) {
	codec := CodecVP8
	if v.Codec == lordshare.CodecH264 {
		codec = CodecH264
	}
	e.mu.Lock()
	w, h := e.w, e.h
	e.mu.Unlock()
	// The track keeps the frame queued past this call.
	e.Stream.EncodedFrame(codec, v.Key, time.Duration(v.Ms)*time.Millisecond, slices.Clone(v.Data), w, h)
}

func (e *EncodedSource) audio(first uint32, pkt []byte) {
	e.mu.Lock()
	d := audioDefault
	if delta := first - e.last; e.last != 0 && delta > 0 && delta < 48000 {
		d = time.Duration(delta) * time.Second / 48000
	}
	e.last = first
	e.mu.Unlock()
	e.Stream.EncodedAudio(slices.Clone(pkt), d)
}

func (e *EncodedSource) size(w, h int) {
	e.mu.Lock()
	e.w, e.h = w, h
	e.mu.Unlock()
	e.Stream.EncodedResized()
}
