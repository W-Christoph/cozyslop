# CLAUDE.md

## Conventions

- Read files before editing. Prefer small edits over rewrites.
- Be concise. Test before declaring done: `go vet ./... && go test ./...` in
  `server/`, `npm run build` in `web/`.
- Match the surrounding code: comment density, naming, idiom.
- User instructions override this file.

## Network access

- If a host is unreachable (firewall 403 "Approval required", blocked, or
  timing out), stop and ask the user to enable it. Name the exact host and
  port and say what it is needed for. Do not silently work around it
  (switching mirrors, vendoring, skipping the step) unless the user says to.
- Docker builds in the sandbox need the proxy:
  `docker build --network host --build-arg http_proxy=$HTTP_PROXY --build-arg https_proxy=$HTTPS_PROXY ...`

## Layout

- `worker/`: room image, unmodified neko XFCE image plus Firefox, VLC
  and Ristretto.
  Configure neko through environment variables in `compose.yaml`, never by
  patching neko.
- `server/`: Go. The server is the only neko admin; browsers get per-tab
  neko member tokens and reach neko only through the allowlisted proxy.
- `node/`: image of the agent (`server/cmd/node`) for a room on another
  computer (`compose.node.yaml`, `docs/home-hosting.md`).
- `web/`: Preact + TypeScript + Vite. Colours and spacing come from
  `src/styles/tokens.css`; every component has its own `.module.css`.