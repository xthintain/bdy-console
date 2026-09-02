# timeline-agents：生命周期、重组与 GC 契约

> 状态：设计稿（已与 `internal/bdynd/timeline/state.go` 对齐）
> 范围：只描述块的生命周期、重组/拆分/回收的操作步骤、触发条件与安全检查。
> 边界：引用块/索引时只使用抽象 ID（`block_id`、`node_id`、`object_id`），**不定义**块二进制格式字段，也**不定义** SQLite 表字段细节。
> 关联：块格式见 `block-format.md`，本地索引见 `index-schema.md`，集成边界见 `integration-contracts.md`。

## 1. 目标

在「少请求、大文件化、可精确拆分」的前提下，让存储随时间自动收敛：

- 新提交先落小节点，尽快可用。
- 达到阈值后向上合并（segment → archive），减少远端文件数。
- 历史被删除/重写后，回收不再被引用的数据。
- 所有优化动作都必须是**可重试、可恢复、先写新块后切指针**的。

## 2. 生命周期状态机（权威定义，对应 `state.go`）

块状态（统一、唯一）：

```text
pending      本地生成、尚未 flush
creating     正在组装或上传（中途失败可安全丢弃重来）
sealed       内容已最终化并通过校验，可被引用
active       被当前 timeline/ref 引用，是有效数据
frozen       已并入更大的块（归档），不再独立可变
superseded   被更新的重组块取代，仍在远端，等待回收
garbage      不再被任何引用，且已过宽限期
deleting     已发起远端删除
deleted      远端删除已确认
```

合法转换（实现于 `CanTransition`）：

```text
pending      -> creating | active | superseded | garbage
creating     -> sealed | garbage
sealed       -> active | superseded
active       -> frozen | superseded
frozen       -> superseded
superseded   -> garbage
garbage      -> deleting
deleting     -> deleted
```

各块类型允许的状态（实现于 `ValidateState`）：

```text
node       : pending | active | superseded | garbage | deleting | deleted
segment    : pending | creating | sealed | active | frozen | superseded | garbage | deleting | deleted
archive    : creating | sealed | active | superseded | garbage | deleting | deleted
checkpoint : creating | sealed | active | superseded | garbage | deleting | deleted
```

> 说明：GC 可达性（`block_states.reachable / mark / gc_round`）是**与生命周期正交**的另一维度，属于本地索引层，不在本状态机内。

## 3. 触发条件

### 3.1 flush（pending → creating → sealed → active）

- 条件：pending 节点数 >= `segment_size`，或用户显式 `flush`。
- 动作：把 N 个节点封装进一个 segment，写本地块，上传远端。
- 幂等：上传前校验 segment 内容哈希；重复 flush 同一组节点必须产生同一 segment（内容寻址天然保证）。

### 3.2 repack（segment → archive）

- 条件：
  - 连续 segment 数量达到 `archive_size / segment_size`；
  - 或用户显式 `repack`。
- 动作：读取一组 segment 的节点，重新封装为一个 archive，上传 archive 与其 index。
- 完成后：archive 标记 active，原 segment 标记 frozen → superseded。

### 3.3 compact（瘦身）

- 条件：
  - archive 内大量节点/对象已不再被任何 ref 可达；
  - 或对象去重后空间浪费明显（碎片评分超过阈值）。
- 动作：仅把 archive 中仍被引用的节点与对象写入新 archive，上传后切换引用。
- 结果：旧 archive 标记 superseded。

### 3.4 split（拆分）

- 条件：
  - archive 中只有少数节点还活着（例如分支被重写、历史被 prune 掉大部分）；
  - 或需要把冷热数据分置。
- 动作：从旧 archive 的 index 读取活节点，按新分组规则生成多个小 archive/segment。
- 结果：旧 archive 标记 superseded。

### 3.5 checkpoint（完整快照）

- 条件：距上次 checkpoint 达到 `checkpoint_interval`（默认 100 节点），或显式触发。
- 动作：从最新可验证状态生成完整项目树快照，上传。
- 结果：新 checkpoint 成为恢复基准。

### 3.6 prune（远端回收）

- 条件：块状态为 `garbage`，且超过 `gc_grace_period`（默认 7 天）。
- 动作：`garbage -> deleting -> deleted`，按安全删除清单删除远端文件。
- 结果：远端文件删除成功后，块状态改为 `deleted`。

## 4. 两阶段删除（安全前提）

任何远端块**不能**在满足以下全部条件之前被删除：

1. 替代块（repack/compact/split/checkpoint 产物）已上传到远端。
2. 替代块的 index 已上传且通过校验。
3. timeline 索引已更新，active ref 已指向新数据。
4. 旧块不再被任何 checkpoint / branch / tag / reachable node 引用。
5. 已超过宽限期（默认 7 天）。
6. 本地删除清单已持久化，可断点续删。

删除顺序（与上传顺序相反）：

```text
1. 上传新块
2. 上传新 index
3. 更新 timeline index
4. 更新 ref
5. （宽限期后）删除旧块
```

## 5. 安全检查

每次优化动作在 commit 前必须通过：

- **可达性**：新数据能被某个 ref/checkpoint 从根回溯到达。
- **完整性**：新块/新 index 的哈希与本地记录一致。
- **引用计数守恒**：没有任何 live 引用还指向即将 superseded 的旧块。
- **可重放**：优化过程记录的变更日志（operations）能重放得到相同结果，失败后可从中断点续跑。

### 失败重试

- 上传中途失败：丢弃半成品，`creating` 块不污染远端，重试重新组装。
- 校验失败：删除本地临时文件，重新下载/重新生成。
- 切换引用失败：不删除旧数据，保留 superseded 标记，下次运行继续。

## 6. 碎片评分与优化优先级

给每个 archive/segment 一个简单的碎片度：

```text
fragmentation = live_nodes / total_nodes          (节点利用率)
object_reuse  = live_objects / stored_objects      (对象复用率)
```

- 当 `fragmentation < 0.5` 或 `object_reuse < 0.4`，且空间收益超过阈值时，触发 compact。
- 优先级：
  1. prune 已 superseded 且过期的块（释放远端空间）。
  2. compact 高碎片 archive。
  3. repack 待合并 segment。
  4. 周期性 checkpoint（保证恢复基准不老化）。

## 7. 冷热分层（可选增强）

```text
hot  最近 pending/segment：小文件、高频访问、优先本地
warm 近期 archive：中等大小
cold 历史 archive：大文件、低频访问
```

- 冷数据优先合并成大 archive（减少请求数）。
- 热数据允许保留更多小块（提交快、恢复快）。
- 重组时把冷热块分开，避免频繁变动大文件。

## 8. 不变式汇总

- 永远 append/new，从不原地改写远端块。
- 先上传新块并校验，再切换 index/ref。
- 任何状态下 `pending + creating + sealed + active + frozen + superseded + garbage + deleting + deleted` 必须完整覆盖所有已知块。
- 所有优化动作可重放、可中断、可续跑。
- 回收永远是延迟的，宽限期默认 7 天。
- 生命周期转换必须走 `CanTransition` / `ValidateState`，不允许非法跳转。

## 9. 与其它契约的衔接

- 块格式契约提供每个块的 `index(offset,len,seq)` 与哈希，供 compact/split 精确定位活节点。
- 索引契约提供 `gc_runs` / `repack_runs` 状态表，供优化动作记录断点。
- 集成契约提供 `verify/flush/repack/prune` 的 CLI 触发面。
