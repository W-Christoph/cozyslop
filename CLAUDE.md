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

- `worker/`: room image, unmodified neko XFCE image plus Firefox and VLC.
  Configure neko through environment variables in `compose.yaml`, never by
  patching neko.
- `server/`: Go. The server is the only neko admin; browsers get per-tab
  neko member tokens and reach neko only through the allowlisted proxy.
- `web/`: Preact + TypeScript + Vite. Colours and spacing come from
  `src/styles/tokens.css`; every component has its own `.module.css`.
- The old CozyCast code (`../` when checked out inside the cozycast repo) is
  reference only.

## Codex for grunt work

Codex CLI (reasoning effort **high**, full permissions) does bulk,
mechanical, clearly specified work: component ports, CSS splitting,
renames, boilerplate, tests, log digging. Claude scopes the task, writes the
brief, and reviews the result. Small edits: just do them yourself.

```bash
codex exec -C "$PWD" "$(cat brief.md)" < /dev/null
```

- Run via Bash with `timeout: 600000`, or in the background for long tasks.
  Split anything over ~10 minutes.
- Brief and report go in the scratchpad dir, not the repo. The brief must
  stand alone: goal, acceptance criteria, pointers, boundaries (never commit,
  push or spawn agents), and a report written to `<scratchpad>/report.md`.
- Read `git diff` and the report before trusting the result.
- Review: `codex review --uncommitted`.
- Never use effort `ultra` and don't enable `multi_agent`.
