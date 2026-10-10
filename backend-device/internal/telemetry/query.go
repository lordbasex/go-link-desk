// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package telemetry

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strings"
	"time"
)

// Run is one time a room was on. Ended is zero while it still runs.
type Run struct {
	ID      string    `json:"id,omitempty"` // the history's game id
	Started time.Time `json:"started"`
	Ended   time.Time `json:"ended,omitzero"`
	Game    string    `json:"game"`
}

// PeerInfo is one participant of a room: the name it used last and when
// it was first and last seen.
type PeerInfo struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	First time.Time `json:"first"`
	Last  time.Time `json:"last"`
}

// RoomInfo is one room with telemetry.
type RoomInfo struct {
	Room    string    `json:"room"`
	First   time.Time `json:"first"`
	Last    time.Time `json:"last"`
	Runs    int       `json:"runs"`
	Samples int       `json:"samples"`
	Events  int       `json:"events"`
}

// Event is one entry of a room's log.
type Event struct {
	At    time.Time       `json:"at"`
	Level Level           `json:"level"`
	Kind  string          `json:"kind"`
	Peer  string          `json:"peer,omitempty"`
	Msg   string          `json:"msg"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// Series is one metric over time in buckets of Step: the average and the
// highest value of each bucket, NaN where there was no sample (JSON null).
type Series struct {
	Peer   string     `json:"peer,omitempty"`
	Kind   string     `json:"kind"`
	Metric string     `json:"metric"`
	Avg    []Optional `json:"avg"`
	Max    []Optional `json:"max"`
}

// Optional is a number or nothing (null in JSON).
type Optional float64

// MarshalJSON writes null for NaN, else the number rounded to 0.1.
func (o Optional) MarshalJSON() ([]byte, error) {
	f := float64(o)
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return []byte("null"), nil
	}
	return json.Marshal(math.Round(f*10) / 10)
}

func ms(t time.Time) int64 { return t.UnixMilli() }

func fromMs(v int64) time.Time { return time.UnixMilli(v) }

// Runs lists the times a room was on, oldest first.
func (s *Store) Runs(room string) ([]Run, error) {
	rows, cancel, err := s.query(`SELECT id, started, ended, game FROM runs WHERE room = ? ORDER BY started`, room)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer rows.Close()
	var out []Run
	for rows.Next() {
		var started int64
		var ended sql.NullInt64
		var r Run
		if err := rows.Scan(&r.ID, &started, &ended, &r.Game); err != nil {
			return nil, err
		}
		r.Started = fromMs(started)
		if ended.Valid {
			r.Ended = fromMs(ended.Int64)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Peers lists a room's participants, first seen first.
func (s *Store) Peers(room string) ([]PeerInfo, error) {
	rows, cancel, err := s.query(`SELECT peer, name, first, last FROM peers WHERE room = ? ORDER BY first`, room)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer rows.Close()
	var out []PeerInfo
	for rows.Next() {
		var p PeerInfo
		var first, last int64
		if err := rows.Scan(&p.ID, &p.Name, &first, &last); err != nil {
			return nil, err
		}
		p.First, p.Last = fromMs(first), fromMs(last)
		out = append(out, p)
	}
	return out, rows.Err()
}

// FindRun returns the room and the run named id (a history game id, or
// its first characters when they name one run).
func (s *Store) FindRun(id string) (room string, run Run, err error) {
	id = strings.TrimPrefix(strings.TrimSpace(id), "#")
	if id == "" {
		return "", Run{}, ErrNoRun
	}
	rows, cancel, err := s.query(`SELECT room, id, started, ended, game FROM runs WHERE id LIKE ? || '%' LIMIT 2`, id)
	if err != nil {
		return "", Run{}, err
	}
	defer cancel()
	defer rows.Close()
	n := 0
	for rows.Next() {
		var started int64
		var ended sql.NullInt64
		if err := rows.Scan(&room, &run.ID, &started, &ended, &run.Game); err != nil {
			return "", Run{}, err
		}
		run.Started = fromMs(started)
		if ended.Valid {
			run.Ended = fromMs(ended.Int64)
		}
		n++
	}
	if n != 1 {
		return "", Run{}, ErrNoRun
	}
	return room, run, rows.Err()
}

// ErrNoRun means no single run has that id.
var ErrNoRun = errors.New("telemetry: no game with that id")

// Rooms lists every room with telemetry.
func (s *Store) Rooms() ([]RoomInfo, error) {
	rows, cancel, err := s.query(`SELECT room, min(started), max(coalesce(ended, started)), count(*) FROM runs GROUP BY room ORDER BY min(started)`)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer rows.Close()
	var out []RoomInfo
	for rows.Next() {
		var r RoomInfo
		var first, last int64
		if err := rows.Scan(&r.Room, &first, &last, &r.Runs); err != nil {
			return nil, err
		}
		r.First, r.Last = fromMs(first), fromMs(last)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		_ = s.db.QueryRow(`SELECT count(*) FROM samples WHERE room = ?`, out[i].Room).Scan(&out[i].Samples)
		_ = s.db.QueryRow(`SELECT count(*) FROM events WHERE room = ?`, out[i].Room).Scan(&out[i].Events)
	}
	return out, nil
}

// Events returns a room's log between from and to, oldest first, at most
// limit entries (0: all) of at least level min ("" or info: all).
func (s *Store) Events(room string, from, to time.Time, limit int, min Level) ([]Event, error) {
	q := `SELECT ts, level, kind, peer, msg, data FROM events WHERE room = ? AND ts >= ? AND ts <= ?`
	args := []any{room, ms(from), ms(to)}
	switch min {
	case Warn:
		q += ` AND level IN ('warn', 'error')`
	case Error:
		q += ` AND level = 'error'`
	}
	q += ` ORDER BY ts`
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, cancel, err := s.query(q, args...)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var ts int64
		var level, data string
		if err := rows.Scan(&ts, &level, &e.Kind, &e.Peer, &e.Msg, &data); err != nil {
			return nil, err
		}
		e.At, e.Level = fromMs(ts), Level(level)
		if data != "{}" {
			e.Data = json.RawMessage(data)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// sample is one stored sample, decoded.
type sample struct {
	ts         int64
	peer, kind string
	m          Metrics
}

// samples calls fn for each sample of a room between from and to, oldest
// first.
func (s *Store) samples(room string, from, to time.Time, fn func(sample)) error {
	rows, cancel, err := s.query(`SELECT ts, peer, kind, data FROM samples WHERE room = ? AND ts >= ? AND ts <= ? ORDER BY ts`, room, ms(from), ms(to))
	if err != nil {
		return err
	}
	defer cancel()
	defer rows.Close()
	for rows.Next() {
		var sm sample
		var data string
		if err := rows.Scan(&sm.ts, &sm.peer, &sm.kind, &data); err != nil {
			return err
		}
		if json.Unmarshal([]byte(data), &sm.m) == nil {
			fn(sm)
		}
	}
	return rows.Err()
}

// Series returns the metrics asked for ("kind.metric", e.g. "peer.rtt_ms";
// none: every one) between from and to in buckets of step, one series per
// participant and metric.
func (s *Store) Series(room string, from, to time.Time, step time.Duration, metrics []string) ([]Series, error) {
	if step <= 0 || !to.After(from) {
		return nil, nil
	}
	n := int(to.Sub(from)/step) + 1
	want := map[string]bool{}
	for _, m := range metrics {
		want[m] = true
	}
	type acc struct {
		sum, max []float64
		cnt      []int
	}
	type key struct{ peer, kind, metric string }
	all := map[key]*acc{}
	var order []key
	err := s.samples(room, from, to, func(sm sample) {
		i := int((sm.ts - ms(from)) / step.Milliseconds())
		if i < 0 || i >= n {
			return
		}
		for name, v := range sm.m {
			if len(want) > 0 && !want[sm.kind+"."+name] {
				continue
			}
			k := key{sm.peer, sm.kind, name}
			a := all[k]
			if a == nil {
				a = &acc{sum: make([]float64, n), max: make([]float64, n), cnt: make([]int, n)}
				for j := range a.max {
					a.max[j] = math.Inf(-1)
				}
				all[k] = a
				order = append(order, k)
			}
			a.sum[i] += v
			a.cnt[i]++
			a.max[i] = math.Max(a.max[i], v)
		}
	})
	if err != nil {
		return nil, err
	}
	out := make([]Series, 0, len(order))
	for _, k := range order {
		a := all[k]
		sr := Series{Peer: k.peer, Kind: k.kind, Metric: k.metric, Avg: make([]Optional, n), Max: make([]Optional, n)}
		for i := 0; i < n; i++ {
			if a.cnt[i] == 0 {
				sr.Avg[i], sr.Max[i] = Optional(math.NaN()), Optional(math.NaN())
				continue
			}
			sr.Avg[i], sr.Max[i] = Optional(a.sum[i]/float64(a.cnt[i])), Optional(a.max[i])
		}
		out = append(out, sr)
	}
	return out, nil
}

// Export writes a room's raw telemetry between from and to as JSON lines,
// oldest first: its runs and participants, then every sample and event in
// time order. It is what `device telemetry export` prints.
func (s *Store) Export(w io.Writer, room string, from, to time.Time) error {
	s.Sync()
	bw := bufio.NewWriter(w)
	enc := json.NewEncoder(bw)
	runs, err := s.Runs(room)
	if err != nil {
		return err
	}
	for _, r := range runs {
		if err := enc.Encode(map[string]any{"type": "run", "id": r.ID, "started": r.Started, "ended": r.Ended, "game": r.Game}); err != nil {
			return err
		}
	}
	peers, err := s.Peers(room)
	if err != nil {
		return err
	}
	for _, p := range peers {
		if err := enc.Encode(map[string]any{"type": "peer", "peer": p.ID, "name": p.Name, "first": p.First, "last": p.Last}); err != nil {
			return err
		}
	}
	events, err := s.Events(room, from, to, 0, Info)
	if err != nil {
		return err
	}
	next := 0
	writeEvents := func(until int64) error {
		for ; next < len(events) && ms(events[next].At) <= until; next++ {
			e := events[next]
			line := map[string]any{"type": "event", "at": e.At, "level": e.Level, "kind": e.Kind, "msg": e.Msg}
			if e.Peer != "" {
				line["peer"] = e.Peer
			}
			if len(e.Data) > 0 {
				line["data"] = e.Data
			}
			if err := enc.Encode(line); err != nil {
				return err
			}
		}
		return nil
	}
	var werr error
	err = s.samples(room, from, to, func(sm sample) {
		if werr != nil {
			return
		}
		if werr = writeEvents(sm.ts); werr != nil {
			return
		}
		line := map[string]any{"type": "sample", "at": fromMs(sm.ts), "kind": sm.kind, "m": sm.m}
		if sm.peer != "" {
			line["peer"] = sm.peer
		}
		werr = enc.Encode(line)
	})
	if err == nil {
		err = werr
	}
	if err == nil {
		err = writeEvents(math.MaxInt64)
	}
	if err != nil {
		return err
	}
	return bw.Flush()
}

// shortPeer is a peer id cut for messages (the first 8 characters).
func shortPeer(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return strings.TrimSpace(id)
}
