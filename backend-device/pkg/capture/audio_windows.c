// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build windows && !headless

#define COBJMACROS
#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <mmdeviceapi.h>
#include <audioclient.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#include "audio.h"

// The computer's sound on Windows: a WASAPI loopback stream of the default
// output, what its speakers play, converted by Windows to the format asked
// (48 kHz stereo, 16 bits). A loopback stream says nothing while nothing
// plays: the reader fills those gaps with silence, so the sound keeps its
// pace. While something plays, a packet comes every device period
// (10 ms): a late one is waited for, never padded with silence, which
// would crackle and add time the sound does not have. Times come from
// the performance counter and naps from a high-resolution timer, as
// GetTickCount and Sleep move by 15.6 ms steps.

#ifndef AUDCLNT_STREAMFLAGS_LOOPBACK
#define AUDCLNT_STREAMFLAGS_LOOPBACK 0x00020000
#endif
#ifndef AUDCLNT_STREAMFLAGS_AUTOCONVERTPCM
#define AUDCLNT_STREAMFLAGS_AUTOCONVERTPCM 0x80000000
#endif
#ifndef AUDCLNT_STREAMFLAGS_SRC_DEFAULT_QUALITY
#define AUDCLNT_STREAMFLAGS_SRC_DEFAULT_QUALITY 0x08000000
#endif

static const GUID dk_CLSID_MMDeviceEnumerator = {0xBCDE0395, 0xE52F, 0x467C, {0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E}};
static const GUID dk_IID_IMMDeviceEnumerator = {0xA95664D2, 0x9614, 0x4F35, {0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6}};
static const GUID dk_IID_IAudioClient = {0x1CB9AD4C, 0xDBFA, 0x4C32, {0xB1, 0x78, 0xC2, 0xF5, 0x68, 0xA7, 0x03, 0xB2}};
static const GUID dk_IID_IAudioCaptureClient = {0xC8ADBD64, 0xE71E, 0x48A0, {0xA4, 0xDE, 0x18, 0x5C, 0x39, 0x5C, 0xD3, 0x17}};

typedef struct {
	IAudioClient *client;
	IAudioCaptureClient *cap;
	int frame;        // bytes of one sample frame (all channels)
	int period;       // bytes of a read: 10 ms
	uint8_t *ring;    // what came and was not read yet
	size_t size, have;
	double due;       // when the next silent read is due (ms)
	double heard;     // when the stream last gave a packet (ms)
	double quiet;     // ms without one that mean nothing plays
	HANDLE timer;     // a high-resolution timer for naps (NULL: Sleep)
} dk_wasapi;

#ifndef CREATE_WAITABLE_TIMER_HIGH_RESOLUTION
#define CREATE_WAITABLE_TIMER_HIGH_RESOLUTION 0x00000002
#endif

static double now_ms(void) {
	static LARGE_INTEGER freq;
	if (!freq.QuadPart) QueryPerformanceFrequency(&freq);
	LARGE_INTEGER c;
	QueryPerformanceCounter(&c);
	return (double)c.QuadPart * 1000.0 / (double)freq.QuadPart;
}

// nap waits about 2 ms (the high-resolution timer is Windows 10 1803's;
// before it, Sleep).
static void nap(dk_wasapi *a) {
	if (a->timer) {
		LARGE_INTEGER due;
		due.QuadPart = -20000; // 2 ms, in 100 ns, relative
		if (SetWaitableTimer(a->timer, &due, 0, NULL, NULL, FALSE)) {
			WaitForSingleObject(a->timer, INFINITE);
			return;
		}
	}
	Sleep(1);
}

