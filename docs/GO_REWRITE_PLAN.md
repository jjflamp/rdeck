# RESP.app (RedisDesktopManager 2022) Go 重写实现方案

> 基于对本仓库（C++/Qt6/QML 版 RESP.app 2022.5）的完整源码分析，制定使用 Go 语言重新实现的方案。
> 编写日期：2026-09-10

---

## 第一部分：现有项目分析

### 1.1 技术栈与规模

- C++ / Qt6（Widgets + Quick + Charts + Svg）/ QML，约 224 个源文件
- Redis 协议库 `qredisclient`（**闭源预编译**，公开仓库 2022-10 后移除 SSH 实现）
- Python 嵌入（pyotherside）：值格式化器 + RDB 解析
- 入口 `src/main.cpp`，主工程 `src/resp.pro`（目标名 `resp`）

### 1.2 分层架构

```
┌─────────────────────────────────────────────────────┐
│  QML UI 层 (src/qml)                                │
│  主窗口 app.qml：连接树 + SplitView 标签容器          │
│  (WelcomeTab / ServerActionTabs / ValueTabs /        │
│   Consoles) + 各种对话框                              │
├─────────────────────────────────────────────────────┤
│  QAbstractItemModel 层 (src/modules)                 │
│  connections-tree / value-editor / console /         │
│  server-actions / bulk-operations / common /         │
│  extension-server                                    │
├─────────────────────────────────────────────────────┤
│  应用模型层 (src/app, src/app/models)                │
│  Application / Events(信号总线) / ConnectionsManager │
│  ServerConfig / TreeOperations / key-models(9种)     │
├─────────────────────────────────────────────────────┤
│  Redis 协议库 qredisclient（闭源预编译 lib）          │
│  传输层 / 命令队列 / RESP 解析 / 集群路由 / SSH 隧道   │
│  + Python 嵌入 (pyotherside + src/py)                │
└─────────────────────────────────────────────────────┘
```

### 1.3 目录职责

| 目录 | 职责 |
|---|---|
| `src/app/` | Application 类（模型初始化、QML 注册）、Events 事件总线、qmlutils/jsonutils、qcompress（压缩支持） |
| `src/app/models/` | connectionsmanager、connectionconf（ServerConfig）、connectiongroup、configmanager、treeoperations |
| `src/app/models/key-models/` | Redis key 类型模型：string / hash / list / set / zset / stream / ReJSON / BloomFilter / unknown |
| `src/modules/connections-tree/` | 左侧连接树：model、operations 接口、items（server/database/namespace/key/loadmore）、内存统计 |
| `src/modules/value-editor/` | tabsmodel（打开的 key 标签页）、valueviewmodel、embeddedformattersmanager、syntaxhighlighter、largetextmodel |
| `src/modules/console/` | Redis 命令行 + 基于 `resources/commands.json` 的自动补全 |
| `src/modules/server-actions/` | serverstatsmodel：INFO / SlowLog / 客户端列表 / PubSub 监控 |
| `src/modules/bulk-operations/` | 批量操作：DELETE_KEYS / COPY_KEYS / IMPORT_RDB_KEYS / TTL |
| `src/modules/extension-server/` | 外部格式化器 REST 客户端（OpenAPI 生成） |
| `src/qml/` | 与 modules 一一对应的 QML 界面 |
| `src/py/` | Python 格式化器（binary/cbor/msgpack/phpserialize/pickle）+ RDB 解析（rdbtools） |
| `src/resources/` | commands.json（命令补全元数据）、图标/字体、i18n 翻译（zh_CN/zh_TW/es/ja/uk） |
| `3rdparty/` | qredisclient、pyotherside、lz4、zstd、snappy、brotli、simdjson、fakeit（多数子模块本地未 checkout） |

### 1.4 核心机制详解（Go 重写必须还原的点）

#### 连接管理
- `ConnectionsManager`（`src/app/models/connectionsmanager.h`）：多继承 ConnectionsTree::Model + BulkOperations::ConnectionsModel
- JSON 配置文件（configmanager）+ 连接分组（connectiongroup）
- Redis URL 解析（`parseConfigFromRedisConnectionString`）、连接导入/导出、测试连接
- SSH 密码运行时询问（`askForSshPassword` + AskSecretDialog），**不写入 connections.json**

