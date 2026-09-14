## What

<!-- One or two sentences: what does this PR change? -->

## Why

<!-- Link the issue, or describe the motivation -->

## How to verify

<!-- Steps a reviewer can follow to see the change working -->

## Checklist

- [ ] `go test ./...` passes
- [ ] `go vet ./...` is clean
- [ ] User-facing strings added to both `frontend/src/locales/en.json` and `zh_CN.json`
- [ ] No `window.confirm` / `window.prompt` / `alert` in UI code
- [ ] Values crossing the IPC boundary remain base64-encoded
