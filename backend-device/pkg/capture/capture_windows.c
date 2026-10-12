// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build windows && !headless

#define COBJMACROS
#define WIN32_LEAN_AND_MEAN
#include <initguid.h>
#include <windows.h>
#include <d3d11.h>
#include <dxgi1_2.h>
#include <limits.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "capture_windows.h"

// The screen of a Windows desktop: the Desktop Duplication API copies the
// monitor's picture on its GPU when the desktop changes, and the copy is
// read back once a frame. Where it cannot run (a remote desktop session, a
// rotated monitor, a display driver without it) GDI reads the screen
// instead, and the GPU is tried again every few seconds. Neither can read
// the secure desktop (the lock screen, a UAC prompt): meanwhile the last
// picture stays.

void dk_win_dpi(void) {
	typedef BOOL(WINAPI * set_fn)(HANDLE);
	HMODULE u = GetModuleHandleW(L"user32.dll");
	set_fn set = u ? (set_fn)(void *)GetProcAddress(u, "SetProcessDpiAwarenessContext") : NULL;
	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2; it fails, harmlessly, when
	// the manifest set it already.
	if (set) set((HANDLE)(LONG_PTR)-4);
}

static void utf8(const WCHAR *w, char *out, int size) {
	if (WideCharToMultiByte(CP_UTF8, 0, w, -1, out, size, NULL, NULL) == 0) out[0] = 0;
}

// The monitor's own name ("DELL U2720Q", from its EDID) for its GDI
// device name (\\.\DISPLAY1); 0 when Windows has none.
static int monitor_name(const WCHAR *gdi, char *out, int size) {
	UINT32 np = 0, nm = 0;
	if (GetDisplayConfigBufferSizes(QDC_ONLY_ACTIVE_PATHS, &np, &nm) != ERROR_SUCCESS || np == 0) return 0;
	DISPLAYCONFIG_PATH_INFO *paths = calloc(np, sizeof *paths);
	DISPLAYCONFIG_MODE_INFO *modes = calloc(nm ? nm : 1, sizeof *modes);
	int ok = 0;
	if (paths && modes && QueryDisplayConfig(QDC_ONLY_ACTIVE_PATHS, &np, paths, &nm, modes, NULL) == ERROR_SUCCESS) {
		for (UINT32 i = 0; i < np && !ok; i++) {
			DISPLAYCONFIG_SOURCE_DEVICE_NAME src;
			memset(&src, 0, sizeof src);
			src.header.type = DISPLAYCONFIG_DEVICE_INFO_GET_SOURCE_NAME;
			src.header.size = sizeof src;
			src.header.adapterId = paths[i].sourceInfo.adapterId;
			src.header.id = paths[i].sourceInfo.id;
			if (DisplayConfigGetDeviceInfo(&src.header) != ERROR_SUCCESS || wcscmp(src.viewGdiDeviceName, gdi) != 0) continue;
			DISPLAYCONFIG_TARGET_DEVICE_NAME tgt;
			memset(&tgt, 0, sizeof tgt);
			tgt.header.type = DISPLAYCONFIG_DEVICE_INFO_GET_TARGET_NAME;
			tgt.header.size = sizeof tgt;
			tgt.header.adapterId = paths[i].targetInfo.adapterId;
			tgt.header.id = paths[i].targetInfo.id;
			if (DisplayConfigGetDeviceInfo(&tgt.header) == ERROR_SUCCESS && tgt.monitorFriendlyDeviceName[0]) {
				utf8(tgt.monitorFriendlyDeviceName, out, size);
				ok = out[0] != 0;
			}
		}
	}
	free(paths);
	free(modes);
	return ok;
}

typedef struct {
	dk_monitor *out;
	int max, n, names;
} monitors;

static BOOL CALLBACK on_monitor(HMONITOR m, HDC dc, LPRECT r, LPARAM p) {
	(void)dc;
	(void)r;
	monitors *ms = (monitors *)p;
	if (ms->n >= ms->max) return FALSE;
	MONITORINFOEXW mi;
	memset(&mi, 0, sizeof mi);
	mi.cbSize = sizeof mi;
	if (!GetMonitorInfoW(m, (MONITORINFO *)&mi)) return TRUE;
	dk_monitor *o = &ms->out[ms->n++];
	memset(o, 0, sizeof *o);
	o->x = mi.rcMonitor.left;
	o->y = mi.rcMonitor.top;
	o->w = mi.rcMonitor.right - mi.rcMonitor.left;
	o->h = mi.rcMonitor.bottom - mi.rcMonitor.top;
	o->primary = (mi.dwFlags & MONITORINFOF_PRIMARY) != 0;
	if (ms->names && !monitor_name(mi.szDevice, o->name, sizeof o->name)) snprintf(o->name, sizeof o->name, "Display %d", ms->n);
	return TRUE;
}

