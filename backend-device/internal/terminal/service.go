// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

//go:build !windows

package terminal

import (
	"encoding/binary"
	"encoding/json"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/pion/webrtc/v4"

	"github.com/lordbasex/go-link-desk/backend-device/internal/models"
)

// Channel is the DataChannel the terminals travel on. Every message is
// one terminal's bytes after its 4-byte id (big-endian): the shell's
// output to the viewer, what the viewer types to the shell.
const Channel = "terminal"

const (
	defaultCols, defaultRows = 80, 24
	maxSize                  = 1000 // columns or rows a viewer may ask for
	readChunk                = 32 << 10
	// The channel's buffer: past high the output waits until it drains to
	// low, so a flood (cat of a big file) never fills memory. The shells
	// are never held up: what does not go out stays in their history.
	highWater = 4 << 20
	lowWater  = 1 << 20
	// History is what each terminal keeps of its output, shown again to a
	// viewer that comes (back).
	History = 2 << 20
	// MaxTabs bounds the terminals open at once.
	MaxTabs = 16
	// maxName bounds a terminal's name, in characters.
	maxName = 40
)

// Service keeps the computer's terminals, like screen: each is a shell with
// a name that goes on running when the viewer leaves, and comes back, with
// what it showed, when a viewer comes. Only the viewer in session sees
// them (Allow); it types only with Control. A shell ending (exit) closes
// its terminal, not the session.
type Service struct {
	shell ShellConfig
	send  func(peerID string, msg []byte) bool // the control channel
	log   *slog.Logger
	start func(ShellConfig, uint16, uint16) (shellIO, error)
	typed func(peerID string)
	now   func() time.Time

	mu      sync.Mutex
	tabs    map[uint32]*tab
	nextID  uint32
	cols    uint16
	rows    uint16
	viewer  *viewer                        // the viewer in session, once its channel and permissions are there
	waiting map[string]*webrtc.DataChannel // channels not let in yet
	allowed map[string]models.Permissions  // permissions that came before the channel
}

// shellIO is what Service needs of a Shell (a test's fake has it too).
type shellIO interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Resize(cols, rows uint16) error
	Done() <-chan struct{}
	ExitCode() int
	Close()
}

type tab struct {
	id      uint32
	name    string
	sh      shellIO
	started time.Time
	history []byte // the last History bytes of output
}

type viewer struct {
	peer    string
	dc      *webrtc.DataChannel
	control bool
	open    bool
	drained chan struct{} // the channel's buffer went under lowWater
	sendMu  sync.Mutex    // frames in order: the history first, then what follows
	gone    chan struct{} // closed when the viewer leaves
	goneSet sync.Once
}

// leave stops what goes to v; true the first time.
func (v *viewer) leave() (first bool) {
	v.goneSet.Do(func() { close(v.gone); first = true })
	return first
}

// detach lets v go: the terminals stay, running.
func (t *Service) detach(v *viewer) {
	t.mu.Lock()
	if t.viewer == v {
		t.viewer = nil
	}
	t.mu.Unlock()
	if v.leave() {
		t.log.Info("terminals detached: they go on running", "peer_id", v.peer)
	}
}

// TabInfo is a terminal as the viewer lists it (terminal_tabs).
type TabInfo struct {
	ID      uint32 `json:"id"`
	Name    string `json:"name"`
	Started int64  `json:"started"` // unix ms
}

// NewService builds the service; send delivers a control message.
func NewService(shell ShellConfig, send func(peerID string, msg []byte) bool, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		shell: shell, send: send, log: log, now: time.Now,
		tabs: make(map[uint32]*tab), cols: defaultCols, rows: defaultRows,
		waiting: make(map[string]*webrtc.DataChannel), allowed: make(map[string]models.Permissions),
		start: func(c ShellConfig, cols, rows uint16) (shellIO, error) { return StartShell(c, cols, rows) },
	}
}

// OnTyped is told whenever a viewer types (the idle timer).
func (t *Service) OnTyped(fn func(peerID string)) { t.typed = fn }

// Channel takes a viewer's terminal channel (StreamService.OnChannel),
// made before the viewer is let in: nothing goes on it until Allow.
func (t *Service) Channel(peerID string, dc *webrtc.DataChannel) {
	if dc.Label() != Channel {
		return
	}
	t.mu.Lock()
	perm, ok := t.allowed[peerID]
	if !ok {
		t.waiting[peerID] = dc
	}
	t.mu.Unlock()
	dc.OnClose(func() {
		t.mu.Lock()
		if t.waiting[peerID] == dc {
			delete(t.waiting, peerID)
		}
		t.mu.Unlock()
	})
	if ok {
		t.attach(peerID, dc, perm)
	}
}

