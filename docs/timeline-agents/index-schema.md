# timeline-agents：本地索引数据库契约（SQLite）

> 本文件定义**本地 SQLite 索引契约**，服务对象为类 git 的 timeline 对象存储（nodes / refs / checkpoints / segments / archives / objects / block_states / operations）。
> **边界**：只定义数据库表、字段、状态机、查询输入输出与所需索引；**不定义块的二进制格式**（块载荷落盘于 `objects` 对象仓库文件，DB 仅存元数据与指针）。
> **状态机对齐**：块生命周期状态采用 `internal/bdynd/timeline/state.go` 的权威定义（`pending/creating/sealed/active/frozen/superseded/garbage/deleting/deleted`）。本契约只在其上叠加 GC 可达性等正交维度。
> 关联：块格式见 `block-format.md`，生命周期见 `lifecycle-gc.md`，集成边界见 `integration-contracts.md`。

## 角色与定位

本地库 = **远端对象的持久化信封（envelope）+ 纯缓存派生物**的混合体：

- **可重建（cache / 派生）**：凡是能仅凭 `objects`（不可变内容寻址载荷）+ `operations`（追加式可重放日志）重算出来的表/视图，都是缓存。丢库可全量重建，不丢权威。
- **不可重建（权威 / 远端事实）**：对象内容本身（块字节）、指向远端/上游的引用、需要人工或外部确认的状态机中间态。

目标状态机覆盖三类操作路径：**恢复（recovery）**、**GC（垃圾回收）**、**repack（重打包/去重压缩）**。

## 1. 表总览

| 表 | 类型 | 可重建 | 权威说明 |
| --- | --- | --- | --- |
| `nodes` | 节点（树/图顶点） | 部分 | 顶点集合由 operation 重放可重建，但 `metadata`（标注/别名）含本地人工信息不可全量重建 |
| `refs` | 命名引用（分支/标签/远端指针） | 部分 | 远程/上游引用为远端事实不可重建；本地移动指针可重建 |
| `objects` | 内容寻址对象（不可变 blobs/树/chunks） | ✅ 是 | 纯内容寻址，全可重建 |
| `segments` | 中块（NodeBlock 容器） | ✅ 是 | 由 objects + operations 切分可重建 |
| `checkpoints` | 恢复断点（完整快照锚点） | 部分 | 锚点可由 operations 重放重建；但"用户主动标记的 checkpoint"语义保留 |
| `archives` | 大块（归档批次） | 部分 | 归档清单可重建；归档后指向外部存储介质的指针为外部事实 |
| `block_states` | 块生命周期状态 + GC 可达性 | ✅ 是 | 生命周期与 GC 可达集可由节点图遍历重算，全可重建 |
| `operations` | 追加式变更日志（操作日志） | ❌ 权威 | 重建源头，不可自发重建 |

## 2. 权威与缓存判定规则

| 判定维度 | 规则 |
| --- | --- |
| 内容寻址（哈希）→ 值语义变化 | `objects` 的一切列均由内容哈希推导，视为缓存 |
| 追加式可重放 | 任何能被 `operations` 重放等价再生的行 = 缓存 |
| 外部事实/人工确认 | refs 的上游指针、archives 的外部介质定位、checkpoint 的人工标记 = 权威（不可重建） |
| 中间态（进行中） | GC/repack/recovery 的过程状态 = 权威（重建过程本身不能凭空再生"当前任务进度"） |

## 3. 表定义

### 3.1 `operations(t0)` — 追加式变更日志（权威源）

单写者追加，作为全库逻辑时钟与重建源。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `seq` | INTEGER PRIMARY KEY AUTOINCREMENT | 单调号，逻辑时间戳（REPLACE 不适用，禁止使用 UPDATE） |
| `op_type` | TEXT | `add_object` / `update_ref` / `move_ref` / `mark_checkpoint` / `start_gc` / `finish_gc` / `start_repack` / `finish_repack` / `archive` / `segment_split` / `segment_merge` / `transition_state` |
| `op_time` | INTEGER | epoch 毫秒 |
| `payload` | TEXT(JSON) | 操作参数（负载对象，不含块字节） |
| `range_lo` / `range_hi` | INTEGER 可空 | 影响的对象/段 id 范围（便于重放分段做增量索引） |
| `author` | TEXT 可空 | 标识发起方（本地/远端源），日志审计 |

