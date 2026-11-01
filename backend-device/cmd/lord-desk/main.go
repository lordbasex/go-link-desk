// Command lord-desk is stage 0 of go-link Desk on LordOS: a throwaway proof
// of concept. It takes the session's encoded video from remote.sock and
// serves it to one go-link viewer through signalhub, accepting every viewer
// without asking. For a test session with nothing real on it, never more.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/lordbasex/go-link-desk/backend-device/internal/controllers"
	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
	"github.com/lordbasex/go-link-desk/backend-device/internal/services"
	screencap "github.com/lordbasex/go-link-desk/backend-device/pkg/capture"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/ids"
	"github.com/lordbasex/go-link-desk/backend-device/pkg/signalclient"
)

// proto.h
const (
	rpSize     = 1
	rpVFrame   = 40
	rpVideo    = 32
	rpKeyframe = 33
	rpRate     = 34
	rpPtr      = 11
	rpBtn      = 12
	rpWheel    = 13
	rpKey      = 14

	lordH264 = 1
	lordVP8  = 2

	hz = 30
)

// session is the connection to remote.sock.
type session struct {
	conn net.Conn
	mu   sync.Mutex
	w, h atomic.Int32
}

func (s *session) send(t byte, d []byte) {
	b := make([]byte, 5+len(d))
	binary.LittleEndian.PutUint32(b, uint32(1+len(d)))
	b[4] = t
	copy(b[5:], d)
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.conn.Write(b)
}

func (s *session) video(codec, kbps int) {
	d := make([]byte, 8)
	d[0] = byte(codec)
	binary.LittleEndian.PutUint16(d[2:], hz)
	binary.LittleEndian.PutUint32(d[4:], uint32(kbps))
	s.send(rpVideo, d)
}

func (s *session) displays() []screencap.Display {
	w, h := int(s.w.Load()), int(s.h.Load())
	if w == 0 {
		w, h = 1280, 800
	}
	return []screencap.Display{{ID: 1, Name: "LordOS", Width: w, Height: h, PointsWidth: w, PointsHeight: h, Primary: true}}
}

// counters are what the stats line reports.
type counters struct {
	frames, keys, bytes atomic.Int64
	firstNS             atomic.Int64 // time of the first frame after an RP_VIDEO
}

func (s *session) read(c *counters, stream *services.StreamService, log *slog.Logger) error {
	var hdr [5]byte
	var buf []byte
	for {
		if _, err := io.ReadFull(s.conn, hdr[:]); err != nil {
			return err
		}
		n := int(binary.LittleEndian.Uint32(hdr[:])) - 1
		if n < 0 || n > 40<<20 {
			return fmt.Errorf("bad message length %d", n)
		}
		if cap(buf) < n {
			buf = make([]byte, n)
		}
		buf = buf[:n]
		if _, err := io.ReadFull(s.conn, buf); err != nil {
			return err
		}
		switch hdr[4] {
		case rpSize:
			if n >= 4 {
				s.w.Store(int32(binary.LittleEndian.Uint16(buf)))
				s.h.Store(int32(binary.LittleEndian.Uint16(buf[2:])))
				log.Info("session size", "w", s.w.Load(), "h", s.h.Load())
			}
		case rpVFrame:
			if n < 8 {
				continue
			}
			codec, key := services.CodecVP8, buf[1] != 0
			if buf[0] == lordH264 {
				codec = services.CodecH264
			}
			ms := binary.LittleEndian.Uint32(buf[4:])
			c.firstNS.CompareAndSwap(0, time.Now().UnixNano())
			c.frames.Add(1)
			c.bytes.Add(int64(n - 8))
			if key {
				c.keys.Add(1)
			}
			frame := slices.Clone(buf[8:])
			stream.EncodedFrame(codec, key, time.Duration(ms)*time.Millisecond, frame, int(s.w.Load()), int(s.h.Load()))
		}
	}
}

