// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux && !headless

#include <errno.h>
#include <fcntl.h>
#include <poll.h>
#include <pthread.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>
#include <wayland-client.h>

#include "wlr_data_control_linux.h"

// A Wayland desktop's clipboard through its data-control protocol
// (zwlr_data_control_manager_v1: KDE Plasma, Sway, Hyprland), which lets a
// program read and set the clipboard without a window of its own. Its own
// connection and thread: the clipboard is handed over through pipes, and
// whoever owns it must answer while they do. GNOME has no such protocol:
// there the X11 clipboard (XWayland mirrors it both ways) does.

static pthread_mutex_t mu = PTHREAD_MUTEX_INITIALIZER;
static pthread_cond_t cv = PTHREAD_COND_INITIALIZER;
static int started, ready;
static int wake[2];
static struct wl_display *wd;
static struct wl_seat *seat;
static struct zwlr_data_control_manager_v1 *manager;
static struct zwlr_data_control_device_v1 *device;

// An offer being announced, and the best text type it has.
typedef struct {
	struct zwlr_data_control_offer_v1 *offer;
	int rank; // 0: no text
	char mime[64];
	int refs; // the clipboard and the primary selection may share one
} offer_t;

static offer_t *current;  // the clipboard now (NULL: empty)
static offer_t *primary;  // the primary selection (unused, but held)
static int changes;       // every copy, by anyone
static char *own;         // the text this app copied, or NULL
static int own_len;
static struct zwlr_data_control_source_v1 *source; // ours while we own it
static int owned;         // the clipboard is ours
static int want_own;      // set it on the next turn
static int want_read;     // read it on the next turn
static char *read_text;
static int read_len, read_done;

static void nudge(void) { (void)!write(wake[1], "x", 1); }

static int rank_of(const char *mime) {
	if (!strcmp(mime, "text/plain;charset=utf-8")) return 4;
	if (!strcmp(mime, "UTF8_STRING")) return 3;
	if (!strcmp(mime, "text/plain")) return 2;
	if (!strcmp(mime, "TEXT") || !strcmp(mime, "STRING")) return 1;
	return 0;
}

static void offer_mime(void *data, struct zwlr_data_control_offer_v1 *o, const char *mime) {
	offer_t *of = data;
	(void)o;
	int r = rank_of(mime);
	if (r > of->rank) {
		of->rank = r;
		strncpy(of->mime, mime, sizeof of->mime - 1);
	}
}

static const struct zwlr_data_control_offer_v1_listener offer_listener = {.offer = offer_mime};

static void drop_offer(offer_t *of) {
	if (!of || --of->refs > 0) return;
	zwlr_data_control_offer_v1_destroy(of->offer);
	free(of);
}

// The offer behind o, one more reference to it.
static offer_t *hold_offer(struct zwlr_data_control_offer_v1 *o) {
	if (!o) return NULL;
	offer_t *of = zwlr_data_control_offer_v1_get_user_data(o);
	of->refs++;
	return of;
}

static void device_data_offer(void *data, struct zwlr_data_control_device_v1 *d, struct zwlr_data_control_offer_v1 *o) {
	(void)data;
	(void)d;
	offer_t *of = calloc(1, sizeof *of);
	of->offer = o;
	zwlr_data_control_offer_v1_add_listener(o, &offer_listener, of);
}

static void device_selection(void *data, struct zwlr_data_control_device_v1 *d, struct zwlr_data_control_offer_v1 *o) {
	(void)data;
	(void)d;
	offer_t *of = hold_offer(o);
	pthread_mutex_lock(&mu);
	drop_offer(current);
	current = of;
	changes++;
	pthread_cond_broadcast(&cv);
	pthread_mutex_unlock(&mu);
}

static void device_finished(void *data, struct zwlr_data_control_device_v1 *d) {
	(void)data;
	zwlr_data_control_device_v1_destroy(d);
	device = NULL;
}

static void device_primary(void *data, struct zwlr_data_control_device_v1 *d, struct zwlr_data_control_offer_v1 *o) {
	(void)data;
	(void)d;
	offer_t *of = hold_offer(o);
	drop_offer(primary);
	primary = of;
}

static const struct zwlr_data_control_device_v1_listener device_listener = {
	.data_offer = device_data_offer,
	.selection = device_selection,
	.finished = device_finished,
	.primary_selection = device_primary,
};

// Someone pastes what this app copied: write it to them.
static void source_send(void *data, struct zwlr_data_control_source_v1 *s, const char *mime, int32_t fd) {
	(void)data;
	(void)s;
	(void)mime;
	pthread_mutex_lock(&mu);
	char *t = NULL;
	int n = own_len;
	if (own) {
		t = malloc(n);
		memcpy(t, own, n);
	}
	pthread_mutex_unlock(&mu);
	for (int off = 0; t && off < n;) {
		ssize_t w = write(fd, t + off, n - off);
		if (w < 0 && errno == EINTR) continue;
		if (w <= 0) break;
		off += w;
	}
	free(t);
	close(fd);
}

