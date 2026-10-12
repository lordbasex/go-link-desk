// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build (linux || windows) && !headless

package capture

/*
#include <stdlib.h>
#include "audio.h"
*/
import "C"

import (
	"runtime"
	"unsafe"
)

// audioFrame is how much sound each read takes: 10 ms.
const audioFrame = AudioRate / 100 * AudioChannels

// audioTap reads the computer's sound on a thread of its own and hands it
// over 10 ms at a time, until stopped.
type audioTap struct {
	stop chan struct{}
	done chan struct{}
}

// startAudio starts reading the sound; nil when the system has none to
// read.
func startAudio(on func([]int16)) *audioTap {
	h := C.dk_audio_open(C.int(AudioRate), C.int(AudioChannels), C.int(audioFrame*2))
	if h == nil {
		return nil
	}
	a := &audioTap{stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(a.done)
		defer C.dk_audio_close(h)
		buf := make([]int16, audioFrame)
		for {
			select {
			case <-a.stop:
				return
			default:
			}
			if C.dk_audio_read(h, unsafe.Pointer(&buf[0]), C.int(len(buf)*2)) != 0 {
				return
			}
			on(append([]int16(nil), buf...))
		}
	}()
	return a
}

// Stop ends the reading (within a read, 10 ms) and waits for it.
func (a *audioTap) Stop() {
	close(a.stop)
	<-a.done
}