void *dk_audio_open(int rate, int channels, int frame_bytes) {
	// Any COM apartment this thread already has does; WASAPI's objects
	// work from every thread of the process's multithreaded one.
	HRESULT co = CoInitializeEx(NULL, COINIT_MULTITHREADED);
	(void)co;
	IMMDeviceEnumerator *en = NULL;
	IMMDevice *dev = NULL;
	IAudioClient *client = NULL;
	IAudioCaptureClient *cap = NULL;
	if (FAILED(CoCreateInstance(&dk_CLSID_MMDeviceEnumerator, NULL, CLSCTX_ALL, &dk_IID_IMMDeviceEnumerator, (void **)&en))) return NULL;
	HRESULT hr = IMMDeviceEnumerator_GetDefaultAudioEndpoint(en, eRender, eConsole, &dev);
	IMMDeviceEnumerator_Release(en);
	if (FAILED(hr)) return NULL;
	hr = IMMDevice_Activate(dev, &dk_IID_IAudioClient, CLSCTX_ALL, NULL, (void **)&client);
	IMMDevice_Release(dev);
	if (FAILED(hr)) return NULL;
	WAVEFORMATEX wf;
	memset(&wf, 0, sizeof wf);
	wf.wFormatTag = WAVE_FORMAT_PCM;
	wf.nChannels = (WORD)channels;
	wf.nSamplesPerSec = (DWORD)rate;
	wf.wBitsPerSample = 16;
	wf.nBlockAlign = (WORD)(channels * 2);
	wf.nAvgBytesPerSec = (DWORD)(rate * channels * 2);
	DWORD flags = AUDCLNT_STREAMFLAGS_LOOPBACK | AUDCLNT_STREAMFLAGS_AUTOCONVERTPCM | AUDCLNT_STREAMFLAGS_SRC_DEFAULT_QUALITY;
	hr = IAudioClient_Initialize(client, AUDCLNT_SHAREMODE_SHARED, flags, 2000000 /* 200 ms */, 0, &wf, NULL);
	if (SUCCEEDED(hr)) hr = IAudioClient_GetService(client, &dk_IID_IAudioCaptureClient, (void **)&cap);
	if (SUCCEEDED(hr)) hr = IAudioClient_Start(client);
	if (FAILED(hr)) {
		if (cap) IAudioCaptureClient_Release(cap);
		IAudioClient_Release(client);
		return NULL;
	}
	dk_wasapi *a = calloc(1, sizeof *a);
	if (!a) {
		IAudioClient_Stop(client);
		IAudioCaptureClient_Release(cap);
		IAudioClient_Release(client);
		return NULL;
	}
	a->client = client;
	a->cap = cap;
	a->frame = channels * 2;
	a->period = frame_bytes;
	a->size = (size_t)rate * a->frame; // a second
	a->ring = malloc(a->size);
	// Three device periods without a packet: nothing plays.
	REFERENCE_TIME def = 0;
	IAudioClient_GetDevicePeriod(client, &def, NULL);
	a->quiet = 3.0 * (double)def / 10000.0;
	if (a->quiet < 30) a->quiet = 30;
	a->timer = CreateWaitableTimerExW(NULL, NULL, CREATE_WAITABLE_TIMER_HIGH_RESOLUTION, TIMER_ALL_ACCESS);
	a->heard = now_ms();
	a->due = a->heard + 10;
	if (!a->ring) {
		dk_audio_close(a);
		return NULL;
	}
	return a;
}

// Takes what the stream has into the ring (the oldest goes when it is
// full): how many packets came, or -1 when the stream is gone (the output
// device changed).
static int drain(dk_wasapi *a) {
	for (int got = 0;; got++) {
		UINT32 next = 0;
		HRESULT hr = IAudioCaptureClient_GetNextPacketSize(a->cap, &next);
		if (FAILED(hr)) return -1;
		if (next == 0) return got;
		BYTE *data = NULL;
		UINT32 frames = 0;
		DWORD flags = 0;
		hr = IAudioCaptureClient_GetBuffer(a->cap, &data, &frames, &flags, NULL, NULL);
		if (FAILED(hr)) return -1;
		size_t n = (size_t)frames * a->frame;
		if (n > a->size) n = a->size;
		if (a->have + n > a->size) {
			size_t drop = a->have + n - a->size;
			memmove(a->ring, a->ring + drop, a->have - drop);
			a->have -= drop;
		}
		if (flags & AUDCLNT_BUFFERFLAGS_SILENT)
			memset(a->ring + a->have, 0, n);
		else
			memcpy(a->ring + a->have, data, n);
		a->have += n;
		IAudioCaptureClient_ReleaseBuffer(a->cap, frames);
	}
}

int dk_audio_read(void *h, void *buf, int bytes) {
	dk_wasapi *a = h;
	for (;;) {
		int got = drain(a);
		if (got < 0) return -1;
		double now = now_ms();
		if (got > 0) a->heard = now;
		if (a->have >= (size_t)bytes) {
			memcpy(buf, a->ring, bytes);
			memmove(a->ring, a->ring + bytes, a->have - bytes);
			a->have -= bytes;
			a->due = now + 10;
			return 0;
		}
		// Nothing playing: what came, then silence, a read every 10 ms.
		if (now - a->heard >= a->quiet && now >= a->due) {
			memcpy(buf, a->ring, a->have);
			memset((uint8_t *)buf + a->have, 0, bytes - a->have);
			a->have = 0;
			a->due = now >= a->due + 50 ? now + 10 : a->due + 10;
			return 0;
		}
		nap(a);
	}
}

void dk_audio_close(void *h) {
	dk_wasapi *a = h;
	if (!a) return;
	if (a->client) IAudioClient_Stop(a->client);
	if (a->cap) IAudioCaptureClient_Release(a->cap);
	if (a->client) IAudioClient_Release(a->client);
	if (a->timer) CloseHandle(a->timer);
	free(a->ring);
	free(a);
}
