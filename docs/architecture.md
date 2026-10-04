# Architecture

How the server is put together and why. The old CozyCast behaviour this
replaces is documented in the inventory used for the rewrite; deviations from
it are deliberate and listed under [Changes from CozyCast](#changes-from-cozycast).

## Processes

- **server** (Go, `server/`): accounts, sessions, rooms, chat, permissions,
  uploads, the web UI, and the admin of every neko instance.
- **room-<name>** (neko, `worker/`): one per room. Desktop, capture, encoding,
  WebRTC, input. Configured by environment; never patched.

## Identity

Every connection has an **identity key**, which groups all tabs of one person:

| Who | Key | Display |
|---|---|---|
| Account | `u:<user id>` | nickname, colour, avatar from the account |
| Anonymous | `a:<anon id>` | "Anonymous", colour derived from the anon id, default avatar |

The anon id is a hash of the random value in the `cozy_anon` cookie (HttpOnly,
1 year), so an anonymous user keeps their identity across reloads and
reconnects (and can still edit their own messages). Everyone in the room sees
the anon id; the cookie value itself is never sent to other users, so knowing
an anon id is not enough to act as that person. Bans on anonymous users apply
to the anon id **and** the IP address.

Each browser tab is additionally a **client** with its own id, which is also
its neko member id.

## The neko admin token

neko runs as the desktop's user, so whoever holds the remote can read the
admin token of their room's neko (the desktop has a terminal) and use neko's
admin API from inside the room. Hiding the token would mean running neko
under another user than the desktop, which needs a patched image. So the
person holding the remote is treated as in control of their room's neko, and
that is kept from reaching further:

- Every room has its own token, derived from one secret and the room's name
  (`config.NekoToken`, `worker/entrypoint.sh`). The secret never reaches the
  desktop's processes.
- Every room is on its own Docker network, shared with the server only.
- The proxy only lets through neko tokens the server issued to a tab still
  in the room, so a member created with the admin token is no use from
  outside.
- The room containers run with `no-new-privileges` (`compose.yaml`): neko's
  image gives the desktop user passwordless sudo, and this turns it off. The
  remote holder is an ordinary user in the container, not root.

Every 30 seconds the server compares neko's members with the tabs in the
room, deletes members that belong to no tab and sets profiles that differ
from the tab's rights again (`Room.checkMembers`). This repairs what a failed
call to neko left behind (a rights change that did not reach neko). It also
undoes changes made with the admin token, but it is not a defence against
them: someone on the desktop can repeat a change faster than it is undone.
What ends that is taking away their remote right and restarting the room.

## Auth

- Passwords: bcrypt (cost 10). Hashes imported from CozyCast (`$2a$10$…`)
  verify unchanged.
- Sessions: opaque random token in the `cozy_session` cookie (HttpOnly,
  SameSite=Lax, Secure when served over HTTPS). The database stores only its
  SHA-256. Sessions expire after 30 days without use; use slides the expiry.
  Logout deletes the session. Disabling or deleting an account deletes its
  sessions and closes its sockets.
- The room WebSocket authenticates with the same cookie during the upgrade;
  same-origin is enforced on upgrade, so other sites cannot open it.
- Login and registration are rate limited per IP. With
  `COZYCAST_TRUST_PROXY=true`, the client IP is the last entry of the last
  `X-Forwarded-For` header, validated as an IP (IPv4-mapped addresses are
  unmapped); an invalid entry falls back to the peer address. Use exactly
  one proxy in front: it must append to `X-Forwarded-For`, and the server
  must not be reachable around it.

## Data (SQLite)

One file, `data/cozycast.db`, WAL mode, foreign keys on. Schema migrations are
embedded SQL files applied in order at startup (`server/internal/store/migrations`).

- `users`: id, username (lowercase, unique), password_hash, nickname,
  name_color, avatar (file name or empty), admin, verified, disabled,
  created_at.
- `sessions`: token_hash, user_id, created_at, expires_at, last_seen_at.
- `settings`: key/value. `message` (front page text), `registration`
  (`open` | `invite`).
- `rooms`: name, access (`public` | `account` | `verified` | `invite`),
  hidden, remote_ownership, center_remote, default_remote, default_image,
  default_upload, screen (desktop size, e.g. `1280x720@30`, applied through
  neko's API), stream (which capture pipeline viewers watch, e.g.
  `b2500-s100-veryfast` = 2.5 Mbit/s at full size, x264 preset veryfast).
  The worker generates one pipeline per bitrate, stream size and preset
  (`worker/entrypoint.sh`, configurable via
  `STREAM_BITRATES`/`STREAM_SCALES`/`X264_PRESETS`); neko only runs those
  being watched, and the server learns the list from neko when it connects.
  Everyone in a room watches the same pipeline, so a room encodes once.
  The server enforces this: it carries each tab's neko WebSocket itself,
  rewrites requests for a pipeline to the room's stream, and moves
  connected viewers when the setting changes. Rooms themselves come from configuration (`COZYCAST_ROOMS`); a row
  holds their settings. Clearing a room's screen applies
  `COZYCAST_DEFAULT_SCREEN`, one container default for all rooms, parsed and
  validated at startup. Compose sets it from the same `SCREEN` value as
  `NEKO_DESKTOP_SCREEN`; when unset, clearing leaves the desktop size alone.
- `room_permissions`: (room, user_id) unique; remote, image, upload, trusted,
  invited, invite_name, banned, banned_until.
- `anon_bans`: room, anon_id, ip, banned_until.
- `invites`: code, room, temporary, name, remote, image, upload, uses,
  max_uses, expires_at, created_at.
- `chat_messages`: id, room, identity key, user_id, nickname/colour snapshot,
  anonymous, type (`text` | `image` | `video`), body, media, edited,
  created_at. Kept for 1 hour, at most 1000 per room; media files are deleted
  with their message.

## Permissions

Per room, an identity's effective rights are computed in one place
(`room.Rights`) and pushed everywhere they matter (the browser, neko's member
profile, upload endpoints) whenever any input changes:

- **remote**: trusted ∨ permission.remote ∨ room.default_remote ∨ temporary invite grant
- **image** (chat images/videos): account ∧ (trusted ∨ permission.image ∨ room.default_image ∨ grant)
- **upload** (files into the desktop): trusted ∨ permission.upload ∨ room.default_upload ∨ grant
- **admin**: global account flag; admins are also trusted everywhere.

Admission to a room:

| access | who may join |
|---|---|
| public | everyone not banned |
| account | accounts, invited, admins |
| verified | verified accounts, invited, admins |
| invite | invited or trusted accounts, temporary-invite holders, admins |

Bans are checked first and apply to admins too, except that an admin can
always unban through the admin pages.

Remote control is neko's "host". `remote_ownership` maps to neko's
`implicit_hosting = false`: an occupied remote cannot be taken, only released
or reset by an admin. The server follows neko's host changes over an admin
WebSocket and broadcasts who holds the remote. Containers boot with implicit
hosting off so ownership remains enforced until the server explicitly sets
the room's value during preparation and after every observer reconnect,
including turning it on for rooms without remote ownership. The observer
has a 10-second dial deadline, requires `system/init` within 10 seconds,
and pings every 20 seconds with a 10-second deadline. Failed connections
reconnect with backoff; healthy quiet connections have no read deadline.

## Room WebSocket

`GET /api/rooms/{room}/ws[?access=<temporary invite>]`, JSON messages
`{"type": ..., ...}`. The temporary invite, if any, is checked against this
room and only counted once the join succeeds. Message types are defined in
`server/internal/hub/protocol.go` and mirrored in `web/src/room/protocol.ts`.

Admin actions that change stored state (permissions, bans, room settings)
are REST endpoints; they notify the hub, which applies the change to live
connections immediately.

Room restarts are disabled by default. With `COZYCAST_DOCKER=true`, the server
uses the Docker Engine Unix socket (`COZYCAST_DOCKER_SOCKET`, default
`/var/run/docker.sock`). It inspects its own container using its hostname to
find the Compose project, falling back to `COZYCAST_DOCKER_PROJECT` if that
fails. Without either project it logs a warning and matches by service name
only; ambiguous matches are rejected. A room's neko URL hostname is its
Compose service name (e.g. `room-default`). Each restart looks up the running
container again by Compose labels, then restarts it with a 10-second stop
timeout.

`welcome.restart` advertises availability. Admins may restart at any time;
trusted users may restart once per hour per room, shared across users and
tabs. Every accepted restart, including an admin restart, starts that
cooldown. It is held in memory and resets when the server restarts. The
server broadcasts `restarting` with the requester's nickname before issuing
the restart asynchronously; Docker failures are logged. Browsers reconnect
their desktop streams and obtain fresh neko tokens. The desktop restarts for
everyone; Firefox restores the previous tabs from its saved session after
both clean shutdowns and crashes, without a recovery prompt.

The socket mount grants the server control over Docker on the host, even
though this feature only invokes room container restarts. This is a security
trade-off: a compromised server could control other containers and the host.
Enable the commented lines in `compose.yaml` only when this access is wanted;
the non-root server also needs the socket's group via `DOCKER_GID` (see
`.env.example`). Leaving the option off keeps container control entirely off.

## Changes from CozyCast

- Sessions expire and can be revoked; logout works.
- Anonymous identities survive reconnects; anonymous bans are enforced on the
  server (cookie + IP); chat rate limits apply to everyone.
- Temporary invites are bound to their room and consumed only on success.
  Re-redeeming a permanent invite on the same account does not use it up.
- Permission, role, ban and account changes reach connected users at once.
- Global settings (front page message, registration mode) are persisted.
- Shared imported avatar files are deleted only after their last user
  reference is removed. Avatars are decoded, center-cropped, resized and re-encoded; chat images are fully decoded to validate them and served with a locked-down content type.
- New: desktop upload permission, change own password.
- Stream settings (desktop size, frame rate, quality preset) are changed live
  through neko's API, without restarting anything. Restarting a room from the
  UI uses opt-in Docker socket access, with the security trade-off above.
