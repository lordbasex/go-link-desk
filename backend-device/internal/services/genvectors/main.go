// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

// Command genvectors writes protocol/vectors/link-proof.json. It is a
// developer tool, run by hand when the link-proof format changes; the
// signatures it bakes in are what every client verifies against.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/lordbasex/go-link-desk/backend-device/pkg/linkauth"
)

const (
	nonce  = "nonce-0123456789abcdef"
	hostFp = "sha-256 EF:A2:35:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:EE"
	viewFp = "sha-256 10:20:30:40:50:60:70:80:90:A0:B0:C0:D0:E0:F0:01:02:03:04:05:06:07:08:09:0A:0B:0C:0D:0E:0F:10:11"
)

func main() {
	host, err := linkauth.NewKey()
	must(err)
	link, err := linkauth.NewKey()
	must(err)
	hostSig, err := host.Sign(linkauth.HostProofMessage(nonce, hostFp, viewFp))
	must(err)
	linkSig, err := link.Sign(linkauth.LinkProofMessage(nonce, hostFp, viewFp))
	must(err)
	unlinkSig, err := host.Sign(linkauth.HostUnlinkMessage(nonce, hostFp, viewFp))
	must(err)

	out := map[string]any{
		"description": "Link authentication for go-link-desk unattended (reach) access. " +
			"Each host has an ECDSA P-256 device key; each browser link has its own ECDSA P-256 key. " +
			"A public key is the 65-byte uncompressed point (0x04||X||Y), a signature is the 64-byte P1363 pair (R||S), both base64url without padding. " +
			"ECDSA signs the SHA-256 of the message. The messages are: host proof = \"go-link-desk host proof v1\\n\"+nonce+\"\\n\"+host_fingerprint+\"\\n\"+viewer_fingerprint, " +
			"link proof = \"go-link-desk link proof v1\\n\"+..., host unlink = \"go-link-desk host unlink v1\\n\"+.... " +
			"ECDSA signatures are randomized, so a client reproduces the messages and VERIFIES the signatures here against the given public keys (it cannot reproduce the exact bytes). " +
			"A fingerprint is the first sha-256 a=fingerprint line of that side's SDP, \"sha-256 \"+UPPERHEX.",
		"host_key":           host.Private(),
		"host_pub":           host.Public(),
		"link_key":           link.Private(),
		"link_pub":           link.Public(),
		"nonce":              nonce,
		"host_fingerprint":   hostFp,
		"viewer_fingerprint": viewFp,
		"host_proof_sig":     hostSig,
		"link_proof_sig":     linkSig,
		"host_unlink_sig":    unlinkSig,
		"fingerprint_cases": []map[string]string{
			{
				"sdp":         "v=0\r\na=group:BUNDLE 0\r\na=fingerprint:sha-512 AA:BB\r\na=fingerprint:SHA-256 ab:cd:ef\r\n",
				"fingerprint": "sha-256 AB:CD:EF",
			},
			{
				"sdp":         "v=0\r\na=group:BUNDLE 0\r\n",
				"fingerprint": "",
			},
		},
	}
	b, err := json.MarshalIndent(out, "", "  ")
	must(err)
	fmt.Println(string(b))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
