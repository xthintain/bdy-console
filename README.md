# bdy

`bdy` is a Linux command-line tool for managing Baidu Netdisk through the Baidu Netdisk Open Platform. It provides:

- OAuth device-code login.
- Bash-style cloud file commands under `/apps/baiduyunStorage`.
- `bdy nd` Git-like NetDisk project versioning under `.bdynd/`.
- Git-LFS-style large file storage backed by Baidu Netdisk.
- A lightweight snapshot sync mode for simple folder sync.

Secrets and tokens are stored only in the user config directory, not in this repository.

## Install

Build locally:

```bash
go build -o bdy ./cmd/bdy
```

Install as a Linux command:

```bash
sudo install -m 0755 bdy /usr/bin/bdy
```

Build a hardened release binary:

```bash
./scripts/build-secure.sh
```

The secure build strips symbols, removes build paths, emits a SHA-256 checksum, and uses optional `garble`/`upx` hardening when those tools are installed. No local binary can be made impossible to unpack, but this raises reverse-engineering cost.

## Authentication

### Get App Credentials

Create or open your Baidu Netdisk Open Platform application before running `bdy auth login`.

1. Open the Baidu Netdisk Open Platform developer console: <https://pan.baidu.com/union>.
2. Sign in with your Baidu account and complete the required developer verification if the console asks for it.
3. Create an application for Baidu Netdisk Open Platform API access.
4. Open the application detail page and copy these values:
   - `AppID`
   - `AppKey`
   - `SecretKey`
   - `SignKey`
5. Keep those values private. Treat `SecretKey`, `SignKey`, access tokens, and refresh tokens as secrets.

### Configure Local Credentials

Configure your Baidu Netdisk Open Platform app credentials. Use your own values; do not commit them.

```bash
bdy config set-app \
  --app-id '<AppID>' \
  --app-key '<AppKey>' \
  --secret-key '<SecretKey>' \
  --sign-key '<SignKey>'
```

The config is saved to:

```text
~/.config/bdy/config.json
```

The file is written with mode `0600`. It may contain app credentials, access tokens, and refresh tokens, so it must not be uploaded or committed.

Start device-code login:

```bash
bdy auth login
```

The command prints:

- a verification URL
- a user code
- a QR-code URL

Open the URL, enter the code or scan the QR code, and approve the `basic,netdisk` scope.

Check login status:

```bash
bdy auth status
```

Temporary read-only login:

```bash
bdy auth login --temporary 1d
```

Temporary auth is stored in `~/.config/bdy/temporary.json`, expires after the requested duration, and is restricted by `bdy` to clone, view, fetch, pull, download, index, and search style commands. Write commands return `temporary read-only auth forbids write operation`.

See [docs/commands.md](docs/commands.md) for the full command reference and read-only policy table.

## Cloud Command Space

`bdy cmd` is the bash-style cloud file namespace. Its default root is:

```text
/apps/baiduyunStorage
```

Show current cloud working directory:

```bash
bdy cmd pwd
```

Temporarily switch the target directory for the current shell:

```bash
bdy cmd cd git
bdy cmd pwd
```

`bdy cmd cd` records a temporary cwd for the current shell session. Closing the terminal or using a different shell returns to `/apps/baiduyunStorage`. The older `eval "$(bdy cmd cd git)"` form is still accepted because `cd` also prints an `export BDY_CMD_CWD=...` line.

Common commands:

```bash
bdy cmd ls
bdy cmd ls -al
bdy cmd ll
bdy cmd la
bdy cmd mkdir -p logs/archive
bdy cmd touch logs/today.txt
bdy cmd cat -n logs/today.txt
bdy cmd vim notes/todo.txt
bdy cmd find -name '.*\.txt$' -type f
bdy cmd grep -i 'report' logs
bdy cmd rm -rf logs/archive
bdy cmd history -n 20
```

Global flags can be placed before the space command:

```bash
bdy -h
bdy -v
bdy -C git cmd ls
bdy --json cmd ls
```

Notes:

- `find` and `grep` search path and filename metadata, not file contents.
- `cat` downloads a temporary copy and writes it to stdout.
- `vim` downloads a temporary copy, opens `$EDITOR` or `vim`, then uploads it back.
- `rm -rf` deletes remote paths through the Baidu file manager API.

## Whole Netdisk Access

Use `home` only when you explicitly want to inspect the whole netdisk. Normal `cmd`, `lfs`, and sync commands stay under `/apps/baiduyunStorage`.

```bash
bdy home ls /
bdy home ls /apps
bdy home ls /Document
```

`home` can also run the same bash-style file commands against the whole netdisk. The explicit `cmd` word is accepted as a composition alias:

```bash
bdy home mkdir /tmp/demo
bdy home cmd mkdir /tmp/demo
bdy home cmd ls /tmp
bdy home cmd rm -rf /tmp/demo
```

