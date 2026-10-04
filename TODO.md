# TODO

From the full-codebase review on 2026-10-03 (three Codex runs, every file
read; findings checked against the code). State as of the end of that day.

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

UI redesign of 2026-10-04: left over from the design reviews (a visual
critique, a code audit of the design system, a Codex review; the hands-on UX
and accessibility test was stopped early)
- [ ] Not looked at again after the last fixes: no screenshots were taken
      after the final batch (light-theme name colours, dimmed rows, the ban
      date width, the colour picker's focus ring).
- [ ] UX test never finished: keyboard order, focus and Escape in every
      window, and contrast after the fixes, were not re-measured.
- [ ] Unsaved profile changes (nickname, colour, new avatar) are dropped
      without a word when the settings section is switched or the window
      is closed.
- [ ] Changing the avatar is only the picture itself (hover text and a
      small badge); the critique wanted a visible "Change avatar" button.
- [ ] Permission tables: the row that adds a permission is still a table
      row (suggested: a toolbar above the table); the ban date is a plain
      date input (suggested: a chip that opens it).
- [ ] "Room not found" and the other kick screens have no site header and
      their own buttons ("Login"/"Home"), unlike the 404 page ("Log
      in"/"Back to rooms"); `KickedScreen` hand-rolls its links because a
      unit test looks for plain `<a>` elements.
- [ ] The Restart button looks neutral although its confirmation is red
      (suggested: a danger-outline `Button` variant).
- [ ] Admin "Server settings" has no page title; the other admin pages do.
- [ ] Messages in table cells and in the header menu are raw
      `role="alert"`/`role="status"` paragraphs, not `Notice`
      (`PermissionRow`, `CurrentRoomUserRow`, `RoomDefaultsRow`, `Header`,
      `AvatarChooser`); a compact `Notice` is missing.
- [ ] Focus is shown in three ways: `--focus-ring`, an accent border with a
      soft ring on inputs, and the browser's outline on text links; the
      ring's gap is always `--bg-surface`, also on other backgrounds.
- [ ] No type scale (0.82/0.85/0.87/0.9rem all serve as "small text"),
      radii of 5px and 6px outside the scale (a `--radius-sm` is missing),
      six different breakpoints (520-780px) with no shared definition.
- [ ] Duplicated CSS: the red hover of ghost delete buttons (four copies,
      wants a `Button` variant), the `.actions` module of the two settings
      forms, `.form`, `.label`, the close button, the spinner, and the
      cropper styles in `AvatarChooser` and `ScreenshotModal`. Four
      components import another component's module.
- [ ] Selectors that lean on another component's markup (`.actions > p`,
      `.toolbar > div`, `Whisper`'s `.form > label:first-child`,
      `:last-of-type` in `MessageGroup`); `!important` for the tinted table
      rows; class overrides on `Button`/`Input` that win only by import
      order.
- [ ] Two ways to say the same thing: `Modal` `compact` and `size="sm"`,
      `Button` `accent` and `variant="primary"`. Unused: `loggedIn`, some
      legacy tokens (`--color-select`, `--color-disabled`,
      `--color-orange-ping`), `public/svg/logosymbol.svg`,
      `image_upload_cat.svg`.
- [ ] Anonymous users are named five ways (`Anon(xxxx)`, `Anon xxxx`,
      `Anonymous (xxxx)`); use `userIdentity()` everywhere. "..." and "…"
      are mixed in loading texts.
- [ ] `CurrentRoomUserRow` and `BanModal` show the room's `UserAvatar`
      (legacy tokens) inside a window.
- [ ] In the room, untouched by the redesign: the chat picture preview
      (`MediaModal`) sits under the toolbar in fullscreen with see-through
      chat and has no focus trap; personal and room settings have the same
      icon in the toolbar; on phones the delete/edit buttons of a message
      are 18px.
- [ ] `tests/chat-browser.cjs` (optional browser test) waits for a "Live"
      label that no longer exists; it failed before the redesign too.

Housekeeping
- [ ] Orphaned media files (crash between rename and insert, failed unlink)
      are never reconciled.
- [ ] Legacy avatar import cannot resume after a crash.

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
