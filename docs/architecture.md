# Architecture

How the server is put together and why. The old CozyCast behaviour this
replaces is documented in the inventory used for the rewrite; deviations from
it are deliberate and listed under [Changes from CozyCast](#changes-from-cozycast).

## Processes

- **server** (Go, `server/`): accounts, sessions, rooms, chat, permissions,
  uploads, the web UI, and the only admin of every neko instance.
- **room-<name>** (neko, `worker/`): one per room. Desktop, capture, encoding,
  WebRTC, input. Configured by environment; never patched.

## Identity

Every connection has an **identity key**, which groups all tabs of one person:

| Who | Key | Display |
|---|---|---|
| Account | `u:<user id>` | nickname, colour, avatar from the account |
| Anonymous | `a:<anon id>` | "Anonymous", colour derived from the anon id, default avatar |

The anon id is a random value in the `cozy_anon` cookie (HttpOnly, 1 year), so
an anonymous user keeps their identity across reloads and reconnects (and can
still edit their own messages). Bans on anonymous users apply to the anon id
**and** the IP address.

Each browser tab is additionally a **client** with its own id, which is also
its neko member id.

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
- Login and registration are rate limited per IP.

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
  default_upload. Rooms themselves come from configuration
  (`COZYCAST_ROOMS`); a row holds their settings.
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
WebSocket and broadcasts who holds the remote.

## Room WebSocket

`GET /api/rooms/{room}/ws[?access=<temporary invite>]`, JSON messages
`{"type": ..., ...}`. The temporary invite, if any, is checked against this
room and only counted once the join succeeds. Message types are defined in
`server/internal/hub/protocol.go` and mirrored in `web/src/room/protocol.ts`.

Admin actions that change stored state (permissions, bans, room settings)
are REST endpoints; they notify the hub, which applies the change to live
connections immediately.

## Changes from CozyCast

- Sessions expire and can be revoked; logout works.
- Anonymous identities survive reconnects; anonymous bans are enforced on the
  server (cookie + IP); chat rate limits apply to everyone.
- Temporary invites are bound to their room and consumed only on success.
  Re-redeeming a permanent invite on the same account does not use it up.
- Permission, role, ban and account changes reach connected users at once.
- Global settings (front page message, registration mode) are persisted.
- Uploaded images are decoded and re-encoded; avatars are resized.
- New: desktop upload permission, change own password.
- Stream settings and room restarts from the UI require opting in to Docker
  socket access (planned); otherwise they live in `.env`.
