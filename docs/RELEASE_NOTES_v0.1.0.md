# RDeck v0.1.0

First public release of RDeck — a Redis desktop client built from scratch in Go
(Wails v2 + go-redis v9 + Vue 3).

## Highlights

- **Connections** — direct / TLS / SSH tunnel (password, private key, ssh-agent
  incl. Windows named pipe), TOFU host-key verification; automatic
  standalone / cluster / sentinel detection
- **Key browsing** — key-space SCAN with namespace-grouped virtualized tree,
  all databases listed with key counts, per-db filter history
- **Value editor** — paged viewing & row-level editing for string / hash /
  list / set / zset / stream; TTL, rename, delete
- **Formatters** — hex, hexdump, JSON, BASE64, bit string, CBOR, MsgPack, PHP
  serialize, Pickle (protocol ≤ 2, read-only); transparent compression handling
  for gzip / zlib / lz4 / zstd / snappy / brotli and Magento session/cache
  wrappers; Extension Server support
- **Console** — dedicated raw RESP2 session, command completion, MONITOR,
  full quote/escape parsing
- **Server panel** — live INFO charts, SlowLog, client list, PubSub
- **Bulk operations** — delete, cross-connection copy (DUMP+RESTORE,
  TTL-preserving), bulk TTL, RDB import
- **Security** — passwords stored in the OS keychain, never written to disk
- **Desktop polish** — dark mode, i18n (English / 简体中文), keychain-backed
  secret storage, single binary per platform

## Notes

- Requires a Redis server ≥ 5.0 (streams); ReJSON / Bloom filter keys are
  shown metadata-only unless the modules are loaded server-side
- Values crossing the UI boundary are base64-encoded — binary content is never
  corrupted by the editor

## Thanks

Built with [Wails](https://wails.io), [go-redis](https://github.com/redis/go-redis)
and [Vue](https://vuejs.org). See [NOTICE.md](NOTICE.md) for asset and trademark notices.
