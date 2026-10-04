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
- [ ] Mobile keyboard on a real phone: capitals, autocorrect, Enter and
      Backspace. Text goes through neko's paste (Ctrl+V), so typing into
      the desktop's terminal is expected not to work.
- [ ] In a room's terminal, `sudo` is refused (`no-new-privileges`).
- [ ] Check what neko's `filetransfer/update` message contains for a viewer
      without the upload right. It is sent to every viewer on connect; if
      it lists the desktop's Downloads, filter it in the proxy.

## 2. Open: worth fixing, not blocking

Sessions
- [ ] Logout / password reset does not close room sockets already connected.
- [ ] The session cookie's 30-day lifetime does not slide with the database
      expiry.

Moderation and rights
- [ ] An IP ban only kicks the one identity, not other anonymous tabs from
      that IP.
- [ ] Join and permission-refresh races can admit a user, or restore a right,
      just after it was revoked.
- [ ] A failed join can still consume a limited invite use.
- [ ] A personal stream choice ("unless that feature is enabled"): a room
      setting that lets the proxy pass a viewer's own pipeline choice. Today
      every viewer is pinned to the room's stream (`docs/ideas.md`).

Frontend
- [ ] The chat draft is cleared before the server accepts the message.

Housekeeping
- [ ] Orphaned media files (crash between rename and insert, failed unlink)
      are never reconciled.
- [ ] Legacy avatar import cannot resume after a crash.
- [ ] Account ids can be reused after deleting the newest account.

## 3. Done on 2026-10-04

- [x] Stream settings offer only 16:9 resolutions.
- [x] Passwords over 72 bytes are refused (400) wherever a password is set,
      including the initial admin password; at login they are simply wrong.
- [x] Expired anonymous bans are deleted with the hourly session sweep.
- [x] JSON bodies with data after the first value are refused.
- [x] The chat rate limit is checked before a media upload is read, and
      answers 429.
- [x] Shutdown closes the room sockets and waits for them before the
      database closes.
- [x] A mouse button released outside the desktop is released on the
      desktop too; held buttons are released on blur and when the remote is
      lost.
- [x] The media preview closes when its message is deleted.
- [x] A cancelled avatar crop is discarded.
- [x] Admins can change their own verified flag.
- [x] Vite dev server proxies `/media`.
- [x] `docs/architecture.md`: Firefox restores its tabs after a restart.
- [x] The image right is checked at the moment a media message is posted.
- [x] Rooms boot with remote takeover off; the server turns it on for rooms
      without remote ownership once it has applied the room's settings.
- [x] The neko observer's WebSocket has a handshake and init deadline and a
      ping; a dead connection is noticed and reconnected.
- [x] Clearing the screen setting restores the default
      (`COZYCAST_DEFAULT_SCREEN`, from `SCREEN` in `.env`).
- [x] An imported avatar shared by several accounts is deleted only when
      the last of them lets go of it.
- [x] With `COZYCAST_TRUST_PROXY`, the client address is the last
      `X-Forwarded-For` entry (the one the proxy added), not the first.
- [x] An unknown room answers `kicked` / `not_found`; the page says "Room
      not found".
- [x] The desktop reconnects after a neko outage that was not announced
      (`neko_unavailable`).
- [x] Desktop uploads can be cancelled and time out after two hours.
- [x] `me` follows login and logout in other tabs, a 401 from any request
      and the page becoming visible again.
- [x] Mobile keyboard: all text is inserted through neko's paste, only
      Enter, Backspace and Delete are keys (capitals arrived lowercase).
- [x] Room containers run with `no-new-privileges`: no sudo on the desktop.
- Decided: an open tab keeps the chat it has, beyond the one-hour /
  1000-message retention.
- Decided: Tab stays with the desktop while holding the remote; a keyboard
  way out is an idea for later (`docs/ideas.md`).

## 4. Done on 2026-10-03

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