// Allow follows the session's permissions (SessionService.OnPermissions):
// seeing attaches the viewer to the terminals, Control lets it type, and
// nothing (the session ended) detaches it. The terminals stay.
func (t *Service) Allow(peerID string, perm models.Permissions) {
	t.mu.Lock()
	if !perm.View {
		delete(t.allowed, peerID)
		delete(t.waiting, peerID)
		v := t.viewer
		t.mu.Unlock()
		if v != nil && v.peer == peerID {
			t.detach(v)
		}
		return
	}
	if v := t.viewer; v != nil && v.peer == peerID {
		v.control = perm.Control
		t.mu.Unlock()
		return
	}
	dc, ok := t.waiting[peerID]
	if !ok {
		t.allowed[peerID] = perm
		t.mu.Unlock()
		return
	}
	delete(t.waiting, peerID)
	t.mu.Unlock()
	t.attach(peerID, dc, perm)
}

// attach makes peerID the viewer of the terminals: a first one is opened
// when there is none, and once its channel is open it gets the list and
// each terminal's history.
func (t *Service) attach(peerID string, dc *webrtc.DataChannel, perm models.Permissions) {
	v := &viewer{peer: peerID, dc: dc, control: perm.Control, drained: make(chan struct{}, 1), gone: make(chan struct{})}
	t.mu.Lock()
	delete(t.allowed, peerID)
	old := t.viewer
	t.viewer = v
	empty := len(t.tabs) == 0
	t.mu.Unlock()
	if old != nil {
		t.detach(old)
	}
	if empty {
		if _, err := t.open(""); err != nil {
			t.log.Error("cannot start the shell", "shell", t.shell.Path, "err", err)
		}
	}
	dc.SetBufferedAmountLowThreshold(lowWater)
	dc.OnBufferedAmountLow(func() {
		select {
		case v.drained <- struct{}{}:
		default:
		}
	})
	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		if msg.IsString || len(msg.Data) < 4 {
			return
		}
		id := binary.BigEndian.Uint32(msg.Data)
		t.mu.Lock()
		tb := t.tabs[id]
		ok := t.viewer == v && v.control && tb != nil
		t.mu.Unlock()
		if ok {
			_, _ = tb.sh.Write(msg.Data[4:])
			if t.typed != nil {
				t.typed(peerID)
			}
		}
	})
	dc.OnClose(func() { t.detach(v) })
	welcome := func() {
		t.mu.Lock()
		if t.viewer != v || v.open {
			t.mu.Unlock()
			return
		}
		v.open = true
		ids := t.sortedIDs()
		replay := make([][]byte, 0, len(ids))
		for _, id := range ids {
			if h := t.tabs[id].history; len(h) > 0 {
				replay = append(replay, frame(id, h))
			}
		}
		v.sendMu.Lock() // output from now on waits for the history
		t.mu.Unlock()
		t.sendTabsTo(v, !empty)
		for _, f := range replay {
			t.send1(v, f)
		}
		v.sendMu.Unlock()
		t.log.Info("terminals attached", "peer_id", peerID, "terminals", len(ids), "may_type", perm.Control)
	}
	dc.OnOpen(welcome)
	if dc.ReadyState() == webrtc.DataChannelStateOpen {
		welcome()
	}
}

// open starts a new terminal (its name, or "N" after its number).
func (t *Service) open(name string) (uint32, error) {
	t.mu.Lock()
	if len(t.tabs) >= MaxTabs {
		t.mu.Unlock()
		return 0, errTooMany
	}
	cols, rows := t.cols, t.rows
	t.nextID++
	id := t.nextID
	t.mu.Unlock()
	sh, err := t.start(t.shell, cols, rows)
	if err != nil {
		return 0, err
	}
	if name = cleanName(name); name == "" {
		name = defaultName(id)
	}
	tb := &tab{id: id, name: name, sh: sh, started: t.now()}
	t.mu.Lock()
	t.tabs[id] = tb
	t.mu.Unlock()
	t.log.Info("terminal opened", "id", id, "shell", t.shell.Path, "user", t.shell.User)
	go t.pump(tb)
	t.sendTabs()
	return id, nil
}

// pump keeps a terminal's output, and sends it to the viewer, until its
// shell ends.
func (t *Service) pump(tb *tab) {
	buf := make([]byte, readChunk)
	for {
		n, err := tb.sh.Read(buf)
		if n > 0 {
			b := append([]byte(nil), buf[:n]...)
			t.mu.Lock()
			tb.history = append(tb.history, b...)
			if over := len(tb.history) - History; over > 0 {
				tb.history = append([]byte(nil), tb.history[over:]...)
			}
			v := t.viewer
			if v != nil && !v.open {
				v = nil // it gets the history when it opens
			}
			t.mu.Unlock()
			if v != nil {
				t.deliver(v, frame(tb.id, b))
			}
		}
		if err != nil {
			break
		}
	}
	<-tb.sh.Done()
	tb.sh.Close()
	t.mu.Lock()
	_, mine := t.tabs[tb.id]
	delete(t.tabs, tb.id)
	v := t.viewer
	t.mu.Unlock()
	if !mine {
		return // closed by the viewer
	}
	t.log.Info("terminal ended", "id", tb.id, "code", tb.sh.ExitCode())
	if v != nil {
		msg, _ := json.Marshal(struct {
			Type string `json:"type"`
			ID   uint32 `json:"id"`
			Code int    `json:"code"`
		}{"terminal_exit", tb.id, tb.sh.ExitCode()})
		t.send(v.peer, msg)
	}
	t.sendTabs()
}

