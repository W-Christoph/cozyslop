# TODO

From the full-codebase review on 2026-10-03 (three Codex runs, every file
read; findings checked against the code). State as of the end of that day.

## 1. Open: when deploying

Nothing in the code blocks a deployment any more. These are steps and
checks on the real server.

- [ ] Push `main` (the review fixes are local commits).
- [ ] Set `DOMAIN` in `.env` (or put TLS in front). Without it, logins go
      over plain HTTP.
- [ ] Rebuild both images: `docker compose up -d --build`. The room image
      changed (`entrypoint.sh` derives the room's neko token,
      `import-home.sh`), and the room now needs `COZYCAST_ROOM` and its own
      network, as in `compose.yaml`.
- [ ] Migration: `sudo chown 65532:65532 import/cozycast-export.tar.gz`
      before the first start (`docs/migration.md`).
- [ ] Watch a room in a real browser: video and sound play, and the
      picture follows a stream change in the room settings. The server now
      carries neko's WebSocket itself; signalling was tested against neko
      3.1.6 and in headless Chromium, playback was not.
- [ ] Check what neko's `filetransfer/update` message contains for a viewer
      without the upload right. It is sent to every viewer on connect; if
      it lists the desktop's Downloads, filter it in the proxy.

## 2. Open: worth fixing, not blocking

Sessions
- [ ] Logout / password reset does not close room sockets already connected.
- [ ] Passwords over 72 bytes pass validation, then fail with a 500 (bcrypt).
- [ ] The session cookie's 30-day lifetime does not slide with the database
      expiry.

Moderation and rights
- [ ] An IP ban only kicks the one identity, not other anonymous tabs from
      that IP.
- [ ] Join and permission-refresh races can admit a user, or restore a right,
      just after it was revoked.
- [ ] Image right is checked, then the media is posted under a second lock.
- [ ] A failed join can still consume a limited invite use.
- [ ] Remote ownership is unenforced for a moment after a room restart.
- [ ] A personal stream choice ("unless that feature is enabled"): a room
      setting that lets the proxy pass a viewer's own pipeline choice. Today
      every viewer is pinned to the room's stream (`docs/ideas.md`).

Frontend
- [ ] The chat draft is cleared before the server accepts the message.
- [ ] The desktop does not reconnect after a neko outage unless a restart
      was announced.
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
- [ ] The neko observer's WebSocket has no handshake timeout or liveness
      check.
- [ ] `X-Forwarded-For` is trusted as-is with `COZYCAST_TRUST_PROXY`
      (off by default).
- [ ] Account ids can be reused after deleting the newest account.
- [ ] `docs/architecture.md` says a restart loses open tabs; Firefox
      restores them.

## 3. Done on 2026-10-03

- [x] Anonymous impersonation: the public `a:<id>` was the `cozy_anon`
      cookie; it is now a hash of it.
- [x] Kicked, banned or disabled users are out of the room at once; a
      closing socket gets one write timeout.
- [x] A browser that fell behind reconnects instead of being closed with
      the "kicked" code.
- [x] Rooms load their settings before the server listens; a failed load
      stops startup.
- [x] Chat GIFs: 100 million pixels over all frames, two decodes at a time.
- [x] Request bodies have a deadline per route; connections an idle timeout.
- [x] Every room message counts against a per-person rate limit.
- [x] One encoder per room: the server carries neko's WebSocket, pins every
      viewer to the room's stream (`neko.PinStream`) and moves viewers when
      the stream changes, including a reset to the default.
- [x] The upload right no longer allows downloads (only `POST` reaches
      neko's file transfer).
- [x] neko admin token, readable by whoever holds the remote (the desktop
      keeps its terminal and file manager, by decision), is contained; see
      "The neko admin token" in `docs/architecture.md`:
  - [x] the proxy only accepts neko tokens the server issued;
  - [x] one token per room, one Docker network per room;
  - [x] every 30 seconds, neko members the server does not know are deleted
        and changed profiles reset. This also repeats neko updates and
        deletions that failed.
- [x] A tab leaving while its neko member was being created left the member
      behind.
- [x] Migration: `import-home.sh` no longer imports half an archive,
      `export-cozycast.sh` fails when a copy fails, and the guide says to
      hand the archive to the server's user.
- [x] A deleted chat message shows "deleted" instead of vanishing.
