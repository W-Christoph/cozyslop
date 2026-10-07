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
its neko member id. Its IP is recorded at join. An anonymous ban stores the
target's anon id and the IP of its most recently joined live tab, and removes
all tabs of every anonymous identity in that room with a tab on that IP.
Accounts on the IP are unaffected; an ordinary kick removes only its target.

Account ids are never reused after deletion. Chat retains an account's
identity key after its user reference is cleared, so a new account cannot
inherit that account's message edit rights.

## The neko admin token

neko runs as the desktop's user, so whoever holds the remote can read the
admin token of their room's neko (the desktop has a terminal) and use neko's
admin API from inside the room. Hiding the token would mean running neko
under another user than the desktop, which needs a patched image. So the
person holding the remote is treated as in control of their room's neko, and
that is kept from reaching further:

- Configured rooms derive their token from one secret and the room's name
  (`config.NekoToken`, `worker/entrypoint.sh`), unchanged. Registered rooms get
  32 random bytes encoded as URL-safe base64, stored as-is in SQLite. The
  admin API returns a token only at registration or rotation, never in lists.
  Their worker receives `COZYCAST_NEKO_TOKEN`, which takes precedence over
  derivation. Neither that variable nor `COZYCAST_NEKO_SECRET` reaches the
  desktop's processes; only the resulting `NEKO_SESSION_API_TOKEN` does.
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
  SHA-256. Sessions expire after 30 days without use; use slides the expiry
  at most once an hour and renews the cookie with the same 30-day lifetime
  when the database expiry is extended. Invalid sessions are never renewed.
  Logout deletes the session and closes its live room tabs. Password change
  deletes every other session and closes their tabs, keeping the current
  session's tabs. An admin password reset deletes all of the user's sessions
  and closes all of their tabs. Disabling or deleting an account deletes its
  sessions and closes its sockets.
- The room WebSocket authenticates with the same cookie during the upgrade;
  same-origin is enforced on upgrade, so other sites cannot open it.
  Account identities carry the stored session hash internally, and each tab
  retains it; it is never sent to clients. After deleting session rows, the
  HTTP layer tells the hub to remove matching tabs across all rooms with
  kick reason `session` and close code 4000, using the ordinary kick cleanup
  for neko members and proxy connections. Authentication just before a
  revocation followed by a join just after it can still admit a tab.
- Login and registration are rate limited per IP. With
  `COZYCAST_TRUST_PROXY=true`, the client IP is the last entry of the last
  `X-Forwarded-For` header, validated as an IP (IPv4-mapped addresses are
  unmapped); an invalid entry falls back to the peer address. Use exactly
  one proxy in front: it must append to `X-Forwarded-For`, and the server
  must not be reachable around it.