int dk_monitors(dk_monitor *out, int max, int names) {
	monitors ms = {out, max, 0, names};
	EnumDisplayMonitors(NULL, NULL, on_monitor, (LPARAM)&ms);
	return ms.n;
}

// A picture GDI can draw on: a top-down 32-bit DIB section.
typedef struct {
	HDC dc;
	HBITMAP bmp, old;
	uint8_t *bits;
	int w, h;
} dib;

static void dib_free(dib *d) {
	if (!d->dc) return;
	SelectObject(d->dc, d->old);
	DeleteObject(d->bmp);
	DeleteDC(d->dc);
	memset(d, 0, sizeof *d);
}

static int dib_size(dib *d, int w, int h) {
	if (d->dc && d->w == w && d->h == h) return 1;
	dib_free(d);
	BITMAPINFO bi;
	memset(&bi, 0, sizeof bi);
	bi.bmiHeader.biSize = sizeof bi.bmiHeader;
	bi.bmiHeader.biWidth = w;
	bi.bmiHeader.biHeight = -h;
	bi.bmiHeader.biPlanes = 1;
	bi.bmiHeader.biBitCount = 32;
	bi.bmiHeader.biCompression = BI_RGB;
	void *bits = NULL;
	HBITMAP b = CreateDIBSection(NULL, &bi, DIB_RGB_COLORS, &bits, NULL, 0);
	if (!b) return 0;
	HDC dc = CreateCompatibleDC(NULL);
	if (!dc) {
		DeleteObject(b);
		return 0;
	}
	d->dc = dc;
	d->old = SelectObject(dc, b);
	d->bmp = b;
	d->bits = bits;
	d->w = w;
	d->h = h;
	return 1;
}

struct dk_cap {
	int x, y, w, h; // the monitor area read
	// The Desktop Duplication of that monitor, and where its picture is
	// copied for the CPU to read.
	ID3D11Device *dev;
	ID3D11DeviceContext *ctx;
	IDXGIOutputDuplication *dup;
	ID3D11Texture2D *staging;
	ULONGLONG retry_at; // when to try the GPU again
	dib pic;            // the screen's last picture
	dib shot;           // that picture with the pointer drawn in
	uint8_t *prev;      // GDI: the last grab, to skip still screens
	size_t prev_len;
	int fresh;  // pic changed since it was last handed over
	int ow, oh; // the size last handed over
	// The pointer last drawn into a picture.
	int cur_drawn;
	HCURSOR cur;
	POINT cur_at;
};

dk_cap *dk_open(void) {
	dk_cap *c = calloc(1, sizeof *c);
	if (c) c->x = c->y = INT_MIN;
	return c;
}

static void dup_close(dk_cap *c) {
	if (c->staging) ID3D11Texture2D_Release(c->staging);
	if (c->dup) IDXGIOutputDuplication_Release(c->dup);
	if (c->ctx) ID3D11DeviceContext_Release(c->ctx);
	if (c->dev) ID3D11Device_Release(c->dev);
	c->staging = NULL;
	c->dup = NULL;
	c->ctx = NULL;
	c->dev = NULL;
}

void dk_close(dk_cap *c) {
	if (!c) return;
	dup_close(c);
	dib_free(&c->pic);
	dib_free(&c->shot);
	free(c->prev);
	free(c);
}

static int dup_start(dk_cap *c, IDXGIAdapter *a, IDXGIOutput *o) {
	D3D_FEATURE_LEVEL level;
	if (FAILED(D3D11CreateDevice(a, D3D_DRIVER_TYPE_UNKNOWN, NULL, 0, NULL, 0, D3D11_SDK_VERSION, &c->dev, &level, &c->ctx))) return 0;
	IDXGIOutput1 *o1 = NULL;
	HRESULT hr = IDXGIOutput_QueryInterface(o, &IID_IDXGIOutput1, (void **)&o1);
	if (SUCCEEDED(hr)) {
		hr = IDXGIOutput1_DuplicateOutput(o1, (IUnknown *)c->dev, &c->dup);
		IDXGIOutput1_Release(o1);
	}
	if (FAILED(hr)) {
		dup_close(c);
		return 0;
	}
	return 1;
}

