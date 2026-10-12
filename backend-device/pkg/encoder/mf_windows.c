// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

#define COBJMACROS
#define WIN32_LEAN_AND_MEAN
#include <initguid.h>
#include <windows.h>
#include <mfapi.h>
#include <mferror.h>
#include <mftransform.h>
#include <codecapi.h>
#include <icodecapi.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#include "mf_windows.h"

// codecapi.h names its keys with C++'s __uuidof: in C they are made from
// its STATIC_ values.
static const GUID key_rate_mode = {STATIC_CODECAPI_AVEncCommonRateControlMode};
static const GUID key_bitrate = {STATIC_CODECAPI_AVEncCommonMeanBitRate};
static const GUID key_low_latency = {STATIC_CODECAPI_AVLowLatencyMode};
static const GUID key_b_frames = {STATIC_CODECAPI_AVEncMPVDefaultBPictureCount};
static const GUID key_gop = {STATIC_CODECAPI_AVEncMPVGOPSize};
static const GUID key_force_key = {STATIC_CODECAPI_AVEncVideoForceKeyFrame};

// H.264 from Media Foundation: Windows' own encoder transform (an MFT),
// called synchronously: I420 in, Annex B access units out, low latency, a
// constant bitrate, no B-frames. The GPU's encoders (NVIDIA, Intel, AMD)
// are asynchronous transforms: this takes the synchronous ones, which
// every Windows 10 and 11 has in software (not the N editions without the
// Media Feature Pack, where it fails and the stream uses VP8). Media
// Foundation is loaded when an encoder is first opened, not linked: the
// program starts on a Windows without it.

typedef HRESULT(WINAPI *startup_fn)(ULONG, DWORD);
typedef HRESULT(WINAPI *shutdown_fn)(void);
typedef HRESULT(WINAPI *enum_fn)(GUID, UINT32, const MFT_REGISTER_TYPE_INFO *, const MFT_REGISTER_TYPE_INFO *, IMFActivate ***, UINT32 *);
typedef HRESULT(WINAPI *media_type_fn)(IMFMediaType **);
typedef HRESULT(WINAPI *sample_fn)(IMFSample **);
typedef HRESULT(WINAPI *buffer_fn)(DWORD, IMFMediaBuffer **);

static struct {
	INIT_ONCE once;
	int ok;
	startup_fn Startup;
	shutdown_fn Shutdown;
	enum_fn TEnumEx;
	media_type_fn CreateMediaType;
	sample_fn CreateSample;
	buffer_fn CreateMemoryBuffer;
} mf = {INIT_ONCE_STATIC_INIT};

static BOOL CALLBACK load_mf(PINIT_ONCE once, PVOID p, PVOID *ctx) {
	(void)once;
	(void)p;
	(void)ctx;
	HMODULE m = LoadLibraryExW(L"mfplat.dll", NULL, LOAD_LIBRARY_SEARCH_SYSTEM32);
	if (!m) return TRUE;
	mf.Startup = (startup_fn)(void *)GetProcAddress(m, "MFStartup");
	mf.Shutdown = (shutdown_fn)(void *)GetProcAddress(m, "MFShutdown");
	mf.TEnumEx = (enum_fn)(void *)GetProcAddress(m, "MFTEnumEx");
	mf.CreateMediaType = (media_type_fn)(void *)GetProcAddress(m, "MFCreateMediaType");
	mf.CreateSample = (sample_fn)(void *)GetProcAddress(m, "MFCreateSample");
	mf.CreateMemoryBuffer = (buffer_fn)(void *)GetProcAddress(m, "MFCreateMemoryBuffer");
	mf.ok = mf.Startup && mf.Shutdown && mf.TEnumEx && mf.CreateMediaType && mf.CreateSample && mf.CreateMemoryBuffer;
	return TRUE;
}

struct mf_enc {
	IMFTransform *t;
	ICodecAPI *api;
	int w, h, fps;
	int64_t duration;   // a frame's, in 100 ns
	IMFSample *out;     // the output sample, when the transform wants ours
	IMFMediaBuffer *buf;
	DWORD out_size;
	int provides;       // the transform makes its own output samples
	IMFSample *given;   // one it made, held until the next output
	int hardware;
	uint8_t *copy;      // the last output, copied out of the sample
	DWORD copy_size;
};