#### 连接模式（Normal / Cluster / Sentinel）
- 通过 `INFO` 解析判断（`treeoperations.cpp:505` 的 `mode()`）
- Cluster：`CLUSTER SLOTS` → 槽位表 + CRC16 路由；`MOVED`/`ASK` 自动重定向（有重定向次数上限）；`overrideClusterHost` 处理内网地址映射；`getClusterKeys` 遍历所有 master 聚合 SCAN
- Sentinel：`SENTINEL masters` 解析出 master host/port 后重连，处理 sentinel 自身 AUTH

#### 命令执行（qredisclient 内部）
- 三条队列：`m_commands`（用户）/ `m_internalCommands`（SELECT/AUTH 等）/ `m_runningCommands`（已发送待响应）
- 非 cluster 模式下命令带 db 索引时自动先发 `SELECT`
- Pipeline：`Command::addToPipeline` 合并多条命令，默认外包 `MULTI/EXEC`，可关闭事务；响应逐条消费
- Pub/Sub 与 MONITOR：按 channel 分发 push 消息
- 异步基于 AsyncFuture（QFuture 风格），owner 销毁自动取消命令
- 兼容阿里云 `iscan`（SCAN 被禁用时替换）

#### 键空间扫描与树构建
- `SCAN MATCH pattern COUNT n`，scanLimit 默认 10000（`app/scanLimit` 设置）
- namespace 按 `:` 分隔符聚合（qredisclient 内置 `lua/namespace_scan.lua` 脚本）
- 树节点：server → database → namespace → key，含 "loadmore" 节点
- 内存统计：批量 `MEMORY USAGE` 聚合

#### 值分页加载（key-models）
- 抽象基类 `ValueEditor::Model`（`keymodel.h`）：loadRows / loadRowsCount / addRow / updateRow / removeRow / setKeyName / setTTL / filter，全部异步
- 每种类型绑定两条命令：
  - Hash = `HLEN` + `HSCAN key cursor COUNT n`（游标式）
  - Set = `SCARD` + `SSCAN`
  - ZSet = `ZCARD` + `ZSCAN`
  - List = `LLEN` + `LRANGE start end`（区间式）
  - Stream = `XRANGE`（自带 count 逻辑）
  - String / ReJSON / BloomFilter = 单值模式
- `rowcache.h` 缓存已加载行 + `m_scanCursor` 游标状态；`valueviewmodel` 用帧窗口（m_startFramePosition / m_lastLoadedRowFrameSize）配合前端 Pagination 翻页
- keyfactory 按 `TYPE` 结果实例化，识别 `ReJSON-RL`、`BF/CF`（布隆/布谷鸟）、`stream`、`unknown`

#### 格式化器体系（三套并存）
1. **QML 内置**（`src/qml/value-editor/editors/formatters/ValueFormatters.qml` + hexy.js）：
   Plain Text、HEX、HEX TABLE、JSON（pretty/minify）、BASE64→Text、BASE64→JSON
2. **Python 嵌入**（pyotherside，`src/py/formatters/`）：binary（bitstring 位串）、cbor、msgpack、phpserialize（PHP 序列化/Session）、pickle
   - 统一协议 `base.py`：返回 `[error, output, read_only, decode_format]`
3. **Extension Server**（2022.4+，`src/modules/extension-server/`）：REST/OpenAPI 外部格式化服务，支持返回 JSON 与 `image/*` 可视化（`docs/server_spec.yaml`、`docs/extension-server.md`）

#### 压缩识别与解压（qcompress）
- 魔数猜测 `guessFormat()` + compress/decompress
- 支持：GZIP、LZ4（含 RAW）、ZSTD、SNAPPY、BROTLI、GZIP_PHP
- Magento 专用组合格式：MAGENTO_SESSION_GZIP/LZ4/SNAPPY、MAGENTO_CACHE_*
- 编辑前自动解压展示，保存前重新压缩（`MultilineEditor.qml:137-160`）
- 无魔数算法支持手动选择并记忆上次选择

#### 批量操作
- `BulkOperations::Manager`（`bulkoperationsmanager.h`）：`DELETE_KEYS / COPY_KEYS（跨连接跨库）/ IMPORT_RDB_KEYS / TTL`
- RDB 导入：Python rdbtools 将 RDB 还原为 Redis 命令流灌入目标库（`operations/rdbimport.cpp` + `src/py/rdb/__init__.py`）
- 注意：**开源版没有通用 JSON/CSV 数据导出**，仅有单值另存文件 + 连接配置导入导出

