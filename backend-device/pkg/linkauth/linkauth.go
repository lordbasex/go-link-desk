// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Package linkauth holds the asymmetric primitives that authenticate an
// unattended (reach) link in both directions.
//
// Each host keeps one long-lived ECDSA P-256 device key; each browser keeps
// one non-extractable ECDSA P-256 key per link. When a browser links, it
// sends its public key (the host stores it in place of a token hash) and
// pins the host's public key. On every later reach the host signs the
// connection's nonce and DTLS fingerprints with its device key, the browser
// verifies that against the pinned key, then the browser signs the same with
// its link key and the host verifies it. Nothing replayable is ever stored,
// and a message the host signs (a refusal of a revoked link included) cannot
// be forged by a signaling server that never held the device key.
//
// The wire encodings are the ones every client can produce and read:
// a public key is the 65-byte uncompressed X9.63 point (0x04 ‖ X ‖ Y), a
// signature is the 64-byte IEEE P1363 pair (R ‖ S), both base64url without
// padding. ECDSA signs the SHA-256 of the message (ES256). The private
// device key is the 32-byte big-endian scalar, base64url, kept in device.json.
package linkauth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"math/big"
)

// Message prefixes. The version guards against a future format passing for
// this one, and the "host"/"link" split keeps a host proof from ever reading
// as a browser proof. A message is the prefix, then the nonce, the host
// fingerprint and the viewer fingerprint, each on its own line.
const (
	hostProofPrefix  = "go-link-desk host proof v1\n"
	linkProofPrefix  = "go-link-desk link proof v1\n"
	hostUnlinkPrefix = "go-link-desk host unlink v1\n"
)

var b64 = base64.RawURLEncoding

// HostProofMessage is what the host signs to prove itself on a reach.
func HostProofMessage(nonce, hostFingerprint, viewerFingerprint string) []byte {
	return message(hostProofPrefix, nonce, hostFingerprint, viewerFingerprint)
}

// LinkProofMessage is what the browser signs to prove its link on a reach.
func LinkProofMessage(nonce, hostFingerprint, viewerFingerprint string) []byte {
	return message(linkProofPrefix, nonce, hostFingerprint, viewerFingerprint)
}

// HostUnlinkMessage is what the host signs to tell a browser its link is gone
// (revoked or unknown). Binding it to the connection's nonce and fingerprints
// keeps a signaling server from replaying it or forging it.
func HostUnlinkMessage(nonce, hostFingerprint, viewerFingerprint string) []byte {
	return message(hostUnlinkPrefix, nonce, hostFingerprint, viewerFingerprint)
}

func message(prefix, nonce, hostFingerprint, viewerFingerprint string) []byte {
	return []byte(prefix + nonce + "\n" + hostFingerprint + "\n" + viewerFingerprint)
}

// Key is a host's long-lived device signing key. The stored and public
// forms are rendered once, when the key is made or read, so the accessors
// never fail.
type Key struct {
	priv    *ecdsa.PrivateKey
	private string
	public  string
}

// NewKey makes a fresh device key.
func NewKey() (Key, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return Key{}, err
	}
	return fromPrivate(priv)
}

// ParseKey reads a device key from its stored 32-byte scalar (base64url).
func ParseKey(s string) (Key, error) {
	d, err := b64.DecodeString(s)
	if err != nil {
		return Key{}, err
	}
	if len(d) != 32 {
		return Key{}, errors.New("linkauth: a device key is 32 bytes")
	}
	priv, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), d)
	if err != nil {
		return Key{}, err
	}
	return fromPrivate(priv)
}

func fromPrivate(priv *ecdsa.PrivateKey) (Key, error) {
	privBytes, err := priv.Bytes()
	if err != nil {
		return Key{}, err
	}
	pubBytes, err := priv.PublicKey.Bytes()
	if err != nil {
		return Key{}, err
	}
	return Key{priv: priv, private: b64.EncodeToString(privBytes), public: b64.EncodeToString(pubBytes)}, nil
}

// Private is the stored form of the device key (base64url of the 32-byte
// scalar). It never leaves device.json.
func (h Key) Private() string { return h.private }

// Public is the device's public key, the 65-byte X9.63 point, base64url. The
// browser pins this when it links.
func (h Key) Public() string { return h.public }

// Sign signs a message (its SHA-256) and returns the 64-byte R‖S, base64url.
func (h Key) Sign(msg []byte) (string, error) {
	return sign(h.priv, msg)
}

// PublicKey is a parsed public key (a host's, or a browser's link key).
type PublicKey struct {
	pub *ecdsa.PublicKey
}

// ParsePublic reads a public key from its 65-byte X9.63 point (base64url). It
// checks the point is on P-256.
func ParsePublic(s string) (PublicKey, error) {
	raw, err := b64.DecodeString(s)
	if err != nil {
		return PublicKey{}, err
	}
	if len(raw) != 65 || raw[0] != 4 {
		return PublicKey{}, errors.New("linkauth: a public key is a 65-byte uncompressed point")
	}
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), raw)
	if err != nil {
		return PublicKey{}, err
	}
	return PublicKey{pub: pub}, nil
}

// Verify reports whether sig (64-byte R‖S, base64url) is this key's signature
// over msg. It never panics on malformed input; it returns false.
func (p PublicKey) Verify(msg []byte, sig string) bool {
	if p.pub == nil {
		return false
	}
	raw, err := b64.DecodeString(sig)
	if err != nil || len(raw) != 64 {
		return false
	}
	r := new(big.Int).SetBytes(raw[:32])
	s := new(big.Int).SetBytes(raw[32:])
	return ecdsa.Verify(p.pub, sum(msg), r, s)
}

func sign(priv *ecdsa.PrivateKey, msg []byte) (string, error) {
	r, s, err := ecdsa.Sign(rand.Reader, priv, sum(msg))
	if err != nil {
		return "", err
	}
	out := make([]byte, 64)
	copy(out[:32], bigTo32(r))
	copy(out[32:], bigTo32(s))
	return b64.EncodeToString(out), nil
}

func sum(msg []byte) []byte {
	h := sha256.Sum256(msg)
	return h[:]
}

// bigTo32 is a 32-byte big-endian render, left-padded with zeros.
func bigTo32(n *big.Int) []byte {
	out := make([]byte, 32)
	n.FillBytes(out)
	return out
}
