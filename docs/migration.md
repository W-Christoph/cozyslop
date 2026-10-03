# Move an existing CozyCast instance

Use an empty database on the new server. Import runs automatically before
initial admin creation; later starts skip it when accounts already exist.
Passwords keep working, and users log in once on the new server.

1. Copy this repository's `migrate/export-cozycast.sh` to the old server.
   From the old CozyCast checkout, run:

   ```bash
   ./export-cozycast.sh
   # If Docker needs root: sudo ./export-cozycast.sh
   ```

   You can also pass the old checkout path as the first argument. If the
   script cannot identify Postgres, set `POSTGRES_CONTAINER` to its running
   container name (with sudo, use `sudo env POSTGRES_CONTAINER=name ...`).

2. Copy the resulting file to `import/` next to the new `compose.yaml`,
   using a secure transfer such as `scp`:

   ```bash
   ssh new-server 'mkdir -p /path/to/cozycast-next/import'
   scp cozycast-export.tar.gz new-server:/path/to/cozycast-next/import/
   ```

3. On the new server, set up `.env` from `.env.example` (including
   `PUBLIC_IP` and `NEKO_API_TOKEN`). Configure the same room names in
   `COZYCAST_ROOMS` in `compose.yaml`, with a worker for each room.

   ```bash
   cp .env.example .env
   # Edit .env and configure rooms, then:
   docker compose up -d --build
   docker compose logs server
   ```

4. Check for `legacy import complete`, its counts, and any skipped items
   (`docker compose logs server`), and for `import-home:` lines in
   `docker compose logs room-default`. Check that you can log in. Delete `cozycast-export.tar.gz` from both
   machines; it contains password hashes. The mounted `import/` directory
   can stay empty. A failed database import rolls back and stops startup.

Migrated: accounts and bcrypt password hashes, nicknames, colours, admin
and verified flags, disabled/locked/expired account status, valid referenced
avatars, room access and permission defaults, remote ownership and pointer
centering, per-user permissions, invitations, trust, active bans, and valid
invite codes with their use counts and expiry. Duplicate permission rows
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
Large Downloads make a large archive; the script prints its size.

Not migrated: chat history, existing logins/refresh tokens, unused email and
password-expired flags, or stream settings (resolution, frame rate, codecs,
etc.; set them again in the room's admin settings). The global front-page message and
registration setting lived only in memory in the old server; set them again
through the new admin UI. Liquibase bookkeeping is not imported.

Outside Compose, set `COZYCAST_IMPORT` to the archive path. Leaving it unset
disables import.
