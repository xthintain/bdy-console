# bdy Command Reference

This document lists the supported command surfaces and marks whether each command is allowed when using temporary read-only auth.

Temporary read-only auth is local policy enforcement in `bdy`. Baidu Netdisk OAuth for this app still uses the platform scope `basic,netdisk`; the CLI blocks write operations before they reach the API.

## Authentication

```bash
bdy config set-app --app-id ID --app-key KEY --secret-key SECRET --sign-key SIGN
bdy auth login
bdy auth login --temporary 1d
bdy auth status
```

`bdy auth login --temporary 1d` stores a temporary token in `~/.config/bdy/temporary.json`, marks it read-only, and limits its lifetime to the requested duration. Supported duration examples: `12h`, `24h`, `1d`, `7d`.

Temporary read-only mode allows `auth status` and another `auth login`. It blocks `config set-app` while the temporary token is active.

## Cloud App Space

Root:

```text
/apps/baiduyunStorage
```

Read commands allowed in temporary read-only mode:

```bash
bdy cmd pwd
bdy cmd cd datasets
bdy cmd ls
bdy cmd ls -al logs
bdy cmd ll logs
bdy cmd la logs
bdy cmd find -name '.*\.txt$' -type f logs
bdy cmd grep -i report logs
bdy cmd cat -n logs/report.txt
bdy cmd history -n 20
```

Write commands blocked in temporary read-only mode:

```bash
bdy cmd mkdir -p logs/archive
bdy cmd touch logs/today.txt
bdy cmd vim notes/todo.txt
bdy cmd rm -rf logs/archive
bdy cmd delete logs/archive
```

Use a temporary shell-scoped target path:

```bash
eval "$(bdy cmd cd datasets/2026-04)"
bdy cmd pwd
bdy cmd ls -al raw
```

## Whole Netdisk

Use `home` only for explicit whole-netdisk inspection.

Read commands allowed in temporary read-only mode:

```bash
bdy home ls /
bdy home cmd ls /Document
bdy home cmd find -type d -name '.*backup.*' /
bdy home cmd grep -i invoice /Document
bdy home cmd cat /Document/notes/readme.txt
```

Write commands blocked in temporary read-only mode:

```bash
bdy home cmd mkdir /tmp/demo
bdy home cmd touch /tmp/demo/a.txt
bdy home cmd vim /tmp/demo/a.txt
bdy home cmd rm -rf /tmp/demo
```

## NetDisk Version Storage

Local repository root:

```text
.bdynd/
```

Ignore file:

```text
.bdyndignore
```

Supported ignore examples:

```gitignore
# comments and blank lines are ignored
dist/
tmp/
*.log
.env
*.key
secret/
/root-only-cache/
```

The ignore matcher supports directory patterns ending with `/`, extension globs such as `*.log`, exact file paths, and root paths beginning with `/`. `bdy nd add .`, `diff`, and `stash` skip ignored files.

Common write workflow:

```bash
bdy nd init
printf 'hello\n' > a.txt
bdy nd add a.txt
bdy nd commit -m 'first'
bdy nd branch feature
bdy nd switch feature
bdy nd merge main
bdy nd tag v1
bdy nd push
```

Temporary read-only mode blocks the write workflow above.

Read and download commands allowed in temporary read-only mode:

```bash
bdy nd clone /apps/baiduyunStorage/nd/repos/demo demo-clone
bdy nd fetch
bdy nd pull
bdy nd status
bdy nd log
bdy nd show <commit>
bdy nd diff
bdy nd index
bdy nd search --type txt --name report --since 2026-01-01
```

Write commands blocked in temporary read-only mode:

```bash
bdy nd init
bdy nd add a.txt
bdy nd commit -m 'message'
bdy nd rm a.txt
bdy nd rm --cached keep-local.txt
bdy nd mv old.txt new.txt
bdy nd restore a.txt
bdy nd reset --hard HEAD~1
bdy nd branch feature
bdy nd switch feature
bdy nd checkout v1
bdy nd tag v1
bdy nd merge feature
bdy nd stash push -m wip
bdy nd stash pop
bdy nd push
```

## Built-In `nd lfs`

Allowed in temporary read-only mode:

```bash
bdy nd lfs status
bdy nd lfs ls-files
bdy nd lfs fetch
bdy nd lfs checkout
```

Blocked in temporary read-only mode:

```bash
bdy nd lfs track '*.bin'
bdy nd lfs untrack '*.bin'
bdy nd lfs push
```

## Pack Object Storage

Pack workflow for high-throughput batch data:

```bash
bdy nd pack --name ingest-2026-04-29
bdy nd index
bdy nd search --type log --name error --since 2026-04-29
bdy nd pack push
```

Allowed in temporary read-only mode:

```bash
bdy nd pack fetch 20260429055009-7b4c0a60da9d
bdy nd index
bdy nd search --type png --since 2026-04-01 --until 2026-04-30
```

Blocked in temporary read-only mode:

```bash
bdy nd pack --name batch-001
bdy nd pack push
```

## Legacy Git-LFS-Style Commands

Allowed in temporary read-only mode:

```bash
bdy lfs status
bdy lfs ls-files
bdy lfs fetch
bdy lfs checkout
```

Blocked in temporary read-only mode:

```bash
bdy lfs install
bdy lfs track '*.zip'
bdy lfs untrack '*.zip'
bdy lfs push
bdy lfs clean
bdy lfs smudge
```

## Legacy Snapshot Sync

Allowed in temporary read-only mode:

```bash
bdy status
bdy ls
bdy pull
bdy remote
bdy sync status
bdy sync ls
bdy sync pull
bdy sync remote
```

Blocked in temporary read-only mode:

```bash
bdy init
bdy add README.md
bdy commit -m 'snapshot'
bdy push
bdy rm old.txt
bdy mv old.txt new.txt
bdy sync init
bdy sync add README.md
bdy sync commit -m 'snapshot'
bdy sync push
```

## Secure Build

Build a hardened binary:

```bash
./scripts/build-secure.sh
```

The secure build script uses:

- `go build -trimpath`
- stripped linker flags `-s -w`
- optional `garble -literals -tiny` when `garble` is installed
- optional `upx --best --lzma` when `upx` is installed
- SHA-256 checksum output next to the binary

This hardens the binary and raises reverse-engineering cost. It cannot make a local executable impossible to unpack or reverse engineer.