#### Server 面板
- INFO 图表（ServerCharts）、SlowLog（ServerSlowlog）、客户端列表（ServerClients）、PubSub 监控（ServerPubSub）

### 1.5 扩展机制
- 无传统 IPC / 本地动态库插件（Native formatters 已 EOL）
- 三层扩展：① Python 嵌入格式化器；② Extension Server（HTTP REST）；③ qredisclient transporter 抽象（库级）

### 1.6 关键结论

原项目三大"重写难点"——闭源协议库（qredisclient）、Python 嵌入、闭源 SSH 隧道——在 Go 生态中全部有成熟替代；其中 SSH 隧道在开源版中本就缺失（`createTransporter()` 抛 `SSHSupportException`，见 `docs/install.md:51`），Go 版反而是**补强项**。真正的工作量大头在 **UI 层**。

---

## 第二部分：Go 实现方案

### 2.1 技术选型

| 维度 | 选择 | 理由 |
|---|---|---|
| **UI 框架** | **Wails v2**（Go + Web 前端）⭐推荐 | Go 方法直接绑定给前端调用、事件总线内置（EventsEmit/EventsOn）；UI 用 Web 技术复刻 QML 布局，工作量与还原度最平衡。单二进制交付，体积远小于 Qt |
| UI 备选 | Fyne / Gio / Tauri+Go sidecar / Electron | Fyne 纯 Go 但树/表格/富文本编辑器控件生态弱，复刻 RDM 重表格 UI 成本高 |
| **前端** | Vue 3 + Element Plus（或 React + AntD）+ Monaco Editor + ECharts | Monaco 解决大文本编辑 + JSON 高亮（对应 largetextmodel + syntaxhighlighter）；ECharts 对应 ServerCharts |
| **Redis 客户端** | **go-redis v9**（`redis.UniversalClient`） | 一套 API 覆盖 standalone/failover(sentinel)/cluster；**MOVED/ASK 重定向、slot 路由、cluster pipeline 分组由库原生处理**——qredisclient 最复杂的约 1500 行集群代码可整体删除 |
| **SSH 隧道** | `golang.org/x/crypto/ssh` | 以 SSH conn 实现 `DialContext` 注入 go-redis 的 `Dialer`；支持密码/私钥/ssh-agent（含 1Password），原生跨平台 |
| **TLS** | 标准库 `crypto/tls` | 对应 sslCaCertPath/sslPrivateKeyPath/sslLocalCertPath/ignoreSSLErrors，含 TLS-over-SSH（AWS ElastiCache） |
| **压缩** | 标准库 gzip + `klauspost/compress`(zstd) + `pierrec/lz4` + `golang/snappy` + `andybalholm/brotli` | 对齐 qcompress 全部格式 + 魔数猜测 + Magento 组合格式 |
| **格式化** | `fxamacker/cbor`、`vmihailenco/msgpack`、phpserialize 自实现（短小成熟）；pickle 降级为"协议 ≤2 基础类型只读"（见 2.5 风险 #2） | 取代 Python 嵌入，**不再需要内嵌 Python 运行时** |
| **RDB 解析** | `github.com/hdt3213/rdb` | 纯 Go RDB 解析器，遍历 + 还原命令，直接替换 rdbtools，导入性能更好 |
| **密钥安全存储** | `zalando/go-keyring`（系统钥匙串） | SSH 密码等敏感字段不落 connections.json，与原版策略一致 |
| **配置/设置** | JSON 文件（`~/.resp/` 或 `--settings-dir`） | 兼容原版 QSettings 目录结构便于迁移 |
| **i18n** | 前端 vue-i18n | ⚠️ 复用原版翻译文案受 GPLv3 约束，按发布意图决策（见 4.1-②） |
| **命令补全** | 复用 `src/resources/commands.json`（自用/开源）；闭源发布则从 redis-doc 重新生成 | ⚠️ 同上，GPLv3 合规（见 4.1-②） |
| **测试** | go test + miniredis（单测） | 对应 tests/unit_tests |

### 2.2 架构与目录设计

核心原则：**`internal/` 下所有 core 包不依赖任何 UI**，通过接口 + 事件总线与前端解耦（对应原版 `Events` 信号总线）。Wails service 层只是薄封装。

