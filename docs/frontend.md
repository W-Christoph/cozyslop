# Frontend

Preact + TypeScript + Vite in `web/`. The room (stream, controls, user list,
chat) is a port of CozyCast's
(`cozycast-server/npm-website/src/private/js/` in the old repository): same
layout, same behaviour, and by default the same look. Everything around it
(rooms page, login, admin area, and the windows opened from a room: personal
settings, room settings, files) has its own design, built from the shared
parts in `components/ui/`.

## Layout

```
web/src/
  main.tsx            entry
  app/App.tsx         router (preact-iso) and page shell
  app/state.ts        session (me), server settings, preferences, page title
  api.ts              fetch wrapper (api.get/post/...), shared response types
  room/protocol.ts    room WebSocket messages (mirror of server/internal/hub/protocol.go)
  room/store.ts       RoomStore: one joined room as signals + actions
  room/useRoom.ts     hook that owns a RoomStore for a component's lifetime
  neko/               neko protocol client (video, input); do not use from pages
  pages/              one component per route; pages/admin/ for the admin area
  components/         everything else, grouped in folders by area (room/, chat/, admin/, ...)
  components/ui/      shared parts of pages and windows: Icon, Dialog, SettingsLayout
                      (navigation left, section right), Section/SettingRow/ToggleRow,
                      Field/Input/Select/Checkbox, Switch, RadioCards, Slider, Badge,
                      Notice, Avatar, EmptyState
  components/settings/ the personal settings window (account, appearance, chat, room,
                      notifications); opened through `settingsOpen` in app/state.ts
  styles/tokens.css   design tokens (themes, accent colours)
  styles/base.css     element defaults only
web/public/           static files served at / (svg/, png/, audio/), same paths as CozyCast
```

## Conventions

- Function components, TypeScript strict. One component per file, file named
  after the component (`ChatInput.tsx`).
- **Styles**: every component that needs styling has a sibling CSS module
  (`ChatInput.module.css`) imported as `styles`. Class names camelCase. Only
  `tokens.css` and `base.css` are global. Shared style modules live in
  `components/ui/`; only `settings/ChatPreview` borrows component modules.
  Use tokens (below) instead of raw colours; spacing via `--space-*` where it fits. When porting, copy the
  relevant rules from the old `styles.css` into the module, drop dead rules,
  and keep the visual result identical.
- **Two sets of tokens**: the room uses the `--color-*` tokens carried over
  from CozyCast and the font `--font-sans`; pages and windows use the
  semantic ones (`--bg-*`, `--text*`, `--border*`, `--accent*`, `--danger*`,
  `--success*`) and `--font-ui`. A theme (`<html data-theme>`) defines both
  sets; the accent (`<html data-accent>`) is also the room's highlight
  colour (`--color-orange`).
- **Menus are windows**: anything with more than a couple of controls opens
  as a `Modal` (sizes `sm`, `md`, `lg`, `xl`) or, with several sections, a
  `SettingsWindow`, not in the chat sidebar. Buttons are `Button` (`variant="primary"` for the one main action,
  `ghost`/`danger`/`danger-outline`/`danger-ghost` otherwise); messages are `Notice`.
  Personal preferences apply as they are changed; forms that write to the
  server have a save button.
- **Chat display**: `preferences.chatStyle` (`classic`, `modern`, `compact`)
  is set as `data-chat-style` on the chat panel and styled in the chat's own
  modules; `settings/ChatPreview` draws with those same modules.
  Chat input and inline editing share mention suggestions: `@` at the start
  or after whitespace lists up to eight other room users, filtered by nickname
  (case-insensitive prefixes first, then substrings). Arrows select, Tab/Enter
  or a tap completes, and Escape dismisses. Mentions use nicknames with
  whitespace removed (`A Friend` becomes `@AFriend`), as the existing parser
  and notification sound expect. Nicknames the parser cannot mention (such as
  names containing `@` or parsed as links) are omitted. The accessible listbox
  appears above the textarea, inside the fullscreen element when present.