static void source_cancelled(void *data, struct zwlr_data_control_source_v1 *s) {
	(void)data;
	zwlr_data_control_source_v1_destroy(s);
	pthread_mutex_lock(&mu);
	if (source == s) {
		source = NULL;
		owned = 0;
	}
	pthread_mutex_unlock(&mu);
}

static const struct zwlr_data_control_source_v1_listener source_listener = {
	.send = source_send,
	.cancelled = source_cancelled,
};

static void registry_global(void *data, struct wl_registry *r, uint32_t name, const char *iface, uint32_t version) {
	(void)data;
	if (!strcmp(iface, wl_seat_interface.name) && !seat) {
		seat = wl_registry_bind(r, name, &wl_seat_interface, 1);
	} else if (!strcmp(iface, zwlr_data_control_manager_v1_interface.name)) {
		manager = wl_registry_bind(r, name, &zwlr_data_control_manager_v1_interface, version < 2 ? version : 2);
	}
}

static void registry_remove(void *data, struct wl_registry *r, uint32_t name) {
	(void)data;
	(void)r;
	(void)name;
}

static const struct wl_registry_listener registry_listener = {.global = registry_global, .global_remove = registry_remove};

// Reads the current offer's text (on the loop's thread: other programs
// write it, and only Wayland requests of ours go out meanwhile).
static void read_current(void) {
	pthread_mutex_lock(&mu);
	offer_t *of = current;
	int mine = owned;
	pthread_mutex_unlock(&mu);
	char *text = NULL;
	int len = 0;
	if (mine) {
		pthread_mutex_lock(&mu);
		if (own) {
			text = malloc(own_len + 1);
			memcpy(text, own, own_len);
			text[own_len] = 0;
			len = own_len;
		}
		pthread_mutex_unlock(&mu);
	} else if (of && of->rank > 0) {
		int p[2];
		if (pipe(p) == 0) {
			zwlr_data_control_offer_v1_receive(of->offer, of->mime, p[1]);
			wl_display_flush(wd);
			close(p[1]);
			int cap = 4096;
			text = malloc(cap);
			// Up to a second, and 8 MB at most. Wayland events keep being
			// answered meanwhile: the program that copied may need ours
			// (a clipboard manager taking over what this app copied).
			int wfd = wl_display_get_fd(wd);
			struct timespec t0;
			clock_gettime(CLOCK_MONOTONIC, &t0);
			for (;;) {
				struct timespec now;
				clock_gettime(CLOCK_MONOTONIC, &now);
				int left = 1000 - (int)((now.tv_sec - t0.tv_sec) * 1000 + (now.tv_nsec - t0.tv_nsec) / 1000000);
				if (left <= 0) break;
				while (wl_display_prepare_read(wd) != 0) wl_display_dispatch_pending(wd);
				wl_display_flush(wd);
				struct pollfd pf[2] = {{p[0], POLLIN, 0}, {wfd, POLLIN, 0}};
				int r = poll(pf, 2, left);
				if (r > 0 && (pf[1].revents & POLLIN)) {
					wl_display_read_events(wd);
				} else {
					wl_display_cancel_read(wd);
				}
				wl_display_dispatch_pending(wd);
				if (r < 0 && errno == EINTR) continue;
				if (r <= 0) break;
				if (!(pf[0].revents & (POLLIN | POLLHUP))) continue;
				if (len + 4096 + 1 > cap) {
					if (cap >= 8 << 20) break;
					cap *= 2;
					text = realloc(text, cap);
				}
				ssize_t n = read(p[0], text + len, cap - len - 1);
				if (n < 0 && errno == EINTR) continue;
				if (n <= 0) break;
				len += n;
			}
			text[len] = 0;
			close(p[0]);
		}
	}
	pthread_mutex_lock(&mu);
	free(read_text);
	read_text = text;
	read_len = len;
	read_done = 1;
	pthread_cond_broadcast(&cv);
	pthread_mutex_unlock(&mu);
}

static void take_ownership(void) {
	struct zwlr_data_control_source_v1 *s = zwlr_data_control_manager_v1_create_data_source(manager);
	zwlr_data_control_source_v1_add_listener(s, &source_listener, NULL);
	zwlr_data_control_source_v1_offer(s, "text/plain;charset=utf-8");
	zwlr_data_control_source_v1_offer(s, "text/plain");
	zwlr_data_control_source_v1_offer(s, "UTF8_STRING");
	zwlr_data_control_source_v1_offer(s, "STRING");
	zwlr_data_control_source_v1_offer(s, "TEXT");
	zwlr_data_control_device_v1_set_selection(device, s);
	pthread_mutex_lock(&mu);
	struct zwlr_data_control_source_v1 *old = source;
	source = s;
	owned = 1;
	pthread_mutex_unlock(&mu);
	if (old) zwlr_data_control_source_v1_destroy(old);
}