static void com_in(void) {
	// Media Foundation's objects work from every thread of the process's
	// multithreaded apartment; each call makes sure its thread is in one.
	CoInitializeEx(NULL, COINIT_MULTITHREADED);
}

static HRESULT set_u32(ICodecAPI *api, const GUID *key, UINT32 v) {
	VARIANT var;
	VariantInit(&var);
	var.vt = VT_UI4;
	var.ulVal = v;
	return ICodecAPI_SetValue(api, key, &var);
}

static HRESULT set_bool(ICodecAPI *api, const GUID *key, int v) {
	VARIANT var;
	VariantInit(&var);
	var.vt = VT_BOOL;
	var.boolVal = v ? VARIANT_TRUE : VARIANT_FALSE;
	return ICodecAPI_SetValue(api, key, &var);
}

// The first synchronous H.264 encoder that takes I420, which Windows
// names IYUV (hardware ones first, though those are asynchronous and so
// not listed).
static IMFTransform *find_encoder(int *hardware) {
	MFT_REGISTER_TYPE_INFO in = {MFMediaType_Video, MFVideoFormat_IYUV};
	MFT_REGISTER_TYPE_INFO out = {MFMediaType_Video, MFVideoFormat_H264};
	IMFActivate **acts = NULL;
	UINT32 n = 0;
	if (FAILED(mf.TEnumEx(MFT_CATEGORY_VIDEO_ENCODER, MFT_ENUM_FLAG_SYNCMFT | MFT_ENUM_FLAG_LOCALMFT | MFT_ENUM_FLAG_SORTANDFILTER, &in, &out, &acts, &n)))
		return NULL;
	IMFTransform *t = NULL;
	for (UINT32 i = 0; i < n; i++) {
		if (!t && SUCCEEDED(IMFActivate_ActivateObject(acts[i], &IID_IMFTransform, (void **)&t))) {
			UINT32 flags = 0;
			IMFActivate_GetUINT32(acts[i], &MF_TRANSFORM_FLAGS_Attribute, &flags);
			*hardware = (flags & MFT_ENUM_FLAG_HARDWARE) != 0;
		}
		IMFActivate_Release(acts[i]);
	}
	CoTaskMemFree(acts);
	return t;
}

static IMFMediaType *video_type(const GUID *subtype, int w, int h, int fps) {
	IMFMediaType *m = NULL;
	if (FAILED(mf.CreateMediaType(&m))) return NULL;
	IMFMediaType_SetGUID(m, &MF_MT_MAJOR_TYPE, &MFMediaType_Video);
	IMFMediaType_SetGUID(m, &MF_MT_SUBTYPE, subtype);
	IMFMediaType_SetUINT64(m, &MF_MT_FRAME_SIZE, ((UINT64)w << 32) | (UINT32)h);
	IMFMediaType_SetUINT64(m, &MF_MT_FRAME_RATE, ((UINT64)fps << 32) | 1);
	IMFMediaType_SetUINT64(m, &MF_MT_PIXEL_ASPECT_RATIO, ((UINT64)1 << 32) | 1);
	IMFMediaType_SetUINT32(m, &MF_MT_INTERLACE_MODE, MFVideoInterlace_Progressive);
	return m;
}

// Makes the output sample the transform asks for (when it does not make
// its own).
static int ready_output(mf_enc *e) {
	MFT_OUTPUT_STREAM_INFO info;
	memset(&info, 0, sizeof info);
	if (FAILED(IMFTransform_GetOutputStreamInfo(e->t, 0, &info))) return 0;
	e->provides = (info.dwFlags & (MFT_OUTPUT_STREAM_PROVIDES_SAMPLES | MFT_OUTPUT_STREAM_CAN_PROVIDE_SAMPLES)) != 0;
	if (e->provides) return 1;
	DWORD size = info.cbSize ? info.cbSize : (DWORD)(e->w * e->h * 3 / 2);
	if (e->out && size <= e->out_size) return 1;
	if (e->buf) IMFMediaBuffer_Release(e->buf);
	if (e->out) IMFSample_Release(e->out);
	e->buf = NULL;
	e->out = NULL;
	if (FAILED(mf.CreateSample(&e->out))) return 0;
	if (FAILED(mf.CreateMemoryBuffer(size, &e->buf))) return 0;
	IMFSample_AddBuffer(e->out, e->buf);
	e->out_size = size;
	return 1;
}