Use command-level help for details:

```bash
bdy cmd mkdir --help
bdy home cmd mkdir --help
bdy lfs --help
bdy sync --help
```

## Git-LFS-Style Large Files

`bdy lfs` stores large file contents under:

```text
/apps/baiduyunStorage/lfs
```

Git stores pointer files. Real file contents are cached locally under `.bdy/lfs/objects` and uploaded by SHA-256 object ID.

Inside a Git repository:

```bash
bdy lfs install
bdy lfs track '*.zip'
git add .gitattributes
git add large.zip
git commit -m 'track large file'
bdy lfs push
```

On another checkout:

```bash
git clone <repo>
bdy lfs install
bdy lfs pull
```

Useful commands:

```bash
bdy lfs status
bdy lfs ls-files
bdy lfs fetch
bdy lfs checkout
```

Pointer format:

```text
version https://bdy-lfs/spec/v1
oid sha256:<hash>
size <bytes>
```

## NetDisk Version Storage

`bdy nd` is the new Git-like project versioning surface. It does not require a real `.git` repository.

```bash
bdy nd init
printf 'hello\n' > a.txt
bdy nd add a.txt
bdy nd commit -m 'first'
bdy nd status
bdy nd log
```

Local repository state is stored under:

```text
.bdynd/
```

Ignore local files with `.bdyndignore`:

```gitignore
# cache and build output
dist/
tmp/
*.log

# private files
.env
*.key
secret/
```

`bdy nd add .`, `diff`, and `stash` skip ignored files. Patterns support comments, blank lines, `dir/`, `*.ext`, exact paths, and root paths such as `/dist/`.

The implemented commands include `init`, `add`, `commit`, `status`, `log`, `show`, `diff`, `rm`, `mv`, `restore`, `reset`, `pack`, `index`, `branch`, `switch`, `checkout`, `tag`, `remote`, `push`, `fetch`, `pull`, `clone`, `merge`, and `stash`. Built-in `nd lfs` tracking and object sync are available. Advanced porcelain is planned in `docs/superpowers/plans/2026-04-29-bdy-nd-lfs.md`.

Branch and tag commands are also available:

```bash
bdy nd diff
bdy nd restore a.txt
bdy nd rm old.txt
bdy nd rm --cached keep-local.txt
bdy nd mv draft.txt docs/draft.txt
bdy nd reset --hard HEAD~1
bdy nd branch feature
bdy nd switch feature
bdy nd branch
bdy nd tag v1
bdy nd checkout v1
bdy nd remote set-url origin /apps/baiduyunStorage/nd/repos/demo
bdy nd push
bdy nd fetch
bdy nd pull
bdy nd clone /apps/baiduyunStorage/nd/repos/demo demo-clone
bdy nd merge feature
```

For high-throughput data storage, pack a committed snapshot into a single local data file plus a JSON manifest:

```bash
bdy nd pack --name batch-001
bdy nd index
bdy nd search --type txt --name report --since 2026-01-01
bdy nd pack push
bdy nd pack fetch 20260429055009-7b4c0a60da9d
```

Pack files live under:

```text
.bdynd/packs/
```

`pack` is intended for batch object/database-style workloads where many files should be uploaded later as fewer large objects. `index` and `search` read local pack manifests; they do not scan Baidu Netdisk.

Search filters:

```bash
bdy nd search --type txt
bdy nd search --name report
bdy nd search --since 2026-01-01 --until 2026-12-31
```

`--type` matches file extensions, `--name` matches filename substrings case-insensitively, and `--since`/`--until` filter by the pack manifest creation time. Dates accept `YYYY-MM-DD` or RFC3339.

Remote pack sync stores files under:

```text
/apps/baiduyunStorage/nd/repos/<repo-name>/packs/
```

`pack push` uploads all local pack files and manifests that are not already present remotely. `pack fetch` currently fetches explicit pack IDs; use `bdy nd index` after fetching to inspect local manifests.

Temporarily save dirty worktree changes without creating a commit:

```bash
bdy nd stash push -m 'wip'
bdy nd stash list
bdy nd stash pop
```

Large files can be tracked by the built-in `nd lfs` layer:

```bash
bdy nd lfs track '*.bin'
bdy nd add large.bin
bdy nd lfs status
bdy nd lfs ls-files
bdy nd lfs checkout
bdy nd lfs push
bdy nd lfs fetch
bdy nd lfs pull
bdy nd lfs untrack '*.bin'
```

Tracked large-file content is cached under:

```text
.bdynd/lfs/objects/sha256/
```

Remote sync uses Baidu Netdisk as the remote object store. Configure an explicit remote root:

```bash
bdy nd remote set-url origin /apps/baiduyunStorage/nd/repos/demo
bdy nd remote
```

