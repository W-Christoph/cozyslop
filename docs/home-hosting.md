# Home hosting with a relay (planned)

A planned feature; nothing here is built yet. It lets someone run the rooms
on a small box at home that provides only the computing. All traffic goes
through a VPS: viewers, and the websites the room opens, see the VPS's
address, as if the rooms ran there. The home network and IP address stay
hidden, and the home upload speed does not limit the number of viewers.

The default deployment runs on one machine, and neko sends every viewer
their own copy of the stream. At home that means port forwarding, a visible home IP,
and an upload of viewers × bitrate (10 viewers at 2.5 Mbit/s = 25 Mbit/s).

## Overview

Two machines. The VPS is the only thing the internet sees; the home box does
the computing and only dials out.

```
viewers  ──HTTPS + WebRTC──▶ hub (VPS)  ◀══ WireGuard, home dials out ══ node (home box)
websites ◀─room's browsing── Go server                                   Docker
                             media relay                                 neko room containers
                             NAT for the rooms                           video encoding
                             SQLite, Let's Encrypt
```

- **hub**: the existing Go server (accounts, chat, permissions, UI, HTTPS,
  SQLite) plus a media relay inside it. Low CPU, high bandwidth.
- **node**: Docker with the unmodified neko room containers. No open ports,
  no port forwarding; works behind carrier NAT.
- **tunnel**: WireGuard. The node connects to the hub. The server reaches
  each room's neko API through it (rooms are already URLs in
  `COZYCAST_ROOMS` or database registrations), and the relay pulls media
  through it. It is also the room containers' only route to the internet:
  the hub does the NAT.

The home line then carries one stream per watched room up, whatever the
number of viewers, and what the room's browser downloads. Viewers and
websites only ever see the hub's address.

Single-machine deployments stay as they are: the relay is an option
(`COZYCAST_RELAY` or similar), and without it browsers talk to neko directly
as today.

## The media relay

Kurento and later LiveKit had this role in the old CozyCast: the worker sent
one stream, the media server made the copies. The rewrite dropped it because
neko serves viewers itself. This brings back the role only, as a forwarder
inside the Go server, not as another service.

