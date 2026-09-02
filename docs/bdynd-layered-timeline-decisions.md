# bdynd Layered Timeline Pack Decisions

This document freezes the core technical decisions for the Baidu Netdisk backed snapshot database. It is intentionally scoped to architecture decisions only; implementation steps, APIs, and tests belong in the later plan tasks.

## Goal

`bdynd` should preserve the exact project state at every commit node while avoiding repeated full-project uploads. Baidu Netdisk should mostly store large immutable files, while local metadata provides precise lookup, verification, recovery, repacking, and garbage collection.

## Non-Goals

- Do not use Baidu Netdisk as a random-access database file.
- Do not require online file locking or remote transactional writes.
- Do not store dependency/cache directories by default.
- Do not make delta chains so complex that recovery depends on many fragile transformations.

## Q71: Snapshot Form

Decision: use a layered timeline model.

```text
CheckpointBlock -> complete project tree at a selected node
NodeBlock       -> one commit's delta from parent to current node
SegmentBlock    -> a group of NodeBlocks, default 5 nodes
ArchiveBlock    -> a group of SegmentBlocks, default 100 nodes
```

A commit node is logically a complete snapshot, but physically it is stored as a delta operation list plus content objects. Recovery uses the nearest checkpoint and applies later node deltas in order until the target node.

Node delta operations are:

```text
put     create or replace one file
 delete  remove one tracked file
chmod   update mode metadata when needed
```

Rename can be represented as `delete + put` in the first version. This is less compact than a native rename op, but it keeps recovery deterministic and simpler.

Ignored directories and files are excluded before diffing. The first version should reuse `.bdyndignore` rules and add recommended defaults such as `.git/`, `.bdynd/`, `node_modules/`, `vendor/`, `dist/`, `build/`, `target/`, `.cache/`, `*.log`, and secret-like files when configured.

## Q72: Block Boundary And Split Accuracy

Decision: every layer uses distinct begin/end labels, but machine parsing must depend on structured headers, lengths, offsets, and checksums rather than text scanning alone.

Layer markers:

```text
BDYDB-CHECKPOINT-BEGIN / BDYDB-CHECKPOINT-END
BDYDB-ARCHIVE-BEGIN    / BDYDB-ARCHIVE-END
BDYDB-SEGMENT-BEGIN    / BDYDB-SEGMENT-END
BDYDB-NODE-BEGIN       / BDYDB-NODE-END
```

Each record header must include:

```text
magic
version
record_kind
record_id
header_length
payload_length
payload_sha256
record_sha256 or footer_sha256
```

The labels make damaged packs inspectable and resynchronizable. The length fields make non-aligned node extraction exact. The checksums prove that extracted node/segment/archive data is complete.

When a requested node does not align with a segment or archive boundary, the system downloads the containing large block and uses the archive index plus NodeBlock boundaries to apply only the required node range. No logical accuracy is lost by storing multiple nodes in one big file.

## Q73: Remote Request Strategy

Decision: optimize for few remote requests by storing immutable large packs and small cached indexes.

Remote layout:

```text
/apps/baiduyunStorage/nd/repos/<repo>/
  refs/
    heads/main.json
  timelines/
    main.index.json
  checkpoints/
    checkpoint-main-000100.pack.zst
    checkpoint-main-000200.pack.zst
  archives/
    archive-main-000001-000100.pack.zst
    archive-main-000001-000100.index.json
    archive-main-000101-000200.pack.zst
    archive-main-000101-000200.index.json
```

Default sizing:

```text
segment_size = 5 nodes
archive_size = 100 nodes
checkpoint_interval = 100 nodes
large_file_threshold = 4 MiB
chunk_size = 4 MiB
compression = zstd
```

The normal restore path should require approximately:

```text
1 ref file
0-1 timeline index file when not cached
1 checkpoint pack
1 archive pack for the target range
```

If a target range crosses an archive boundary, the system may fetch more archive packs. The checkpoint interval should align with archive size to keep typical recovery to one checkpoint plus one archive.

## Q74: Integrity, Repack, Split, And Garbage Collection

Decision: all remote data files are immutable; optimization rewrites new blocks and then switches indexes/refs only after verification.

Upload order:

```text
1. upload new archive/checkpoint pack
2. upload its index
3. update timeline index
4. update branch ref last
```

Recovery verification order:

```text
1. verify pack sha256
2. verify contained segment/node sha256
3. verify object sha256
4. materialize files
5. verify final tree_hash_after
```

If any verification fails, delete the local temporary file and redownload. Never expose a partially verified restore result as a valid project state.

Block lifecycle states:

```text
pending     local-only node/segment not flushed yet
active      referenced by current timeline/index
superseded  replaced by a newer compacted/repacked block
garbage     unreferenced and past grace checks
deleting    delete requested from remote
deleted     remote delete confirmed
```

Repack rules:

- Pending NodeBlocks become SegmentBlocks when the segment reaches `segment_size` or on explicit flush.
- SegmentBlocks become ArchiveBlocks when enough contiguous segments exist.
- ArchiveBlocks may be compacted when many contained nodes or objects are no longer referenced.
- ArchiveBlocks may be split when only a small subset of nodes remains live or when a branch history rewrite makes ranges sparse.
- Repack, compact, and split never modify an existing remote pack in place.

Prune rules:

- Old blocks are deleted only after their replacements are uploaded, indexed, verified, and referenced by the active timeline.
- A grace period, default 7 days, protects against accidental deletion immediately after repack.
- No checkpoint, active branch, tag, or reachable node may reference a block before it is pruned.

## Frozen Defaults

```text
model = Layered Timeline Pack
node = 1 commit delta
segment_size = 5 nodes
archive_size = 100 nodes
checkpoint_interval = 100 nodes
compression = zstd
content_id = sha256
ignore_file = .bdyndignore
gc_grace_period = 7d
remote_mutability = append/new only, refs updated last
```

## Open Items For Later Plan Tasks

- Exact binary header encoding and Go structs.
- Local SQLite schema and migration code.
- CLI command naming for flush, repack, compact, split, verify, and prune.
- Whether zstd is implemented through a Go dependency or optional external binary.
- Whether native rename ops are worth adding after the first stable version.