**不变量**：`seq` 严格递增；`op_type` 一经追加不可修改；重放时按 `seq` 升序执行。

索引所需：

- `operations(seq)` 主键（默认）
- `operations(op_time)` — GC/repack 时效性裁剪
- `operations(op_type)` — 按类型重放/统计

### 3.2 `objects(o_id)` — 内容寻址不可变对象（可重建）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `o_id` | TEXT PRIMARY KEY | 内容哈希（sha-256） |
| `o_kind` | TEXT | `blob`（整文件）/ `chunk`（大文件分块）/ `tree` / `commit`（timeline 快照点） |
| `o_size` | INTEGER | 载荷字节数（不含头） |
| `ctime` | INTEGER | 首次写入 epoch ms |
| `ref_count` | INTEGER | 当前被引用计数（重建字段，维护由引用计数收账） |
| `pin` | BOOLEAN 默认 0 | 人工/配置钉住（阻止 GC 收回），**权威**（不可重建） |
| `archive_batch` | TEXT 可空 | 归属 archive 批次 id |
| `payload_path` | TEXT | 载荷文件相对路径（对象仓库；不放块字节进 DB） |

**不变量**：`o_id` 唯一；内容一经写入不可变；`ref_count` 应等于当前有效引用数（可由 `refs`+`nodes` 派生校验）。

索引所需：

- `objects(o_id)` 主键
- `objects(ref_count, pin)` — GC：找可回收（pin=0 且 ref_count=0）
- `objects(o_kind)` — repack：按类型分组同类对象批量压缩
- `objects(archive_batch)` — archive 检索/按批删除

### 3.3 `nodes(node_id)` — 顶点/树（部分可重建）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `node_id` | TEXT PRIMARY KEY | 逻辑顶点 id（对应 commit 节点） |
| `kind` | TEXT | `root` / `dir` / `item` / `alias` |
| `o_id` | TEXT 可空 | 指向 objects 的载荷（NULL=空/占位节点） |
| `parent_id` | TEXT 可空 | 父节点（NULL=根） |
| `json_path` | TEXT 可空 | 规范化 JSON 指针路径（可对树做前缀查询） |
| `metadata` | TEXT(JSON) | 本地标注/别名，**部分权威** |
| `seq` | INTEGER | 全局单调节点序号 |
| `ctime` / `mtime` | INTEGER | 创建/修改时间 |

外键：`o_id` → `objects(o_id)`；`parent_id` → `nodes(node_id)`（递归树）。

索引所需：

- `nodes(node_id)` 主键
- `nodes(parent_id)` — 子树遍历/递归删除，GC 可达性 BFS
- `nodes(o_id)` — 顶点→对象引用，GC 起始集收集器
- `nodes(json_path)` — 路径前缀恢复查询
- `nodes(seq)` — 时间线顺序恢复

### 3.4 `refs(ref_id)` — 命名引用（部分可重建）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `ref_id` | TEXT PRIMARY KEY | 引用名（如 `main`、`tags/v1`、`remote/upstream/main`） |
| `ref_type` | TEXT | `branch` / `tag` / `remote` / `local` |
| `target_o_id` | TEXT 可空 | 指向 objects（commit/checkpoint 锚点） |
| `target_node_id` | TEXT 可空 | 或指向 nodes 顶点 |
| `remote_src` | TEXT 可空 | 上游来源标识，**权威**（远端事实，不可重建） |
| `updated_at` | INTEGER | 最后指向时间 |

外键：`target_o_id` → `objects`；`target_node_id` → `nodes`。

索引所需：