// deliver sends one frame after whatever is being sent.
func (t *Service) deliver(v *viewer, f []byte) {
	v.sendMu.Lock()
	defer v.sendMu.Unlock()
	t.send1(v, f)
}

// send1 sends one frame, waiting while the channel is full; a viewer that
// left gets nothing more.
func (t *Service) send1(v *viewer, f []byte) {
	if v.dc.BufferedAmount() > highWater {
		select {
		case <-v.drained:
		case <-v.gone:
			return
		}
	}
	select {
	case <-v.gone:
		return
	default:
	}
	_ = v.dc.Send(f)
}

func frame(id uint32, b []byte) []byte {
	f := make([]byte, 4+len(b))
	binary.BigEndian.PutUint32(f, id)
	copy(f[4:], b)
	return f
}

// Tabs lists the terminals, oldest first.
func (t *Service) Tabs() []TabInfo {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]TabInfo, 0, len(t.tabs))
	for _, id := range t.sortedIDs() {
		tb := t.tabs[id]
		out = append(out, TabInfo{ID: tb.id, Name: tb.name, Started: tb.started.UnixMilli()})
	}
	return out
}

func (t *Service) sortedIDs() []uint32 {
	ids := make([]uint32, 0, len(t.tabs))
	for id := range t.tabs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// sendTabs tells the viewer the terminals now (terminal_tabs).
func (t *Service) sendTabs() {
	t.mu.Lock()
	v := t.viewer
	t.mu.Unlock()
	if v != nil {
		t.sendTabsTo(v, false)
	}
}

// sendTabsTo sends v the list; resumed says the terminals were there
// before it came (the first list after a viewer comes back).
func (t *Service) sendTabsTo(v *viewer, resumed bool) {
	msg, _ := json.Marshal(struct {
		Type    string    `json:"type"`
		Tabs    []TabInfo `json:"tabs"`
		Max     int       `json:"max"`
		Resumed bool      `json:"resumed,omitempty"`
	}{"terminal_tabs", t.Tabs(), MaxTabs, resumed})
	t.send(v.peer, msg)
}

// HandleControl takes the viewer's terminal messages: terminal_size
// {cols, rows} (every terminal shows at the viewer's size), terminal_open
// {name}, terminal_close {id}, terminal_rename {id, name}.
func (t *Service) HandleControl(peerID, kind string, data []byte) {
	var m struct {
		ID   uint32 `json:"id"`
		Name string `json:"name"`
		Cols int    `json:"cols"`
		Rows int    `json:"rows"`
	}
	if json.Unmarshal(data, &m) != nil {
		return
	}
	t.mu.Lock()
	v := t.viewer
	ok := v != nil && v.peer == peerID
	may := ok && v.control
	t.mu.Unlock()
	if !ok {
		return
	}
	switch kind {
	case "terminal_size":
		if m.Cols < 1 || m.Rows < 1 || m.Cols > maxSize || m.Rows > maxSize {
			return
		}
		t.mu.Lock()
		t.cols, t.rows = uint16(m.Cols), uint16(m.Rows)
		shells := make([]shellIO, 0, len(t.tabs))
		for _, tb := range t.tabs {
			shells = append(shells, tb.sh)
		}
		t.mu.Unlock()
		for _, sh := range shells {
			_ = sh.Resize(uint16(m.Cols), uint16(m.Rows))
		}
	case "terminal_open":
		if !may {
			return
		}
		if _, err := t.open(m.Name); err != nil {
			t.log.Warn("cannot open a terminal", "err", err)
		}
	case "terminal_close":
		if !may {
			return
		}
		t.mu.Lock()
		tb := t.tabs[m.ID]
		delete(t.tabs, m.ID)
		t.mu.Unlock()
		if tb != nil {
			tb.sh.Close()
			t.log.Info("terminal closed", "id", m.ID)
			t.sendTabs()
		}
	case "terminal_rename":
		name := cleanName(m.Name)
		if !may || name == "" {
			return
		}
		t.mu.Lock()
		tb := t.tabs[m.ID]
		if tb != nil {
			tb.name = name
		}
		t.mu.Unlock()
		if tb != nil {
			t.sendTabs()
		}
	}
}

// Close hangs every terminal up (the service stops).
func (t *Service) Close() {
	t.mu.Lock()
	tabs := t.tabs
	t.tabs = make(map[uint32]*tab)
	t.mu.Unlock()
	for _, tb := range tabs {
		tb.sh.Close()
	}
}

func defaultName(id uint32) string {
	if id == 1 {
		return "shell"
	}
	return "shell " + itoa(id)
}

func itoa(n uint32) string {
	if n == 0 {
		return "0"
	}
	var b [10]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// cleanName keeps printable characters on one line, at most maxName.
func cleanName(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if n == maxName {
			break
		}
		if unicode.IsSpace(r) {
			r = ' '
		} else if !unicode.IsPrint(r) {
			continue
		}
		b.WriteRune(r)
		n++
	}
	return strings.TrimSpace(b.String())
}

type tooMany struct{}

func (tooMany) Error() string { return "terminal: too many terminals open" }

var errTooMany error = tooMany{}
