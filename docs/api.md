# HTTP API

JSON request and response field names are case-sensitive. JSON bodies accept
only the documented fields and are limited to 64 KiB. Invalid JSON or unknown
fields return 400; oversized bodies return 413. Errors use
`{"error":"<message shown to the user>"}`. Internal failures return 500
`"Something went wrong."`. JSON responses set `Cache-Control: no-store`.
A 204 response has no body.

Authentication uses the HttpOnly `cozy_session` cookie (SameSite=Lax, Secure on
HTTPS). Sessions expire after 30 days without use. Authenticated requests
extend the database expiry at most once an hour; those requests also renew
the session cookie's 30-day Max-Age with the same attributes as login. Other
requests do not renew it; expired or unknown sessions clear the session cookie.
Anonymous identity uses the HttpOnly `cozy_anon` cookie. State-changing
requests with an `Origin` whose host (including port) differs from the request
host return 403
`"cross-origin request rejected"`. WebSocket upgrades also enforce same origin.

“Account” below means a logged-in, enabled account; missing or invalid sessions
return 401 `"Please log in."` on account-only endpoints. Every admin endpoint
requires an account with global admin rights: 401 for anonymous callers, 403
`"Admins only."` for other accounts.

Unix timestamps are integer seconds unless explicitly marked as milliseconds.
Request fields omitted from full replacements take their JSON zero values;
PATCH admin flags preserve omitted or null fields.

## Response objects

| Object | Fields |
|---|---|
| Own account | `username`, `nickname`, `nameColor`, `avatarUrl`, `admin`, `verified` |
| Admin account | Own account fields plus `disabled`, `createdAt`; excludes database ID and password hash |
| Global settings | `message`, `registration` (`open` or `invite`) |
| Room settings | `name`, `access` (`public`, `account`, `verified`, `invite`), `hidden`, `remoteOwnership`, `defaultRemote`, `defaultImage`, `defaultUpload` |
| Permission | `room`, `username`, `remote`, `image`, `upload`, `trusted`, `invited`, `inviteName`, `banned`, `bannedUntil` (seconds or null; null with `banned: true` means forever) |
| Anonymous ban | `id`, `room`, `ip`, `bannedUntil` (seconds or null for forever); excludes anonymous cookie ID |
| Invite view | `code`, `room`, `temporary`, `name`, `remote`, `image`, `upload`, `uses`, `maxUses` (integer or null for unlimited), `expiresAt` (seconds or null for never), `createdAt`, `valid`, `path` |

Invite `valid` means neither expired nor exhausted. `path` is `/invite/<code>`
for account invites and `/access/<code>` for temporary room-access invites.

## Accounts and public settings

| Method and path | Caller | Request body | Response | Notable errors |
|---|---|---|---|---|
| `GET /api/settings` | Anyone | None | 200 global settings | 500 on storage failure |
| `POST /api/auth/login` | Anyone | `{"username":"alice","password":"…"}` | 200 `{"user":<own account>}`; starts a session | 401 `"Wrong username or password."` (also for disabled accounts); 429 login rate limit |
| `POST /api/auth/legacy` | Anyone | `{"token":"…"}`: the refresh token an old CozyCast left in the browser's `localStorage` (`docs/migration.md`) | 200 `{"user":<own account>}`; starts a session and spends the token | 401 `"That login is no longer valid."` (unknown, used, or a disabled account); 429 login rate limit |
| `POST /api/auth/logout` | Anyone | None | 204; deletes the current session, closes its live room tabs and clears its cookie | 500 on storage failure |
| `POST /api/auth/register` | Anyone | `{"username":"alice","password":"…","inviteCode":"…"}`; code optional in open mode | 201 `{"user":<own account>}`; creates account and starts a session; an account invite also grants its room permission atomically | 400 username/password validation or `"That invite is invalid or has expired."`; 403 `"Registration requires an invite."` when invite-only and no code; 409 `"That username is taken."`; 429 registration rate limit |
| `GET /api/me` | Anyone | None | 200 `{"user":<own account>}` or `{"user":null}` | Invalid sessions become anonymous |
| `PATCH /api/me` | Account | `{"nickname":"Alice","nameColor":"#f90"}`; both required | 200 `{"user":<own account>}` | 400 invalid nickname/colour; 409 `"That nickname is another account's username."` |
| `POST /api/me/password` | Account | `{"current":"…","new":"…"}` | 204; keeps the current session and its room tabs; ends every other session and closes its room tabs | 403 `"Your current password is wrong."`; 400 invalid new password; 429 attempt rate limit |
| `POST /api/me/avatar` | Account | Multipart form field `avatar`; PNG, JPEG, GIF or WebP, at most 5 MiB | 200 `{"user":<own account>}`; saves a 256×256 PNG and deletes the previous avatar if no account still references it | 415 `"Use a PNG, JPEG, GIF or WebP image."`; 413 `"File is too large."`; 400 malformed multipart or missing/duplicate field |
| `DELETE /api/me/avatar` | Account | None | 200 `{"user":<own account>}`; clears the avatar and deletes its file if no account still references it | Common account errors |

