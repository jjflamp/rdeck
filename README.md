# RDeck

[中文说明](README.zh-CN.md)

A Redis desktop client, built from scratch in Go — Wails v2 + go-redis v9 + Vue 3.

> Redis is a trademark of Redis Ltd. RDeck is an independent open-source project,
> not affiliated with or endorsed by Redis Ltd or the RESP.app / RedisDesktopManager project.


## Features

- **Connections**: direct / TLS (CA, client certs, skip-verify) / SSH tunnel (password, private key, ssh-agent incl. Windows named pipe; TOFU host-key verification)
- **Deployment modes**: automatic standalone / cluster / sentinel detection
- **Key browsing**: key-space SCAN + namespace-grouped virtualized tree, multi-db switching, filters (with per-db history)
- **Value editor**: paged viewing & row-level editing for string / hash / list / set / zset / stream, TTL, rename, delete
- **Formatters**: hex / hexdump / JSON / BASE64 / bit string / CBOR / MsgPack / PHP serialize / Pickle (protocol ≤ 2, read-only); compression detection & transparent decompress/recompress for gzip / zlib / lz4 / zstd / snappy / brotli + Magento session/cache wrappers; Extension Server external formatters
- **Console**: dedicated raw-connection session, command completion, MONITOR, quote/escape parsing
- **Server panel**: INFO cards & live charts, SlowLog, client list, PubSub
- **Bulk operations**: delete / cross-connection copy (DUMP+RESTORE, TTL-preserving) / bulk TTL / RDB import
- **Security**: Redis & SSH passwords stored in the OS keychain by default — never written to disk
- **Extras**: i18n (en / zh_CN), dark mode, one-click import of RESP.app connection configs, base64 binary-safe value transport, event throttling

## Build

```bash
# Requirements: Go 1.25+, Node 22+, Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

go test ./...     # unit tests + real-Redis integration tests (skipped when no local Redis)
wails build       # artifact in build/bin/
wails dev         # dev mode with hot reload
```

The CI matrix (macOS / Windows / Linux) lives in `.github/workflows/build.yml`.

## Running

- Settings directory: `~/.rdeck/` (override with `--settings-dir`), contains
  `connections.json`, `settings.json` and the SSH `known_hosts.json`
- Passwords live in the OS keychain (service: `rdeck`)

## Implementation notes

- Every value is transported base64-encoded across the IPC boundary (binary-safety rule)
- Console / MONITOR / PubSub run on a dedicated raw RESP2 connection (`internal/resp`), never on the pooled client
- SCAN pagination issues single-step cursors via `client.Do`, avoiding go-redis' auto-looping helpers
- go-redis is pinned to RESP2 to align reply semantics with the original client

## License

The code is released under the [MIT](LICENSE) license. See [NOTICE.md](NOTICE.md) for
third-party asset and trademark notices.