If no remote is configured, `bdy nd push`, `fetch`, `pull`, and `lfs push/fetch/pull` use:

```text
/apps/baiduyunStorage/nd/repos/<current-directory-name>
```

## Advanced Examples

### Shell-Scoped Cloud Workspace

Use `cmd cd` when you want several cloud operations to target the same app-space folder without changing the global default:

```bash
bdy cmd mkdir -p datasets/2026-04/raw
bdy cmd cd datasets/2026-04
bdy cmd pwd
bdy cmd touch raw/ingest.log
bdy cmd ls -al raw
bdy cmd grep -i report raw
bdy cmd history -n 10
```

The cwd is stored for the current shell session. A new terminal falls back to `/apps/baiduyunStorage`.

### Whole-Netdisk Inspection

Use `home` when you intentionally want to operate outside the isolated app folder:

```bash
bdy home cmd ls /
bdy home cmd find -type d -name '.*backup.*' /
bdy home cmd grep -i invoice /Document
bdy home cmd cat /Document/notes/readme.txt
```

### Versioned Project With Remote

Create a project, make a branch, merge it, then push the version database to Baidu Netdisk:

```bash
mkdir research-notes
cd research-notes
bdy nd init
printf 'baseline\n' > notes.txt
bdy nd add notes.txt
bdy nd commit -m 'baseline notes'

bdy nd branch experiment
bdy nd switch experiment
printf 'new observation\n' >> notes.txt
bdy nd add notes.txt
bdy nd commit -m 'add experiment observation'

bdy nd switch main
bdy nd merge experiment
bdy nd tag v1
bdy nd remote set-url origin /apps/baiduyunStorage/nd/repos/research-notes
bdy nd push
```

Clone or update another checkout:

```bash
bdy nd clone /apps/baiduyunStorage/nd/repos/research-notes research-notes-copy
cd research-notes-copy
bdy nd fetch
bdy nd pull
```

### Built-In Large File Tracking

Track large binary data inside `bdy nd` without using Git or Git LFS:

```bash
bdy nd init
bdy nd lfs track '*.bin'
bdy nd lfs track '*.zip'
cp /data/model.bin .
cp /data/archive.zip .
bdy nd add model.bin archive.zip
bdy nd commit -m 'add large dataset artifacts'
bdy nd lfs status
bdy nd lfs push
bdy nd push
```

On another machine:

```bash
bdy nd clone /apps/baiduyunStorage/nd/repos/model-store model-store
cd model-store
bdy nd lfs fetch
bdy nd lfs checkout
```

### Batch Object Store Workflow

Use pack files for high-throughput storage when many files should become fewer larger remote objects:

```bash
bdy nd init
mkdir -p logs/2026-04-29 images
cp /var/log/app/*.log logs/2026-04-29/
cp /data/camera/*.png images/
bdy nd add logs images
bdy nd commit -m 'batch import 2026-04-29'

bdy nd pack --name ingest-2026-04-29
bdy nd index
bdy nd search --type log --name error --since 2026-04-29
bdy nd search --type png --since 2026-04-01 --until 2026-04-30
bdy nd pack push
```

Fetch a known pack ID and search it locally:

```bash
bdy nd init
bdy nd remote set-url origin /apps/baiduyunStorage/nd/repos/data-lake
bdy nd pack fetch 20260429055009-7b4c0a60da9d
bdy nd index
bdy nd search --type log --name ingest
```

### Stash, Restore, And Reset

Save dirty work, inspect changes, restore one file, or reset a branch:

```bash
printf 'temporary note\n' >> notes.txt
bdy nd diff
bdy nd stash push -m 'temporary notes'
bdy nd status
bdy nd stash list
bdy nd stash pop

bdy nd restore notes.txt
bdy nd reset --mixed HEAD~1
bdy nd reset --hard HEAD
```

## Snapshot Sync

The older snapshot sync mode stores metadata in `.bdy/` and syncs committed snapshots to:

```text
/apps/baiduyunStorage/workspace
```

```bash
bdy init
bdy status
bdy add notes.txt docs
bdy commit -m 'snapshot'
bdy push
bdy pull
```

The canonical grouped form is also supported:

```bash
bdy sync init
bdy sync status
bdy sync add notes.txt docs
bdy sync commit -m 'snapshot'
bdy sync push
bdy sync pull
```

This is not a full Git database. It is a simple manifest-based sync workflow.

## Privacy And Ignored Files

Do not upload or commit:

- `~/.config/bdy/config.json`
- `.bdy/`
- `.env` files
- built binaries
- logs, caches, temporary files
- private documents or downloaded cloud files

See `.gitignore` for the repository ignore rules.

## Development

Run tests:

```bash
go test ./...
```

Build release binary:

```bash
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o dist/bdy ./cmd/bdy
```
