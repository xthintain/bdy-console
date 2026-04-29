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
eval "$(bdy cmd cd git)"
bdy cmd pwd
```

This sets `BDY_CMD_CWD` in the current shell only. Closing the terminal or using a new shell without that environment variable returns to `/apps/baiduyunStorage`.

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

The implemented commands include `init`, `add`, `commit`, `status`, `log`, `show`, `branch`, `switch`, `checkout`, `tag`, `remote`, `push`, `fetch`, and `pull`. Built-in `nd lfs` tracking and object sync are available. Clone, merge, and advanced porcelain are planned in `docs/superpowers/plans/2026-04-29-bdy-nd-lfs.md`.

Branch and tag commands are also available:

```bash
bdy nd branch feature
bdy nd switch feature
bdy nd branch
bdy nd tag v1
bdy nd checkout v1
bdy nd remote set-url origin /apps/baiduyunStorage/nd/repos/demo
bdy nd push
bdy nd fetch
bdy nd pull
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
