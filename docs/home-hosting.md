# Home hosting (planned)

A planned feature, built in steps (see Build order; steps 1 and 2 are
done). Someone lends a computer at
home to run a room. The main server (the hub, on a VPS) does everything
else: viewers, and the websites the room opens, only ever see the hub's
address. The home network and IP address stay hidden, and the home needs
no port forwarding.

Adding the room takes no copying of tokens or addresses: the host enters
the hub's address, their terminal shows a code, an admin sees a request
with the same code on the admin page and accepts it. From then on the room
reconnects by itself whenever it or the hub restarts or the network drops.

The default deployment stays as it is: one machine, rooms in
`compose.yaml`. Rooms added by URL and token (README, "Adding a room
without a restart") stay too, for rooms on the hub's own network.

## Overview

```
viewers  ──HTTPS + WebRTC──▶ hub (VPS)  ◀══ WireGuard, home dials out ══ node (home PC)
websites ◀─room's browsing── Go server                                   agent
                             WireGuard (userspace)                       room (neko)
                             media forwarding
                             NAT for the rooms
```

- **hub**: the existing Go server. New: a WireGuard endpoint inside the
  process ([wireguard-go](https://git.zx2c4.com/wireguard-go) with gVisor's
  network stack), so the VPS needs no root, kernel module or extra
  container, only one more UDP port. Built and checked in the sandbox: it
  adds about 6 MB to the server binary.
- **node**: the home PC, with Docker and `compose.node.yaml`: a small
  **agent** container and one room container. The agent runs on the home
  PC, never on the VPS: it dials out, keeps the tunnel up and locks down
  the room's network. The room is the usual
  unmodified neko image. No open ports.
- **tunnel**: WireGuard from the agent to the hub. It is the room's only
  network: the server reaches the room's neko through it, viewers' media
  goes through it, and the room's browsing leaves through the hub.

One node runs one room. Several rooms at one home means several nodes
(each its own compose project); managing them from the admin page is
later work (see Build order).

## Pairing

### What the host does

```bash
COZYCAST_HUB=cozy.example.com docker compose -f compose.node.yaml up -d
docker compose -f compose.node.yaml logs -f agent
```

```
Pairing with cozy.example.com as "christoph-pc".
Code: K7F2-9QXD
Ask an admin to accept this code under Admin > Rooms. Waiting...
Accepted as room "christoph-pc". Tunnel up, room starting.
```

`COZYCAST_ROOM_NAME` proposes a name (default: the machine's host name).
Once paired, the agent remembers everything in its volume; later starts
connect without a code.

### What the admin does

The Rooms tab gets a **Requests** list: proposed name, code, the
requester's IP address and how long ago it arrived. The admin compares the
code with the one the host reads out to them (chat, voice), then:

- **Accept**: as a new room (name editable), or as the new host of an
  existing paired room (keeps its chat, settings and permissions; for a
  reinstalled or replaced PC).
- **Reject**: the agent shows "Rejected" and stops.

Requests expire after 10 minutes. At most 20 are pending, 3 per IP
address, and creating them is rate limited, so strangers cannot flood the
list.

### The code

Both sides compute it from the same three values:

```
code = first 40 bits of SHA-256("cozycast-pair" || hubKey || nodeKey || nonce),
       Crockford base32, shown as XXXX-XXXX
```

`hubKey` and `nodeKey` are the two WireGuard public keys and `nonce` is
random per request, from the hub. If anyone in between swapped a key, the
two codes differ. With a domain (HTTPS) TLS already proves the hub's
identity; without one the code is the only check, so the agent warns
when pairing over plain HTTP:

```
Warning: cozy.example.com is plain HTTP. Compare the code carefully;
anyone between you and the server could otherwise pose as it.
```

### Messages

All over the hub's normal HTTP(S) address. Nothing secret crosses them.

| Request | Body / answer |
|---|---|
| `POST /api/nodes/pair` | `{"name":"christoph-pc","nodeKey":"<base64>"}` → 201 `{"id","secret","hubKey","nonce","tunnelPort","expiresAt"}`. The same key while still pending returns the same request with a new expiry. |
| `GET /api/nodes/pair/{id}`, `Authorization: Bearer <secret>` | Long poll, up to 30 s: `{"status":"pending"}`, `"rejected"`, `"expired"`, or `{"status":"accepted","address":"10.77.0.2","hubAddress":"10.77.0.1"}`. 404 when the hub no longer knows the request (expired, hub restarted): the agent asks again and shows the new code. |
| `GET /api/admin/pairing` | Admin: pending requests `{id,name,code,ip,createdAt}`. |
| `POST /api/admin/pairing/{id}/accept` | Admin: `{"name":"christoph-pc"}` or `{"replace":"oldroom"}`. |
| `DELETE /api/admin/pairing/{id}` | Admin: reject. |

Pending requests live in memory only. Accepting stores the node.

After acceptance the agent brings up the tunnel and fetches its room's
settings from the hub **inside the tunnel**, at
`http://10.77.0.1/node/config` (a listener on the tunnel only):

```json
{"room":"christoph-pc","nekoToken":"...","publicIp":"203.0.113.10",
 "mediaPort":52100,"dns":["9.9.9.9","1.1.1.1"]}
```

The agent fetches it again after every reconnect, so the hub can change
these without pairing again. The neko token only ever travels inside the
tunnel.

### Stored on the hub

- The hub's WireGuard key pair, created on first start in the data
  directory. Losing it means every node pairs again.
- Per paired room, in `registered_rooms` (migration 0010): node public
  key, tunnel address, and where the node was last seen (saved every 30 s
  when it changes). The neko URL is `http://<tunnel address>:8080` and
  never shown, like other room addresses; any room whose address is
  inside the tunnel network is reached through the tunnel, and admins
  cannot register such addresses by hand. Media port to come (step 5).
- Paired rooms appear as "Paired" in the Rooms tab and the admin API; their
  address and token cannot be changed, only the room removed.
- Removing the room in the admin page removes the WireGuard peer at once:
  the node is cut off. Pairing it again needs a new request.

Server settings: `COZYCAST_TUNNEL_PORT` (UDP; tunnels are off without it,
since Docker's published ports bypass host firewalls like ufw),
`COZYCAST_TUNNEL_NET` (`10.77.0.0/24`). To come: `COZYCAST_PUBLIC_IP` (the
hub's address for media), `COZYCAST_MEDIA_PORTS` (`52100-52199`, one per
paired room, UDP and TCP).

## The node

`compose.node.yaml` has three services:

- **net**: does nothing but hold the network namespace (a pause
  container). The other two join it (`network_mode: service:net`), so it
  survives the agent restarting.
- **agent**: Go, built from this repository (`cozycast-node` image). Needs
  `NET_ADMIN` and `/dev/net/tun`, inside its own namespace only. It runs
  WireGuard (wireguard-go on a TUN device), sets the routes and firewall,
  writes the room's environment, and keeps reconnecting.
- **room**: the usual worker image. Its entrypoint waits for the file the
  agent writes (`COZYCAST_ENV_FILE`, with `COZYCAST_ROOM`,
  `COZYCAST_NEKO_TOKEN`, `NEKO_WEBRTC_NAT1TO1`, `NEKO_WEBRTC_UDPMUX`,
  `NEKO_WEBRTC_TCPMUX`), reads it, then starts as today. Nothing secret is
  in the compose file. A change that needs the room restarted (a new hub
  IP) is announced in the agent's log.

### Networking in the namespace

Fail closed: without the tunnel the room has no network at all.

- Default route: the tunnel. The only other route is to the hub's public
  address, for the tunnel itself.
- nftables, set by the agent:
  - out through the home network (`eth0`): only WireGuard to the hub, and
    DNS to look up the hub, both only from the agent's user (root; the
    desktop user cannot become root).
  - in from the tunnel: only the hub's address, only neko (8080), the title
    and play helpers (8081, 8082) and the media port.
  - nothing in from the home network.
- DNS for the room: the resolvers from the config, reached through the
  tunnel.

The room cannot reach the home router, other devices or the host PC
(`ideas.md`, "Isolate the room from the LAN").

## The hub side

- **Reaching neko**: `neko.NewClient` gets a dial function; for paired
  rooms it dials through the tunnel. The same transport is used by the
  places that today use the default HTTP client: the observer WebSocket
  (`neko/events.go`), the proxied WebSocket and file transfers
  (`httpapi/proxy.go`), and the title and play helpers.
- **The room's internet**: the hub's network stack accepts the room's
  TCP and UDP connections to anywhere and opens them again from the hub
  (gVisor's TCP and UDP forwarders), so websites see the hub's address.
  Refused: private, loopback, link-local and carrier-grade NAT ranges, the
  cloud metadata address, the tunnel network (no node reaches another) and
  the hub's own address. UDP flows time out after a minute of silence.
- **Media**: until the relay exists, the hub forwards each paired room's
  media port (UDP and TCP, public) to the room's neko through the tunnel.
  neko announces the hub's IP (`NEKO_WEBRTC_NAT1TO1`) and that port, so
  viewers connect to the hub. Each viewer's stream still crosses the home
  upload (viewers × bitrate) until the relay. This is neko's own documented
  setup for SSH port forwarding ([networking](https://neko.m1k1o.net/docs/v3/customization/networking)):
  `NAT1TO1` set to the address viewers use, one multiplexed port forwarded.
  Its examples also set `NEKO_WEBRTC_ICELITE=1`, which suits a room that
  only ever announces the hub's address; to be tested for paired rooms.

## When something goes offline

Nothing here needs anyone to do anything; everything retries forever.

| What happens | Effect |
|---|---|
| Home network drops, PC sleeps, home IP changes | WireGuard keepalive (25 s) re-establishes the tunnel when the network is back; roaming is built in. |
| Agent crashes or restarts | Docker restarts it; the namespace (`net`) and the room keep running; the agent recreates the tunnel and rules. The room has no network in between. |
| Room container restarts | The hub's existing reconnect (`neko.WatchHost`, backoff up to 10 s) picks it up. |
| Hub restarts | Peers are loaded from the database with where each node was last seen, so the hub starts the handshake itself as soon as it needs neko (tested). Keepalives alone would not do: WireGuard only renews a session that stopped answering when it has real data to send, so a node would otherwise wait up to two minutes. The agent's regular check over the tunnel is the second way back. |
| Node never comes back | The room shows offline; an admin can remove it, or accept a new request as its replacement. |

What viewers see, new:

- The server tells tabs when the desktop goes away and comes back:
  `{"type":"desktop","state":"offline"}` after 5 s without neko (short
  blips stay silent), and `{"type":"desktop","state":"online"}` when the
  observer is connected again. Today tabs only notice their own neko
  socket closing and retry with backoff up to 30 s.
- While offline the video area says "The room's desktop is offline. It
  comes back by itself." Chat, the user list and settings keep working:
  they live on the hub.
- On `online` tabs ask for a neko token at once instead of waiting for
  their backoff.

Admins see per room: Online, Offline since <time>, or Waiting for host
(paired, never connected), and the time of the last tunnel handshake.
The Rooms tab refreshes this by itself.

This also helps rooms on the hub's own machine (a crashed container), so
it can come first.

## Security

- The hub can only reach neko, its two helpers and the media port of
  each room; the node's firewall enforces it, not just the hub.
- The room's only way out is the tunnel; with it down there is no
  network, and it never falls back to the home connection. It cannot open
  the home router or other devices. Needed before anyone else gets the
  remote.
- Websites see the hub's address, never the home IP. The costs: streaming
  sites treat datacenter addresses worse (blocks, sign-in prompts), and
  abuse complaints about what a room opens go to the VPS account.
- The neko token crosses only the tunnel. Admin pages and API never show
  room addresses or tokens of paired rooms.
- A compromised hub can control the room (as today) but cannot reach the
  home network: the node's firewall, not the hub, decides that.
- Client IPs for bans and rate limits are unaffected: browsers connect to
  the hub directly.
- Pairing is open to anyone who knows the hub's address, so it is capped,
  rate limited and expires; nothing happens until an admin accepts a
  matching code.

## The media relay

Later: until then the hub forwards each viewer's stream from the node.

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
- It replaces the per-room media forwarding: neko's `NEKO_WEBRTC_NAT1TO1`
  becomes the node's tunnel address, and only the relay talks to it.

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

## Hardware (node)

The first node is a donated desktop PC. The notes below were written for
an **Orange Pi 5** (RK3588S, eight cores), the other candidate.

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

Each step works on its own and is tested before the next.

1. **Offline handling** (done). The `desktop` messages, the offline notice, an
   immediate reconnect, and online/offline in the Rooms tab. Useful for
   every deployment. Done when stopping and starting a room container
   shows offline and then brings the picture back by itself.
2. **Tunnel in the hub** (done). Userspace WireGuard, the hub key, peers
   from the database, neko reached through the tunnel. Tested with a second
   in-process WireGuard endpoint serving a fake neko; a node container
   follows with the agent in step 3.
3. **Pairing.** The API, the Requests list and the agent's code, up to a
   working tunnel. Done when a fresh node is accepted without copying
   anything, and keeps working across restarts of both sides.
4. **The node's network.** The namespace, firewall and routes, the room's
   environment file, and the hub's forwarding of the room's connections.
   Done when a website opened in the room sees the hub's address, the
   room's browser cannot open the home router, and the room has no network
   with the tunnel down.
5. **Media through the hub.** Forwarded media ports. Done when a viewer
   sees the room and only ever the hub's address (also in the browser's
   WebRTC details). Needs a real VPS; the development sandbox has no
   incoming UDP.
6. **Measure the node.** One room with a video playing, CPU per stream
   setting, home upload with several viewers.
7. **Input over the WebSocket**, then **the relay** (see above). After this
   the home upload is one stream per watched room.
8. **Hardware encoding**, only if step 6 shows software encoding is too
   slow (see Hardware).
9. **Several rooms per node** and room creation from the admin page: the
   agent starts room containers itself on request.

## Alternatives considered

- **One HTTPS connection** from the node instead of WireGuard (a
  multiplexed WebSocket): works where UDP is blocked and needs no
  `NET_ADMIN`, but media would go over TCP, and only the room's Firefox
  could browse through the hub (as a proxy), not the whole room.
- **Kernel WireGuard on the VPS**: needs root or `NET_ADMIN` for the server
  and iptables for the NAT; the userspace version needs neither.
- **Tokens and addresses copied by hand** (what registering by URL does):
  error-prone for a host who is not an admin, and needs an address the hub
  can reach, which a home PC without port forwarding does not have.
- **A neko plugin** instead of the outside server: a plugin lives in one
  room and cannot hold accounts or cross-room state; neko's docs call
  external plugins experimental and tie them to the exact neko build.
- **TURN (coturn)**: hides the home IP but forwards each viewer's copy; the
