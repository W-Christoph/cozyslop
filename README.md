# cozyslop

Watch things together in a shared remote browser. A from-scratch rewrite of
[CozyCast](https://github.com/Vorlent/cozycast) that keeps its UX and throws
away its backend, built to run cheaply on servers without a GPU.

**Status: prototype.** The stream, remote control and per-user permissions
work end to end. Accounts, chat and the rest of the CozyCast UI are not
ported yet (see [Roadmap](#roadmap)). The app still calls itself CozyCast.

## Why a rewrite

The old stack had grown to nine containers for what is, at its core, "encode
one desktop, send it to a few dozen browsers":

| Old | Problem | Now |
|---|---|---|
| GStreamer → Rust `whipclientsink` → LiveKit Ingress → Redis → LiveKit Server | Five hops for one already-encoded stream. Transcoding was off, so LiveKit only forwarded packets. The Rust plugin made worker builds long and fragile. A 10,000-port UDP range to open. | [neko](https://github.com/m1k1o/neko) captures, encodes once and sends WebRTC to every viewer itself. One UDP+TCP port per room. |
| Lua worker with vendored FFI, luarocks, xdotool | Hand-rolled capture supervision and input mapping. | neko does input (keysyms, keyboard layouts, clipboard, uploads). |
| Micronaut + Groovy + GORM on the JVM, Postgres, Liquibase | Heavy for a small app, slow to build, outdated dependencies. | One Go binary with the UI embedded; SQLite planned. |
| nginx, certbot, keystore scripts | Complicated SSL setup. | Automatic HTTPS in the server (planned). |
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
cp .env.example .env    # set PUBLIC_IP and NEKO_API_TOKEN
docker compose up -d --build
```

Open `http://<server>/room/default`. Open ports `80/tcp` (web) and
`52000/udp` + `52000/tcp` (media; one port per room).

Second room: uncomment `room-second` in `compose.yaml` and add it to
`COZYCAST_ROOMS`.

## Develop

```bash
cd server && go vet ./... && go test ./...
cd web && npm install && npm run build      # outputs to server/webui/dist
```

Run the server with `COZYCAST_WEB_DIR=server/webui/dist` to serve the UI from
disk, or use `npm run dev` (Vite proxies `/api` and `/neko` to
`COZYCAST_SERVER`, default `http://localhost:8080`).

| Server variable | Default | |
|---|---|---|
| `COZYCAST_NEKO_API_TOKEN` | required | must match the rooms' `NEKO_SESSION_API_TOKEN` |
| `COZYCAST_ROOMS` | `default=http://room-default:8080` | `name=url,name2=url2` |
| `COZYCAST_LISTEN` | `:8080` | |
| `COZYCAST_DEFAULT_REMOTE` | `true` | remote permission for new connections |
| `COZYCAST_DEFAULT_UPLOAD` | `false` | upload permission for new connections |
| `COZYCAST_WEB_DIR` | | serve the UI from this directory instead of the embedded build |

## Measurements

From the prototype, tested end to end with headless Chromium against the
compose stack (AMD Ryzen 7 5800X, no GPU):

- 1280×720 at a steady 30 fps, no dropped frames, audio negotiated.
- YouTube (Big Buck Bunny) playing in the room's Firefox, normal player size:
  **~70% of one core for the whole room**, roughly 40% Firefox decoding and
  30% neko encoding (x264 `veryfast`, 2.5 Mbit/s).
- First picture ~1 s after connecting.
- Server image 16.5 MB (builds in ~16 s); worker image builds in ~50 s with
  no compiling; web bundle 40 KB JS (15 KB gzipped).

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

## Roadmap

1. Accounts: SQLite, login/registration, import of existing CozyCast users
   (bcrypt hashes carry over, so passwords keep working).
2. Chat, user list, room permissions, bans and invites.
3. Port the rest of the CozyCast frontend (chat UI, settings, profiles,
   admin pages, mobile controls).
4. Automatic HTTPS and a short setup guide.
5. Pause encoding when nobody is watching; a CPU benchmark script for real
   servers.

## License

AGPL-3.0-or-later, see [LICENSE](LICENSE). Third-party code is listed in
[NOTICE](NOTICE).
