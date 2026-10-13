# Connections: direct or through the relay

go-link Desk sends the picture, sound and control **directly** between the two devices whenever the network allows it, and through a **relay** (TURN) when it does not. Both are encrypted end to end (DTLS and SRTP); the relay passes on packets it cannot read. Which one a session uses depends on the routers between each device and the internet, not on go-link Desk. This page explains why, what the connection test at [desk.go-link.org/network](https://desk.go-link.org/network) measures, and how to get a direct connection.

## How two devices find each other

When a session starts, each side gathers **candidates**, the addresses the other side could reach it at (ICE, RFC 8445):

| Candidate | Where it comes from | Reaches the other side when |
|---|---|---|
| `host` | The device's own address on its network (`192.168.0.12`). Browsers hide it behind a random `….local` name. | Both are on the same network. |
| `srflx` (server reflexive) | A STUN server answers "this is the address and port I see you at": the router's public address and the port it opened. | Each router lets the other's packets in through the port it opened (hole punching). |
| `relay` | A TURN server lends an address of its own and passes the packets on. | Always: both sides reach the relay. |

The two sides exchange their candidates through signalhub and try every pair; the first that answers wins, direct pairs first. The session's (i) panel says which one it uses (**Direct** or **Relay server**).

## Why a connection ends up on the relay

- **A router that changes the port per destination** ("symmetric NAT", or address-dependent mapping, RFC 4787). The port STUN saw is not the port the router opens toward the other device, so the other side knocks on the wrong door. The test notices it by asking two STUN servers: the same port from both means the router keeps it.
- **Routers in cascade (double NAT).** The provider's modem-router and the customer's own router both translate addresses. Hole punching often still works when both keep the port, but each extra router is one more chance of a symmetric mapping or a short timeout.
- **CGNAT.** The provider shares one public address among many customers (`100.64.0.0/10`, RFC 6598): a NAT the customer cannot configure. Mobile data is almost always CGNAT.
- **UDP blocked.** Office, hotel and university networks often let only TCP out. WebRTC's direct paths are UDP, so only the relay over TCP is left.

A direct connection needs only one of the two sides to be easy: a device on a simple home network can usually reach a device behind a symmetric NAT. When both are difficult, the relay carries the session.

## The connection test

`desk.go-link.org/network` runs, in the browser, the discovery a session makes, without anyone on the other side (`frontend/apps/web/src/network/probe.ts`):

1. **Signaling server:** opens signalhub, takes the STUN and TURN servers from `hello` and times three `rooms_list` round trips (an answer of its own; no room is touched).
2. **Your local network** and **routers in cascade:** only from the apps (below); browsers hide the local address.
3. **UDP** and **public address:** one connection gathers candidates against signalhub's STUN and Google's public STUN (`stun.l.google.com:19302`). An `srflx` answer means UDP leaves the network and gives the public address.
4. **Router's port:** the browser drops a second `srflx` equal to the first, so one public `address:port` from two servers means the router keeps the port; two means it changes it (symmetric).
5. **Relay:** two connections in the page, both forced through the TURN of `hello` (`iceTransportPolicy: "relay"`), connected to each other. Half of the data channel's echo time is this network's round trip to the relay. A few bytes: no bandwidth test. The candidate pair says whether it reached the relay over UDP, TCP or TLS.

**The verdict:**

| Verdict | When |
|---|---|
| **Direct** | UDP leaves and the router keeps the port, with one router (or only IPv6, which has no NAT, answered). |
| **Depends** | The router changes the port, or the apps found routers in cascade or CGNAT: direct only if the other side's network is simple. |
| **Relay** | No UDP, or a symmetric NAT behind the provider's CGNAT. |

Nothing is stored. The page's guides (bridge mode, DMZ, UPnP, asking the provider for a public address or IPv6, allowing outgoing UDP) follow the case the test found.

### What the apps add

A browser cannot see the device's local address or ask the router anything. The go-link Desk apps can, and open the page with what they found after `#d=`, a base64url JSON object. The fragment is never sent to a server; the page shows only fields that parse as addresses or known words, and starts the test by itself.

```json
{ "src": "desktop", "lan": "192.168.0.12", "gw": "192.168.0.1", "wan": "192.168.1.2", "upnp": true, "pmp": false, "net": "ethernet" }
```

| Field | Meaning |
|---|---|
| `src` | `desktop`, `ios` or `android`. Required. |
| `lan` | The device's address on its network. |
| `gw` | Its router (the default gateway). |
| `wan` | The router's own outside address, as the router says over UPnP IGD or NAT-PMP. A private or `100.64.0.0/10` address, or a public one other than what STUN saw, means another router (or the provider's CGNAT) is in front of it. |
| `upnp`, `pmp` | Whether the router answers UPnP IGD or NAT-PMP / PCP. |
| `net` | `wifi`, `cellular`, `ethernet` or `other`. Mobile data counts as CGNAT. |

## Getting a direct connection

- **Two routers:** put the provider's modem in **bridge** mode so only yours is left; if that is not possible, make your router's outside address the modem's **DMZ** host. Turn on **UPnP** in your router.
- **One router:** turn on **UPnP** or **NAT-PMP**. Two devices on the same network always connect directly.
- **CGNAT:** ask the provider for a public IPv4 address, or turn on **IPv6** in the router.
- **UDP blocked:** ask the network's administrators to allow outgoing UDP; at least UDP 3478 to signal.go-link.org lowers the relay's latency.

A fixed UDP port in the computer app, to forward in each router by hand, is planned.

## Relays

The relay is coturn next to signalhub (`signal.go-link.org`, in Virginia, US). A relay closer to both devices lowers the latency of relayed sessions; one in São Paulo, Brazil, is on the way. go-link Desk is free software without ads or accounts: anyone with a virtual machine and a good connection in another region can offer it as a relay, by opening an issue in this repository.

A relay must share signalhub's `TURN_SECRET`, since signalhub hands out short-lived credentials signed with it (the TURN REST scheme). That is fine for relays run by the project; a relay run by someone else would need a secret of its own, which signalhub does not support yet (see [signalhub-proposals.md](signalhub-proposals.md)).

## The map

The relays' map on the test page is generated by `scripts/relay-map.py` from Natural Earth's 1:50m countries (public domain), into `frontend/apps/web/src/network/americas.gen.ts`. Add a relay's coordinates to `RELAYS` in `RelayMap.tsx`.