- `refs(ref_id)` 主键
- `refs(ref_type)` — 分类枚举
- `refs(target_o_id)` — 引用可达性起点（GC 从所有 refs 目标做可达遍历）
- `refs(remote_src)` — 远程同步对照

### 3.5 `segments(seg_id)` — 中块（可重建）

中块是 NodeBlock 的分组容器（默认 5 节点），承载范围查询与分批处理。

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `seg_id` | TEXT PRIMARY KEY | 段 id |
| `start_seq` / `end_seq` | INTEGER | 覆盖的节点 seq 区间 [start, end] |
| `parent_seg_id` | TEXT 可空 | 合并/拆分来源段（NULL=原始段） |
| `o_ids` | TEXT(JSON) | 段内对象 id 清单（恢复时按序加载） |
| `checkpoint_id` | TEXT 可空 | 关联 checkpoint |
| `archive_batch` | TEXT 可空 | 若已归档，指向 archives |
| `state` | TEXT | 生命周期状态（见 `state.go`；segment 合法态：pending/creating/sealed/active/frozen/superseded/garbage/deleting/deleted） |

索引所需：

- `segments(seg_id)` 主键
- `segments(start_seq, end_seq)` — 范围恢复查询 / repack 挑选覆盖区间
- `segments(state)` — GC/archive 状态过滤
- `segments(archive_batch)` — 按批删除/读取

### 3.6 `checkpoints(cp_id)` — 恢复断点（部分可重建）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `cp_id` | TEXT PRIMARY KEY | 断点 id |
| `anchor_seq` | INTEGER | 对应节点 seq（重建基线） |
| `anchor_tree` | TEXT(JSON) | 断点时的树/节点快照摘要（用于恢复位置还原） |
| `kind` | TEXT | `auto` / `manual`（manual 为权威） |
| `created_at` | INTEGER | 创建时间 |
| `state` | TEXT | 生命周期状态（checkpoint 合法态：creating/sealed/active/superseded/garbage/deleting/deleted） |

**状态机**：`creating → sealed → active`；被更新的 checkpoint 取代后 `active → superseded → garbage → ...`；只有 `active`（或 `sealed` 已切换为 active）可作恢复锚点。

索引所需：

- `checkpoints(cp_id)` 主键
- `checkpoints(anchor_seq)` — 恢复时按 seq 定位重放起点
- `checkpoints(state, created_at)` — 清理失效断点

### 3.7 `archives(arch_id)` — 大块（部分可重建）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `arch_id` | TEXT PRIMARY KEY | 批次 id |
| `kind` | TEXT | `full` / `incremental` |
| `manifest` | TEXT(JSON) | 本批打包的对象/段清单（可重建） |
| `external_locator` | TEXT 可空 | 外部介质定位（网盘远端路径），**权威** |
| `seg_ids` | TEXT(JSON) | 本批冻结的段清单 |
| `state` | TEXT | 生命周期状态（archive 合法态：creating/sealed/active/superseded/garbage/deleting/deleted） |
| `created_at` | INTEGER | 创建时间 |

**状态机**：`creating → sealed → active`；`sealed` 表明外部介质已确认写入（置 external_locator 为权威点），之后不可就地变更；被 compact/split 取代后 `active → superseded → garbage`。

索引所需：

- `archives(arch_id)` 主键
- `archives(state, created_at)` — GC 过期批次清理
- `archives(kind)` — 增量/全量统计

### 3.8 `block_states(block_id)` — 生命周期状态 + GC 可达性（可重建）

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `block_id` | TEXT PRIMARY KEY | 对应 `objects.o_id` |
| `kind` | TEXT | 块类型（node/segment/archive/checkpoint） |
| `state` | TEXT | 生命周期状态（对齐 `state.go`） |
| `reachable` | BOOLEAN | 当前 GC 轮次可达性 |
| `ref_count` | INTEGER | 本轮入度计数 |
| `mark` | TEXT | `live` / `dead` / `candidate`（候选带 pin 豁免判定） |
| `gc_round` | INTEGER | 最近一次 GC 轮号 |
| `last_touched` | INTEGER | 最近可达判定时间 |

