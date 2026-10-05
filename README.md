# cozyslop

Watch things together in a shared remote browser. A from-scratch rewrite of
[CozyCast](https://github.com/Vorlent/cozycast) that keeps its UX and throws
away its backend, built to run cheaply on servers without a GPU.

**Status: feature complete, not yet run in production.** Everything below
works end to end in tests; real-world numbers on cheap hosting are still to
come. The app still calls itself CozyCast.

## Features

- **Shared remote desktop**: one XFCE desktop with Firefox (uBlock Origin)
  and VLC per room, streamed to every viewer. Whoever holds the remote
  controls mouse and keyboard; copy and paste work both ways between the
  room and your own clipboard. The browser tab shows the title of the
  window in front on the desktop.
- **Accounts and chat**: registration (open or invite only), profiles with
  avatars and name colours, chat with images, videos and edits. Chat clears
  itself after an hour, at most 1,000 messages are kept.
- **Room permissions**: public, account, verified or invite-only rooms;
  per-user remote, chat image and desktop upload rights; invites with use
  limits; kicks and bans (anonymous viewers by cookie and IP).
- **Files**: drag files onto the stream or use the Files tab; they land in
  the desktop's Downloads folder. The tab also lists that folder, downloads
  and deletes from it (needs the upload right), and plays a file on the
  desktop in VLC (needs the remote right too).
- **Stream settings per room**: resolution, frame rate, bitrate, stream size
  and encoder speed, changed live from the room settings (see
  [Stream settings](#stream-settings)).
- **A room desktop that stays**: files, folders and the Firefox profile
  (open tabs, logins, extensions) live in a Docker volume and survive
  restarts and image updates.
- **Automatic HTTPS** with Let's Encrypt when a domain is set.
- **Migration** of accounts, logins, avatars, permissions, invites and the
  room desktop from an existing CozyCast instance ([guide](docs/migration.md)).

## Why a rewrite

The old stack had grown to nine containers for what is, at its core, "encode
one desktop, send it to a few dozen browsers":

| Old | Problem | Now |
|---|---|---|
| GStreamer → Rust `whipclientsink` → LiveKit Ingress → Redis → LiveKit Server | Five hops for one already-encoded stream. Transcoding was off, so LiveKit only forwarded packets. The Rust plugin made worker builds long and fragile. A 10,000-port UDP range to open. | [neko](https://github.com/m1k1o/neko) captures, encodes once and sends WebRTC to every viewer itself. One UDP+TCP port per room. |
| Lua worker with vendored FFI, luarocks, xdotool | Hand-rolled capture supervision and input mapping. | neko does input (keysyms, keyboard layouts, clipboard, uploads). |
| Micronaut + Groovy + GORM on the JVM, Postgres, Liquibase | Heavy for a small app, slow to build, outdated dependencies. | One Go binary with the UI embedded, and SQLite. |
| nginx, certbot, keystore scripts | Complicated SSL setup. | Automatic HTTPS (Let's Encrypt) in the server. |
| One 2,763-line `styles.css` | Hard to change anything safely. | Design tokens plus one CSS module per component. |

Why neko and not our own worker: it is exactly the worker we would have
written (Go, Pion WebRTC, GStreamer, XTest), but already maintained, with
years of edge cases fixed, prebuilt images (ARM too) and a documented API.
cozyslop runs it **unmodified** and controls it from outside through that
API, so upgrading neko means changing a tag.

Why not neko's own UI: CozyCast's UX (chat, accounts, room permissions,
invites) is the point. neko has no drop-in client library yet, so `web/`
contains a small dependency-free client for its protocol.

## How it fits together

```
browser ──HTTP/WebSocket──▶ server (Go) ──REST API──▶ room-default (neko)
   │                          │  accounts, chat,          XFCE + Firefox + VLC,
   │                          │  permissions, UI          capture, H.264 encode
   │                          └─ proxies /neko/<room>/api/ws ──┘
   └────────── WebRTC media (UDP/TCP 52000) ──────────────────┘
```

- **worker/**: the room image. neko's XFCE image plus Firefox (uBlock Origin,
  set to prefer H.264 so YouTube is cheap to decode in software) and VLC.
- **server/**: for every browser tab it creates a neko member whose
  permissions match the user's (remote control, upload) and hands the tab a
  neko session token. Permission changes apply to the live session. Browsers
  reach only an allowlist of neko endpoints through the server.
- **web/**: Preact + TypeScript + Vite. `src/neko/` speaks the neko v3
  protocol (signalling, WebRTC, binary input over a data channel).

## Run it

Moving from an existing CozyCast instance? See [Migration](docs/migration.md).

Needs Docker with Compose.

```bash
cp .env.example .env    # set PUBLIC_IP, NEKO_API_TOKEN and ADMIN_PASSWORD
docker compose up -d --build
```

Open `http://<server>/room/default` and log in as `admin`. Open ports
`80/tcp` (web), `443/tcp` (with `DOMAIN` set, for HTTPS) and `52000/udp` +
`52000/tcp` (media; one port per room). Copying out of the room into your
clipboard needs HTTPS: browsers only allow it on secure pages.

Second room: uncomment `room-second` in `compose.yaml` and add it to
`COZYCAST_ROOMS`, the server's `networks` and the `networks` at the bottom.

## Develop

```bash
cd server && go vet ./... && go test ./...
cd web && npm install && npm run build      # outputs to server/webui/dist
```

Run the server with `COZYCAST_WEB_DIR=server/webui/dist` to serve the UI from
disk, or use `npm run dev` (Vite proxies `/api` and `/neko` to
`COZYCAST_SERVER`, default `http://localhost:8080`).

`COZYCAST_INIT_ADMIN_PASSWORD` creates `admin` only when the account is
missing during normal startup. To recover a lost password, set it to the new
password and run `cozycast reset-admin` with the usual server environment.
With Compose, set `ADMIN_PASSWORD` in `.env`, then run:

```bash
docker compose run --rm server reset-admin
```

The command resets or creates `admin`, restores admin rights, enables the
account and deletes all its sessions. It prints one line and exits without
starting HTTP listeners or rooms. It is safe to run while the server is
running. An empty or invalid password is rejected without changes; passwords
must have at least 8 characters and at most 72 bytes. Later normal starts
leave an existing account's password unchanged.

| Server variable | Default | |
|---|---|---|
| `COZYCAST_NEKO_SECRET` | required | each room's neko admin token is derived from it and the room's name; the room containers get the same secret and do the same (`COZYCAST_ROOM`) |
| `COZYCAST_NEKO_API_TOKEN` | | instead of the secret: one token used as-is for every room, for a neko you run yourself with `NEKO_SESSION_API_TOKEN` |
| `COZYCAST_ROOMS` | `default=http://room-default:8080` | `name=url,name2=url2` |
| `COZYCAST_DEFAULT_SCREEN` | | container default screen for all rooms (`1280x720@30`); Compose sets it from `SCREEN`; empty leaves the desktop size alone when clearing the room setting |
| `COZYCAST_DATA_DIR` | `data` | database and uploaded chat media |
| `COZYCAST_INIT_ADMIN_PASSWORD` | | creates the `admin` account on first start; sets its password with `reset-admin` |
| `COZYCAST_LISTEN` | `:8080` | HTTP; with a domain set it only redirects and answers Let's Encrypt |
| `COZYCAST_TLS_LISTEN` | `:8443` | HTTPS, used when a domain is set |
| `COZYCAST_DOMAIN` | | host names for automatic HTTPS, comma separated |
| `COZYCAST_ACME_EMAIL` | | optional contact address for Let's Encrypt |
| `COZYCAST_TRUST_PROXY` | `false` | take client IP from the last entry of the last `X-Forwarded-For` header, and scheme from `X-Forwarded-Proto` |
| `COZYCAST_IMPORT` | | old CozyCast export archive, imported into an empty database |
| `COZYCAST_MAX_UPLOAD_MB` | `10` | maximum chat image/video size |
| `COZYCAST_DOCKER` | `false` | opt in to room restarts from the UI |
| `COZYCAST_DOCKER_SOCKET` | `/var/run/docker.sock` | |
| `COZYCAST_DOCKER_PROJECT` | | compose project name, if it cannot be detected |
| `COZYCAST_SOURCE_URL` | this repository | source code link shown to users (AGPL) |
| `COZYCAST_WEB_DIR` | | serve the UI from this directory instead of the embedded build |

With `COZYCAST_TRUST_PROXY=true`, use exactly one proxy in front. It must
append the client IP to `X-Forwarded-For`, and the server must not be reachable
around it. Invalid forwarded IPs fall back to the peer address.

Room restarts from the UI are an [opt-in Docker control feature](docs/architecture.md#room-websocket); enable the commented socket, environment and group settings in `compose.yaml`.

## Stream settings

`SCREEN` in `.env` sets the shared container default; clearing a room's
screen setting restores it.

Admins change these in the room settings; viewers switch over within
seconds without reconnecting:

| Setting | Effect |
|---|---|
| Resolution, frame rate | Size of the room's desktop. |
| Bitrate | Picture quality and bandwidth per viewer. |
| Stream size | Encode at 75/67/50% of the desktop size: less CPU and bandwidth, softer picture. |
| Encoder | x264 speed preset. Faster presets save CPU but look blockier at the same bitrate. |

Everyone in a room watches the same stream, so a room is encoded once no
matter how many people watch. The choices offered come from the room
container's environment (`STREAM_BITRATES`, `STREAM_SCALES`, `X264_PRESETS`
in `.env`); changing those lists needs `docker compose up -d` to recreate the
room, but no rebuild. The room desktop's files survive that.

## Measurements

From the prototype, tested end to end with headless Chromium against the
compose stack (AMD Ryzen 7 5800X, no GPU):

- 1280×720 at a steady 30 fps, no dropped frames, audio negotiated.
- YouTube (Big Buck Bunny) playing in the room's Firefox, normal player size:
  **~70% of one core for the whole room**, roughly 40% Firefox decoding and
  30% neko encoding (x264 `veryfast`, 2.5 Mbit/s).
- Encoder presets, same video at 2.5 Mbit/s, neko's share of one core:
  ultrafast ~21%, superfast ~25%, veryfast ~30%.
- First picture ~1 s after connecting.
- Server image 16.5 MB (builds in ~16 s); worker image builds in ~50 s with
  no compiling. The web bundle is now 212 KB JS (75 KB gzipped).

Still to measure on real hosting: fullscreen playback, several viewers over
the internet, and the old stack on the same machine for comparison.

## neko gotchas

- Omitted member profile fields default to *allowed*; the server always sends
  the full profile.
- With custom `NEKO_CAPTURE_VIDEO_PIPELINES`, `NEKO_CAPTURE_VIDEO_IDS` must
  list the pipeline ids, or neko panics when a viewer connects.
- Scroll deltas are inverted relative to the browser (positive = up).
- neko's on-demand keyframe for new viewers did not take effect with x264, so
  the encoder emits a keyframe every 2 s; with a 10 s interval new viewers
  waited ~9 s for a picture.
- X11 keysyms for capital letters only type uppercase when Shift is also
  held. Real keyboards are fine; text from mobile keyboards should go through
  neko's paste path.

## Ideas

Unused neko features (per-viewer quality, RTMP broadcast, hardware
encoding, ...) and deferred networking work (TURN relay,
hosting at home, LAN isolation) are collected in [docs/ideas.md](docs/ideas.md).
The planned home-hosting setup (a home box behind a VPS that relays the
media) has its own document: [docs/home-hosting.md](docs/home-hosting.md).

## License

AGPL-3.0-or-later, see [LICENSE](LICENSE). Third-party code is listed in
[NOTICE](NOTICE).
