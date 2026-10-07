# Home hosting (planned)

A feature built in steps (see Build order; steps 1 to 5 are done, so a
lent computer runs a room for viewers). Someone lends a computer at
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
`COZYCAST_TUNNEL_NET` (`10.77.0.0/24`), `COZYCAST_PUBLIC_IP` (the hub's
address, announced to viewers; compose passes `PUBLIC_IP`),
`COZYCAST_RELAY_PORT` (`52099`, UDP and TCP, published with the same number
on the host: the media relay, for the viewers of all paired rooms; `0` turns
the relay off), `COZYCAST_MEDIA_PORTS` (`52100-52109`, one per paired room,
UDP and TCP). Each paired room gets its port when accepted (rooms paired
earlier at the next start); removing the room closes it. With the relay the
media ports listen on the hub's loopback address only, for the relay;
without it they are public and have to be published like the relay's.

## The node

`compose.node.yaml` has two services and no special permissions:

- **agent**: Go, built from this repository (`node/Dockerfile`,
  `server/cmd/node`). It runs WireGuard in userspace (like the hub), keeps
  its key and pairing in its volume, writes the room's name and neko token
  to a file the room's entrypoint waits for (`COZYCAST_ENV_FILE`), and
  keeps reconnecting.
- **room**: the usual worker image, nothing secret in the compose file.

### The room's network

The room sits on a Docker network marked `internal`: no route anywhere,
not to the internet, the computer it runs on or the home network, and no
DNS for outside names. The only thing it can reach is the agent, which is
also on the computer's normal network to reach the hub.

- **In**: the agent forwards ports 8080 to 8082 (neko, the title and play
  helpers) of its tunnel address to the room. Nothing else from the tunnel
  reaches the room or the home network.
- **Out**: the room's programs use an HTTP proxy, `agent:3128` (the
  `http_proxy` variables; Firefox follows them, as do curl, VLC and most
  others). The agent carries each connection through the tunnel to the
  hub's proxy, which opens it from the VPS (see "The hub side"). Names are
  looked up there too.
- With the tunnel down there is no way out at all: the room never falls
  back to the home connection.
- What does not work: programs that ignore proxies, and anything but TCP
  (WebRTC calls inside the room's browser, for instance).

Docker 26 and later give internal networks no outside DNS, so the room
cannot even look names up through the home's resolver.

## The hub side

- **Reaching neko**: `neko.NewClient` gets a dial function; for paired
  rooms it dials through the tunnel. The same transport is used by the
  places that today use the default HTTP client: the observer WebSocket
  (`neko/events.go`), the proxied WebSocket and file transfers
  (`httpapi/proxy.go`), and the title and play helpers.
- **The room's internet**: an HTTP proxy (`internal/egress`, CONNECT and
  plain HTTP) on the hub's tunnel address, port 3128, so websites see the
  hub's address. It looks names up itself and connects to the address it
  checked. Refused (403): private, loopback, link-local (the cloud metadata
  address), carrier-grade NAT, multicast and reserved ranges, which covers
  the tunnel network and the hub's Docker networks, and port 25.