Session revocation sends affected room tabs `{"type":"kicked","reason":"session"}`
and closes their sockets with code 4000; their neko members and proxy
connections are removed too. Tabs of other accounts and anonymous browsers
are unaffected.

Profile and avatar changes reach live room connections as `user_updated`.
Avatars are decoded (GIF uses its first frame), center-cropped to a square,
resized with Catmull–Rom interpolation and re-encoded as PNG. Images wider or
taller than 8000 pixels, or over 40 megapixels, are rejected before full decode
with the same 415 error as unsupported or invalid images.

Usernames are 2–12 ASCII letters/digits, optionally separated by single `-`,
`_` or `.`; stored lowercase and looked up case-insensitively. Passwords are
at least 8 characters and at most 72 bytes
(`"Passwords must be at least 8 characters and at most 72 bytes."` on validation failure).
Nicknames are 1–12 printable characters from the account validation's ASCII /
Latin-1 ranges, without leading, trailing or double spaces. A nickname may
match the caller's own username, but may not match another account's username
(case-insensitively). Colours are `#rgb` or `#rrggbb` hex values.

Login permits an initial burst of 10 attempts per IP, replenishing one every
30 seconds. Registration permits three attempts that reach account creation
per IP, replenishing one every 10 minutes. Password change shares the login
attempt limiter. Failed invite redemption during registration counts toward
its limit; malformed usernames/passwords do not.

## Admin users

All callers: admin.

| Method and path | Request body | Response | Notable errors |
|---|---|---|---|
| `GET /api/admin/users` | None | 200 array of admin accounts, ordered by username | Common admin errors |
| `PATCH /api/admin/users/{username}` | Any subset of `{"admin":true,"verified":true,"disabled":false}` | 200 updated admin account; disabling deletes all sessions; flags reach live room connections | 404 `"Unknown user."`; 403 `"You can't remove your own admin rights or disable yourself."`; 409 `"There must be at least one admin."` |
| `DELETE /api/admin/users/{username}` | None | 204; deletes account, sessions and permissions; deletes the avatar file if no account still references it; disconnects live room connections | 404 unknown user; 403 `"You can't delete yourself."`; 409 `"Remove admin rights first."` |
| `POST /api/admin/users/{username}/password` | `{"password":"…"}` | 204; resets password, deletes all of the user's sessions and closes all of their live room tabs with reason `session` | 404 unknown user; 400 password validation |

Admins may change their own verification flag, but may not change their own
admin or disabled flag. At least one enabled admin must remain.

## Password reset links

