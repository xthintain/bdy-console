# bdynd 分层时间线快照数据库 — 块格式契约

> 状态：设计稿（已修订：整块级校验升级 sha256，补对象区，标签与决策 Q72 对齐）
> 范围：仅定义 **BlockReader / BlockWriter** 对外必须支持的概念字段与磁盘字节布局。
> 约束：本契约**不依赖本地 SQLite schema**，也不规定索引表的内部结构；SQLite 只被当作「索引/定位」的可选实现之一，索引概念体现在每个块的 header 与 footer 中，具体落库方式由其它子任务负责。
> 设计目标：适配百度网盘的大文件化快照存储 —— **少请求、大文件批量承载、精确拆节点、强校验、可重组、可回收**。

---

## 1. 术语与层级

四级逻辑单位，由小到大：

- **NodeBlock（小块）** — 单个 commit 节点的增量快照。最小可精确定位/分发的单元。
- **SegmentBlock（中块 / Pack）** — 一组 NodeBlock 的顺序容器，默认含 5 个节点，内部仍可逐个拆出 NodeBlock。
- **ArchiveBlock（大块 / Archive）** — 一组 SegmentBlock 的顺序容器，默认含 100 个节点，是网盘侧的主文件粒度。
- **CheckpointBlock（检查点）** — 周期性的完整项目快照（默认每 100 个节点生成一次），作为恢复基准线。

**包含关系**（逻辑树）：

```text
CheckpointBlock
 ├─ ArchiveBlock(s)
 │    └─ SegmentBlock(s)
 │         └─ NodeBlock(s)
 └─ object_section（完整树所引用的全部内容对象）
```

物理上每个块是一个**自描述、可校验的记录串**。外层块在带上自己的首尾标签的同时，逐个序列化内层块的字面字节（"原文内嵌"，便于整包拉取后跳过内层边界直接定位到某个 NodeBlock）。

---

## 2. 通用帧结构（Frame）

所有块与块内记录共享同一个**帧（frame）**前缀，作为「标签 + 长度 + 校验」四件套的基础。帧分为 **文本头** 与 **二进制载荷** 两部分。

### 2.1 字节序与字号

- 一切多字节整数一律 **little-endian（小端）**。
- 长度与计数使用 **变长无符号整数（LEB128 / uvarint）**，与 Go `encoding/binary.Uvarint` 兼容；跨语言对接以「最多 10 字节、续位=最高位」为准。
- 字符串均以 **uvarint 前缀长度 + UTF-8 字节** 编码（length-prefixed string，记作 `LPSTR`）。

### 2.2 Frame 内核（Kernel）

每个帧/记录统一由以下内核字段构成，供 Reader 快速跳过与校验边界：

| 字段 | 编码 | 含义 |
| --- | --- | --- |
| `magic` | 固定 4 字节魔数（见下） | 标识帧类型，也是扫描锚点 |
| `kind` | 1 字节枚举 | 记录/块类型（Node/Segment/Archive/Checkpoint/…） |
| `flags` | 1 字节位掩码 | 见 §2.4 |
| `payloadLen` | uvarint | 载荷（不含本帧头）的字节数 |
| `payload` | `payloadLen` 字节 | 载荷本体 |
| `crc32` | 4 字节 | 对 `magic..payload` 全部字节的 CRC-32C（Castagnoli）校验，用于快速边界校验 |

**magic 常量**（四字节 ASCII，均以 `0xBD` 前缀保证不与普通文本混淆）：

```text
NODE  = 0xBD 0x0B 'N' 'B'      // NodeBlock frame
SEGM  = 0xBD 0x0B 'S' 'P'      // SegmentBlock frame
ARCH  = 0xBD 0x0B 'A' 'K'      // ArchiveBlock frame
CKPT  = 0xBD 0x0B 'C' 'K'      // CheckpointBlock frame
OBJ   = 0xBD 0x0B 'O' 'J'      // 对象载荷帧（object_section 内）
RECF  = 0xBD 0x0B 'R' 'C'      // 内部记录 / 子条目 frame
TRAIL = 0xBD 0x0B 'E' 'O'      // trailer / 结束标记 frame
```