**不变量**：`reachable=1` 当且仅当存在一条从任 `refs` 目标出发、经 `nodes`/`segments` 引用链可达的路径。`dead` 且非 `pin` 且生命周期 `state ∈ {superseded, garbage}` 的对象进入回收候选集。生命周期状态转换必须满足 `CanTransition`（见 `state.go`）。

索引所需：

- `block_states(block_id)` 主键
- `block_states(gc_round, mark)` — 单轮 GC 的写入/清理
- `block_states(reachable)` — 扫出不可达候选
- `block_states(state)` — 生命周期过滤（prune 候选）
- `block_states(ref_count)` — 引用收账递减热点

### 3.9 `gc_runs` / `repack_runs` — 过程状态（权威中间态）

> 归入 operations 体系的"运行中任务"登记，**过程进度为权威**（不能靠重放再生"当前任务到了第几步"）。

`gc_runs(round, started_at, finished_at, status)`：`queued / scanning / marking / sweeping / done`。

`repack_runs(round, started_at, finished_at, status, verified_at, old_objs, new_objs)`：`queued / reading / rewriting / verifying / done`。

索引所需：

- `gc_runs(status)` / `repack_runs(status)` — 查"是否有进行中任务"，防并发重入
- `repack_runs(round)` — 幂等去重

## 4. 状态机汇总

| 实体 | 状态机（对齐 `state.go`） |
| --- | --- |
| 块生命周期 | `pending → creating → sealed → active → frozen → superseded → garbage → deleting → deleted`（按块类型裁剪合法态） |
| `checkpoints.state` | `creating → sealed → active → superseded → garbage → deleting → deleted` |
| `archives.state` | `creating → sealed → active → superseded → garbage → deleting → deleted` |
| `segments.state` | `pending → creating → sealed → active → frozen → superseded → garbage → deleting → deleted` |
| `block_states.mark` | `candidate ⇄ live / dead`（随 GC 轮翻转，`candidate` 经 sweep 转 `dead`） |
| `gc_runs.status` | `queued → scanning → marking → sweeping → done`（任一阶段可 fail→ 回 `queued` 重试） |
| `repack_runs.status` | `queued → reading → rewriting → verifying → done` |

## 5. 恢复查询（Recovery Queries）

目标输入：`checkpoint_id`（或 `refs` 名）+ 期望恢复到的 `seq`。输出：有序的 `objects` 载荷指针 + 重建后的树。

| 步骤 | 查询（伪 SQL / 意图） | 依赖索引 |
| --- | --- | --- |
| 取断点 | `SELECT * FROM checkpoints WHERE cp_id=? AND state='active'` | P(cp_id), P(state,created_at) |
| 定位重放起点 | `SELECT anchor_seq FROM checkpoints; SELECT * FROM operations WHERE seq >= ? ORDER BY seq` | P(seq) |
| 重放增量 | 从 `anchor_seq` 起到目标 `seq` 顺序重放 operations，更新 nodes/refs/block_states | operations(seq) 有序 |
| 组批拉对象 | `SELECT payload_path FROM objects WHERE o_id IN (...)` | P(o_id) |
| 按段恢复（长链） | `SELECT o_ids FROM segments WHERE start_seq<=? AND end_seq>=?` | P(start_seq,end_seq) |
| 重建树 | `WITH RECURSIVE t AS (SELECT * FROM nodes WHERE node_id=? UNION ALL ...) SELECT ...` | P(node_id), P(parent_id) |

## 6. GC 查询（Garbage Collection）

目标输入：无（全库可达性）或 `roots`（重启时的可用节点清单）。输出：`block_states.mark` 标记集 + 待删 `objects.o_id`。