// Starts duplicating the GPU output that shows exactly the area read, when
// it is not rotated (rotated ones come unrotated: GDI reads those).
static int dup_open(dk_cap *c) {
	IDXGIFactory1 *f = NULL;
	if (FAILED(CreateDXGIFactory1(&IID_IDXGIFactory1, (void **)&f))) return 0;
	int ok = 0;
	IDXGIAdapter1 *a = NULL;
	for (UINT i = 0; !ok && IDXGIFactory1_EnumAdapters1(f, i, &a) != DXGI_ERROR_NOT_FOUND; i++) {
		IDXGIOutput *o = NULL;
		for (UINT j = 0; !ok && IDXGIAdapter1_EnumOutputs(a, j, &o) != DXGI_ERROR_NOT_FOUND; j++) {
			DXGI_OUTPUT_DESC d;
			if (SUCCEEDED(IDXGIOutput_GetDesc(o, &d)) && d.AttachedToDesktop && d.DesktopCoordinates.left == c->x &&
			    d.DesktopCoordinates.top == c->y && d.DesktopCoordinates.right - d.DesktopCoordinates.left == c->w &&
			    d.DesktopCoordinates.bottom - d.DesktopCoordinates.top == c->h &&
			    (d.Rotation == DXGI_MODE_ROTATION_IDENTITY || d.Rotation == DXGI_MODE_ROTATION_UNSPECIFIED))
				ok = dup_start(c, (IDXGIAdapter *)a, o);
			IDXGIOutput_Release(o);
		}
		IDXGIAdapter1_Release(a);
	}
	IDXGIFactory1_Release(f);
	return ok;
}

// Copies the duplicated picture into pic when the desktop changed (or pic
// has none yet): 1 when it did, 0 when nothing changed, -1 when the
// duplication is lost (a desktop switch, a display mode change).
static int dup_read(dk_cap *c) {
	DXGI_OUTDUPL_FRAME_INFO info;
	IDXGIResource *res = NULL;
	HRESULT hr = IDXGIOutputDuplication_AcquireNextFrame(c->dup, 0, &info, &res);
	if (hr == DXGI_ERROR_WAIT_TIMEOUT) return 0;
	if (FAILED(hr)) return -1;
	int got = 0;
	// LastPresentTime is 0 when only the pointer moved.
	if (info.LastPresentTime.QuadPart != 0 || !c->pic.dc) {
		got = -1;
		ID3D11Texture2D *tex = NULL;
		if (SUCCEEDED(IDXGIResource_QueryInterface(res, &IID_ID3D11Texture2D, (void **)&tex))) {
			D3D11_TEXTURE2D_DESC td;
			ID3D11Texture2D_GetDesc(tex, &td);
			if ((int)td.Width == c->w && (int)td.Height == c->h) {
				if (!c->staging) {
					D3D11_TEXTURE2D_DESC sd = td;
					sd.MipLevels = 1;
					sd.ArraySize = 1;
					sd.SampleDesc.Count = 1;
					sd.SampleDesc.Quality = 0;
					sd.Usage = D3D11_USAGE_STAGING;
					sd.BindFlags = 0;
					sd.CPUAccessFlags = D3D11_CPU_ACCESS_READ;
					sd.MiscFlags = 0;
					if (FAILED(ID3D11Device_CreateTexture2D(c->dev, &sd, NULL, &c->staging))) c->staging = NULL;
				}
				D3D11_MAPPED_SUBRESOURCE m;
				if (c->staging && dib_size(&c->pic, c->w, c->h)) {
					ID3D11DeviceContext_CopyResource(c->ctx, (ID3D11Resource *)c->staging, (ID3D11Resource *)tex);
					if (SUCCEEDED(ID3D11DeviceContext_Map(c->ctx, (ID3D11Resource *)c->staging, 0, D3D11_MAP_READ, 0, &m))) {
						for (int y = 0; y < c->h; y++)
							memcpy(c->pic.bits + (size_t)y * c->w * 4, (const uint8_t *)m.pData + (size_t)y * m.RowPitch, (size_t)c->w * 4);
						ID3D11DeviceContext_Unmap(c->ctx, (ID3D11Resource *)c->staging, 0);
						got = 1;
					}
				}
			}
			ID3D11Texture2D_Release(tex);
		}
	}
	IDXGIResource_Release(res);
	IDXGIOutputDuplication_ReleaseFrame(c->dup);
	return got;
}