| Method and path | Caller | Request body | Response | Notable errors |
|---|---|---|---|---|
| `POST /api/admin/users/{username}/password-reset` | Admin | None | 201 `{"token":"…","path":"/reset/…","expiresAt":1234567890}`; secret returned only on issuance | 404 `"Unknown user."`; common admin errors |
| `POST /api/auth/password-reset/check` | Anyone (no session) | `{"token":"…"}` | 200 `{"valid":true,"username":"alice"}`; does not consume the link | 404 generic invalid-link error; 429 reset attempt limit |
| `POST /api/auth/password-reset/redeem` | Anyone (no session) | `{"token":"…","password":"…"}` | 204; changes password, consumes the link, revokes all sessions and legacy logins atomically, and closes live room tabs | 400 password validation; 404 generic invalid-link error; 429 reset attempt limit |

Links contain 32 random bytes encoded as unpadded URL-safe base64. Only the
SHA-256 of the token is stored. They last 24 hours and work once; issuance
invalidates earlier unused links for the account. Any other password change,
including admin reset and CLI admin recovery, invalidates outstanding links.
Expired and used rows are cleaned up when a link is issued.

Issuance uses the same global admin right as direct password reset. Admin
accounts may be targets; disabled flags and room bans are preserved, so a
reset does not enable an account or lift a ban. Redemption does not log in.
Missing, malformed, expired, replaced and used tokens all return 404
`"This reset link is invalid or has expired. Ask a moderator for a new link."`.
Check and redeem share a separate per-IP limiter: burst 10, replenishing one
attempt every 30 seconds; 429 says
`"Too many reset link attempts. Try again in a minute."`.

Public API calls carry the token in the JSON body rather than the URL.
`/reset/<token>` pages set `Referrer-Policy: no-referrer`, `Cache-Control:
no-store` and a Content Security Policy restricting requests to this server.
The SPA also sets a no-referrer meta policy, including for client navigation
and invite links. Tokens are not logged by the application.

## Admin permissions

All callers: admin.

| Method and path | Request body | Response | Notable errors |
|---|---|---|---|
| `GET /api/admin/permissions?room=<optional>` | None | 200 permission array; all rooms if the filter is absent/empty; ordered by room then username | Common admin errors |
| `PUT /api/admin/permissions/{room}/{username}` | Full replacement: `{"remote":false,"image":false,"upload":false,"trusted":false,"invited":false,"banned":false,"inviteName":"","bannedUntil":null}` | 200 saved permission with username; applies to live room connections | 404 `"Unknown room."` / `"Unknown user."`; 400 invite name over 64 characters |
| `DELETE /api/admin/permissions/{room}/{username}` | None | 204; removes the override and refreshes live rights | 404 unknown user or `"Unknown permission."` if there is no stored row |