- **Media**: the hub forwards each paired room's media port (UDP and TCP)
  through the tunnel to the agent, which forwards it to the room's neko
  (`internal/fwd`: one flow per sender for UDP, ended after a minute of
  silence). With the relay (the default, see "The media relay") the port
  listens on the hub's loopback address and the relay is its only user.
  Without it (`COZYCAST_RELAY_PORT=0`) the port is public: neko announces
  the hub's IP (`NEKO_WEBRTC_NAT1TO1`) and that port, with ICE lite, so
  viewers connect to the hub, and each viewer's stream crosses the home
  upload (viewers × bitrate). This is neko's own documented
  setup for SSH port forwarding ([networking](https://neko.m1k1o.net/docs/v3/customization/networking)):
  `NAT1TO1` set to the address viewers use, one multiplexed port forwarded.
  Its examples also set `NEKO_WEBRTC_ICELITE=1`, which suits a room that
  only ever announces the hub's address; paired rooms use it.

### Built so far (steps 3 to 5)

`compose.node.yaml` runs the agent (`node/Dockerfile`, `server/cmd/node`)
and the room. The agent keeps its key and pairing in its volume, writes
`COZYCAST_ROOM` and `COZYCAST_NEKO_TOKEN` to a file the room's entrypoint
waits for (`COZYCAST_ENV_FILE`), and forwards ports 8080 to 8082 of its
tunnel address to the room. It checks in every 15 s; the check also keeps
WireGuard's session alive. Each start sends a new boot ID with the check, so
the server drops connections through the old tunnel at once (including
requests in flight and kept-alive connections) instead of finding them dead
at its next ping. A rejected computer remembers it and does not ask again
until its state is deleted. The room's network is as described in "The
room's network"; checked in the sandbox from inside the room: no direct
connection, no DNS, the host and the home router out of reach, its Firefox
using the proxy.

The settings the agent fetches include the room's media port and the hub's
address; the agent writes them as neko's `NEKO_WEBRTC_UDPMUX`, `TCPMUX`,
`NAT1TO1` and `ICELITE`, and forwards the port from the tunnel to the room.
When they change, the agent's log asks to restart the room container (the
agent cannot restart it). Before this, a paired room's neko looked up its
public address itself and announced the home's to every viewer.

## When something goes offline

Nothing here needs anyone to do anything; everything retries forever.

| What happens | Effect |
|---|---|
| Home network drops, PC sleeps, home IP changes | WireGuard keepalive (25 s) re-establishes the tunnel when the network is back; roaming is built in. |
| Agent crashes or restarts | Docker restarts it; the room keeps running, without network in between; the server reconnects as soon as the agent checks in again. |
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
  each room: the agent forwards those and nothing else.
- The room's only way out is the agent's proxy into the tunnel; with it
  down there is no network, and it never falls back to the home
  connection. It cannot open the home router, other devices or the
  computer it runs on: Docker gives its network no route. Needed before
  anyone else gets the remote.
- Websites see the hub's address, never the home IP. The costs: streaming
  sites treat datacenter addresses worse (blocks, sign-in prompts), and
  abuse complaints about what a room opens go to the VPS account.
- The neko token crosses only the tunnel. Admin pages and API never show
  room addresses or tokens of paired rooms.
- A compromised hub can control the room (as today) but cannot reach the
  home network: the agent forwards only the room's ports, and the room has
  no route.
- Client IPs for bans and rate limits are unaffected: browsers connect to
  the hub directly.
- Pairing is open to anyone who knows the hub's address, so it is capped,
  rate limited and expires; nothing happens until an admin accepts a
  matching code.

## The media relay

Built (`server/internal/relay`, `server/internal/httpapi/relay.go`); on for
every paired room unless `COZYCAST_RELAY_PORT=0`.

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
  (PLI) when a viewer joins, at most twice a second. neko 3.1 does not act
  on it, so in practice a new viewer waits for the next regular keyframe
  (every 2 seconds).
- Lost packets: the relay answers viewers' retransmission requests (NACK)
  from a short buffer, so one viewer's bad connection costs the home line
  nothing.
- One media port (UDP and TCP) on the hub for all rooms, instead of one per
  room.
- It is the only user of the per-room media forwarding, which no longer
  listens publicly. The relay connects to the forwarded port on the hub's
  loopback address, whatever address neko announces (the agent and neko's
  settings are the same with and without the relay).

As built:

- The relay's neko member is `cozycast-relay`, created when it starts
  watching and deleted when it stops, 15 seconds after the last viewer left
  (so a reload does not restart the stream). The hub's member cleanup leaves
  it alone.
- neko sends its candidates before its offer; the relay keeps them until
  the offer is there.
- neko's RTP header extensions are removed before forwarding (their numbers
  are agreed per connection); one viewer's failed write does not stop the
  others.
