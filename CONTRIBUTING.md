# Contributing to RDeck

Thanks for your interest in contributing! This document covers everything you
need to get started.

## Development setup

Requirements: **Go 1.25+**, **Node 22+**, [Wails CLI](https://wails.io):

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest

npm install --prefix frontend   # or let `wails build/dev` do it
wails dev                       # dev mode with hot reload
wails build                     # release build into build/bin/
```

## Tests

```bash
go test ./...                        # unit tests
go test ./... -count=1               # bypass cache
```

Integration tests in `internal/integration` run against a **real Redis on
localhost:6379** and skip automatically when none is reachable. To run the full
suite including them:

```bash
docker run -d -p 6379:6379 redis     # or use your local instance
go test ./... -count=1
```

Please include tests for bug fixes and new features. The integration suite is
the place for anything miniredis cannot emulate (DUMP/RESTORE, multi-section
INFO, ACL …).

## Architecture pointers

- `docs/GO_REWRITE_PLAN.md` — the full design & review document this codebase
  was built from; read it before large changes
- `internal/redisclient` — connection layer (dialers, SSH tunnel, sessions)
- `internal/keymodel` — per-type key loading/editing
- `internal/formatter` — value formatters & compression codecs
- `app/` — Wails bindings (thin facade over `internal/*`)
- `frontend/` — Vue 3 UI

Rules of thumb baked into the codebase (please keep them):

1. Values crossing the IPC boundary are **base64-encoded** (binary safety)
2. Console / MONITOR / PubSub use **dedicated raw RESP2 connections**, never
   the pooled client
3. SCAN pagination uses single-step cursors via `client.Do` — never go-redis'
   auto-looping `*Scan` helpers
4. go-redis stays on **RESP2** (`Protocol: 2`)
5. High-frequency events are batched (~200 ms) before `EventsEmit`
6. Go dependencies only: no new heavy runtime deps without discussion

## UI / i18n

- User-facing strings go through vue-i18n — add them to **both**
  `frontend/src/locales/en.json` and `zh_CN.json`
- Don't use `window.confirm` / `window.prompt` / `alert` — they are
  unreliable inside WKWebView; use the in-app dialogs

## Commits & pull requests

- Keep commits focused; use concise imperative messages
  (e.g. `fix: list pagination overlapping on load-more`)
- PRs: describe the *user-visible* change and how to verify it; link issues
- CI must pass on all three platforms (build + `go test ./...`)

## Reporting bugs

Open a GitHub issue using the bug template. Include: RDeck version, OS,
Redis version/mode (standalone/cluster/sentinel), steps to reproduce, and the
full error text shown in the UI.

## License

By contributing you agree that your contributions are licensed under the
[MIT License](LICENSE) that covers the project.
