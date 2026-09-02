# Project Principles

These principles guide the bdynd layered timeline snapshot database. They are written as implementation constraints, not as broad preferences.

## 1. Preserve Exact Project States

Every commit node must represent an exact restorable project state. The physical storage may use deltas, grouped packs, compression, and deduplication, but the logical model remains a complete snapshot at each node.

A restore is valid only when the final materialized tree matches the target node's expected tree hash.

## 2. Optimize For Baidu Netdisk's Shape

Baidu Netdisk is treated as durable object storage, not as a live database or random-access filesystem.

Remote writes should create immutable files whenever possible. Mutable remote state should be limited to refs and small timeline/index files, updated only after their referenced packs are uploaded and verified.

Prefer fewer large remote files over many small files. Local indexes provide precision; remote packs provide request efficiency.

## 3. Keep Logical Granularity Small And Physical Granularity Large

A NodeBlock is one commit delta. A SegmentBlock groups nodes. An ArchiveBlock groups segments. A CheckpointBlock stores a complete tree.

The system must be able to recover a non-boundary node from inside a larger block without ambiguity. Distinct begin/end labels, structured headers, payload lengths, offsets, and checksums are all part of the block contract.

## 4. Never Trust A Partial File

All downloaded data must be verified before use. Verification moves from outside to inside:

```text
pack checksum
segment checksum
node checksum
object checksum
final tree checksum
```

When verification fails, delete the temporary file and download again. Do not expose partially restored content as a valid checkout.

## 5. Reuse Unchanged Content

Repeated full-project uploads are avoided by comparing the current tree with the parent tree and storing only changes. File contents are addressed by SHA-256 and reused across nodes, segments, archives, branches, and checkpoints when possible.

The first implementation should favor stable file-level deduplication and simple large-file chunking. More complex delta compression can be added later only if it does not weaken recovery guarantees.

## 6. Ignore Rebuildable And Unsafe Inputs

Dependency directories, build outputs, caches, VCS internals, local bdynd state, logs, and secrets should not enter snapshots by default. `.bdyndignore` remains the user-facing rule file, with safe defaults layered underneath.

The snapshot database stores project source state, not machine-local noise.

## 7. Write New Blocks, Then Switch Pointers

Repacking, splitting, compaction, checkpoint creation, and synchronization must not mutate existing remote packs in place.

The safe update order is:

```text
write new local block
verify new local block
upload new remote block
upload new remote index
update timeline metadata
update branch ref last
```

This keeps refs from pointing at missing or partially uploaded data.

## 8. Treat Cleanup As Delayed Garbage Collection

Superseded blocks are not deleted immediately. They move through lifecycle states such as `active`, `superseded`, `garbage`, `deleting`, and `deleted`.

A remote block can be pruned only when no active ref, checkpoint, tag, reachable node, or active timeline index references it, and only after a grace period.

## 9. Make Repacking Deterministic

Given the same live nodes, objects, sizing rules, and compression settings, repack should produce predictable metadata and stable references where practical. Determinism makes debugging, verification, and interrupted retries much easier.

If byte-for-byte deterministic compression is not practical in the first version, the index and content hashes must still be deterministic enough to prove correctness.

## 10. Keep Recovery Simpler Than Compression

Compression and storage efficiency must never make restore logic fragile. The preferred order of optimization is:

```text
ignore unnecessary files
store node deltas
deduplicate by SHA-256
chunk large files
pack many records together
compress packs
optionally add advanced deltas later
```

If an optimization makes it hard to explain how a node is restored, it should wait.

## 11. Local Database Is An Index, Not The Source Of Truth

The local SQLite database speeds up lookup, planning, and garbage collection. It can be rebuilt from remote refs, timelines, pack indexes, checkpoints, and archive metadata.

The durable source of truth is the verified remote object set plus the active refs and timelines.

## 12. Failure Should Be Restartable

Every long operation should be restartable after interruption. Pending blocks, uploaded-but-unreferenced blocks, superseded blocks, and failed downloads must have explicit states so the next run can continue or safely clean up.

No operation should require manual repair merely because a process exited halfway through an upload or repack.