```
redis-go/
├── cmd/resp/main.go                 # Wails 入口
├── wails.json
├── internal/
│   ├── redisclient/                 # ← qredisclient 对应物
│   │   ├── config.go                #   ConnectionConfig（字段对齐 ServerConfig + toJsonObject 兼容）
│   │   ├── dialer.go                #   直连 / TLS / SSH隧道 三种拨号器
│   │   ├── tunnel_ssh.go            #   x/crypto/ssh 隧道（密码交互回调、agent、私钥）
│   │   ├── connection.go            #   Connection 门面：INFO 模式检测、db 列表、版本解析
│   │   ├── scanner.go               #   键空间 SCAN + namespace 按分隔符分组建树
│   │   └── pagedscan.go             #   HSCAN/SSCAN/ZSCAN 游标分页器（有状态）
│   ├── connections/                 # ← connectionsmanager / configmanager / connectiongroup
│   │   ├── manager.go               #   增删改 / 分组 / 导入导出 / Redis URL 解析 / 测试连接
│   │   └── store.go                 #   JSON 读写 + keyring 密钥存取（兼容原版 connections.json）
│   ├── keymodel/                    # ← app/models/key-models
│   │   ├── model.go                 #   KeyModel 接口（对齐 keymodel.h 语义）
│   │   ├── factory.go               #   TYPE 分发（ReJSON / BF / CF / Stream / unknown）
│   │   ├── string.go hash.go list.go set.go zset.go stream.go
│   │   └── rowcache.go              #   已加载行缓存 + scan cursor + 帧窗口状态
│   ├── formatter/                   # ← qcompress + src/py/formatters + ValueFormatters.qml
│   │   ├── compress.go              #   魔数猜测 + 7 种压缩 + Magento 组合格式
│   │   ├── builtin.go               #   hex / hexdump / json pretty-minify / base64
│   │   ├── binary.go cbor.go msgpack.go php.go pickle.go
│   │   └── extserver.go             #   Extension Server REST 客户端（保留兼容，docs/server_spec.yaml）
│   ├── tree/                        # ← connections-tree：树节点模型 / 过滤 / MEMORY USAGE 统计
│   ├── console/                     # ← console：命令执行 / 补全 / MONITOR
│   ├── serverstats/                 # ← server-actions：INFO 采集 / SlowLog / Clients / PubSub
│   ├── bulkops/                     # ← bulk-operations：delete / copy / ttl / rdb_import
│   │   └── rdbimport.go             #   hdt3213/rdb 解析 → RESTORE 回放（带进度事件）
│   ├── settings/                    #   全局设置（字体/字号/valueSizeLimit/scanLimit/locale…）
│   └── events/bus.go                #   事件总线 → Wails EventsEmit 推给前端
├── app/                             # Wails service 绑定层（core 的薄封装，对应 Q_INVOKABLE 面）
└── frontend/                        # Vue3
    └── src/
        ├── views/                   #   连接树 / ValueTabs / Console / ServerTabs / Settings
        ├── components/              #   ValueTable / Pagination / 各类编辑器 / 对话框
        └── formatters/              #   前端内置格式化器（对应 ValueFormatters.qml + hexy.js）
```

### 2.3 关键设计

#### ① 连接层（对齐 qredisclient 语义）

```go
type Connection struct {
    cfg    Config
    dialer Dialer                 // direct | tls | ssh(+tls)
    client redis.UniversalClient  // go-redis 按模式创建 Standalone/Failover/Cluster
    mode   Mode                   // 通过 INFO 解析判定: normal/cluster/sentinel
}

// SSH 隧道：以 ssh 会话为底层拨号器
func (d *SSHDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error)
```

- go-redis `ClusterClient` 自动处理 MOVED/ASK 重定向与按节点 pipeline
- Sentinel 用 `FailoverClient`（masterName + sentinel 密码），比原版手写 `SENTINEL masters` 更稳
- 阿里云 iscan 兼容：SCAN 报 unknown command 时降级 iscan

#### ② 分页扫描（保留原版游标语义）