> 文本前缀 `0xBD 0x0B` 非 UTF-8 合法连续出现的常见序列，任何用户文件正文中出现整段 magic 的概率可忽略；即使出现，帧内 `payloadLen` + `crc32` 双重自洽也会排除误判（见 §8 扫描策略）。

### 2.3 BEGIN / END 标签（与决策 Q72 对齐）

每个块的字节流以一对 **BEGIN** / **END** 文本标签包裹（延续纯文本可观测性，便于人工 grep 与最低成本的电文边界识别）：

```text
---- BDYDB-<NAME>-BEGIN <spec>/<version> <id> ----
<BINARY FRAME 序列>
---- BDYDB-<NAME>-END <id> <sha256hex> ----
```

- `<NAME>` 取 `NODE` / `SEGMENT` / `ARCHIVE` / `CHECKPOINT`。
- `BEGIN` 行与 `END` 行均为单行、以 `\n` 结尾；行内以空格分隔字段。
- `END` 行尾的 `<sha256hex>` 是**整块字节（含 BEGIN..END 之间全部内容）** 的 **SHA-256** 十六进制小写，用于不解析帧时快速自检整块完整性。
- 若块被截断（无 END 行），Reader 必须视该块**损坏**并按 §8 处理。
- 内层块以**原文（含其自身 BEGIN/END）** 写在父块载荷中，因此父块可整包搬运内部子块，无需解压。

### 2.4 flags 位掩码

| 位 | 值 | 含义 |
| --- | --- | --- |
| 0 | `0x01` | 载荷已压缩（压缩算法见 header 的 `compression` 字段） |
| 1 | `0x02` | 本记录为增量（delta），需基于前序节点回放 |
| 2 | `0x04` | 本记录为完整快照（适用于 Checkpoint / 独立 NodeBlock） |
| 3 | `0x08` | 本帧为对象载荷（object_section 内） |
| 4–7 | — | 保留，置 0 |

---

## 3. NodeBlock

### 3.1 用途

单个 commit 节点的增量变化。只记录「相对上一个节点的差异」，占用小、单个可精确获取。

### 3.2 记录格式

```text
---- BDYDB-NODE-BEGIN 1/0 <node_id> ----
<FRAME: magic=NODE>
    header: {
        node_id        : LPSTR   // 节点标识（commit hash）
        parent_node_id : LPSTR   // 父节点标识（首个节点为空串）
        project_id     : LPSTR
        author         : LPSTR   // 可有符号化作者
        ts_ms          : uvarint // 提交时间（Unix 毫秒）
        seq            : uvarint // 全局单调序号
        spec, version  : uvarint // 格式规格与版本
        compression    : byte    // 0=none, 1=zstd
        payload_len    : uvarint // 载荷字节数（未压缩）
        payload_sha256 : LPSTR   // 载荷 SHA-256（未压缩）
    }
    delta_ops        : 帧载荷（新增/修改/删除对象操作序列，见 §3.3）
    object_refs      : 帧载荷（辅助对象引用索引，见 §3.4）
    <FRAME: TRAIL>            // 结束标记帧
---- BDYDB-NODE-END <node_id> <sha256hex> ----
```

### 3.3 delta_ops 载荷

顺序的 diff 操作流，每条为：

```text
opcode : byte           // 1=upsert_path 2=delete_path 3=move_path 4=attr_change …
path   : LPSTR          // 相对项目根路径
value  : 可选 LPSTR     // upsert 时的新内容对象 id（sha256）/ hash
```

### 3.4 object_refs 载荷

一批 `object_id → 远端位置(offset,size) / sha256` 的索引项，供恢复时以少请求方式按需拉取内容。

---

## 4. SegmentBlock

### 4.1 用途