static void *loop(void *arg) {
	(void)arg;
	int fd = wl_display_get_fd(wd);
	for (;;) {
		while (wl_display_prepare_read(wd) != 0) wl_display_dispatch_pending(wd);
		wl_display_flush(wd);
		struct pollfd pf[2] = {{fd, POLLIN, 0}, {wake[0], POLLIN, 0}};
		if (poll(pf, 2, -1) < 0 && errno != EINTR) {
			wl_display_cancel_read(wd);
			break;
		}
		if (pf[0].revents & POLLIN) {
			if (wl_display_read_events(wd) < 0) break;
		} else {
			wl_display_cancel_read(wd);
		}
		if (pf[0].revents & (POLLERR | POLLHUP)) break;
		wl_display_dispatch_pending(wd);
		if (pf[1].revents & POLLIN) {
			char buf[64];
			while (read(wake[0], buf, sizeof buf) > 0) {}
		}
		pthread_mutex_lock(&mu);
		int own_now = want_own, read_now = want_read;
		want_own = want_read = 0;
		pthread_mutex_unlock(&mu);
		if (own_now && device) take_ownership();
		if (read_now) read_current();
	}
	pthread_mutex_lock(&mu);
	ready = 0;
	read_done = 1;
	pthread_cond_broadcast(&cv);
	pthread_mutex_unlock(&mu);
	return NULL;
}

// Connects to the desktop; 1 when it has the data-control protocol.
int wlc_start(void) {
	pthread_mutex_lock(&mu);
	if (started) {
		int r = ready;
		pthread_mutex_unlock(&mu);
		return r;
	}
	started = 1;
	pthread_mutex_unlock(&mu);
	// Only the first caller gets here; the handlers the roundtrips run
	// take mu themselves.
	wd = wl_display_connect(NULL);
	if (!wd || pipe(wake) != 0) return 0;
	fcntl(wake[0], F_SETFL, O_NONBLOCK);
	fcntl(wake[1], F_SETFL, O_NONBLOCK);
	struct wl_registry *reg = wl_display_get_registry(wd);
	wl_registry_add_listener(reg, &registry_listener, NULL);
	wl_display_roundtrip(wd);
	if (!seat || !manager) {
		wl_display_disconnect(wd);
		wd = NULL;
		return 0;
	}
	device = zwlr_data_control_manager_v1_get_data_device(manager, seat);
	zwlr_data_control_device_v1_add_listener(device, &device_listener, NULL);
	wl_display_roundtrip(wd); // the clipboard as it is now
	pthread_t th;
	int ok = pthread_create(&th, NULL, loop, NULL) == 0;
	pthread_mutex_lock(&mu);
	ready = ok;
	pthread_mutex_unlock(&mu);
	return ok;
}

int wlc_changes(void) {
	pthread_mutex_lock(&mu);
	int c = changes;
	pthread_mutex_unlock(&mu);
	return c;
}

static void deadline(struct timespec *ts, int ms) {
	clock_gettime(CLOCK_REALTIME, ts);
	ts->tv_nsec += (long)ms * 1000000;
	ts->tv_sec += ts->tv_nsec / 1000000000;
	ts->tv_nsec %= 1000000000;
}

// The clipboard's text (malloc'd, the caller frees it), or NULL.
char *wlc_text(int *len) {
	pthread_mutex_lock(&mu);
	read_done = 0;
	want_read = 1;
	pthread_mutex_unlock(&mu);
	nudge();
	struct timespec ts;
	deadline(&ts, 1500);
	pthread_mutex_lock(&mu);
	while (!read_done) {
		if (pthread_cond_timedwait(&cv, &mu, &ts) != 0) break;
	}
	char *t = read_done ? read_text : NULL;
	*len = read_done ? read_len : 0;
	if (read_done) read_text = NULL;
	pthread_mutex_unlock(&mu);
	return t;
}

// Owns the clipboard with text; the change count after it.
int wlc_set(const char *text, int len) {
	pthread_mutex_lock(&mu);
	free(own);
	own = malloc(len + 1);
	memcpy(own, text, len);
	own[len] = 0;
	own_len = len;
	int before = changes;
	want_own = 1;
	pthread_mutex_unlock(&mu);
	nudge();
	struct timespec ts;
	deadline(&ts, 500);
	pthread_mutex_lock(&mu);
	while (changes == before) {
		if (pthread_cond_timedwait(&cv, &mu, &ts) != 0) break;
	}
	int c = changes;
	pthread_mutex_unlock(&mu);
	return c;
}