- Hash/Set/ZSet：`*Scan(ctx, key, cursor, match, count)`；rowcache 保存 `scanCursor` + 已加载行；前端翻页按帧窗口（对应 `m_startFramePosition`）
  - ⚠️ **go-redis 陷阱（第二轮评审发现）**：go-redis 的 `HScan/SScan/ZScan` helper 内部会自动循环直到游标归零，与"单步游标分页"语义冲突（大 Hash 会被一次性扫完）。必须用 `client.Do(ctx, "HSCAN", key, cursor, "COUNT", n)` 裸发命令、自行解析 `[cursor, [values]]` 回复
  - ⚠️ go-redis v9 默认 RESP3，建议显式 `Protocol: 2` 与原版响应语义对齐（push 消息 / nil / map 类型在 RESP3 下形态有差异）
- List：`LRange(key, start, min(rowCount, start+count)-1)` 区间式
- Stream：`XRange` + count 分页 + 过滤器
- 键空间：`SCAN MATCH pattern COUNT n` + pipeline 批量 `TYPE`；namespace 在 Go 端按 `:` 分隔符分组建树（替代 namespace_scan.lua），受 scanLimit 限制 + "加载更多"节点
- 大值保护：保留 `valueSizeLimit`——超限只取前 N 字节 + "加载完整值"按钮

#### ③ 事件流（对应 C++ 信号）

```
Go core  --EventsEmit("value:loaded" | "tree:changed" | "bulk:progress"
                       | "console:output" | "pubsub:message" | "ssh:ask-secret")--> 前端
前端     -- 调用绑定方法 (OpenValueTab / RunCommand / RequestBulkOperation
                       / SaveConnections ...) --> Go core
```

#### ④ Formatter 统一接口（对应 base.py 协议）

```go
type Formatter interface {
    Name() string
    Decode(data []byte) (output string, format string, readOnly bool, err error)
    Encode(input string) ([]byte, error)
}
```

#### ⑤ RDB 导入（替换 Python rdbtools）

`hdt3213/rdb` 流式解析 → 按 db/类型/key 模式过滤 → `RESTORE key ttl serialized REPLACE` 回放（pipeline 批量 + 进度事件），支持跨连接导入。

### 2.4 实施路线图

| 阶段 | 内容 | 交付物 | 预估 |
|---|---|---|---|
| **P0 骨架** | Wails 脚手架、目录、事件总线、配置存储、设置持久化 | 可运行空壳 + 连接配置 CRUD | ~1 周 |
| **P1 连接核心** | dialer 三件套（直连/TLS/SSH）、go-redis 接入、模式检测、连接树 + SCAN/namespace、db 切换、测试连接 | 能浏览所有库的键树 | ~2 周 |
| **P2 值编辑器** | 9 种 key model、分页 rowcache、格式化器全家桶、压缩识别/重压缩、增删改行 / TTL / 重命名 | 核心日常可用 | ~2-3 周 |
| **P3 Console + Server 面板** | 命令执行、补全（复用 commands.json）、MONITOR、INFO 图表、SlowLog、客户端列表、PubSub | | ~1-2 周 |
| **P4 批量操作** | 删除 / 跨连接复制 / TTL / RDB 导入（RESTORE 回放） | | ~1 周 |
| **P5 打磨发布** | i18n、暗色模式、QuickStart、连接导入导出（兼容原版 connections.json）、Extension Server 设置、打包（mac/win/linux，Wails 单二进制） | 正式版 | ~1-2 周 |

核心 Go 代码量预估：8k–12k 行。

> **工期修正（评审后）**：上表为理想排期，实际按 ×1.5–2 估算，即 **3–4.5 个月**。
> 并先定义 MVP cut line：**MVP = P0 + P1 + P2**（能连接、浏览、编辑即可日常使用）；
> Extension Server 兼容、Magento 组合格式、pickle、RDB 导入全部后置为 P6+。

### 2.5 风险与对策

| # | 风险 | 对策 |
|---|---|---|
| 1 | UI 工作量占总工程 50%+ | 先做 P1/P2 可用骨架，对话框样式从简，后续迭代；复用 Element Plus 组件 |
| 2 | Pickle formatter 无完整 Go 实现 | 原版本就是只读（read_only=True）；Go 版明确降级：仅支持**协议 ≤2 的基础类型（dict/list/str/int），只读**，复杂对象降级 hex 展示；列为 P2 后置项，不阻塞主线 |
| 3 | Magento 组合格式冷门 | 实现极简单（两层包装），照 `src/app/qcompress.cpp` 抄魔数与流程即可 |
| 4 | 原版用户配置迁移 | `internal/redisclient/config.go` 字段名对齐 `ConnectionConfig::toJsonObject`，提供一键导入原版 connections.json |
| 5 | 大集群 / 大库扫描性能 | pipeline 批量 TYPE；SCAN COUNT 自适应；tree 增量事件推送而非全量刷新 |
| 6 | Wails v2 平台差异（Windows WebView2） | CI 矩阵打包验证；必要时升级 Wails v3 |