// Reads the area with GDI into pic: 1 when it changed, 0 when it did not,
// -1 when it cannot be read.
static int gdi_read(dk_cap *c) {
	if (!dib_size(&c->pic, c->w, c->h)) return -1;
	HDC screen = GetDC(NULL);
	if (!screen) return -1;
	// CAPTUREBLT: layered windows (menus, tooltips) too.
	BOOL ok = BitBlt(c->pic.dc, 0, 0, c->w, c->h, screen, c->x, c->y, SRCCOPY | CAPTUREBLT);
	ReleaseDC(NULL, screen);
	if (!ok) return -1;
	GdiFlush();
	size_t len = (size_t)c->w * c->h * 4;
	if (c->prev && c->prev_len == len && memcmp(c->prev, c->pic.bits, len) == 0) return 0;
	if (c->prev_len != len) {
		free(c->prev);
		c->prev = malloc(len);
		c->prev_len = c->prev ? len : 0;
	}
	if (c->prev) memcpy(c->prev, c->pic.bits, len);
	return 1;
}

dk_frame *dk_grab(dk_cap *c, int x, int y, int w, int h, int ow, int oh, int cursor, int *err, int *how) {
	*err = 0;
	*how = 0;
	if (w <= 0 || h <= 0 || ow < 2 || oh < 2) {
		*err = 1;
		return NULL;
	}
	ow &= ~1;
	oh &= ~1;
	if (x != c->x || y != c->y || w != c->w || h != c->h) {
		// Another monitor, or this one's mode changed: start over.
		dup_close(c);
		dib_free(&c->pic);
		free(c->prev);
		c->prev = NULL;
		c->prev_len = 0;
		c->x = x;
		c->y = y;
		c->w = w;
		c->h = h;
		c->retry_at = 0;
		c->ow = c->oh = 0;
	}

	ULONGLONG now = GetTickCount64();
	if (!c->dup && now >= c->retry_at && !dup_open(c)) c->retry_at = now + 3000;
	int r = -1;
	if (c->dup) {
		r = dup_read(c);
		if (r < 0) {
			dup_close(c);
			c->retry_at = now + 1000;
		} else {
			*how = 1;
		}
	}
	if (r < 0) {
		r = gdi_read(c);
		if (r >= 0) *how = 2;
	}
	if (r < 0 || !c->pic.dc) return NULL; // the secure desktop: keep the last picture
	if (r == 1) c->fresh = 1;

	// The pointer, drawn in when the viewer wants it in the picture: a
	// new frame when it moves or changes shape too.
	CURSORINFO ci;
	memset(&ci, 0, sizeof ci);
	ci.cbSize = sizeof ci;
	int show = cursor && GetCursorInfo(&ci) && (ci.flags & CURSOR_SHOWING) && ci.hCursor;
	int moved = show ? (!c->cur_drawn || ci.hCursor != c->cur || ci.ptScreenPos.x != c->cur_at.x || ci.ptScreenPos.y != c->cur_at.y) : c->cur_drawn;
	if (!c->fresh && !moved && ow == c->ow && oh == c->oh) return NULL;
	c->fresh = 0;
	c->ow = ow;
	c->oh = oh;
	c->cur_drawn = 0;

	dib *src = &c->pic;
	if (show && dib_size(&c->shot, c->w, c->h)) {
		memcpy(c->shot.bits, c->pic.bits, (size_t)c->w * c->h * 4);
		ICONINFO ii;
		int hx = 0, hy = 0;
		if (GetIconInfo(ci.hCursor, &ii)) {
			hx = (int)ii.xHotspot;
			hy = (int)ii.yHotspot;
			if (ii.hbmMask) DeleteObject(ii.hbmMask);
			if (ii.hbmColor) DeleteObject(ii.hbmColor);
		}
		DrawIconEx(c->shot.dc, ci.ptScreenPos.x - c->x - hx, ci.ptScreenPos.y - c->y - hy, ci.hCursor, 0, 0, 0, NULL, DI_NORMAL);
		GdiFlush();
		src = &c->shot;
		c->cur_drawn = 1;
		c->cur = ci.hCursor;
		c->cur_at = ci.ptScreenPos;
	}
	return dk_scale(src->bits, src->w * 4, src->w, src->h, ow, oh, 0);
}