- **State**: app-wide state from `app/state.ts`; room state and actions from
  the `RoomStore` (pass the store down as a prop or via a context created in
  the room page). Local UI state with hooks. Signals are read with `.value`
  in render.
- **Server calls**: only through `api.ts`. `ApiError.message` is meant for
  users; show it as-is. Endpoints are documented in `docs/api.md`.
- **No `alert()`/`confirm()` for normal flow**: inline messages or the shared
  modal. A confirm step stays where CozyCast had one (deleting accounts,
  restarting).
- Links are plain `<a href>`; preact-iso handles navigation. Use
  `useLocation().route(path)` for programmatic navigation.
- Accessibility basics: buttons are `<button>`, inputs have labels, modals
  close on Escape and on backdrop click.
- Keep components small; split when a file passes ~200 lines.

- Room keyboard shortcuts are defined in `components/room/shortcuts.ts`;
  `useRoomShortcuts` matches keys and pauses them while typing, holding the
  remote or opening a window. Personal settings has a Shortcuts page.
  `room/RoomTooltip` supplies toolbar and upload tooltips, sharing the room
  surface and arrow styles with `UserCard` in `ui/RoomTooltipSurface.module.css`.

- Stream states share a picture, sentence-case title and optional detail on
  black. Connection loss stays inside the stream; chat is readable with sending
  disabled until reconnecting. Fullscreen errors clear after five seconds.
- Room URLs render the layout immediately, with the stream's loading state
  covering the session lookup, server connection and desktop connection.
  Joining waits for `/api/me` and any legacy login to finish; other pages keep
  the whole-page session loading screen.
- Portrait phones use one playback/chat toolbar row and a keyboard-accessible
  More menu for secondary actions. The stream follows the desktop aspect ratio,
  capped at 55% of the viewport; remote controls keep their own row. Touch message
  actions appear on tap and dismiss outside, reserving space only when visible.
  Personal settings carries the license footer, including on phones.
- The room sidebar contains only chat; its toolbar button shows unread messages
  while closed. Avatar cards show identity, remote and sound state
  on hover, focus or tap. The volume track shows its current level.

## Admin rooms

The Rooms tab at `/admin/rooms` lists room sources, connection state (Online,
Connecting, or Offline since a time) and people counts, refreshed every 10
seconds. Neko addresses are never shown: Change address starts empty. Admins can register a room, change its address, rotate
its token or remove it; configured rooms are read-only and refer to
`COZYCAST_ROOMS`. Add and New token show the secret only in the issuing modal,
with copy buttons for the token and the container's `COZYCAST_ROOM` /
`COZYCAST_NEKO_TOKEN` environment. Rotation requires restarting the container.
Removal confirms that viewers are disconnected and saved chat, settings and
permissions return when the name is registered again.

## Password reset links

Accounts in the admin area keep the direct password reset and add a Reset
link action. Its modal generates a link, allows copying it for private
out-of-band delivery, and explains the 24-hour lifetime, single use and
replacement of earlier links. It uses the same global admin right as direct
reset; room trust does not grant account management.

`/reset/:token` is public, even for an already logged-in visitor. It checks
the link, shows the account username and new/repeat password inputs, and uses
the shared form fields and notices. Invalid or expired links show an
InfoScreen asking for a new link from a moderator. Successful redemption
replaces the URL with `/login?passwordReset=1`, reloads cached account state,
and shows a short login confirmation. It does not create a session.

Tokens stay in component state and API POST bodies, with no local storage or
third-party requests. The HTML sets a no-referrer meta policy for SPA
navigation; the server also sends no-referrer, no-store and a same-origin
Content Security Policy on reset page loads.

## Old -> new names

### CSS variables (old `styles.css` -> `tokens.css`)