### 2.6 与原版的能力对照

| 能力 | 原版（开源部分） | Go 版 |
|---|---|---|
| SSH 隧道 | ❌ 闭源库才有 | ✅ 原生支持（密码/私钥/agent） |
| Cluster 路由/重定向 | ✅ 手写 ~1500 行 | ✅ go-redis 原生 |
| Sentinel | ✅ 手写 | ✅ go-redis FailoverClient |
| 值格式化 | Python 嵌入 + QML | ✅ 纯 Go，无 Python 依赖 |
| RDB 导入 | Python rdbtools | ✅ hdt3213/rdb，更快 |
| Extension Server | ✅ REST | ✅ 保留兼容 |
| 单二进制分发 | ❌ Qt 动态库打包 | ✅ Wails 单文件（mac/win/linux） |

---

## 第三部分：可行性评审与修订（2026-09-10）

评审结论：**方案整体可行，核心选型（go-redis v9 + x/crypto/ssh + Wails）不推翻**，但存在以下必须修改的设计缺陷与需要下调的预期。

### 3.1 必须修改的问题（正确性 / 架构级）

#### ① 二进制安全传输——接口层铁律
Wails/前端 IPC 走 JSON 序列化。Redis 的 value 是任意 `[]byte`（非法 UTF-8、二进制、压缩数据），若直接以 string 传给前端，JSON 编码时会被 UTF-8 replacement character 破坏，**用户一保存数据就损坏**——比功能缺失严重得多。

> **修订**：service 层强制约定——所有 value 字段以 **base64** 传输（前端解码展示 / 编码回传）。

#### ② Console / MONITOR / PubSub 必须使用专用单连接
go-redis 是连接池模型，`SELECT` 只在连接创建时生效。RDM 的 Console 支持手动 `SELECT n`、`MONITOR`、`CLIENT KILL` 等会话语义，用池化客户端会导致命令随机落在不同连接上，行为错乱。

> **修订**：Console、MONITOR、PubSub 各自持有**专用单连接**（go-redis 的 `redis.NewConn`）；Value 编辑器等才走池化 `UniversalClient`。目录中新增：
> ```
> internal/redisclient/session.go   # 专用单连接封装（console / monitor / pubsub）
> ```

#### ③ 多 db 切换策略
go-redis 客户端创建后 db 固定，而"16 个 db 随手切换"是 RDM 核心体验。

> **修订**：连接管理器维护 `(connID, dbIndex) → client` 映射：standalone 每库一个 client（TCP 连接池本身可复用），cluster 恒 db0 直接返回。P1 实现前必须明确此结构。

#### ④ 高频事件必须节流，否则 UI 卡死
MONITOR 每秒可推上百条、bulk 操作进度、INFO 采样——逐条 `EventsEmit` 会打爆 IPC。

> **修订**：core 侧加批量/节流层：约 100ms 窗口合并刷新；MONITOR 输出走环形缓冲 + 前端拉取分页。

#### ⑤ Console 命令解析器需 1:1 移植
原版 `RedisClient::Command::splitCommandString` 处理引号/转义/多行，否则 `SET key "a b"` 解析即错。列入 P3 工作项。

#### ⑥ Windows ssh-agent 走 named pipe
x/crypto/ssh 的 agent 默认 Unix socket；Windows 需连 `\\.\pipe\openssh-ssh-agent`（对应原版 2022.3 的 Microsoft OpenSSH 支持）。在 `tunnel_ssh.go` 实现时单列。

#### ⑦ Wails v2 现状风险
- v2 已进入维护模式，v3 长期 alpha——锁定 v2 最新稳定版起步，service 接口层做好隔离以便必要时平移
- **Linux 是 Wails 最弱平台**：依赖 webkit2gtk 系统库（版本碎裂），需 AppImage 打包踩坑；Windows 需处理 WebView2 loader
- 若 Linux 为主要目标平台，备选 **Tauri v2 + Go sidecar**（前端不变，代价是引入 Rust 工具链）
- 结论：macOS/Windows 优先时 Wails 仍是对的选择，CI 中加三平台打包矩阵