mf_enc *mf_open(int w, int h, int fps, int kbps, int gop, int high, int *err) {
	com_in();
	*err = 0;
	InitOnceExecuteOnce(&mf.once, load_mf, NULL, NULL);
	if (!mf.ok || FAILED(mf.Startup(MF_VERSION, MFSTARTUP_LITE))) {
		*err = 1;
		return NULL;
	}
	mf_enc *e = calloc(1, sizeof *e);
	if (!e) {
		*err = 2;
		return NULL;
	}
	e->w = w;
	e->h = h;
	e->fps = fps > 0 ? fps : 30;
	e->duration = 10000000LL / e->fps;
	e->t = find_encoder(&e->hardware);
	if (!e->t) {
		*err = 3;
		goto fail;
	}
	IMFTransform_QueryInterface(e->t, &IID_ICodecAPI, (void **)&e->api);
	if (e->api) {
		// Before the types: some settings are read when they are set.
		set_u32(e->api, &key_rate_mode, eAVEncCommonRateControlMode_CBR);
		set_u32(e->api, &key_bitrate, (UINT32)kbps * 1000);
		set_bool(e->api, &key_low_latency, 1);
		set_u32(e->api, &key_b_frames, 0);
		set_u32(e->api, &key_gop, (UINT32)gop);
	}
	// An encoder takes its output type first.
	IMFMediaType *out = video_type(&MFVideoFormat_H264, w, h, e->fps);
	if (!out) {
		*err = 4;
		goto fail;
	}
	IMFMediaType_SetUINT32(out, &MF_MT_AVG_BITRATE, (UINT32)kbps * 1000);
	IMFMediaType_SetUINT32(out, &MF_MT_MPEG2_PROFILE, high ? eAVEncH264VProfile_High : eAVEncH264VProfile_Base);
	HRESULT hr = IMFTransform_SetOutputType(e->t, 0, out, 0);
	IMFMediaType_Release(out);
	if (FAILED(hr)) {
		*err = 5;
		goto fail;
	}
	IMFMediaType *in = video_type(&MFVideoFormat_IYUV, w, h, e->fps);
	if (!in) {
		*err = 4;
		goto fail;
	}
	IMFMediaType_SetUINT32(in, &MF_MT_DEFAULT_STRIDE, (UINT32)w);
	hr = IMFTransform_SetInputType(e->t, 0, in, 0);
	IMFMediaType_Release(in);
	if (FAILED(hr)) {
		*err = 6;
		goto fail;
	}
	if (!ready_output(e)) {
		*err = 7;
		goto fail;
	}
	IMFTransform_ProcessMessage(e->t, MFT_MESSAGE_NOTIFY_BEGIN_STREAMING, 0);
	IMFTransform_ProcessMessage(e->t, MFT_MESSAGE_NOTIFY_START_OF_STREAM, 0);
	return e;
fail:
	mf_close(e);
	return NULL;
}

int mf_hardware(mf_enc *e) { return e->hardware; }

