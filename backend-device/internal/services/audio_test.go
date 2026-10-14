// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package services

import (
	"testing"

	"github.com/pion/webrtc/v4"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/encoder"
)

func TestAudioIsCutInto20msFramesOnlyForListeners(t *testing.T) {
	if !encoder.Codecs {
		t.Skip("built without cgo: no libvpx or libopus")
	}
	track, err := newAudioTrack()
	if err != nil {
		t.Fatal(err)
	}
	var p audioPipe
	none := func() []*webrtc.TrackLocalStaticSample { return nil }
	some := func() []*webrtc.TrackLocalStaticSample { return []*webrtc.TrackLocalStaticSample{track} }
	p.write(make([]int16, audioFrame/2), none)
	if p.enc != nil || len(p.buf) != 0 {
		t.Fatal("sound kept with nobody to hear it")
	}
	p.write(make([]int16, audioFrame+100), some)
	if p.enc == nil || len(p.buf) != 100 {
		t.Fatalf("left %d samples", len(p.buf))
	}
	p.write(make([]int16, audioFrame-100), some)
	if len(p.buf) != 0 {
		t.Fatalf("left %d samples after two whole frames", len(p.buf))
	}
	p.write(nil, none)
	if p.enc != nil {
		t.Fatal("the encoder stayed open")
	}
}

func TestStereoDoublesMono(t *testing.T) {
	got := stereo([]int16{1, -2})
	if len(got) != 4 || got[0] != 1 || got[1] != 1 || got[2] != -2 || got[3] != -2 {
		t.Fatalf("%v", got)
	}
}

func TestAudioGrantedBeforeTheStreamStarts(t *testing.T) {
	s, err := NewStreamService(StreamConfig{IncludeLoopback: true}, NewICEStore())
	if err != nil {
		t.Fatal(err)
	}
	s.SetSender(newFakeSender())
	// The session grants the sound first, then starts the viewer's stream.
	s.SetAudio("P", true)
	if err := s.AddViewer("P", nil); err != nil {
		t.Fatal(err)
	}
	defer s.CloseAll()
	if !s.Hearing() {
		t.Fatal("the grant made before the stream was lost")
	}
	s.SetAudio("P", false)
	if s.Hearing() {
		t.Fatal("still hearing after the grant was taken back")
	}
}
