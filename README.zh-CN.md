# RDeck

[English](README.md)

Redis 桌面客户端，使用 Go 从零实现（Wails v2 + go-redis v9 + Vue 3）。

> Redis 是 Redis Ltd 的商标。RDeck 是独立开源项目，与 Redis Ltd 及 RESP.app / RedisDesktopManager 无关联。


## 功能

- **连接**：直连 / TLS（CA、客户端证书、跳过校验）/ SSH 隧道（密码、私钥、ssh-agent，含 Windows named pipe；TOFU host key 校验）
- **部署模式**：standalone / cluster / sentinel 自动检测
- **键浏览**：键空间 SCAN + namespace 分组虚拟滚动树、多 db 切换、过滤（含 per-db 历史记录）
- **值编辑**：string / hash / list / set / zset / stream 分页查看与行级增删改、TTL、重命名、删 key
- **格式化**：hex / hexdump / JSON / BASE64 / 二进制位串 / CBOR / MsgPack / PHP serialize / Pickle（协议≤2 只读）；gzip / zlib / lz4 / zstd / snappy / brotli + Magento session/cache 组合格式的压缩识别与透明解压/重压；Extension Server 外部格式化器
- **Console**：专用裸连接会话、命令补全、MONITOR、引号/转义解析
- **Server 面板**：INFO 指标卡与实时图表、SlowLog、客户端列表、PubSub
- **批量操作**：删除 / 跨连接复制（DUMP+RESTORE 保留 TTL）/ 批量 TTL / RDB 导入
- **安全**：Redis 密码与 SSH 密码默认存入系统钥匙串，不落盘
- **其他**：i18n（en / zh_CN）、暗色模式、RESP.app 连接配置一键导入、值 base64 二进制安全传输、事件节流

## 构建

> Ubuntu 24.04+ 提示：系统已无 `libwebkit2gtk-4.0-dev`，请安装 `libwebkit2gtk-4.1-dev` 并使用 \`wails build -tags webkit2_41\` 构建。

## 构建

```bash
# 依赖：Go 1.25+, Node 22+, Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

go test ./...     # 单元测试 + 真实 Redis 集成测试（无本地 Redis 自动跳过）
wails build       # 产物在 build/bin/
wails dev         # 开发热重载
```

CI 矩阵（macOS / Windows / Linux）见 `.github/workflows/build.yml`。

## 运行

- 设置目录：`~/.rdeck/`（`--settings-dir` 可覆盖），含 `connections.json`、`settings.json`、SSH `known_hosts.json`
- 密码存于系统钥匙串（service: `rdeck`）

## 实现说明

- 所有 value 经 IPC 传输时 base64 编码（二进制安全铁律）
- Console / MONITOR / PubSub 使用专用裸 RESP2 连接（`internal/resp`），不与连接池混用
- SCAN 分页采用单步游标裸发（`client.Do`），规避 go-redis helper 自动循环陷阱
- go-redis 固定 RESP2，与原版响应语义对齐

## 许可证

本项目代码以 [MIT](LICENSE) 许可发布。详见 [NOTICE.md](NOTICE.md)。