NodeBlock 的中层容器（默认 5 节点）。合并是为了减少网盘文件数，但**内部仍保留每个 NodeBlock 的独立帧边界**，可在不解整个包的情况下精确定位单个 NodeBlock。

### 4.2 记录格式

```text
---- BDYDB-SEGMENT-BEGIN 1/0 <segment_id> ----
<FRAME: magic=SEGM, flags=压缩位>
    header: {
        segment_id      : LPSTR
        archive_id      : LPSTR   // 归属的 ArchiveBlock id
        begin_seq       : uvarint // 首节点全局序号
        end_seq         : uvarint // 末节点全局序号
        node_count      : uvarint // 内含 NodeBlock 数
        spec, version   : uvarint
        compression     : byte
        payload_sha256  : LPSTR   // 载荷 SHA-256（未压缩）
    }
    index: {                       // 子块偏移表（小，不依赖外部索引）
        count  : uvarint
        per_entry: { node_seq: uvarint; offset: uvarint; len: uvarint; entry_sha256: LPSTR } × count
    }
    <FRAME: RECF> <NodeBlock 原文字节×1>      // 子块以内层 unwrapped 字节内嵌
    <FRAME: RECF> <NodeBlock 原文字节×2>
    …
    <FRAME: TRAIL>
---- BDYDB-SEGMENT-END <segment_id> <sha256hex> ----
```

> 因为 `index` 内保存每个 NodeBlock 相对本包开头的 `offset,len,entry_sha256`，Reader 只凭 SegmentBlock 即可 O(count) 或经二分跳读定位单个 NodeBlock，**无需任何外部 SQLite 索引**。外部索引只作为进一步的加速/缓存。

---

## 5. ArchiveBlock

### 5.1 用途

SegmentBlock 的外层容器（默认 100 节点），**网盘侧的主文件粒度**。整包拉取后内部可逐个解出 Segment → Node。

### 5.2 记录格式

```text
---- BDYDB-ARCHIVE-BEGIN 1/0 <archive_id> ----
<FRAME: magic=ARCH, flags=压缩位>
    header: {
        archive_id      : LPSTR
        project_id      : LPSTR
        prev_archive_id : LPSTR   // 前一个 archive，用于顺序回放
        begin_seq       : uvarint
        end_seq         : uvarint
        segment_count   : uvarint
        node_count      : uvarint // 本大块覆盖节点总数
        spec, version   : uvarint
        compression     : byte
        payload_sha256  : LPSTR   // 载荷 SHA-256（未压缩）
    }
    index: {
        count  : uvarint
        per_entry: { segment_seq: uvarint; offset: uvarint; len: uvarint; entry_sha256: LPSTR } × count
    }
    node_section: {
        <FRAME: RECF> <SegmentBlock 原文字节×1>
        <FRAME: RECF> <SegmentBlock 原文字节×2>
        …
    }
    object_section: {              // 本范围新增/被引用的内容对象（见 §5.3）
        <FRAME: OBJ, flags=对象载荷位> <object_id(LPSTR) + 载荷字节>
        …
    }
    <FRAME: TRAIL>
---- BDYDB-ARCHIVE-END <archive_id> <sha256hex> ----
```

### 5.3 object_section（对象区，本轮修正补齐）

- 存放本 archive 范围内**新增且此前未出现在更早 archive** 的内容对象（整文件 blob 或大文件 4MB chunk，均以 sha256 内容寻址）。
- 已在旧 archive 存在的对象不重复存放，恢复时按 `object_refs` 引用旧位置。
- 对象帧用 `magic=OBJ` + `flags=0x08`，载荷为 `object_id(LPSTR) + 原始字节`。
- 每个对象单独用 sha256 标识；恢复时逐个校验对象 sha256 后再拼装文件。

---

## 6. CheckpointBlock

### 6.1 用途

周期性完整项目快照（默认每 100 节点），作为恢复的基准线。恢复时：最近 Checkpoint + 其后连续 Node 增量回放。

### 6.2 记录格式