### 3.2 需要下调 / 修正的预期

| 项 | 原方案 | 修正 |
|---|---|---|
| 总工期 8–11 周 | 偏乐观（UI 占 50%+，含兼容迁移） | **×1.5–2，按 3–4.5 个月排期**，先定义 MVP cut line |
| MVP 范围 | P0–P4 全量 | **MVP = P0 + P1 + P2**；Extension Server 兼容、Magento 格式、pickle、RDB 导入后置为 P6+ |
| 密钥存储 | 与原版一致（connections.json 明文 + SSH 密码运行时询问） | **超越原版：默认 keyring 存所有密码**，老配置导入时迁移（原版明文存 Redis 密码是弱点） |
| 测试 | 仅 miniredis | 补充：单测 miniredis + 集成测试 docker-compose（standalone/cluster/sentinel 三套）+ 原版 connections.json 迁移用例 |

### 3.3 评审后确认可行的部分

| 判断 | 结论 |
|---|---|
| go-redis v9 替代 qredisclient | ✅ MOVED/ASK、slot 路由、cluster pipeline 均为库原生能力；HSCAN 游标分页语义与原版一致（均为顺序加载、不能跳页，行为对齐） |
| SSH 隧道（ssh.Dial + conn.Dial 注入 Dialer） | ✅ 成立，且补上原开源版缺失的能力 |
| hdt3213/rdb RDB 导入 | ✅ 流式解析 + RESTORE 回放，保留 object encoding，还原度高于原版 rdbtools |
| 纯 Go 格式化器（cbor/msgpack/php/binary） | ✅ 库成熟、工作量小 |
| 总体架构（core 与 UI 解耦 + 事件总线） | ✅ 方向正确，需补 3.1-②③④ 的组件边界 |

---

## 第四部分：第二轮评审补充（2026-09-10）

第二轮评审聚焦第一轮未覆盖的协议细节、法律合规与完整性缺口。结论：**核心架构不变，新增 5 项硬性约束与若干范围补充**。

### 4.1 硬性约束（新增）

#### ① go-redis SCAN 家族 helper 的"自动循环"陷阱
go-redis v9 的 `HScan/SScan/ZScan` 辅助方法**内部自动循环迭代直到游标归零**才返回——与 RDM 的单步游标分页语义冲突，误用会导致超大 Hash 被一次扫完、内存打爆。

> **修订**：分页统一用 `client.Do(ctx, "HSCAN", key, cursor, "COUNT", n)` 裸发命令，自行解析 `[cursor, [values]]` 回复。封装在 `internal/redisclient/pagedscan.go`，禁止在业务代码中直接调用 SCAN helper。

#### ② GPLv3 许可证合规（已确认本项目 LICENSE = GPL v3）
方案中"复用 `src/resources/commands.json`、复用 `translations/` 文案"有法律风险：复用 GPL 资源后项目构成衍生作品。

> **修订**：按发布意图三选一：
> - **纯自用**：无约束，按原方案复用
> - **开源发布**：项目整体遵循 GPLv3，资源直接复用
> - **闭源/商用发布**：必须重制资源——命令元数据从 redis-doc 重新生成、翻译重写、图标全部替换；且不得复制原仓库源码逻辑之外的 GPL 文件

#### ③ SSH host key 验证策略
x/crypto/ssh 默认拒绝未知主机——不定义策略 SSH 功能直接不可用（原版 libssh2 基本静默接受）。

> **修订**：实现"首次信任（TOFU）+ 指纹存储 + 主机密钥变更时告警确认"策略，指纹存于配置目录 `known_hosts`。

#### ④ SSH 隧道生命周期是有状态的
`SSHDialer` 不能无状态：需持有单个 `ssh.Client` 复用（避免每命令建隧道）、keepalive 心跳、断线自动重建（对应原版 transporter 的 reconnect 语义）。

> **修订**：`tunnel_ssh.go` 提供 `ensureSession()` + 互斥锁保护重建；redisclient 的重连逻辑统一挂在 dialer 层。

