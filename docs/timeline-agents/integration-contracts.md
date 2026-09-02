# timeline-agents：bdynd 集成边界与 CLI/remote 接口契约

> 状态：设计稿（已修订：RemoteTransfer 补全、timeline index 时序）
> 范围：定义 timeline 分层快照数据库如何接入现有 `internal/bdynd`、`internal/cli`、`internal/baidu`，明确包间接口边界与命令面。
> 边界：不定义 SQLite 表字段，不定义块二进制格式。那些见 `index-schema.md` 与 `block-format.md`。

## 1. 包职责划分

```text
internal/baidu         百度网盘 HTTP 客户端（传输原语）
internal/config        token/配置加载
internal/cli           命令解析、帮助、输出；临时只读策略
internal/bdynd         现有 Git-like 版本对象模型（commit/tree/blob/ref）
internal/bdynd/timeline  新的分层快照数据库（本契约的核心）
```

### timeline 子包对外暴露（概念层）

```go
// 块读写
type BlockReader interface{ ... }
type BlockWriter interface{ ... }

// 数据层元数据（对应 internal/bdynd/timeline/block.go）
type BlockMeta struct {
    ID, Kind, State, Size, SHA256, RemotePath, CreatedAt
}

// 生命周期操作（对应 internal/bdynd/timeline/state.go 的 CanTransition/ValidateState）
Flush(...)      // pending -> segment（creating -> sealed -> active）
Repack(...)     // segment -> archive
Compact(...)    // archive 瘦身
Split(...)      // archive 拆分
Checkpoint(...) // 生成完整快照
Prune(...)      // 延迟删除垃圾块（garbage -> deleting -> deleted）

// 恢复与校验
Verify(...)     // 校验一个块/节点完整性
Restore(...)    // 恢复到指定节点
```

### 与 bdynd 主包的边界

- bdynd 主包继续负责 `commit`/`add`/`status`/`log`/`branch` 等现有语义，生成**节点变更操作**（put/delete/chmod）。
- timeline 子包负责把这些操作按分层模型**封装成块、持久化、上传、索引、回收**。
- 二者通过一个窄接口衔接：

```go
type TimelineSink interface {
    AppendNode(node NodeMeta) error       // 接收一个 commit 节点
    Flush() error                          // 显式 flush
    VerifyNode(id string) error
    RestoreNode(id string) (string, error) // 返回恢复出的 tree 根或目录
}
```

bdynd 主包不感知块格式与远端布局，timeline 不感知 commit 的语义细节（只消费 node 元数据）。

## 2. 与 baidu remote store 的边界

现有 `internal/cli/nd_remote_help.go` 已定义 `RemoteStore` 接口：

```go
type RemoteStore interface {
    UploadFile(ctx, localPath, remotePath) error
    DownloadFile(ctx, remotePath, localPath) error
    Exists(ctx, remotePath) (bool, error)
    ListFiles(ctx, remoteRoot) ([]string, error)
    DeleteFiles(ctx, remotePaths []string) error
}
```

timeline 子包应依赖一个更窄的传输接口（避免与 bdynd 主包耦合，但**完整覆盖远端布局所需操作**）：

```go
type RemoteTransfer interface {
    UploadFile(ctx, localPath, remotePath) error
    DownloadFile(ctx, remotePath, localPath) error
    Exists(ctx, remotePath) (bool, error)
    ListFiles(ctx, remoteRoot) ([]string, error)
    DeleteFiles(ctx, remotePaths []string) error
}
```

- `internal/cli` 负责把 `baidu.Client` 适配成 `RemoteTransfer`。
- timeline 子包只调用 `RemoteTransfer`，不直接触碰 HTTP/token。
- `RemoteTransfer` 与现有 `RemoteStore` 的差异仅是去掉了 `List` 别名，保留 `ListFiles`/`DeleteFiles` 以支持 prune 的远端删除。

## 3. CLI 命令面

建议在 `bdy nd` 下新增子命令，保持现有命名风格：

```text
bdy nd timeline init                    初始化分层存储（一次）
bdy nd timeline status                  查看块/节点生命周期状态
bdy nd timeline verify [node|pack]      校验块完整性
bdy nd timeline flush                   把 pending 节点合并为 segment 并上传
bdy nd timeline repack                  把 segment 合并为 archive
bdy nd timeline compact                 瘦身碎片 archive
bdy nd timeline split <archive-id>      拆分稀疏 archive
bdy nd timeline checkpoint              生成完整快照
bdy nd timeline prune [--older-than 7d] 回收垃圾块
bdy nd timeline restore <commit>        恢复到指定节点
```

现有 `bdy nd commit` 自动调用 `TimelineSink.AppendNode`；`push/fetch/pull/clone` 在未来需要感知 timeline 布局。

### timeline index 文件时序（本轮补齐）

决策 Q73 的远端 `timelines/<branch>.index.json` 更新时序：

- 由 timeline 子包在**任何块上传成功后**负责重写该分支的 index（增量追加）。
- 更新顺序固定为：上传块 → 上传块 index → 更新 timeline index → 更新 ref。
- timeline index 内容包含：该分支从最近 checkpoint 起的所有 archive/segment/node 的定位（远端路径、offset、sha256）。
- 本地 SQLite 只是 timeline index 的缓存；远端 `timelines/<branch>.index.json` 是跨机器重建本地库的可信源之一。

### 临时只读策略

- 只读命令：`verify`、`restore`、`timeline status` 应允许在临时只读模式执行。
- 写命令：`flush`、`repack`、`compact`、`split`、`checkpoint`、`prune`、`init` 应被临时只读模式阻止。
- 需在 `internal/cli/cli.go` 的 `temporaryReadOnlyAllowsND` 增加对应分支。

## 4. 错误处理与输出契约

- 所有 timeline 命令失败时返回错误，由 `Run` 统一输出 `error: ...`。
- 进度信息写 `out`（stdout），诊断日志走 `Logger`（见 `logging.go`），互不混淆。
- 校验失败必须返回明确错误（如 `block <id> corrupt`），绝不静默返回残缺数据。

## 5. 接入顺序建议

1. timeline 子包提供核心概念与块读写（BlockReader/Writer）。
2. 接入本地 SQLite 索引（`index-schema.md`）。
3. 接入生命周期操作（`lifecycle-gc.md`）。
4. cli 增加 timeline 子命令与临时只读策略。
5. 把 `commit` 的节点变更接入 `TimelineSink.AppendNode`。
6. 把 `RemoteTransfer` 适配到 baidu 客户端，打通 push/fetch/pull/clone 与 prune 的远端删除。

## 6. 禁止跨越的边界

- timeline 子包**不**直接调用 `baidu.Client`（必须经 `RemoteTransfer`）。
- timeline 子包**不**修改 bdynd 主包的 commit/tree 对象格式。
- cli 适配层**不**实现块格式或 SQLite schema 细节。
- 任何包**不**写其它契约文件之外的文档/代码。