- The relay follows the room's stream setting itself (`signal/video` on its
  own session); tabs' sessions are no longer moved. A tab that connects
  while neko still sends another stream gets its offer once neko has
  switched (3 s at most): tabs reconnect when the frames get bigger, and
  must not get the smaller ones first.
- When the relay's connection to neko starts again, viewers stay connected.
  Sequence numbers and timestamps carry on from the last packet; a new
  stream's own numbers would look like old packets to them.
- From a tab, every `signal/*` message stops at the hub. Tabs have no data
  channel, so input goes over the WebSocket (below).
- Checked in the sandbox with headless Chromium: two viewers of one paired
  room and one of another, all connected to the hub's relay port over UDP,
  1280×720 at 30 fps, first picture after about 3 s; neko had one watching
  session per room; a viewer kept its picture when another left, and got
  it back each time the relay was thrown out of neko (three times in a
  row); the pointer moved from a tab's WebSocket; the media ports were
  closed from outside. With three viewers the server used 5% of one core
  (Ryzen 5800X) and 47 MB. Not measured yet: the home upload on a real
  line.

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

Start with 1. Built: without an open data channel the client sends every
input event over the WebSocket (`control/move`, `scroll`, `buttondown`,
`buttonup`, `keydown`, `keyup`); checked against neko in the sandbox (the
pointer moved with no WebRTC connection at all).

## Hardware (node)

Everything is built to run on ARM (arm64) as well as x86. Tried on
2026-10-07 on a cloud ARM server (Hetzner, Ampere, 2 vCPUs, 4 GB) as the
room for an x86 hub: the room and agent images built there, the room paired
and was ready 8 s after it was accepted, and a viewer got 1280×720 at
30 fps through the relay. The room's container used 57% of one core while
its idle desktop was being watched (x264 `veryfast`, 720p, 30 fps), under
1% with nobody watching, and about 910 MB. Not tried there: a video playing
in the room. Still unknown: the Orange Pi below, whose fast cores are the
same design at a lower clock.

How it is built:

- The room image builds on neko's images, which exist for arm64 (checked:
  Firefox, the x264 encoder and the paths the room image uses are all
  there; Debian 13, where its added packages exist for arm64 too). It is
  built on the computer itself (`compose.node.yaml` has `build:`), so an
  ARM computer builds its own.
- The server and agent images cross-compile: `docker buildx build
  --platform linux/arm64` works on an x86 machine without emulation
  (checked: arm64 binaries).
- If a computer is too weak, the room's settings go down to 500 kbit/s, a
  quarter of the pixels (stream size 50%) and x264's `ultrafast`; the
  desktop's resolution and frame rate can drop too.

The notes below were written for an **Orange Pi 5** (RK3588S, eight cores).

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

## Trying it out (x86)

