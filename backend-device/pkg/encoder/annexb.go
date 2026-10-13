// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package encoder

import "bytes"

// params keeps the last SPS and PPS. withParams makes every keyframe
// start with them, which is how a viewer starts the picture: the encoder
// sends them with the first one, and they are kept for those that come
// without.
type params struct{ params []byte }

func (v *params) withParams(au []byte) ([]byte, bool) {
	var params []byte
	key, hasSPS := false, false
	for _, nal := range nalUnits(au) {
		switch nal[0] & 0x1f {
		case 5:
			key = true
		case 7:
			hasSPS = true
			params = append(params, startCode...)
			params = append(params, nal...)
		case 8:
			params = append(params, startCode...)
			params = append(params, nal...)
		}
	}
	if hasSPS {
		v.params = params
	}
	if key && !hasSPS && v.params != nil {
		au = append(append([]byte(nil), v.params...), au...)
	}
	return au, key
}

var startCode = []byte{0, 0, 0, 1}

// nalUnits splits Annex B into its NAL units (without start codes).
func nalUnits(b []byte) [][]byte {
	var out [][]byte
	start := -1
	for i := 0; i+2 < len(b); {
		if b[i] == 0 && b[i+1] == 0 && b[i+2] == 1 {
			if start >= 0 {
				out = append(out, bytes.TrimRight(b[start:i], "\x00"))
			}
			i += 3
			start = i
			continue
		}
		i++
	}
	if start >= 0 && start < len(b) {
		out = append(out, b[start:])
	}
	n := 0
	for _, nal := range out {
		if len(nal) > 0 {
			out[n] = nal
			n++
		}
	}
	return out[:n]
}
