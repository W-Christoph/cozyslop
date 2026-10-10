# Import from the old CozyCast

The script that exported an old CozyCast instance is no longer part of this
repository (it was `migrate/export-cozycast.sh`; `git log` has it). The
server still imports an archive it made, `cozycast-export.tar.gz`, and
this is what happens with one. Between two servers of this project, see
"Moving the server" (accounts, chat, settings) and "Moving a room's desktop"
in the README: two separate files.

Use an empty database on the new server. Import runs automatically before
initial admin creation; later starts skip it when accounts already exist.
Passwords keep working. Browsers that were logged in to the old server stay
logged in when the new one runs at the same address (see
[Logins](#logins)); everyone else logs in once.

1. Copy the archive to `import/` next to the new `compose.yaml`,
   using a secure transfer such as `scp`:

   ```bash
   ssh new-server 'mkdir -p /path/to/cozycast-next/import'
   scp cozycast-export.tar.gz new-server:/path/to/cozycast-next/import/
   ```

   The archive is private (mode 600) and the server container does not run
   as your user, so hand the file to the server's user (uid 65532) on the
   new server. Without this the server stops at startup with
   `permission denied`:

   ```bash
   sudo chown 65532:65532 /path/to/cozycast-next/import/cozycast-export.tar.gz
   ```

2. On the new server, set up `.env` from `.env.example` (including
   `PUBLIC_IP` and `NEKO_API_TOKEN`). Configure the same room names in
   `COZYCAST_ROOMS` in `compose.yaml`, with a worker for each room.

   ```bash
   cp .env.example .env
   # Edit .env and configure rooms, then:
   docker compose up -d --build
   docker compose logs server
   ```

3. Check for `legacy import complete`, its counts, and any skipped items
   (`docker compose logs server`), and for `import-home:` lines in
   `docker compose logs room-default`. Check that you can log in. Delete `cozycast-export.tar.gz` from both
   machines; it contains password hashes. The mounted `import/` directory
   can stay empty. A failed database import rolls back and stops startup.
   A failed desktop import (for example a damaged archive) leaves the room's
   home folder as it was and is tried again on the next start.

## An archive without accounts, or without the desktop

The export could leave out the room desktop, or hold the desktop alone. A
desktop-only archive goes in `import/` under the same name once the server
has its accounts, followed by `docker compose restart room-default`. The
desktop import replaces the Firefox profile the new room has used until
then; files already on the new desktop are kept. A desktop-only archive has
no accounts: the server ignores it once it has accounts, and refuses to
start with it on an empty database. The import unpacks the archive and then
copies the desktop into the room's volume: up to twice the desktop's size
in Docker's storage, next to the archive itself.

## Logins

The old site kept each login as a refresh token in the browser's storage.
The export carries a SHA-256 of every token that was not revoked, never the
token itself, so the archive cannot be used to log in. When such a browser
opens the new site, it hands its token over once and gets a normal session
for it. On the new server the token is then spent. The browser keeps it:
if you go back to the old server, its users are still logged in there.

This needs the new server at the same address as the old one (scheme, host
name and port), because that is what the browser's storage is tied to. An
old site at `http://<ip>` and a new one at `https://<domain>` do not share
it. Like the old server's tokens, a carried-over login does not expire
while it is unused; a password change, a password reset or disabling the
account ends it like any other session. The old server kept at most the
three newest logins per account; older devices log in with their password.

## What is migrated

Migrated: accounts and bcrypt password hashes, nicknames, colours, admin
and verified flags, disabled/locked/expired account status, valid referenced
avatars, room access and permission defaults, remote ownership,
per-user permissions, invitations, trust, active bans, valid
invite codes with their use counts and expiry, and logins (see above). Duplicate permission rows
are merged; unlimited invites stay unlimited. Avatar copy failures are
reported and those accounts use the default avatar. Desktop upload
permission starts disabled.

Room desktop: the old room's home folder (`data/cozycast-worker/cozycast`,
shared by all old workers) goes to the `default` room on its first start:
your files and folders (Desktop, Documents, Downloads, ...) and the Firefox
profile with logins, cookies, bookmarks, extensions and open tabs. Hidden
config folders, caches and worker log/pid files stay behind: they belong to
the old desktop setup. This happens once; a marker in the room's home folder
(`.cozycast-imported`) prevents a second import, so the archive can stay in
`import/` until you delete it. To import again, remove the room's home
volume first (`docker compose down` then `docker volume rm <project>_room-default-home`).

Not migrated: chat history, revoked logins, unused email and
password-expired flags, or stream settings (resolution, frame rate, codecs,
etc.; set them again in the room's admin settings). The global front-page message and
registration setting lived only in memory in the old server; set them again
through the new admin UI. Liquibase bookkeeping is not imported.

Outside Compose, set `COZYCAST_IMPORT` to the archive path. Leaving it unset
disables import.