#### ⑤ RESP 版本固定为 RESP2
go-redis v9 默认 RESP3，push 消息 / nil / map 类型回复形态与原版（RESP2）有差异。

> **修订**：所有客户端显式 `Protocol: 2`，与原版响应语义对齐，降低边角 bug。

### 4.2 完整性补充（对照原版遗漏项）

| 遗漏项 | 补充位置 |
|---|---|
| 批量操作前"受影响 key 预览"（原版 preview 流程） | `internal/bulkops`：先 SCAN 出受影响 key 列表展示，确认后执行 |
| 新建 key 流程（原版 `NewKeyRequest` 信号 → AddKeyDialog） | P2 范围：`app/` 绑定层暴露 CreateKey，前端 AddKeyDialog 按 9 种类型分别渲染表单 |
| per-database filter 历史持久化（原版 filterHistoryTop10） | `internal/settings`：按 (connID, dbIndex) 存最近 10 条过滤模式 |
| RDB 导入 module 类型（ReJSON 等）RESTORE 失败 | `rdbimport.go`：RESTORE 报 unknown module 时跳过该 key 并在结果中报告，不中断导入 |
| Redis 版本兼容性声明 | 明确支持 **Redis ≥ 5**（stream 基线）；ReJSON / Bloom 过滤器依赖目标端已装模块，启动时能力探测，未装则隐藏对应编辑器 |

### 4.3 工程质量约定

1. **core 并发模型**：每个打开的 Tab 一个 state 对象 + `sync.Mutex`；所有对外事件经 channel 汇入 `events/bus.go`，禁止在回调里直接跨 goroutine 共享可变状态
2. **前后端事件契约 contract-first**：事件名与 payload 在 `frontend/src/api/types.ts` 统一维护 TypeScript 类型（与 Wails 生成的绑定对齐），防止字段漂移
3. **性能验收基准**：100 万 key 的树构建 < 3s（流式 + 前端虚拟滚动）；单页 1k 行加载 < 200ms；MONITOR 1000 msg/s 不丢不卡（节流层生效）
4. **可选后置（P6+）**：崩溃/错误上报（sentry-go 替代 Crashpad）、自动更新

### 4.4 第二轮评审结论

方案经两轮评审后进入可执行状态：第一轮修正架构边界（连接模型/事件/IPC），第二轮补齐协议细节与合规约束。后续开工顺序不变：**P0 骨架 → P1 连接核心 → P2 值编辑器（MVP 完成线）**。实现时优先阅读 `GO_REWRITE_PLAN.md` 中的 ⚠️ 标注条目，它们是实现者最容易踩的坑。

---

## 附录：参考文件索引

| 主题 | 原版文件 |
|---|---|
| 连接配置 | `src/app/models/connectionconf.{h,cpp}` |
| 连接管理/密钥询问 | `src/app/models/connectionsmanager.{h,cpp}`、`configmanager.{h,cpp}`、`connectiongroup.{h,cpp}` |
| 树/键扫描入口 | `src/app/models/treeoperations.{h,cpp}` |
| 值模型接口 | `src/modules/value-editor/keymodel.h`、`valueviewmodel.{h,cpp}`、`tabsmodel.{h,cpp}` |
| 具体数据类型 | `src/app/models/key-models/abstractkey.h`、`hashkey.cpp`、`listlikekey.cpp`、`stream.cpp`、`rejsonkey.{h,cpp}`、`bfkey.{h,cpp}`、`keyfactory.cpp`、`rowcache.h` |
| 压缩/编码识别 | `src/app/qcompress.{h,cpp}`、`src/app/qmlutils.{h,cpp}`、`src/qml/value-editor/editors/MultilineEditor.qml` |
| Python 格式化器 | `src/py/formatters/*`（base.py 定义 `[error, output, read_only, decode_format]` 协议） |
| 扩展服务规格 | `docs/server_spec.yaml`、`docs/extension-server.md` |
| RDB 导入 | `src/modules/bulk-operations/operations/rdbimport.{h,cpp}`、`src/py/rdb/__init__.py` |
| 构建依赖/SSH 线索 | `3rdparty/3rdparty.pri`（预编译库链接 `-lqredisclient -lbotan-2 -lssh2`）、`.gitmodules`、`docs/install.md:51` |
| qredisclient 源码 | 本地子模块为空，参考 https://github.com/uglide/qredisclient |
