# bdynd Layered Timeline Verification

This file records the minimum verification closure for the layered timeline snapshot database plan.

## Scope

The current implementation is still at the design and scaffold stage. The verified closure is:

- Architecture decisions are documented.
- Project principles are documented.
- `internal/bdynd/timeline` package exists.
- Default sizing and lifecycle configuration are compiled and tested.
- Local layout helpers are compiled and tested.
- Request id and logging skeletons are compiled and tested.
- Existing bdynd/auth/config/CLI packages still build and test successfully.

## Verified Files

- `docs/bdynd-layered-timeline-decisions.md`
- `docs/project-principles.md`
- `internal/bdynd/timeline/doc.go`
- `internal/bdynd/timeline/config.go`
- `internal/bdynd/timeline/layout.go`
- `internal/bdynd/timeline/logging.go`
- `internal/bdynd/timeline/config_test.go`
- `internal/bdynd/timeline/logging_test.go`

## Commands

Minimum package closure:

```bash
go test ./internal/bdynd/timeline ./internal/bdynd ./internal/auth ./internal/config
go build ./cmd/bdy
```

Result:

```text
ok  baiduyunStorage/internal/bdynd/timeline
ok  baiduyunStorage/internal/bdynd
ok  baiduyunStorage/internal/auth
ok  baiduyunStorage/internal/config
build ok
```

Full repository test:

```bash
go test ./...
```

Result:

```text
?   baiduyunStorage/cmd/bdy [no test files]
ok  baiduyunStorage/internal/auth
ok  baiduyunStorage/internal/baidu
ok  baiduyunStorage/internal/bdynd
ok  baiduyunStorage/internal/bdynd/timeline
ok  baiduyunStorage/internal/cli
ok  baiduyunStorage/internal/config
ok  baiduyunStorage/internal/lfs
ok  baiduyunStorage/internal/repo
ok  baiduyunStorage/pkg/baidund
```

## Notes

A help text assertion was updated during verification so that the restored OAuth login help still includes the existing `SDK token` wording expected by the CLI tests. This preserves the dual auth model: OAuth device-code login and external SDK token import.

No remote Baidu Netdisk mutation was required for this verification step.

## 2026-09-02 全面成型验证（增量）

timeline 分层快照数据库已从脚手架推进到闭环可用：

- `Store` 新增生命周期方法：`VerifyBlock` / `RestoreNode` / `ReplayDeltas` / `BuildCheckpoint` / `UploadCheckpoint` / `Prune` / `RepackSegments` / `Status`（`internal/bdynd/timeline/lifecycle.go` + `db_query.go`）。
- CLI 新增 `bdy nd timeline` 命令面：`init/status/verify/flush/repack/checkpoint/prune/restore`（`internal/cli/nd_timeline.go`），并接入临时只读策略（status/verify/restore 只读，其余写操作被阻止）。
- `bdy nd commit` 自动调用 timeline sink（`ndCommitTimeline`）：commit 后节点自动落库。
- 端到端验证（真实仓库 + 真实远端）：
  - 单 commit：`timeline status` 显示 1 node + 1 active block；checkpoint 生成并校验通过；restore 还原出完整树。
  - 多 commit（2 次改同一文件）：checkpoint --upload 上传真实远端成功，restore 最新 commit 返回最新树，verify 通过，远端数据清理。
  - 5 commit：status 5 nodes，flush 0 pending，checkpoint 正常。
- 全仓 `go test ./... -race` 全绿；行数检查通过。
