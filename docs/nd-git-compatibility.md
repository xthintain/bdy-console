# bdy nd Git Compatibility Reality

`bdy nd` is Git-like, but it is not Git-compatible internally. This document
separates command-name compatibility from implementation compatibility so users
do not mistake `bdy nd` for a drop-in Git replacement.

## Current Implementation Model

`bdy nd` currently stores its own objects under `.bdynd/`:

- blobs, trees, and commits are stored as JSON-oriented nd objects, not Git loose
  objects.
- object IDs use `sha256:<hash>` computed by nd's own object framing.
- commits store a snapshot-style `Entries []IndexEntry` list.
- trees are serialized index-entry lists, not Git tree records.
- the index is `.bdynd/index.json`, not Git's binary index format.
- refs live under `.bdynd/refs`, but the file format and object graph are nd's.
- remotes are Baidu Netdisk paths, not Git protocols or Git remote helpers.
- packs are nd pack manifests and payload files, not Git packfiles.

This means Git itself cannot read a `.bdynd` repository, and `bdy nd` cannot
operate directly on `.git` object databases.

## Command Surface That Exists

These commands exist and provide Git-like workflows, but their internals are nd
native:

| Git command surface | bdy nd command | Implementation status |
| --- | --- | --- |
| `git init` | `bdy nd init` | Creates `.bdynd`, not `.git`. |
| `git add` | `bdy nd add` | Writes nd blobs and JSON index entries. |
| `git commit` | `bdy nd commit` | Creates nd commit JSON with snapshot entries. |
| `git status` | `bdy nd status` | Path-level status from nd index and HEAD. |
| `git log` | `bdy nd log` | Follows nd commit parents. |
| `git show` | `bdy nd show` | Shows nd commit metadata. |
| `git diff` | `bdy nd diff` | Path/status diff only; not Git patch format. |
| `git grep` | `bdy nd grep` | Greps tracked nd blobs. |
| `git clean` | `bdy nd clean` | Removes untracked worktree files. |
| `git rm` | `bdy nd rm` | Removes paths from worktree and nd index. |
| `git mv` | `bdy nd mv` | Moves path and updates nd index. |
| `git restore` | `bdy nd restore` | Restores paths from nd HEAD. |
| `git reset` | `bdy nd reset` | Supports soft, mixed, and hard against nd refs. |
| `git branch` | `bdy nd branch` | Lists, creates, and deletes nd branch refs. |
| `git switch` | `bdy nd switch` | Switches nd branch refs and checkout tree. |
| `git checkout` | `bdy nd checkout` | Checks out nd commit trees. |
| `git tag` | `bdy nd tag` | Lightweight nd refs only. |
| `git remote` | `bdy nd remote` | Stores Baidu Netdisk remote roots. |
| `git push/fetch/pull` | `bdy nd push/fetch/pull` | Transfers nd objects through Baidu Netdisk APIs. |
| `git clone` | `bdy nd clone` | Clones an nd remote root. |
| `git merge` | `bdy nd merge` | Simple file-level merge, not Git ort/recursive. |
| `git rebase` | `bdy nd rebase` | Simple snapshot replay, not Git's sequencer. |
| `git cherry-pick` | `bdy nd cherry-pick` | Simple snapshot replay of one commit. |
| `git stash` | `bdy nd stash` | nd-native stash files. |

## bdy nd Extensions

| Command | Purpose |
| --- | --- |
| `bdy nd ignore apply` | Re-applies `.bdyndignore` to the nd index. |
| `bdy nd lfs ...` | nd-native large object storage. |
| `bdy nd pack ...` | nd-native portable pack files. |
| `bdy nd search ...` | Searches nd pack manifests. |
| `bdy nd push --prune` | Deletes unreachable nd remote object files. |
| `bdy nd pull --force` | Overwrites local nd state with remote HEAD. |

## Major Differences From Real Git

### Object Database

Git object IDs are hashes of canonical object payloads such as
`blob <size>\0<data>`, `tree`, `commit`, and `tag`. `bdy nd` also uses content
addressing, but its tree and commit payloads are JSON structures and are not
Git object payloads. Therefore object IDs, storage paths, and pack compatibility
do not match Git.

### Index

Git uses a binary index with stat metadata, stages, conflict entries, path flags,
and extensions. `bdy nd` uses a JSON map of path to object entry. It does not
support Git's staged conflict states, intent-to-add, assume-unchanged,
skip-worktree, or split index features.

### Merge, Rebase, Cherry-Pick

Git's merge and rebase rely on mature merge machinery, rename detection, patch
application, sequencer state, conflict stages, rerere, and many strategy
options. `bdy nd` currently performs simple file-level snapshot replay and
conflict detection. It is useful for straightforward workflows, but it is not
Git-equivalent.

### Remote Protocol

Git speaks local, SSH, HTTP(S), smart protocol, protocol v2, remote helpers, and
pack negotiation. `bdy nd` uploads and downloads object files through Baidu
Netdisk paths. There is no Git protocol compatibility.

### Pack Format

Git packfiles contain compressed delta objects plus indexes. `bdy nd pack` is a
manifest plus concatenated payload format. It is intentionally not Git packfile
compatible.

## What Would Be Required For True Git Internal Alignment

To make `bdy nd` truly Git-compatible internally, the implementation would need
at least these architectural changes:

1. Store canonical Git loose objects for blob, tree, commit, and tag.
2. Use Git-compatible object IDs, likely SHA-1 for classic Git or SHA-256 with
   Git's hash-transition semantics.
3. Replace `.bdynd/index.json` with a Git-compatible index reader/writer or use
   a proven Git library.
4. Implement Git tree encoding and decoding.
5. Implement commit and tag object encoding exactly as Git expects.
6. Implement packfile read/write and pack index support, or delegate to Git.
7. Implement Git's merge/rebase/cherry-pick semantics through a proven engine.
8. Add remote-helper or protocol integration if real `git` clients must talk to
   Baidu Netdisk storage.

The pragmatic path is to use a Go Git library for Git semantics and keep Baidu
Netdisk as an object/pack transport layer. Re-implementing Git completely from
scratch is high risk and will take substantial work.

## Not Implemented Or Not Suitable In The Current nd Model

| Git feature | Status |
| --- | --- |
| Git protocol compatibility | Not implemented. |
| Git packfile compatibility | Not implemented. |
| `.git` repository interoperability | Not implemented. |
| `git am`, `format-patch` | Not implemented. |
| `git bisect` | Not implemented. |
| `git blame` | Not implemented. |
| `git submodule` | Not implemented. |
| `git worktree` | Not implemented. |
| `git sparse-checkout` | Not implemented. |
| annotated/signed tags | Not implemented. |
| full patch diff/apply | Not implemented. |