Account bans are permission rows with `banned: true`. Removing a permission
also removes its ban and grants. A banned account is denied even if it is an
admin. `trusted` grants room admission and remote/image/upload rights;
`invited` grants admission. Effective rights also depend on room defaults and
global admin rights; see [architecture](architecture.md#permissions).

## Admin room registrations

All endpoints use the usual admin authentication and JSON error format.

| Endpoint | Body | Success | Errors |
|---|---|---|---|
| `GET /api/admin/rooms` | None | 200 sorted array of `{name,source,connected,offlineSince?,userCount,container?}` for all live rooms; source is `configured`, `registered` or `paired`; connected means the authenticated neko event stream is working; `offlineSince` (unix ms) is present while viewers are told the desktop is offline; `container` is `running` or `stopped` when container control is enabled and the lookup succeeds (2 s timeout per room); no tokens or addresses | Common admin errors |
| `POST /api/admin/rooms` | `{"name":"extra","nekoUrl":"http://room-extra:8080"}` | 201 room fields above plus `nekoToken`, returned once | 400 invalid name/URL; 409 `"That room already exists."` (including configured names) |
| `PATCH /api/admin/rooms/{room}` | `{"nekoUrl":"https://neko.example/prefix"}` | 200 room fields, no token; replaces connection immediately | 400 invalid URL; 404 `"Unknown room."`; 409 configured room |
| `POST /api/admin/rooms/{room}/token` | None | 200 room fields plus a fresh `nekoToken`, returned once; replaces connection immediately | 404 unknown; 409 configured room |
| `POST /api/admin/rooms/{room}/start` | None | 200 room fields above, with container state read again; starts the configured room's container | 404 `"Unknown room."`; 409 `"This room's container is not managed by the server."`; 502 `"Docker could not start the room."` |
| `POST /api/admin/rooms/{room}/stop` | None | 200 room fields above, with container state read again; stops the configured room's container with a 10 s grace period | 404 `"Unknown room."`; 409 `"This room's container is not managed by the server."`; 502 `"Docker could not stop the room."` |
| `DELETE /api/admin/rooms/{room}` | None | 204; unpublishes room and disconnects viewers | 404 unknown; 409 configured room |

Names contain one or more ASCII letters, digits, underscores or hyphens.
URLs must be absolute `http`/`https` with a hostname and no credentials,
query (including an empty `?`) or fragment. A path prefix is allowed.
Neko does not need to be reachable at registration; the hub retries.
The address is write-only: no response returns it, so a room at home does
not show its address to admins either.
Configured rooms reject changes with `"This room is managed in COZYCAST_ROOMS."`.
Configuration wins at startup over a stored registration of the same name.

Each token is 32 random bytes encoded as unpadded URL-safe base64. Supply it to
the worker as `COZYCAST_NEKO_TOKEN`; after rotation update the worker yourself.
Lists and PATCH responses never include it. URL/token replacement sends a
terminal `room_changed` kick: viewers reopen the room to reconnect. Removal
sends `not_found`. Removing a registration keeps settings, permissions and chat
history (under normal retention); registering its name again restores them.
The public `GET /api/rooms` reflects registrations and removals immediately.
These APIs register connections only; they do not create containers.

## Pairing computers

Rooms on other computers (`docs/home-hosting.md`). All answer 503 when
`COZYCAST_TUNNEL_PORT` is not set.

| Endpoint | Caller | Body | Success | Errors |
|---|---|---|---|---|
| `POST /api/nodes/pair` | the computer's agent; 10 per minute per IP | `{"name":"home","nodeKey":"<WireGuard public key, base64>"}` | 201 `{id,secret,hubKey,nonce,tunnelPort,expiresAt}`; the same key while pending gets the same request | 400 invalid name/key; 409 key already paired; 429 too many (20 pending, 3 per IP) |
| `GET /api/nodes/pair/{id}` | the agent, `Authorization: Bearer <secret>` | None | long poll up to 25 s: `{"status":"pending"\|"rejected"\|"expired"}`, or `{"status":"accepted",room,address,hubAddress,network}` | 404 unknown or forgotten |
| `GET /api/admin/pairing` | admin | None | 200 pending `[{id,name,code,ip,createdAt,expiresAt}]`, oldest first | |
| `POST /api/admin/pairing/{id}/accept` | admin | `{"name":"room"}` or `{"replace":"paired-room"}` | 200 the room (as in the room list) | 400 invalid name, or neither/both given; 404 no longer waiting; 409 name taken, room not paired, no tunnel address free |
| `DELETE /api/admin/pairing/{id}` | admin | None | 204 | 404 no longer waiting |

The code (`XXXX-XXXX`) is never sent to the computer: the agent computes it
from `hubKey`, its own key and `nonce` (`pairing.Code`), so a swapped key
shows a different code. Requests expire after 10 minutes; answers are kept 10
more minutes for the agent to collect. Accepting a new room gives it the next
free tunnel address and a random neko token; replacing keeps both, and the
old computer's key stops working at once.

Inside the tunnel only, at `http://<server tunnel address>/node/config?boot=<id>`,
a paired computer gets `{"room","nekoToken","mediaPort","publicIp"}` (the
last two when known), recognized by its tunnel
address. A new `boot` ID (the agent started again) makes the server
reconnect to that room at once.

## Admin room settings and moderation

All callers: admin.

| Method and path | Request body | Response | Notable errors |
|---|---|---|---|
| `GET /api/admin/rooms/{room}/settings` | None | 200 room settings | 404 `"Unknown room."` |
| `PUT /api/admin/rooms/{room}/settings` | Room settings: `{"access":"public","hidden":false,"remoteOwnership":false,"defaultRemote":false,"defaultImage":false,"defaultUpload":false,"screen":"1280x720@30","stream":"b2500-s100-veryfast"}`. `screen` and `stream` may be omitted (unchanged); `screen` `""` = `COZYCAST_DEFAULT_SCREEN` (the container default; leaves the screen alone if unset); `stream` is a capture pipeline id from the stream options (`b<kbit/s>-s<percent>-<x264 preset>`), `""` = neko's default; `name` is ignored (the path decides) | 200 saved settings; reloads room settings, rechecks live admission/rights, applies the screen size to the desktop, and viewers switch to the chosen stream | 404 unknown room; 400 invalid access or screen format; 400 screen or stream not offered by the desktop; 503 desktop unreachable while changing the screen or stream |
| `GET /api/admin/rooms/{room}/stream-options` | None | 200 `{"screens":["1920x1080@30",...],"streams":["b2500-s100-veryfast",...]}`: the 16:9 desktop sizes the room supports and the capture pipelines neko offers (default first) | 404 unknown room; 503 desktop unreachable |
| `POST /api/admin/rooms/{room}/bans` | `{"key":"u:123","minutes":null}`; key is an account `u:<id>` or anonymous `a:<anon id>` identity from the room protocol; minutes is an integer or null | 204; null/omitted means permanent ban, 0 means kick, positive means ban for that many minutes; disconnects all tabs | 404 unknown room or `"That user is not in the room."`; 400 negative/out-of-range minutes |
| `GET /api/admin/rooms/{room}/grants` | None | 200 array of `{"key":"a:<anon id>","remote":true,"upload":false}`: what admins gave the anonymous users now in this room | 404 `"Unknown room."` |
| `PUT /api/admin/rooms/{room}/grants` | `{"key":"a:<anon id>","remote":true,"upload":false}`; the user must be in the room | 200 the grant; applies at once, and both `false` takes it back. Anonymous users have no account to save a permission on: a grant lasts while the user is in the room and is gone once their last tab leaves (a page reload does that) | 400 `"Only anonymous users get rights this way; accounts have permissions."`; 404 `"Unknown room."` / `"That user is not in the room."` |
| `GET /api/admin/bans?room=<optional>` | None | 200 active anonymous bans; all rooms if the filter is absent/empty; ordered by room then ID | Account bans are listed under permissions |
| `DELETE /api/admin/bans/{id}` | None | 204; removes an anonymous ban | 400 invalid integer ID; 404 `"Unknown ban."` |

Anonymous bans match the browser's anonymous ID **or** IP. Kicks do not create
a ban and allow the person to rejoin immediately. An anonymous ban also
disconnects all tabs of every anonymous identity in that room with a tab on
the banned IP, including its tabs on other IPs. Accounts on that IP are
unaffected. The stored ban uses the target's anonymous ID and the IP of its
most recently joined live tab; a plain kick removes only the target identity.

## Admin global settings

| Method and path | Caller | Request body | Response | Notable errors |
|---|---|---|---|---|
| `PUT /api/admin/settings` | Admin | `{"message":"…","registration":"open"}`; full replacement | 200 saved global settings | 400 message over 4096 characters or registration other than `open` / `invite` |

## Invites

| Method and path | Caller | Request body | Response | Notable errors |
|---|---|---|---|---|
| `POST /api/admin/invites` | Admin | `{"room":"default","temporary":false,"name":"friends","remote":true,"image":false,"upload":false,"maxUses":null,"expiresInMinutes":null}` | 201 invite view; expiry is current time plus minutes × 60 | 404 `"Unknown room."`; 400 name over 64 characters, maximum uses below 1, expiry minutes below 1 or out of range |
| `GET /api/admin/invites?room=<optional>` | Admin | None | 200 invite view array, including expired/exhausted invites; newest first, then code; all rooms if filter absent/empty | Common admin errors |
| `DELETE /api/admin/invites/{code}` | Admin | None | 204 | 404 `"This invite is invalid or has expired."` when missing |
| `GET /api/invites/{code}` | Anyone | None | 200 `{"room":"default","temporary":false}`; does not consume a use | 404 `"This invite is invalid or has expired."` when missing, expired or exhausted |
| `POST /api/invites/{code}/redeem` | Account | None | 204; grants account invite rights and refreshes live permissions | 404 `"This invite is invalid or has expired."` for missing, temporary, expired or exhausted invites |

Redeeming an account invite sets `invited`, adds its remote/image/upload
grants, and preserves existing trust and bans. A non-empty invite name replaces
the permission's invite name. Each account consumes at most one use per code;
repeating redemption succeeds even after that account exhausted the invite,
but still fails after expiry. Temporary invites are consumed only after a
successful room join, using the WebSocket `access` query parameter.

## Rooms, WebSocket and neko

| Method and path | Caller | Request | Response | Notable errors |
|---|---|---|---|---|
| `GET /api/rooms` | Anyone | No body | 200 array of `{"name":"default","access":"public","userCount":0,"open":true}` ordered by name; `open` means caller may join; hidden rooms appear only if caller may join | 500 on storage failure |
| `GET /api/rooms/{room}/ws?access=<optional temporary invite>` | Anyone admitted by room access rules and bans | WebSocket upgrade with account/anonymous cookies; optional room-bound temporary invite | 101; JSON room protocol, starting with `welcome`; admission denial or an unknown room sends `kicked` and closes with code 4000 | unknown room (`not_found`); cross-origin upgrade rejected; banned/account/verified/invite admission denial |
| `POST /api/rooms/{room}/media` | Identity currently joined through the room WebSocket with image rights | Multipart form field `file`; PNG, JPEG, GIF, WebP, MP4 or WebM | 204; stores the original file and broadcasts an image/video chat message | 404 `"Unknown room."`; 403 `"Join the room first."` / `"You are not allowed to post images."`; 415 `"Unsupported file type."`; 413 `"File is too large."`; 400 malformed multipart or missing/duplicate field |
| `GET /neko/{room}/api/ws?token=<neko token>` | A tab currently in the room, with the neko token the room WebSocket issued to it | WebSocket upgrade; messages follow neko's protocol, except that requests for a capture pipeline (`signal/request`, `signal/video`) always get the room's stream, and the list of the desktop's Downloads folder (`filetransfer/update`) is only passed on, or asked for, with upload rights | neko's messages | 404 unknown room or disallowed path; 403 for a token the server did not issue or whose tab has left; 502 if neko is unavailable; closed when the tab leaves or is kicked |
| `GET /neko/{room}/api/filetransfer?token=<neko token>&filename=<name>` | Holder of a per-tab neko token with upload rights | A file in the desktop's Downloads folder | The file, always as a download (`Content-Disposition: attachment`, `application/octet-stream`, `nosniff`, `Content-Security-Policy: sandbox`): it is never displayed as a page of this site | 403 `"You are not allowed to download files from the room."` without upload rights; 403 for a token the server did not issue or whose tab has left; upstream errors; 502 if upstream unavailable |
| `POST /neko/{room}/api/filetransfer` | Holder of a per-tab neko token with upload rights | Query and body follow neko's file-transfer plugin (an upload); its `DELETE` and subpaths are not proxied (the server deletes for a tab, see `file_delete` below) | Upstream response | Upstream auth/permission errors; 404 unknown room or disallowed path; 403 for a token the server did not issue or whose tab has left; 502 if upstream unavailable |

The proxy accepts exactly these three requests and removes `Origin` before
forwarding. Other neko APIs are inaccessible, and so are neko members the
server did not create: their tokens are refused. No separate CozyCast JSON
schema applies to proxied requests or responses.

The room WebSocket carries presence, chat, typing, activity, mute status,
rights, room settings, remote ownership, moderation and per-tab neko tokens.
`welcome.windowTitle` is the title of the window in front on the room's
desktop (`""` if unknown); `{"type":"window_title","title":"…"}` follows when
it changes (see `docs/architecture.md`).
`welcome.restart` is a boolean indicating whether container control is enabled.
Clients send `{"type":"restart"}` to restart the room's desktop; admins are
always allowed and trusted users are allowed once per hour per room. The
server broadcasts `{"type":"restarting","by":"<nickname>"}` to everyone
before restarting. Disabled, unauthorized and cooldown requests receive an
`error` message through the usual room protocol.
Clients with the upload right send `{"type":"file_delete","name":"<file>"}`
to delete a file of the desktop's Downloads folder, and
`{"type":"file_play","name":"<file>"}` to play one on the desktop (in VLC,
fullscreen); playing also needs the remote right, and with remote ownership
nobody else may hold the remote. The tab that asked gets
`{"type":"file_result","action":"delete"|"play","name":"<file>","error":"…"}`,
with `error` `""` if it worked (see `docs/architecture.md`).
An unknown room upgrades successfully, sends
`{"type":"kicked","reason":"not_found"}`, then closes with code 4000.
Other `kicked.reason` values are `banned`, `account`, `verified`, `invite`,
`kicked`, `deleted`, `session` and `room_changed`.

On join or a `{"type":"neko_token"}` request, failure to issue a token
because neko failed sends `{"type":"neko_unavailable","message":"..."}`
with a user-facing explanation. Clients can retry the token request while
keeping the chat connection. A successful request sends the usual `neko`
message with `token` and `path`.

`welcome.desktop` is `"online"` or `"offline"`. When the room's neko has been
unreachable for 5 seconds the server broadcasts
`{"type":"desktop","state":"offline"}`, and `{"type":"desktop","state":"online"}`
as soon as it is back. While offline, token requests get the `offline` message
again instead of `neko_unavailable`; clients wait for `online` and then ask
for a token. Chat and everything else keep working.

See [the protocol definitions](../server/internal/hub/protocol.go) for message
fields and types. Admission and effective rights are described in
[architecture](architecture.md#permissions).

Chat media files are limited to `COZYCAST_MAX_UPLOAD_MB` MiB (default 10;
positive integer). Uploads stream to temporary files; request bodies allow
an additional 64 KiB of multipart overhead, with the file limit checked
separately. Permission is checked before reading and again before posting.
Content determines the type and extension; the supplied filename and MIME
type do not. Chat images undergo a dimension check with the same limits as
avatars and a full decode (all GIF frames), then are stored unchanged to
preserve animation. MP4 requires an ISO BMFF `ftyp` signature at offset 4;
WebM requires an EBML header. Media files are deleted when their chat messages
are removed or pruned.

## Uploaded files

| Method and path | Caller | Response | Notable errors |
|---|---|---|---|
| `GET /media/avatars/{file}` / `HEAD /media/avatars/{file}` | Anyone | File with `Cache-Control: public, max-age=31536000, immutable` | 404 missing file or invalid filename/path |
| `GET /media/chat/{file}` / `HEAD /media/chat/{file}` | Anyone | File with `Cache-Control: private, max-age=3600`; supports byte ranges (206), including video seeking | 404 missing file or invalid filename/path; 416 unsatisfiable range |

Filenames must match `^[0-9a-f]{32,64}\.(png|jpg|jpeg|gif|webp|mp4|webm)$`;
new uploads use 32 random hex characters and imported avatars may use 64.
Content types are fixed by the extension (`jpg`/`jpeg` → `image/jpeg`, other
image extensions → their `image/*` type, videos → `video/mp4`/`video/webm`),
never sniffed. Responses set `X-Content-Type-Options: nosniff` and
`Content-Security-Policy: default-src 'none'; sandbox`.

## Web UI

| Method and path | Caller | Request | Response | Notable errors |
|---|---|---|---|---|
| `GET /{path...}` / `HEAD /{path...}` | Anyone | No body | Embedded static file; missing paths fall back to `index.html` for client-side routing | Static file server errors; not a JSON API |
