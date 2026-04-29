# bdy nd Design

## Goal

Build an independent Git-like version control system inside `bdy`, with Baidu Netdisk as the remote object store and an internal LFS mechanism for large files. The implementation must not require a real `.git` repository or Git filters.

## Product Boundary

`bdy nd` is the new canonical project management surface:

```bash
bdy nd init
bdy nd status
bdy nd add <path...>
bdy nd commit -m "message"
bdy nd log
bdy nd show <commit>
bdy nd diff
bdy nd branch
bdy nd switch <branch>
bdy nd checkout <ref>
bdy nd restore <path...>
bdy nd reset [--soft|--mixed|--hard] <ref>
bdy nd rm <path...>
bdy nd mv <old> <new>
bdy nd tag <name>
bdy nd merge <branch>
bdy nd stash
bdy nd remote
bdy nd fetch
bdy nd push
bdy nd pull
bdy nd clone <remote>
bdy nd lfs track "*.zip"
bdy nd lfs push
bdy nd lfs pull
```

The old `bdy sync ...` commands remain as compatibility aliases until the new storage model is complete. The old top-level `bdy lfs ...` commands remain documented as legacy and should route users toward `bdy nd lfs ...`.

## Architecture

Local repositories live under `.bdynd/`, not `.git/`. The object database uses content-addressed JSON objects for commits and trees, plus SHA-256 blob objects for normal file contents and LFS objects. Refs live under `.bdynd/refs/heads`, `.bdynd/refs/tags`, and `HEAD` points to the current branch.

Remote repositories live under:

```text
/apps/baiduyunStorage/nd/repos/<repo-name>/
  refs/
  objects/
  lfs/
  packs/
```

The implementation follows the Git model at the porcelain level first: working tree, index, object database, refs, commits, branches, remote synchronization, then higher-level history operations. Plumbing APIs are internal Go packages rather than user-facing commands at first.

## LFS Model

`bdy nd lfs` is built into the bdy repository model. Tracked patterns are stored in `.bdynd/attributes.json`. When a tracked file is added, the index stores a pointer entry containing:

```text
version https://bdy-lfs/spec/v1
oid sha256:<hash>
size <bytes>
```

Real content is stored locally under `.bdynd/lfs/objects/sha256/<prefix>/<hash>` and remotely under `/apps/baiduyunStorage/nd/repos/<repo-name>/lfs/objects/sha256/<prefix>/<hash>`.

## Implementation Phases

1. Core local repository: `init/status/add/commit/log/show/diff/rm/mv/restore/reset`.
2. Refs and branches: `branch/switch/checkout/tag/HEAD/reflog`.
3. Built-in LFS: `nd lfs track/untrack/status/push/fetch/checkout/pull`.
4. Remote synchronization: `remote/fetch/push/pull/clone`.
5. Advanced porcelain: `merge/stash/rebase/cherry-pick/bisect/blame/grep`.

## References

- Git command reference: https://git-scm.com/docs
- Git porcelain/plumbing model: https://git-scm.com/docs/git/2.48.0
- Pro Git plumbing and porcelain: https://git-scm.com/book/en/v2/Git-Internals-Plumbing-and-Porcelain
- Git LFS pointer specification: https://github.com/git-lfs/git-lfs/blob/main/docs/spec.md