On the VPS, in `.env`: `PUBLIC_IP` (the VPS's address), and ideally
`DOMAIN` for HTTPS, and `COMPOSE_FILE=compose.yaml:compose.tunnel.yaml` for
the tunnel and the relay (`./cozycast.sh setup` writes all of this). Open
`51820/udp` and `52099` (UDP and TCP) in the VPS's firewall too.
`docker compose up -d --build`, or `./cozycast.sh start`.

On the home computer (Docker, nothing opened in the router):
`COZYCAST_HUB=<domain or IP> docker compose -f compose.node.yaml up -d --build`,
then `docker compose -f compose.node.yaml logs -f agent` for the code; accept
it under Admin > Rooms.

Worth checking:

- The room plays for a viewer elsewhere; in the browser's WebRTC details
  (`chrome://webrtc-internals`, `about:webrtc`) the only remote address is
  the VPS's.
- A "what is my IP" site opened in the room shows the VPS's address.
- In the room's terminal: `curl --noproxy '*' http://<router address>`
  fails, and so does the router through the proxy (403).
- Restart the home computer, the agent, then the VPS: the room comes back
  by itself each time; viewers see "offline" in between.
- The home upload while two or three people watch: one stream, whatever
  the number of viewers.

Tried on 2026-10-07: a Hetzner cloud server (2 vCPUs, 4 GB, Ubuntu 24.04)
and a home PC running Docker on Windows, behind an ordinary router with
nothing opened. The room played for a phone on mobile data, with a video
running in it; the remote and typing worked; changing the stream worked; a
"what is my IP" site showed the server's address; the router could not be
reached from the room. The home upload stayed at one stream with several
tabs watching. After the agent was taken down and started again the room
was back in 4 s without a new code; after a server restart in under 2 s.
Not tried: the browser's WebRTC details, restarting the home PC, more than
a few viewers, and longer than 15 minutes.

## VPS (hub)

Needs: 1–2 vCPUs, a public IPv4 address, unrestricted UDP, and a large
traffic allowance. One viewer at 2.5 Mbit/s is about 1.1 GB per hour; 10
viewers for four hours a day are about 1.4 TB a month. What the room's
browser downloads passes through the hub as well, in from the website and
out to the node: a 5 Mbit/s source for four hours a day adds about 0.5 TB a
month, counting both directions. Pick the datacenter
closest to the node: everything goes node → hub → viewer, and the remote
holder feels the detour.

The hub does not have to run a room itself: with
`COMPOSE_FILE=compose.yaml:compose.no-room.yaml` in `.env` only the server
is built and run (`COZYCAST_ROOMS=none`). Measured on the test server of 2026-10-07: the
server used about 100 MB and 6% of a core with a paired room being watched,
the idle default room next to it 1.2 GB.

In between: `compose.room-on-demand.yaml` keeps the hub's own room stopped
until an admin starts it under Admin > Rooms, and lets them stop it again.
This is for the hub's own room only; a paired room runs for as long as its
computer runs the agent. The server reaches Docker through a proxy that
lets three kinds of request through (list containers; start one; stop or
restart one), not through the Docker socket, which would be full control
of the machine.

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
3. **Pairing** (done). The API, the Requests list and the agent, up to a
   working tunnel. Tested end to end in one process and with real
   containers in the sandbox: accepted without copying anything; back
   after an agent restart in 1 s, the whole computer in 3 s, the server in
   13 s. The agent forwards neko's ports from the tunnel to the room
   container.
4. **The node's network** (done). The room on an internal network, the
   agent's proxy and the hub's way out (`internal/egress`). Tested end to
   end in one process (a page fetched through agent, tunnel and hub; a
   private address refused) and from inside a room container in the
   sandbox. Still to see on a real VPS: a website reporting the hub's
   address (the sandbox's hub has no internet).
5. **Media through the hub** (done). Forwarded media ports. Tested end to
   end in one process (UDP and TCP both ways) and in the sandbox with
   headless Chromium: connected over UDP to the hub's address and port,
   1280×720 at 30 fps, first picture after about 3 s.
6. **ARM** (done; run on a cloud ARM server, see Hardware). Images built
   for arm64 as well. Measured on the real computer once it runs: CPU per
   stream setting, home upload with several viewers.
7. **Input over the WebSocket**, then **the relay** (done, see above). The
   home upload is one stream per watched room.
8. **Hardware encoding**, only if step 6 shows software encoding is too
   slow (see Hardware).
9. **Several rooms per node** and room creation from the admin page: the
   agent starts room containers itself on request.

## Alternatives considered

- **A network namespace with routes and a firewall** on the node, instead
  of an internal network and a proxy: every program in the room would
  reach the internet, UDP included, but the agent would need `NET_ADMIN`
  and `/dev/net/tun`, one more thing to get right on a lent computer
  (Docker Desktop on Windows, for instance). The proxy needs nothing, and
  isolation comes from Docker itself.

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