- Built with [Pion](https://github.com/pion/webrtc), the WebRTC library neko
  uses. A new dependency for the server.
- For each watched room the relay joins neko as one member with watch rights
  only and receives H.264 video and Opus audio. It connects when the first
  viewer arrives and leaves with the last, so an idle room still encodes
  nothing.
- It forwards the RTP packets to every viewer's own connection. No decoding
  or encoding: all viewers of a room already watch the same pipeline
  (`neko.PinStream`).
- Signaling: the server already carries each tab's neko WebSocket and
  rewrites `signal/*` messages (`server/internal/neko/signal.go`). With the
  relay on, it answers `signal/request`, `signal/answer` and
  `signal/candidate` itself instead of passing them to neko. Each tab keeps
  its own neko member for rights and the remote, as today.
- Keyframes: a new viewer needs one. The relay asks neko for a keyframe
  (PLI) when a viewer joins; the 2-second keyframe interval is the fallback.
- Lost packets: the relay answers viewers' retransmission requests (NACK)
  from a short buffer, so one viewer's bad connection costs the home line
  nothing.
- One media port (UDP and TCP) on the hub for all rooms, instead of one per
  room.
- neko's `NEKO_WEBRTC_NAT1TO1` becomes the node's tunnel address. Its media
  port is not published at home.

### Input

The web client sends mouse and keyboard over a WebRTC data channel to neko
(`web/src/neko/client.ts`, `ondatachannel`). With the relay, tabs have no
peer connection to neko, so input needs another path. Two options:

1. Send input over the neko WebSocket, which the server already proxies. The
   client already sends `control/move` that way in one place. To check
   first: that neko accepts every input event over the WebSocket (keys,
   buttons, scroll). Simplest; costs a little latency under packet loss
   (TCP).
2. The remote holder keeps a direct peer connection to neko through the hub
   (a UDP forward of the room's media port), only for the data channel. More
   moving parts.

Start with 1.

## Room management

Today containers are started separately (usually services in `compose.yaml`);
the admin API can register room connections without a server restart.
Restarting a configured room from the UI needs the Docker socket on the server (`architecture.md`, "Room WebSocket").
With two machines the socket is on the node.

A small agent on the node dials out to the hub and accepts only: create,
start, stop, restart and delete a room container, and report its state. The
hub never gets the Docker socket, so a compromised VPS can manage rooms but
not take over the home box. Creating rooms from the admin page
(`ideas.md`) builds on this. Several nodes can feed one hub.

## Security

- The node's firewall lets the tunnel reach only the room containers' neko
  port and media port.
- The tunnel is the room containers' only route out, DNS included. The
  room's browser cannot open the home router or other devices (`ideas.md`,
  "Isolate the room from the LAN"), and with the tunnel down the room has
  no network: it never falls back to the home connection. Needed before
  anyone else gets the remote.
- Websites see the hub's address, never the home IP. The costs: streaming
  sites treat datacenter addresses worse (blocks, sign-in prompts), and
  abuse complaints about what a room opens go to the VPS account.
- The neko admin token crosses the tunnel, never the open internet.
- Client IPs for bans and rate limits are unaffected: browsers connect to
  the hub directly.

## Hardware (node)

The target is an **Orange Pi 5** (RK3588S, eight cores), because one is
already there.

- It was too weak to run the original CozyCast (the owner's experience).
  Whether it carries one neko room is the first thing to measure.
- Software x264 for one 720p room probably works; not measured. The
  pipelines in `worker/entrypoint.sh` are x264 only today.
- Hardware encoding: the chip can encode in hardware, VP8 included, in
  theory. The drivers used to be poor: ffmpeg found the hardware decoder
  but not the encoder. Their state today is unknown. neko encodes with
  GStreamer, so what counts is a working GStreamer encoder; that is
  expected to need Rockchip's vendor kernel and a custom image with
  Rockchip's GStreamer plugin. Not verified. A VP8 path also needs VP8
  pipelines in the worker, which sets H.264 today.
- No official Widevine on ARM Linux, so DRM sites likely do not play.
- Not verified: that `worker/Dockerfile` builds on ARM.
- Hardware encoding moves the encoder off the CPU. Firefox in the room still
  decodes and draws on the CPU.
- Not measured: how many rooms the box carries. The benchmark script in
  `ideas.md` comes first.

If the Orange Pi 5 turns out too weak:

- **Intel N100/N150 mini PC**, 8–16 GB RAM, SSD. Quick Sync is supported by
  neko's published Intel (VA-API) image. Stock kernel, x86 Firefox with DRM
  (Widevine).
- Not a **Raspberry Pi 5**: no hardware H.264 encoder, and neko has no image
  for its GPU. CPU only.

## VPS (hub)

Needs: 1–2 vCPUs, a public IPv4 address, unrestricted UDP, and a large
traffic allowance. One viewer at 2.5 Mbit/s is about 1.1 GB per hour; 10
viewers for four hours a day are about 1.4 TB a month. What the room's
browser downloads passes through the hub as well, in from the website and
out to the node: a 5 Mbit/s source for four hours a day adds about 0.5 TB a
month, counting both directions. Pick the datacenter
closest to the node: everything goes node → hub → viewer, and the remote
holder feels the detour.

Prices as of 2026-10-04, from review sites, for Europe. Check the provider's
own page before ordering.

| Plan | Per month | Size | Traffic |
|---|---|---|---|
| netcup VPS 500 G12 | ~€5.91 incl. VAT | 2 vCores, 4 GB | no cap; 200 Mbit/s after 2 TB in 24 h |
| OVHcloud VPS-1 | ~€6.49 | 4 cores, 8 GB | unmetered, up to 400 Mbit/s |
| Hetzner CX23 / CAX11 | €5.99 | 2 vCPUs, 4 GB | 20 TB, then €1/TB |

- First choice: netcup VPS 500 G12. Alternative: OVHcloud VPS-1.
- Hetzner's cost-optimized plans have been sold out since September 2026;
  fine if one is in stock.
- Avoid plans with 1–2 TB of traffic, and Oracle's free tier (halved in June
  2026, idle instances are reclaimed).

## Cost

| Item | Cost |
|---|---|
| Orange Pi 5 | already there |
| Mini PC, only if the Orange Pi 5 is too weak | ~€150–250 once (not checked against current prices) |
| VPS | ~€6 a month |
| Domain, for automatic HTTPS | ~€10–15 a year |
| Power, ~10 W | ~€2–3 a month |

For comparison: a 4-core VPS at roughly €8–15 a month runs everything in
software with no home box. The home box pays off with several rooms or
higher quality. It does not help with sites that block datacenter
addresses: the room browses from the VPS either way.

## Build order

Each step works on its own.

1. **Measure the Orange Pi 5.** Build the room image on ARM, run one room
   with a video playing and software x264, note CPU use per stream setting.
   Decides the hardware before anything else is built. Done when there is a
   stream setting the box plays smoothly, or it is ruled out.
2. **Split deployment.** `compose.hub.yaml` and `compose.node.yaml`, a
   WireGuard setup guide, `COZYCAST_ROOMS` pointing at tunnel addresses.
   Media still goes viewer → hub → node by port forwarding on the hub, so
   upload is still viewers × bitrate. Done when a room at home plays for a
   viewer who sees only the hub's address.
3. **Tunnel-only networking** on the node: the room containers' default
   route is the tunnel, the hub does the NAT. Done when a website opened in
   the room sees the hub's address, the room's browser cannot open the
   router, and the room has no network with the tunnel down.
4. **Input over the WebSocket.** Useful by itself as a fallback when the data
   channel fails. Done when the remote works with the data channel disabled.
5. **The relay.** The main work, roughly a week. Cannot be tested in the
   development sandbox (no UDP); needs a real hub and node. Done when home
   upload stays at one stream with several viewers, a new viewer gets a
   picture within a second, and a viewer with packet loss does not disturb
   the others.
6. **Hardware encoding**, only if step 1 shows software encoding is too
   slow. On the Orange Pi 5: vendor kernel, a worker variant with
   Rockchip's GStreamer plugin, H.264 or VP8 pipelines in
   `worker/entrypoint.sh`. On an Intel mini PC: a worker variant on neko's
   Intel image, `/dev/dri` passed into the container, VA-API pipelines.
   Done when a watched room uses the hardware encoder and CPU use drops.
7. **Node agent** and room creation from the admin page.

## Alternatives considered

- **A neko plugin** instead of the outside server: a plugin lives in one
  room and cannot hold accounts or cross-room state; neko's docs call
  external plugins experimental and tie them to the exact neko build.
- **TURN (coturn)**: hides the home IP but forwards each viewer's copy; the
  home upload stays viewers × bitrate.
- **LiveKit or Kurento again** as the relay: they do far more than forward
  one stream, and bring back the services the rewrite removed (ingress,
  Redis, a large UDP port range). Worth reconsidering if the forwarder's
  loss handling turns out to be hard to get right.
- **Mesh VPN** (Tailscale, ZeroTier): no VPS, nothing exposed, but every
  viewer installs a client, and upload is still viewers × bitrate. Fine for
  a few friends.
- **Lower bitrate or stream size**: a room setting today; shrinks the upload
  without removing the limit.
- **Rooms on the VPS**: no home box at all; see Cost.