| 步骤 | 查询 / 意图 | 依赖索引 |
| --- | --- | --- |
| 起始集（roots） | `SELECT target_o_id, target_node_id FROM refs`（全 refs） | P(target_o_id) |
| 引用链遍历 | `SELECT o_id FROM nodes WHERE parent_id=?` / `SELECT o_id FROM nodes WHERE o_id IN (...)` | P(o_id), P(parent_id) |
| 标记可达 | 对每对象写入 `block_states`（reachable=1） | P(block_id) |
| 扫候选 | `SELECT * FROM block_states WHERE reachable=0 AND gc_round=?` join `objects.pin=0` AND `state IN ('superseded','garbage')` | P(reachable), P(gc_round,mark), P(state) |
| 回收 | `DELETE FROM objects WHERE o_id IN (dead AND pin=0)` + 清理 block_states | P(o_id) |
| 收敛校验 | 检查 `objects.ref_count` 与 `block_states.ref_count` 一致 | P(ref_count) |
| 并发防重入 | `SELECT 1 FROM gc_runs WHERE status NOT IN ('done')` 抢锁 | P(status) |

## 7. repack（重打包 / 去重压缩）

目标输入：`seg_id` 区间 或 `o_kind` 分类。输出：重写后的对象集合 + 新旧映射；校验后提交新状态。

| 步骤 | 查询 / 意图 | 依赖索引 |
| --- | --- | --- |
| 挑选待重打包 | `SELECT * FROM segments WHERE start_seq BETWEEN ? AND ? AND state='active'` | P(start_seq,end_seq) |
| 取候选对象 | `SELECT * FROM objects WHERE o_kind=? AND archive_batch IS NULL` | P(o_kind), P(archive_batch) |
| 读取段对象 | `SELECT o_ids FROM segments WHERE seg_id IN (...)` | P(seg_id) |
| 写入新对象 | insert 新 `objects`（新 o_id），同时写 `repack_runs` 进度 | P(o_id) |
| 改写引用 | UPDATE refs/nodes 指向新 o_id（记录到操作日志，可重放） | P(target_o_id) |
| 校验 | 校验新旧对象内容等价（内容寻址保证） | — |
| 提交/幂等 | `UPDATE repack_runs SET status='done' WHERE round=?`；删旧对象走 GC | P(round) |

**重放安全**：repack 必须记 `operations`（`start_repack` / `finish_repack`），使新旧映射可被重放出来，保证重建不丢一致性。

## 8. 查询输入/输出契约（接口层）

| 操作 | 输入 | 输出 |
| --- | --- | --- |
| `recover.checkpoint` | `{cpId \| refName, targetSeq}` | `{objectsPayloadPaths[], nodesTree, hitSeq}` |
| `gc.run` | `{}`（或 `{roots[]}`） | `{round, collectedOid[], freedBytes, records}` |
| `repack.run` | `{kind? \| segRange?}` | `{round, oldOid[], newOid[], mapping}` |
| `archive.create` | `{segIds[], kind}` | `{archId, manifest, externalLocator}` |
| `segment.split` | `{segId, atSeq}` | `{newSegIds[]}` |
| `transition_state` | `{blockId, from, to}` | `{ok, reason}`（须满足 `CanTransition`） |
| 校验查询 | `{expect refCountConsistency}` | `{ok, diffs[]}` |

## 9. 写出与维护策略

- **单写者**：SQLite 默认串行写；所有 mutations 经由 `operations` 追加 + 受影响的缓存表同步更新，保证 crash 后能按 `seq` 重放补齐。
- **事务边界**：一次 operation 的"日志行 + 缓存表更新"在同一 `BEGIN/COMMIT`。
- **重建流程**：`truncate 全缓存表（objects/nodes/refs/segments/checkpoints/archives/block_states） → 按 operations.seq 全量/增量重放 → 校验 ref_count 收敛`；保留 `*.pin`、`archives.external_locator`、`refs.remote_src`、`checkpoints.kind=manual`（权威列）单独重建入口不走重放。
- **索引即契约**：§3 各表后的"索引所需"为必须落地的索引声明；恢复/GC/repack 三查询（§5–§7）只允许依赖这些已声明索引，不得出现未声明的临时全表扫描作为主路径。
