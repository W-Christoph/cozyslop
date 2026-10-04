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
  `tokens.css` and `base.css` are global. Use tokens (below) instead of raw
  colours; spacing via `--space-*` where it fits. When porting, copy the
  relevant rules from the old `styles.css` into the module, drop dead rules,
  and keep the visual result identical.
- **Two sets of tokens**: the room uses the `--color-*` tokens carried over
  from CozyCast and the font `--font-sans`; pages and windows use the
  semantic ones (`--bg-*`, `--text*`, `--border*`, `--accent*`, `--danger*`,
  `--success*`) and `--font-ui`. A theme (`<html data-theme>`) defines both
  sets; the accent (`<html data-accent>`) is also the room's highlight
  colour (`--color-orange`).
- **Menus are windows**: anything with more than a couple of controls opens
  as a `Modal` or, with several sections, a `SettingsWindow`, not in the chat
  sidebar. Buttons are `Button` (`accent` for the one main action, `variant`
  `ghost`/`danger` otherwise); messages are `Notice`. Personal preferences
  apply as they are changed; forms that write to the server have a save
  button.
- **Chat display**: `preferences.chatStyle` (`classic`, `modern`, `compact`)
  is set as `data-chat-style` on the chat panel and styled in the chat's own
  modules; `settings/ChatPreview` draws with those same modules.
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
| `--cozySelect` | `--color-select` |
| `--cozyOrange` / `--cozyOrangePing` | `--color-orange` / `--color-orange-ping` |
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
- No "delete room" (rooms are configured on the server).
- Bans and kicks arrive as a `kicked` message with a reason; show the reason.
- Uploading files into the desktop is a separate permission (`rights.upload`);
  chat images need `rights.image`.
- Clipboard, for whoever holds the remote: Ctrl/Cmd+V pastes the local
  clipboard into the desktop (`control/paste`), and anything copied on the
  desktop is written to the local clipboard (`clipboard/updated`). Copying
  out needs a focused tab and HTTPS or localhost; browsers refuse it
  otherwise and it silently does nothing.
