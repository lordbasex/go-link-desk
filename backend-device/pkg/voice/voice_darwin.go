// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build darwin

package voice

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -fblocks -mmacosx-version-min=13.0
#cgo LDFLAGS: -framework AVFoundation -framework CoreAudio
#include <stdint.h>
#include <stdlib.h>

void *voice_start(int mic, uintptr_t handle, char **err);
void voice_play(void *ref, const int16_t *samples, int n);
void voice_mute(void *ref, int muted);
void voice_stop(void *ref);
int voice_mic_access(void);
int voice_mic_request(void);
*/
import "C"

import (
	"errors"
	"runtime/cgo"
	"sync"
	"unsafe"
)

// Supported reports whether this build has call sound.
const Supported = true

// Engine is the running call sound.
type Engine struct {
	mu      sync.Mutex
	ref     unsafe.Pointer
	handle  cgo.Handle
	stopped bool
}

// Start opens the speaker and, with mic, the microphone; onMic receives
// its sound (16-bit mono at Rate, any length) on the audio thread.
func Start(mic bool, onMic func([]int16)) (*Engine, error) {
	if onMic == nil {
		onMic = func([]int16) {}
	}
	h := cgo.NewHandle(onMic)
	var cerr *C.char
	m := C.int(0)
	if mic {
		m = 1
	}
	ref := C.voice_start(m, C.uintptr_t(h), &cerr)
	if ref == nil {
		h.Delete()
		msg := "unknown error"
		if cerr != nil {
			msg = C.GoString(cerr)
			C.free(unsafe.Pointer(cerr))
		}
		return nil, errors.New("voice: " + msg)
	}
	return &Engine{ref: ref, handle: h}, nil
}

// Play queues the other side's voice (16-bit mono at Rate).
func (e *Engine) Play(samples []int16) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stopped || len(samples) == 0 {
		return
	}
	C.voice_play(e.ref, (*C.int16_t)(unsafe.Pointer(&samples[0])), C.int(len(samples)))
}

// Mute stops sending the microphone (the engine keeps running, so the
// echo cancellation keeps its model of the room).
func (e *Engine) Mute(muted bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.stopped {
		m := C.int(0)
		if muted {
			m = 1
		}
		C.voice_mute(e.ref, m)
	}
}

// Stop closes the microphone and the speaker.
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stopped {
		return
	}
	e.stopped = true
	C.voice_stop(e.ref)
	e.handle.Delete()
}

// HasMicAccess reports whether the app may use the microphone (System
// Settings › Privacy & Security › Microphone).
func HasMicAccess() bool { return C.voice_mic_access() == 1 }

// RequestMicAccess asks the system; the first time it shows its dialog.
func RequestMicAccess() bool { return C.voice_mic_request() == 1 }

//export goVoiceMic
func goVoiceMic(handle C.uintptr_t, samples *C.int16_t, n C.int) {
	fn := cgo.Handle(handle).Value().(func([]int16))
	if n > 0 {
		fn(unsafe.Slice((*int16)(unsafe.Pointer(samples)), int(n)))
	}
}