```text
---- BDYDB-CHECKPOINT-BEGIN 1/0 <checkpoint_id> ----
<FRAME: magic=CKPT, flags=完整快照位|压缩位>
    header: {
        checkpoint_id   : LPSTR
        project_id      : LPSTR
        base_seq        : uvarint // 该快照对应的节点全局序号
        tree_root       : LPSTR   // 完整目录树的根哈希
        object_index    : LPSTR   // 全量对象清单索引引用
        file_count      : uvarint
        total_bytes     : uvarint
        spec, version   : uvarint
        compression     : byte
        payload_sha256  : LPSTR   // 载荷 SHA-256（未压缩）
    }
    payload: {
        full_tree    : 完整目录树序列化（含每个对象 content hash / 远端位置）
        object_section: 本快照引用的全部内容对象（或指向既有 object_section 的索引）
        <FRAME: RECF> 可索引的 object 偏移表（供对象级精确定位）
    }
    <FRAME: TRAIL>
---- BDYDB-CHECKPOINT-END <checkpoint_id> <sha256hex> ----
```

---

## 7. BlockReader / BlockWriter 接口契约

不依赖 SQLite schema；以下为二者**必须支持**的概念字段集合。

### 7.1 BlockWriter

- `WriteNodeBlock(node NodeBlockHeader, deltaOps, objectRefs) (n int, err error)`
- `WriteSegmentBlock(seg SegmentHeader, nodeBlocks [][]byte) (n int, err error)`
- `WriteArchiveBlock(arch ArchiveHeader, segmentBlocks [][]byte, objects [][]ObjectRef) (n int, err error)`
- `WriteCheckpointBlock(ckpt CheckpointHeader, fullTree, objectIndex) (n int, err error)`
- 通用能力：自动计算 `payloadLen`、写 `magic/kind/flags`、追加 `crc32`、包裹 BEGIN/END 标签、整体 **SHA-256**、按 `compression`（0/zstd）压缩载荷。

### 7.2 BlockReader

- `ReadFrameHeader() (magic, kind, flags, payloadLen, err error)` — 用于快跳过。
- `NextBlock() (blockType, blockID, err error)` — 扫描流中的下一个完整块。
- `ReadNodeBlock() (hdr NodeBlockHeader, deltaOps, objectRefs []byte, err error)`
- `ReadSegmentBlock() (hdr SegmentHeader, index []OffEntry)`
- `ReadArchiveBlock() (hdr ArchiveHeader, index []OffEntry, objects []ObjectRef)`
- `ReadCheckpointBlock() (hdr CheckpointHeader, fullTree []byte)`
- `SeekToNode(nodeSeq uint64) error` — 经 `index` 跳读指定 NodeBlock。
- 通用能力：解析主 header、解析子块偏移表、按 offset/len 跳读子块、校验每帧 CRC-32C 与整块 **SHA-256**、解压（zstd）。

### 7.3 概念字段小结

| 字段 | 必填 | 说明 |
| --- | --- | --- |
| `id / *_id` | 是 | 各类块标识 |
| `parent_node_id / prev_archive_id` | 是 | 顺序回放链 |
| `begin_seq / end_seq / base_seq` | 是 | 全局单调序号区间 |
| `node_count / segment_count / file_count` | 是 | 计数器 |
| `index`（offset,len,seq,entry_sha256） | 是 | 内部精确定位表 |
| `payload_sha256`（帧载荷） + 整块 SHA-256（END 行） | 是 | 强校验（防残缺核心） |
| `crc32`（帧级） | 是 | 快速边界/扫描校验 |
| `compression` + flags 压缩位 | 是 | 载荷压缩 |
| `ts_ms / author / seq` | 可选 | 元数据 |

---

## 8. 扫描 / 校验 / 损坏恢复策略

### 8.1 扫描（Scan）

