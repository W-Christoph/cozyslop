# TODO

From the full-codebase review on 2026-10-03 (three Codex runs, every file
read; findings checked against the code). State as of the end of
2026-10-05.

## 1. Open: when deploying

Nothing in the code blocks a deployment any more. These are steps and
checks on the real server.

- [ ] Set `DOMAIN` in `.env` (or put TLS in front). Without it, logins go
      over plain HTTP.
- [ ] Rebuild both images: `docker compose up -d --build`. The room image
      changed (`entrypoint.sh` derives the room's neko token,
      `import-home.sh`), and the room now needs `COZYCAST_ROOM` and its own
      network, as in `compose.yaml`.
- [ ] Migration: `sudo chown 65532:65532 import/cozycast-export.tar.gz`
      before the first start (`docs/migration.md`).
- [ ] Restart a room while people watch (`docker compose restart
      room-default`): the desktop comes back by itself, and in a room without
      remote ownership the remote can be taken over again.

## 2. Open: worth fixing, not blocking

Moderation and rights
- [ ] Join and permission-refresh races can admit a user, or restore a right,
      just after it was revoked. Same window: a room socket authenticated
      just before a logout can still join just after it.
- [ ] A failed join can still consume a limited invite use.
- [ ] A personal stream choice ("unless that feature is enabled"): a room
      setting that lets the proxy pass a viewer's own pipeline choice. Today
      every viewer is pinned to the room's stream (`docs/ideas.md`).

Frontend
- [ ] The chat draft is cleared before the server accepts the message.

UI: left over after the redesign follow-up of 2026-10-05
- [ ] The type scale in `tokens.css` names the sizes in use (13 steps,
      several a pixel apart); it does not yet reduce them.
- [ ] Phones: a message's edit/delete buttons are always shown at 32px with
      room kept free beside the text, so own messages (all of them for
      admins) are at least 36px high. Alternative: show them on tap.
- [ ] Fullscreen chat over the stream is not measured for contrast (there is
      no one background); only names too dark for a dark picture are
      lightened. A darker bubble (0.65) and white timestamps were tried and
      taken back as too far from the room's look.
- [ ] Checked in Chromium only (viewport and touch emulation, mocked
      server): no real phone, no Firefox or Safari, no screen reader.

Housekeeping
- [ ] Orphaned media files (crash between rename and insert, failed unlink)
      are never reconciled.
- [ ] Legacy avatar import cannot resume after a crash.

## 3. Done on 2026-10-05

Chat
- [x] A message keeps its author's picture after the author has left; a
      changed picture reaches the messages already shown.
- [x] Compact chat is not grouped: every message has its time and name, and
      wrapped lines start under the time.
- [x] Room settings are centred like the personal settings.

The leftovers of the UI redesign (four Codex runs, each reviewed)
- [x] Unsaved profile changes ask before a section switch, closing the
      window or logging out ("Keep editing" / "Discard").
- [x] A visible "Change avatar" button.
- [x] Kick screens have the site header and the 404 page's buttons ("Log
      in" / "Back to rooms").
- [x] "Server settings" has a page title; Restart is a `danger-outline`
      button.
- [x] Anonymous users are `Anon(xxxx)` everywhere (`userIdentity()`);
      loading texts use "…".
- [x] Windows show `ui/Avatar`, not the room's `UserAvatar`.
- [x] The media preview is above the toolbar in fullscreen, keeps the
      keyboard inside and gives the focus back (`useDialogFocus`, shared
      with `Dialog`).
- [x] Room settings have their own toolbar icon.
- [x] Permission tables: adding is a form above the table; the ban date is
      a chip that opens the date field, and picking a date turns the ban on.
- [x] A compact `Notice` replaces the raw alert paragraphs.
- [x] One focus outline for pages and windows (`[data-ui]` in `base.css`).
- [x] `--radius-sm`, type sizes from tokens, three layout breakpoints
      (560/640/780px).
- [x] Duplicated CSS moved to shared parts (`danger-ghost` button,
      `FormActions`, `Form.module.css`, `CloseButton`, `Spinner`,
      `Cropper.module.css`, `TableActions.module.css`); only `ChatPreview`
      imports another component's module.
- [x] No selectors on another component's markup, no `!important` for table
      rows; `Input` has `quiet`.
- [x] `Button accent` and `Modal compact` are gone (`variant="primary"`,
      `size="sm"`); unused state, tokens and SVGs deleted.
- [x] Screenshots of every page, window and dialog (dark and light, 1280
      and 390px), keyboard order, focus and Escape in every window, and
      contrast were checked; what failed is fixed: focus on opening a
      dialog, Tab in the header menu, keyboard cropping, phone settings
      that could not be scrolled, a long invite link on phones, muted text,
      dimmed rows and badges, control borders (`--border-control`), and
      chat name colours too close to the background (`chat/nameColor.ts`,
      all themes).
- [x] `tests/chat-browser.cjs` passes again; new optional browser tests:
      `design-system-browser.cjs`, `ux-browser.cjs`, `contrast-browser.cjs`.

## 4. Done on 2026-10-04

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
      Checked on a phone, capitals included. Text typed on a phone does not
      reach programs that do not paste on Ctrl+V (the terminal).
- [x] Room containers run with `no-new-privileges`: no sudo on the desktop.
      Checked in the running room.
- Checked in a browser: a mouse button released outside the desktop, "Room
  not found", logout following other tabs.
- [x] Account ids are never reused (`users.id` is AUTOINCREMENT; migration
      0005 rebuilds the table and skips ids still named in chat).
- [x] The session cookie is renewed whenever the database slides the
      session's expiry.
- [x] `cozycast reset-admin` (`docker compose run --rm server reset-admin`)
      sets the admin password from `.env`, for a lost one.
- [x] Logout, a password change and an admin password reset close the room
      sockets of the sessions they end (`kicked` / `session`).
- [x] Banning an anonymous user also removes the other anonymous users on
      that IP from the room.
- Checked in a browser: video and sound (PC, phone, Firefox), a stream
  change and "Server default" in the stream settings, upload and cancel,
  the media preview closing on delete, a cancelled avatar crop.
- Known, left alone: switching stream settings in quick succession can
  break the stream until the page is reloaded.
- Decided: an open tab keeps the chat it has, beyond the one-hour /
  1000-message retention.
- Decided: Tab stays with the desktop while holding the remote; a keyboard
  way out is an idea for later (`docs/ideas.md`).

## 5. Done on 2026-10-03

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