// drive follows the viewers: starts and stops the encoder, asks for
// keyframes and passes the bitrate on.
func (s *session) drive(ctx context.Context, c *counters, stream *services.StreamService, log *slog.Logger) {
	var codec, kbps int
	var started, lastKey time.Time
	var lastF, lastK, lastB int64
	lastStat := time.Now()
	t := time.NewTicker(50 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			s.video(0, 0)
			return
		case <-t.C:
		}
		want := stream.EncodedWant()
		lc := 0
		switch want.Codec {
		case services.CodecH264:
			lc = lordH264
		case services.CodecVP8:
			lc = lordVP8
		}
		switch {
		case lc != codec:
			log.Info("encoder request", "codec", lc, "kbps", want.Kbps)
			s.video(lc, max(want.Kbps, 300))
			codec, kbps = lc, want.Kbps
			started, lastKey = time.Now(), time.Now()
			c.firstNS.Store(0)
		case lc != 0 && want.Kbps != kbps:
			d := make([]byte, 4)
			binary.LittleEndian.PutUint32(d, uint32(want.Kbps))
			s.send(rpRate, d)
			kbps = want.Kbps
		}
		if lc != 0 && want.NeedKey && time.Since(lastKey) > 500*time.Millisecond {
			s.send(rpKeyframe, nil)
			lastKey = time.Now()
			log.Info("keyframe requested")
		}
		if d := time.Since(lastStat); d >= 5*time.Second {
			f, k, b := c.frames.Load(), c.keys.Load(), c.bytes.Load()
			sec := d.Seconds()
			first := int64(-1)
			if n := c.firstNS.Load(); n != 0 {
				first = (n - started.UnixNano()) / 1e6
			}
			log.Info("stats", "viewers", want.Viewers, "codec", want.Codec, "kbps_target", want.Kbps,
				"fps", fmt.Sprintf("%.1f", float64(f-lastF)/sec), "keys", k-lastK,
				"kbit_s", int(float64(b-lastB)*8/1000/sec), "first_frame_ms", first, "path", stream.PathInfo())
			lastF, lastK, lastB, lastStat = f, k, b, time.Now()
		}
	}
}

// idleSource: the picture comes through EncodedFrame, not VideoFrame.
type idleSource struct{}

func (idleSource) Run(ctx context.Context, _ services.MediaSink) error {
	<-ctx.Done()
	return ctx.Err()
}

func main() {
	sock := flag.String("socket", os.Getenv("XDG_RUNTIME_DIR")+"/lordos/remote.sock", "the session's remote.sock")
	codec := flag.String("codec", "auto", "auto, h264 or vp8")
	relay := flag.Bool("relay-only", false, "carry the session through TURN only")
	sigURL := flag.String("signal", models.DefaultSignalURL, "signalhub URL")
	control := flag.Bool("control", false, "let the viewer use the mouse and keyboard")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(log)
	if err := run(*sock, *codec, *sigURL, *relay, *control, log); err != nil && err != context.Canceled {
		log.Error("stopped", "err", err)
		os.Exit(1)
	}
}

func run(sock, codec, sigURL string, relay, control bool, log *slog.Logger) error {
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return err
	}
	defer conn.Close()
	sess := &session{conn: conn}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	status := services.NewStatusService("lord-desk-poc", "poc", sigURL)
	ice := services.NewICEStore()
	// A throwaway device: new id and secret on every run, kept nowhere.
	var rb [32]byte
	_, _ = rand.Read(rb[:])
	pairing := services.NewPairingService(services.PairingConfig{
		DeviceID: ids.NewUUIDv4(), DeviceSecret: base64.RawURLEncoding.EncodeToString(rb[:]), Logger: log,
	}, status, ice)
	opener := services.NewRoomOpener()
	meta := services.SessionMeta{Name: "lordos-poc", OS: "linux"}
	api, err := services.NewWebRTCAPI(0, false)
	if err != nil {
		return err
	}
	stream, err := services.NewStreamService(services.StreamConfig{
		API: api, Welcome: meta, Source: "screen", Logger: log, RelayOnly: relay,
		ListDisplays: sess.displays,
		PickCodec: func(browser []string) string {
			switch {
			case codec == "vp8":
				return services.CodecVP8
			case codec == "h264", codec == "auto" && slices.Contains(browser, services.CodecH264):
				return services.CodecH264 // the x264 line is Baseline: never High
			}
			return services.CodecVP8
		},
	}, ice)
	if err != nil {
		return err
	}
	stream.SetSource(idleSource{})
	session := services.NewSessionService(opener, status, meta, stream, log)
	log.Warn("every viewer is accepted without asking: a test session only")
	session.SetConsent(services.ConsentFunc(func(context.Context, services.ConsentRequest) (models.Permissions, error) {
		p := models.ViewOnly
		p.Control = control
		return p, nil
	}))
	ctl := controllers.NewControl(log)
	controllers.SessionRoutes(ctl, session)
	stream.OnControl(ctl.Dispatch)
	stream.OnClosed(session.StreamClosed)
	wireInput(stream, session, sess, log)

	client := signalclient.New(signalclient.Config{URL: sigURL, Logger: log, Renewable: func() bool { p, _, _ := session.Active(); return p == "" }},
		services.Dispatcher{pairing, opener, session})
	pairing.SetSender(client)
	opener.SetSender(client)
	session.SetSender(client)
	stream.SetSender(client)

	var c counters
	errc := make(chan error, 3)
	go func() { errc <- client.Run(ctx) }()
	go func() { errc <- stream.Run(ctx) }()
	go func() { errc <- sess.read(&c, stream, log) }()
	go sess.drive(ctx, &c, stream, log)
	err = <-errc
	cancel()
	time.Sleep(100 * time.Millisecond) // the RP_VIDEO 0 goes out
	return err
}