- 以 `magic`（含 `0xBD 0x0B` 前缀）为锚点做**滑动窗口扫描**：命中 magic 后尝试解析 `kind + flags + payloadLen`。
- **三重自洽校验**决定命中有效：① magic 完全匹配；② `payloadLen` 落在文件有效范围内；③ 帧 `crc32` 通过。
- 「文本 BEGIN 标签」仅用于人工可读与首轮候选定位；**真正边界判定始终依赖帧四件套**（标签 + 长度 + checksum + index），避免用户内容中的文本字样导致误切。
- 外层块：优先用 `END <id> <sha256hex>` 行快速验证整块；再结合帧索引精确。

### 8.2 校验（Validate）

- **帧级（快速）**：每帧 `crc32(magic..payload)`。
- **整块级（强）**：`END` 行尾 **SHA-256** vs 实际整块字节。
- **载荷级（强）**：`payload_sha256` vs 未压缩载荷实际哈希。
- **对象级（强）**：object_section 每个对象按内容重算 sha256 与 `object_id` 比对。
- **跨块/回放级**：`begin_seq..end_seq` 连续、`parent_node_id / prev_archive_id` 链一致、`node_count / segment_count` 与实际计数一致。
- Writer 每次写入都必须产出「长度字段与内容一致 + 双重 checksum 一致 + 序列连续」的合法块；任何不一致一律记为损坏。

> 校验强度分层：CRC-32C 只用于帧边界快速跳过；**块/载荷/对象/最终树均以 SHA-256 做最终强校验**，满足「残缺即丢弃重拉」的语义。

### 8.3 损坏恢复（Recovery）

分层降级恢复，尽量保留未损坏部分：

1. **整块可用**：帧与整块校验通过 → 直接正常读取。
2. **块尾截断**（有 BEGIN、缺 END / END 不匹配）：跳过该块，尝试内部已通过帧校验的 NodeBlock —— 用于抢救已提交节点数据。
3. **单帧损坏**：帧 `crc32` 失败 → 丢弃该帧载荷，回到 magic 锚点重新扫描找下一个有效块；被丢的部分记录为 `corrupt` 并报告。
4. **节点间回放断裂**：N 个连续合法块但序号不连续 → 从最近 CheckpointBlock 或最近的连续段重新建起后续回放；断点之前已校验成功的节点视为已恢复。
5. **索引失效**：`index` 表校验失败但各 NodeBlock 帧完好 → 退化为全文扫描重建索引（不做数据丢弃）。
6. **恢复基准**：优先「最近 CheckpointBlock + 其后连续 NodeBlock 增量回放」；若最近 Checkpoint 损坏则回退到更早 Checkpoint，直至得到一条完整且自洽的链。
7. **重组/回收（GC）**：Reshard 时按 §5/§6 的 `index` 精确定位子块、重组新 Archive/Segment；被回收的旧大块在确认所有依赖回放完成、且新块已写入并双重校验通过后，才允许标记删除（先写新、后删旧，保证可重试）。

---

## 9. 兼容性与扩展

- Magic 前缀 `0xBD 0x0B` + 3 字节 kind 预留充足命名空间；新增块类型只需注册新 kind 与 magic，Reader 对未知 magic 一律跳过并在扫描时继续滑动，因此可**前向兼容**旧 Reader 读取含新块类型的流。
- `spec / version` 双字段支持格式演进；Reader 遇到更高 `version` 时应拒绝高版本特性但保留低版本块可读。
- 变长整数 + 长度前缀字符串保证：只要一个帧被完整读出，就可精确 `Seek` 到下一个帧起点 —— 这是「可精确拆节点」与「少请求大文件」同时成立的结构前提。

---

## 10. 待决 / 边界

- 本契约定义字节布局与 Reader/Writer 概念契约，**不包含**：SQLite 索引表结构、百度网盘上传/分片协议、压缩编码细节、对象内容的业务序列化格式 —— 这些交由其它子任务，且必须以本契约的 `index / offset / crc32 / sha256` 字段为对接点。
- 具体压缩级别、默认打包粒度（5/100/100）可在实现期可配置化，不改变布局结构。
- zstd 的实现方式（Go 依赖 vs 外部二进制）在实现期决定。
