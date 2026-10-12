// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build linux

#include <X11/Xlib.h>
#include <X11/Xatom.h>
#include <X11/extensions/Xfixes.h>
#include <fcntl.h>
#include <poll.h>
#include <pthread.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>

// The X11 clipboard (the CLIPBOARD selection, as UTF-8 text) on a thread of
// its own: X hands a selection over by events, so its owner must answer
// them while it holds it. A window nobody sees asks for the text, owns what
// this app copies, and counts every copy (XFixes).

static pthread_mutex_t mu = PTHREAD_MUTEX_INITIALIZER;
static pthread_cond_t cv = PTHREAD_COND_INITIALIZER;
static int started, ready;
static int wake[2];
static Display *dpy;
static Window win;
static Atom CLIP, UTF8, TARGETS, PROP, TEXT_ATOM;
static int fixes_event;

static int changes;     // every copy, by anyone
static char *own;       // the text this app copied (owned), or NULL
static int own_len;
static int want_own;    // own the selection on the next turn
static int owned;       // we own it now
static int want_read;   // read the selection on the next turn
static int reading;     // a conversion is on its way
static char *read_text; // what came
static int read_len, read_done;

static void answer(XSelectionRequestEvent *rq) {
	XSelectionEvent ev = {0};
	ev.type = SelectionNotify;
	ev.display = rq->display;
	ev.requestor = rq->requestor;
	ev.selection = rq->selection;
	ev.target = rq->target;
	ev.time = rq->time;
	ev.property = None;
	Atom prop = rq->property != None ? rq->property : rq->target;
	pthread_mutex_lock(&mu);
	if (own) {
		if (rq->target == TARGETS) {
			Atom t[] = {TARGETS, UTF8, XA_STRING, TEXT_ATOM};
			XChangeProperty(dpy, rq->requestor, prop, XA_ATOM, 32, PropModeReplace, (unsigned char *)t, 4);
			ev.property = prop;
		} else if (rq->target == UTF8 || rq->target == XA_STRING || rq->target == TEXT_ATOM) {
			XChangeProperty(dpy, rq->requestor, prop, rq->target == TEXT_ATOM ? UTF8 : rq->target, 8, PropModeReplace, (unsigned char *)own, own_len);
			ev.property = prop;
		}
	}
	pthread_mutex_unlock(&mu);
	XSendEvent(dpy, rq->requestor, False, 0, (XEvent *)&ev);
	XFlush(dpy);
}

static void finish_read(char *text, int len) {
	pthread_mutex_lock(&mu);
	free(read_text);
	read_text = text;
	read_len = len;
	read_done = 1;
	reading = 0;
	pthread_cond_broadcast(&cv);
	pthread_mutex_unlock(&mu);
}

static void *loop(void *arg) {
	(void)arg;
	int fd = ConnectionNumber(dpy);
	for (;;) {
		pthread_mutex_lock(&mu);
		int own_now = want_own, read_now = want_read;
		want_own = want_read = 0;
		pthread_mutex_unlock(&mu);
		if (own_now) {
			XSetSelectionOwner(dpy, CLIP, win, CurrentTime);
			pthread_mutex_lock(&mu);
			owned = XGetSelectionOwner(dpy, CLIP) == win;
			pthread_mutex_unlock(&mu);
			XFlush(dpy);
		}
		if (read_now) {
			if (XGetSelectionOwner(dpy, CLIP) == None) {
				finish_read(NULL, 0);
			} else {
				pthread_mutex_lock(&mu);
				reading = 1;
				pthread_mutex_unlock(&mu);
				XConvertSelection(dpy, CLIP, UTF8, PROP, win, CurrentTime);
				XFlush(dpy);
			}
		}
		while (XPending(dpy)) {
			XEvent ev;
			XNextEvent(dpy, &ev);
			if (ev.type == fixes_event + XFixesSelectionNotify) {
				pthread_mutex_lock(&mu);
				changes++;
				owned = ((XFixesSelectionNotifyEvent *)&ev)->owner == win;
				pthread_cond_broadcast(&cv);
				pthread_mutex_unlock(&mu);
			} else if (ev.type == SelectionRequest) {
				answer(&ev.xselectionrequest);
			} else if (ev.type == SelectionClear) {
				pthread_mutex_lock(&mu);
				owned = 0;
				pthread_mutex_unlock(&mu);
			} else if (ev.type == SelectionNotify) {
				char *text = NULL;
				int len = 0;
				if (ev.xselection.property != None) {
					Atom type;
					int format;
					unsigned long n, after;
					unsigned char *data = NULL;
					if (XGetWindowProperty(dpy, win, PROP, 0, 1 << 20, True, AnyPropertyType, &type, &format, &n, &after, &data) == Success && data && format == 8) {
						text = malloc(n + 1);
						memcpy(text, data, n);
						text[n] = 0;
						len = (int)n;
					}
					if (data) XFree(data);
				}
				finish_read(text, len);
			}
		}
		struct pollfd p[2] = {{fd, POLLIN, 0}, {wake[0], POLLIN, 0}};
		poll(p, 2, 100);
		if (p[1].revents & POLLIN) {
			char b[16];
			while (read(wake[0], b, sizeof b) > 0) {
			}
		}
	}
	return NULL;
}

static void nudge(void) { (void)!write(wake[1], "x", 1); }

int clip_start(void) {
	pthread_mutex_lock(&mu);
	if (started) {
		int r = ready;
		pthread_mutex_unlock(&mu);
		return r;
	}
	started = 1;
	dpy = XOpenDisplay(NULL);
	int ev, er;
	if (!dpy || !XFixesQueryExtension(dpy, &ev, &er) || pipe(wake) != 0) {
		pthread_mutex_unlock(&mu);
		return 0;
	}
	fixes_event = ev;
	fcntl(wake[0], F_SETFL, O_NONBLOCK);
	fcntl(wake[1], F_SETFL, O_NONBLOCK);
	win = XCreateSimpleWindow(dpy, DefaultRootWindow(dpy), 0, 0, 1, 1, 0, 0, 0);
	CLIP = XInternAtom(dpy, "CLIPBOARD", False);
	UTF8 = XInternAtom(dpy, "UTF8_STRING", False);
	TARGETS = XInternAtom(dpy, "TARGETS", False);
	TEXT_ATOM = XInternAtom(dpy, "TEXT", False);
	PROP = XInternAtom(dpy, "GO_LINK_DESK_CLIP", False);
	XFixesSelectSelectionInput(dpy, win, CLIP, XFixesSetSelectionOwnerNotifyMask | XFixesSelectionWindowDestroyNotifyMask | XFixesSelectionClientCloseNotifyMask);
	XFlush(dpy);
	pthread_t th;
	ready = pthread_create(&th, NULL, loop, NULL) == 0;
	pthread_mutex_unlock(&mu);
	return ready;
}

int clip_changes(void) {
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
char *clip_text(int *len) {
	pthread_mutex_lock(&mu);
	if (owned && own) {
		char *t = malloc(own_len + 1);
		memcpy(t, own, own_len);
		t[own_len] = 0;
		*len = own_len;
		pthread_mutex_unlock(&mu);
		return t;
	}
	read_done = 0;
	want_read = 1;
	pthread_mutex_unlock(&mu);
	nudge();
	struct timespec ts;
	deadline(&ts, 1000);
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
int clip_set(const char *text, int len) {
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