int mf_encode(mf_enc *e, const uint8_t *i420, int64_t pts, int key) {
	com_in();
	DWORD n = (DWORD)(e->w * e->h + 2 * (e->w / 2) * (e->h / 2));
	IMFSample *s = NULL;
	IMFMediaBuffer *b = NULL;
	if (FAILED(mf.CreateSample(&s))) return -1;
	if (FAILED(mf.CreateMemoryBuffer(n, &b))) {
		IMFSample_Release(s);
		return -1;
	}
	BYTE *p = NULL;
	if (FAILED(IMFMediaBuffer_Lock(b, &p, NULL, NULL))) {
		IMFMediaBuffer_Release(b);
		IMFSample_Release(s);
		return -1;
	}
	memcpy(p, i420, n);
	IMFMediaBuffer_Unlock(b);
	IMFMediaBuffer_SetCurrentLength(b, n);
	IMFSample_AddBuffer(s, b);
	IMFMediaBuffer_Release(b);
	IMFSample_SetSampleTime(s, pts);
	IMFSample_SetSampleDuration(s, e->duration);
	if (key && e->api) set_u32(e->api, &key_force_key, 1);
	HRESULT hr = IMFTransform_ProcessInput(e->t, 0, s, 0);
	IMFSample_Release(s);
	if (hr == MF_E_NOTACCEPTING) return 1; // it has output to give first
	return FAILED(hr) ? -2 : 0;
}

int mf_output(mf_enc *e, uint8_t **data, int *len, int64_t *pts) {
	com_in();
	if (e->given) {
		IMFSample_Release(e->given);
		e->given = NULL;
	}
	for (int tries = 0; tries < 2; tries++) {
		MFT_OUTPUT_DATA_BUFFER ob;
		memset(&ob, 0, sizeof ob);
		ob.pSample = e->provides ? NULL : e->out;
		if (!e->provides) IMFMediaBuffer_SetCurrentLength(e->buf, 0);
		DWORD status = 0;
		HRESULT hr = IMFTransform_ProcessOutput(e->t, 0, 1, &ob, &status);
		if (ob.pEvents) IMFCollection_Release(ob.pEvents);
		if (hr == MF_E_TRANSFORM_NEED_MORE_INPUT) return 0;
		if (hr == MF_E_TRANSFORM_STREAM_CHANGE) {
			// The encoder settled its output type: take it as it is.
			IMFMediaType *t = NULL;
			if (SUCCEEDED(IMFTransform_GetOutputAvailableType(e->t, 0, 0, &t))) {
				IMFTransform_SetOutputType(e->t, 0, t, 0);
				IMFMediaType_Release(t);
			}
			if (!ready_output(e)) return -1;
			continue;
		}
		if (FAILED(hr)) return -1;
		IMFSample *s = ob.pSample;
		if (e->provides) e->given = s;
		IMFMediaBuffer *cb = NULL;
		if (!s || FAILED(IMFSample_ConvertToContiguousBuffer(s, &cb))) return -1;
		BYTE *p = NULL;
		DWORD n = 0;
		if (FAILED(IMFMediaBuffer_Lock(cb, &p, NULL, &n))) {
			IMFMediaBuffer_Release(cb);
			return -1;
		}
		// Copied out: the sample is used again for the next output.
		if (n > e->copy_size) {
			free(e->copy);
			e->copy = malloc(n);
			e->copy_size = e->copy ? n : 0;
		}
		if (e->copy) memcpy(e->copy, p, n);
		IMFMediaBuffer_Unlock(cb);
		IMFMediaBuffer_Release(cb);
		if (!e->copy) return -1;
		LONGLONG t = 0;
		IMFSample_GetSampleTime(s, &t);
		*data = e->copy;
		*len = (int)n;
		*pts = t;
		return 1;
	}
	return -1;
}

void mf_set_rate(mf_enc *e, int kbps) {
	com_in();
	if (e->api) set_u32(e->api, &key_bitrate, (UINT32)kbps * 1000);
}

void mf_close(mf_enc *e) {
	if (!e) return;
	com_in();
	if (e->t) {
		IMFTransform_ProcessMessage(e->t, MFT_MESSAGE_NOTIFY_END_OF_STREAM, 0);
		IMFTransform_ProcessMessage(e->t, MFT_MESSAGE_NOTIFY_END_STREAMING, 0);
	}
	if (e->given) IMFSample_Release(e->given);
	if (e->buf) IMFMediaBuffer_Release(e->buf);
	if (e->out) IMFSample_Release(e->out);
	if (e->api) ICodecAPI_Release(e->api);
	if (e->t) IMFTransform_Release(e->t);
	free(e->copy);
	free(e);
	mf.Shutdown();
}
