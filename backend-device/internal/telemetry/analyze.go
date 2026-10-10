// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package telemetry

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

// Verdicts of an incident: where the freeze most likely came from.
const (
	// The host's computer stopped producing frames: the game or the encoder.
	VerdictDevice = "device"
	// Everyone froze at once and lost packets: the host's internet (upload).
	VerdictHostNetwork = "host_network"
	// Everyone froze at once without lost packets: the host's computer or
	// its network, not the players'.
	VerdictHost = "host"
	// Only some players froze: their own internet or computer.
	VerdictGuest = "guest"
	// One player in the room and lost packets: the host's or that player's
	// internet, it cannot tell which.
	VerdictNetwork = "network"
	// The player's browser froze without lost packets (a busy computer or
	// a hidden tab).
	VerdictPlayer = "player"
)

// Incident is a moment the picture stopped for somebody.
type Incident struct {
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Verdict string    `json:"verdict"`
	// Why says, in English, what the data shows (for the log and for
	// whoever reads the report).
	Why string `json:"why"`
	// Peers that froze; empty when it was the device's frames.
	Peers []string `json:"peers,omitempty"`
	// DeviceGapMs is the longest pause between the game's frames.
	DeviceGapMs float64 `json:"device_gap_ms,omitempty"`
	// Lost is the highest packet loss (%) each affected or present
	// participant reported around it.
	Lost map[string]float64 `json:"lost,omitempty"`
	// Voice is true when the players' voice kept flowing through the
	// device during it (so the network between them and the host was up).
	Voice bool `json:"voice,omitempty"`
}

// freezeMinMs is the shortest stop that counts: 250 ms is about 15 frames.
const freezeMinMs = 250

type signal struct {
	start, end int64
	peer       string // "" for the device
	gap        float64
}

// Incidents finds the freezes of a room between from and to and says
// where each most likely came from.
func (s *Store) Incidents(room string, from, to time.Time) ([]Incident, error) {
	s.Sync()
	var sigs []signal
	events, err := s.Events(room, from, to, 0, Info)
	if err != nil {
		return nil, err
	}
	for _, e := range events {
		if e.Kind != "frame_gap" {
			continue
		}
		var d struct {
			GapMs float64 `json:"gap_ms"`
		}
		_ = json.Unmarshal(e.Data, &d)
		if d.GapMs >= freezeMinMs {
			end := ms(e.At)
			sigs = append(sigs, signal{start: end - int64(d.GapMs), end: end, gap: d.GapMs})
		}
	}
	// What was around each moment: who was there, their loss and voice.
	type around struct {
		ts   int64
		peer string
		kind string
		m    Metrics
	}
	var near []around
	err = s.samples(room, from, to, func(sm sample) {
		if sm.kind == "client" {
			if f := sm.m["freeze_ms"]; f >= freezeMinMs {
				sigs = append(sigs, signal{start: sm.ts - int64(f) - 2000, end: sm.ts, peer: sm.peer})
			}
		}
		if sm.peer != "" {
			near = append(near, around{sm.ts, sm.peer, sm.kind, sm.m})
		}
	})
	if err != nil {
		return nil, err
	}
	if len(sigs) == 0 {
		return nil, nil
	}
	sort.Slice(sigs, func(i, j int) bool { return sigs[i].start < sigs[j].start })

	// Signals that overlap (or are less than 1.5 s apart) are one incident.
	var groups [][]signal
	for _, sg := range sigs {
		if n := len(groups); n > 0 {
			last := groups[n-1]
			end := last[0].end
			for _, x := range last {
				end = max(end, x.end)
			}
			if sg.start <= end+1500 {
				groups[n-1] = append(last, sg)
				continue
			}
		}
		groups = append(groups, []signal{sg})
	}

	out := make([]Incident, 0, len(groups))
	for _, g := range groups {
		inc := Incident{Lost: map[string]float64{}}
		start, end := g[0].start, g[0].end
		affected := map[string]bool{}
		for _, x := range g {
			start, end = min(start, x.start), max(end, x.end)
			if x.peer == "" {
				inc.DeviceGapMs = max(inc.DeviceGapMs, x.gap)
			} else {
				affected[x.peer] = true
			}
		}
		inc.Start, inc.End = fromMs(start), fromMs(end)
		present := map[string]bool{}
		voice := false
		for _, a := range near {
			if a.ts < start-5000 || a.ts > end+5000 {
				continue
			}
			present[a.peer] = true
			lost := max(a.m["video_loss_pct"], a.m["rr_loss_pct"])
			if lost > inc.Lost[a.peer] {
				inc.Lost[a.peer] = lost
			}
			if a.ts >= start && a.ts <= end+2000 && a.m["voice_in_pps"] > 0 {
				voice = true
			}
		}
		inc.Voice = voice
		for p := range affected {
			inc.Peers = append(inc.Peers, p)
		}
		slices.Sort(inc.Peers)
		lossy := 0
		for p := range present {
			if inc.Lost[p] >= 2 {
				lossy++
			}
		}
		for p, v := range inc.Lost {
			if v == 0 && !affected[p] {
				delete(inc.Lost, p)
			}
		}
		names := strings.Join(shortAll(inc.Peers), ", ")
		switch {
		case inc.DeviceGapMs > 0:
			inc.Verdict = VerdictDevice
			inc.Why = fmt.Sprintf("the game sent no frame for %.0f ms on the host's computer (game or encoder), so everyone saw it", inc.DeviceGapMs)
			if voice {
				inc.Why += "; voice kept flowing through the device, so the network was up"
			}
		case len(present) >= 2 && len(affected) >= len(present):
			if lossy*2 >= len(present) {
				inc.Verdict = VerdictHostNetwork
				inc.Why = fmt.Sprintf("everyone froze at once and %d of %d lost packets: the host's internet (upload)", lossy, len(present))
			} else {
				inc.Verdict = VerdictHost
				inc.Why = "everyone froze at once with no lost packets and the game kept sending frames: the host's computer or network"
			}
		case len(present) >= 2:
			inc.Verdict = VerdictGuest
			inc.Why = fmt.Sprintf("only %s froze while the others did not: that player's internet or computer", names)
		case lossy > 0:
			inc.Verdict = VerdictNetwork
			inc.Why = fmt.Sprintf("%s froze and lost packets with nobody else to compare: the host's or that player's internet", names)
		default:
			inc.Verdict = VerdictPlayer
			inc.Why = fmt.Sprintf("%s froze with no lost packets and the game kept sending frames: that player's browser or computer", names)
		}
		out = append(out, inc)
	}
	return out, nil
}

func shortAll(ids []string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = shortPeer(id)
	}
	return out
}
