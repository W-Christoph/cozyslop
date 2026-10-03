# TODO: before deploying

From the full-codebase review on 2026-10-03 (three Codex runs, every file
read; findings checked against the code). Tick items off as they land.

## 1. Fix before deploying

- [x] **Anonymous impersonation.** The public key `a:<id>` is the `cozy_anon`
      cookie value and is broadcast in the user list and chat
      (`server/internal/auth/auth.go`). Publish an id derived from the cookie
      instead.
- [x] **Kicked users keep their rights while the socket drains.** `kill` only
      stops writing once the queue is empty; incoming messages are still
      handled (`server/internal/httpapi/socket.go`, `hub/room.go` kickLocked).
- [x] **Slow clients are disconnected for good.** Queue overflow closes with
      the "kicked" code 4000, so the browser never reconnects.
- [x] **Rooms are public until their settings load.** Rooms start as
      `Access: "public"` and load settings in the background; a failed load
      stays public (`server/internal/hub/room.go` newRoom/run).
- [x] **GIF memory exhaustion.** Chat uploads decode every frame with only a
      per-frame size limit (`server/internal/httpapi/media.go` decodeUpload).
- [x] **No body-read or idle timeouts** on the HTTP servers
      (`server/main.go` newServer).
- [ ] **Viewers can start extra encoders.** 72 pipelines are offered and the
      proxy does not restrict which one a viewer requests
      (`worker/entrypoint.sh`, `server/internal/httpapi/proxy.go`).
      Open: enforcing the room's stream means reading neko's signalling
      messages in the proxy. Cheaper for now: offer fewer pipelines
      (`STREAM_BITRATES`, `STREAM_SCALES`, `X264_PRESETS`) and cap room CPU.
- [x] **No rate limit on `typing`, `activity`, `muted` and `chat_edit`**
      (`server/internal/hub/messages.go`).
- [x] **Upload right also allows downloads** from the desktop's Downloads
      folder (`NEKO_FILETRANSFER_USER_DOWNLOAD` in `compose.yaml`; the proxy
      allows every method on `api/filetransfer`).
- [ ] **neko admin token readable from the room desktop.** Firefox inherits
      `NEKO_SESSION_API_TOKEN` (`worker/supervisord.conf`); one token is
      shared by all rooms.
      Open: neko itself runs as the desktop user, so the token cannot be
      hidden from someone with the remote. What helps: one token per room,
      and room containers that cannot reach each other.
- [x] **Migration scripts.**
  - [x] The export archive is mode 0600 but the server runs as UID 65532:
        the import fails and the server restart-loops (`docs/migration.md`).
  - [x] `worker/import-home.sh` can replace the Firefox profile with a
        partial copy and still write the "imported" marker.
  - [x] `migrate/export-cozycast.sh` reports success when file copies fail.
- [ ] **Plain HTTP by default.** With `DOMAIN` unset, logins go over HTTP.
      Deployment setting, not code: set `DOMAIN` (or put TLS in front).

## 2. Worth fixing, not blocking

Sessions
- [ ] Logout / password reset does not close room sockets already connected.
- [ ] Passwords over 72 bytes pass validation, then fail with a 500 (bcrypt).
- [ ] The session cookie's 30-day lifetime does not slide with the database
      expiry.

Moderation and rights
- [ ] An IP ban only kicks the one identity, not other anonymous tabs from
      that IP.
- [ ] A failed neko profile update or member delete is logged, never retried.
- [x] A tab leaving while its neko member is being created left the member
      behind.
- [ ] Join and permission-refresh races can admit a user, or restore a right,
      just after it was revoked.
- [ ] Image right is checked, then the media is posted under a second lock.
- [ ] A failed join can still consume a limited invite use.
- [ ] Remote ownership is unenforced for a moment after a room restart.

Frontend
- [ ] The chat draft is cleared before the server accepts the message.
- [ ] The desktop does not reconnect after a neko outage unless a restart
      was announced.
- [ ] Resetting the stream to default does not switch current viewers.
- [ ] A mouse button stays held if released outside the desktop.
- [ ] Remote control traps keyboard focus (no way to Tab out).
- [ ] Mobile keyboard: single capital letters arrive lowercase.
- [ ] An unknown room shows endless loading.
- [ ] A deleted image or video stays open in the preview modal.
- [ ] `me` is not cleared when the session expires or another tab logs out.
- [ ] Desktop uploads cannot be cancelled and have no timeout.
- [ ] A cancelled avatar crop can still replace the avatar.
- [ ] Admins cannot change their own verified flag in the UI.
- [ ] Vite dev server does not proxy `/media`.

Housekeeping
- [ ] Expired anonymous bans are never deleted (`DeleteExpiredAnonBans` is
      unused).
- [ ] Orphaned media files (crash between rename and insert, failed unlink)
      are never reconciled.
- [ ] An open tab keeps chat beyond the one-hour / 1000-message retention.
- [ ] Deleting or replacing an imported avatar removes a file other imported
      accounts may share.
- [ ] Legacy avatar import cannot resume after a crash.
- [ ] JSON bodies: trailing data after the first object is accepted.
- [ ] Media rate limit runs after the upload was decoded, and returns 500.
- [ ] Shutdown does not close room sockets before the database.
- [ ] Clearing the screen setting does not restore the container default.
- [ ] neko observer / proxy calls have no handshake or header timeout.
- [ ] `X-Forwarded-For` is trusted as-is with `COZYCAST_TRUST_PROXY`
      (off by default).
- [ ] Account ids can be reused after deleting the newest account.
- [ ] `docs/architecture.md` says a restart loses open tabs; Firefox
      restores them.