| old | new |
|---|---|
| `--cozyColor` | `--color-bg` |
| `--cozyColorMenu` | `--color-menu` |
| `--cozyColorAccent` | `--color-accent` |
| `--cozyButton` | `--color-button` |
| `--cozyButtonAccent` | `--color-button-accent` |
| `--cozyScrollbar` / `--cozyScrollThumb` | `--color-scrollbar` / `--color-scroll-thumb` |
| `--cozyHighContrast` | `--color-high-contrast` |
| `--cozyOrange` | `--color-orange` |
| `--cozyTextColor` / `--cozyTextColor70` / `--cozyTextMisc` | `--color-text` / `--color-text-muted` / `--color-text-misc` |
| `--cozyMessageBackground` / `--cozyMessageShadow` / `--cozyMessageHover` | `--color-message-bg` / `--shadow-message` / `--color-message-hover` |
| `--cozySvgFilter` / `--cozyIconChange` | `--icon-filter` / `--icon-invert` |
| `--cozycast-noise` | `--noise` |
| theme classes `defaultDesign` / `legacyDesign` / `lightDesign` | `<html data-theme="default|legacy|light">` (shown as Midnight, Onyx, Light; `dark` = Graphite is new) |

### Old server API -> new

| old | new |
|---|---|
| JWT in localStorage, `authFetch` | session cookie; plain `api.*` calls |
| `GET /api/profile` | `GET /api/me` (`{user: Me | null}`) |
| `POST /api/profile` | `PATCH /api/me` |
| `POST /login`, refresh tokens | `POST /api/auth/login`, `POST /api/auth/logout` |
| `POST /register` | `POST /api/auth/register` (logs in on success) |
| `GET /api/misc`, `POST /api/misc/*` | `GET /api/settings`, `PUT /api/admin/settings` |
| `GET /api/profile/all`, `POST/DELETE /api/profile/{u}` | `/api/admin/users...` |
| `/api/permission/...` | `/api/admin/permissions...` |
| `/api/invite/new|all|{code}` | `/api/admin/invites...` |
| `/api/invite/check/{c}`, `/api/invite/access/{c}`, `/api/invite/use/{c}` | `GET /api/invites/{c}`, `POST /api/invites/{c}/redeem` |
| `GET /api/room` | `GET /api/rooms` |
| `DELETE /api/room/{name}` | removed: rooms come from server configuration |
| room WebSocket `/player/{room}` + `join` message | `/api/rooms/{room}/ws[?access=code]`, see `room/protocol.ts` |
| `localStorage` keys `userSettings`, `design`, `volume`, `muted` | one `preferences` object in `app/state.ts` |
| `banned-<room>` localStorage key | gone: bans are enforced by the server |

### Behaviour changes to reflect in the UI

- Registering logs you in. Invite links remember the code across the
  login/register detour.
- Admins register and manage room connections in the Rooms tab; container
  creation and startup remain separate.
- Bans and kicks arrive as a `kicked` message with a reason; show the reason.
- When the server says the desktop is offline, the video area says so ("It
  reconnects by itself when it is back. Chat still works.") and the store
  stops its token retries; `online` brings an immediate token request.
  Removing a room uses `not_found`; changing a registered room's connection
  uses `room_changed`, with a message to reopen the room. Both are terminal.
- Uploading files into the desktop is a separate permission (`rights.upload`);
  chat images need `rights.image`.
- Clipboard, for whoever holds the remote: Ctrl/Cmd+V pastes the local
  clipboard into the desktop (`control/paste`). A hidden textarea receives
  the browser's native paste event, including in Firefox; the paste chord
  bypasses Guacamole and resets held keys before sending the text. Clipboard
  pastes on desktop and mobile ask for confirmation by default, showing a
  whitespace-preserving preview (at most 2000 Unicode characters, with a
  count of the remainder); accepting sends the full text. Enter accepts,
  Escape/Cancel dismisses, and focus returns to the remote input. Keys do
  not reach the desktop while a dialog is open. Ordinary mobile typing
  remains immediate. "Don't ask again" disables `askBeforePaste`, also
  available as "Ask before pasting into the desktop" in Settings › Room.
  Like the other personal preferences, it persists in this browser's
  `localStorage`, without account sync. Anything copied on the
  desktop is written to the local clipboard (`clipboard/updated`). Copying
  out needs a focused tab and HTTPS or localhost; browsers refuse it
  otherwise and it silently does nothing.