`cozycast reset-admin` reads the usual configuration and applies database
migrations, then resets or creates `admin` using
`COZYCAST_INIT_ADMIN_PASSWORD`. It validates and hashes the password before
opening the database, then restores admin rights, enables the account and
deletes its sessions in one short transaction. Existing profile and
verification fields are preserved; a newly created admin is verified. It
exits without starting listeners, rooms or the legacy import. It does not
notify a separately running server's hub, so its session deletion cannot
close room sockets already open in that process. See the
[recovery instructions](../README.md#develop).

## Data (SQLite)

One file, `data/cozycast.db`, WAL mode, foreign keys on. Schema migrations are
embedded SQL files applied in order at startup (`server/internal/store/migrations`).
The migration runner uses one connection with foreign keys disabled outside
the migration transactions, checks `PRAGMA foreign_key_check` before each
commit and restores enforcement afterward. A failed check rolls the migration
back. The users rebuild preserves ids and seeds its AUTOINCREMENT sequence
from both existing users and numeric account identity keys retained in chat.

- `users`: id, username (lowercase, unique), password_hash, nickname,
  name_color, avatar (file name or empty), admin, verified, disabled,
  created_at.
- `sessions`: token_hash, user_id, created_at, expires_at, last_seen_at.
- `settings`: key/value. `message` (front page text), `registration`
  (`open` | `invite`).
- `registered_rooms`: name (primary key), neko_url, neko_token, created_by
  (user ID, SET NULL on account deletion), created_at (unix seconds).
  Registrations are independent of settings and have no owning node yet.
  Configured rooms take precedence over registrations with the same name,
  with a startup warning; the shadowed registration is retained.
- `rooms`: name, access (`public` | `account` | `verified` | `invite`),
  hidden, remote_ownership, default_remote, default_image,
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

- **remote**: trusted ∨ permission.remote ∨ room.default_remote ∨ temporary invite grant ∨ given
- **image** (chat images/videos): account ∧ (trusted ∨ permission.image ∨ room.default_image ∨ grant)
- **upload** (files into the desktop): trusted ∨ permission.upload ∨ room.default_upload ∨ grant ∨ given
- **admin**: global account flag; admins are also trusted everywhere.

"given" is what an admin gave one anonymous user in the room (remote,
upload). There is no account to store it on: it lasts while that user is in
the room, and is gone once their last tab leaves, which a page reload does
too. Unlike an invite it lets nobody into a room.

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
reconnect with backoff (1 s doubling to 10 s); healthy quiet connections have
no read deadline.

When the observer has been gone for 5 seconds (or the room never reached neko
within 5 seconds of starting), the room tells every tab the desktop is offline
(`desktop` message, `welcome.desktop`) and stops issuing neko tokens; shorter
drops stay silent. The observer's next successful connect announces it online
again, and tabs ask for a token at once instead of waiting for their backoff.
Admins see the time it went offline in the Rooms tab.

## Tunnel

With `COZYCAST_TUNNEL_PORT` set, the server runs one end of a WireGuard
tunnel in the process (`internal/tunnel`: wireguard-go on gVisor's network
stack, no root or kernel module). Paired rooms, on other computers, are
peers; their neko URL is an address inside `COZYCAST_TUNNEL_NET`, and every
connection to such a room (API, observer, proxied WebSockets, file
transfers, title and play helpers) is dialed through the tunnel instead of
the network. The server's private key is `wireguard.key` in the data
directory. See [home-hosting.md](home-hosting.md).

## Room registration and runtime lifetime

At startup the server loads configured rooms and SQLite registrations through
one runtime config builder (neko client, default screen, title and play helpers,
play token). Admin registrations immediately enter the hub's synchronized room
map and start the same settings load, readiness retry, observer, member check
and title polling as startup rooms. Connection status means the authenticated
neko observer has received its initial state and remains connected.

Removing a room unpublishes it, rejects joins through stale references, sends a
terminal `not_found` kick, closes proxied neko sockets and HTTP transfers, and
cancels and waits for its background work. URL/token changes replace the runtime
and send `room_changed`, telling viewers to reopen the room. Neither operation
removes settings, permissions or chat history; normal chat retention still runs.
Registered rooms have no Docker restart hook. Container creation, owning nodes
and tunnels remain future work. `COZYCAST_ROOMS` must name at least one room or be
`none`.

## Room WebSocket

`GET /api/rooms/{room}/ws[?access=<temporary invite>]`, JSON messages
`{"type": ..., ...}`. The temporary invite, if any, is checked against this
room and only counted once the join succeeds. Message types are defined in
`server/internal/hub/protocol.go` and mirrored in `web/src/room/protocol.ts`.

Admin actions that change stored state (registrations, permissions, bans, room settings)
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

## Window title

The browser tab is named after the window in front on the room's desktop,
as in CozyCast. The room image runs a small helper next to neko
(`worker/window-title.py`, port 8081) that answers `GET /title` with
`xdotool getactivewindow getwindowname`. While someone is in the room, the
server asks every two seconds and sends changes to the room
(`welcome.windowTitle`, then `window_title`). The port is not published:
only the server, on the room's network, and the desktop itself reach it.

The helper runs as `nobody`, not as the desktop's user, so whoever holds
the remote cannot stop or replace it. They do choose the title, by naming a
window or opening a page; the server cuts it to one line of 200 characters
and browsers show it as text. A neko without the helper has no title, and
the tab shows the room's name.

## Deleting and playing files

The Files window lists, uploads and downloads through neko's file transfer,
which browsers reach with their own neko token. Deleting and playing are
asked of the server over the room WebSocket (`file_delete`, `file_play`,
answered with `file_result`), so that the server decides who may:

- **Delete** needs the upload right. The server uses neko's own
  `DELETE /api/filetransfer` as neko's admin; browsers cannot reach it.
- **Play** needs the upload and the remote right, and with remote ownership
  nobody else may hold the remote: it changes what everyone watches. The
  room image runs a second helper (`worker/play.py`, port 8082) that
  answers `POST /play?name=<file>` by opening the file in VLC, fullscreen,
  and closing the VLC it started before.

The play helper runs as the desktop's user, because VLC opens on their
desktop; whoever holds the remote can start VLC by hand anyway. Its port is
not published either, but pages open in the room's browser reach it, so
every request must carry the room's neko admin token, which the server and
the desktop's user know. It only plays plain files directly in Downloads. A
room image from before the helper answers nothing, and the person is told.

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
