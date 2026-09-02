# baidund Direct Integration

`pkg/baidund` is the importable API for projects that want Baidu Netdisk-backed
version storage without shelling out to the `bdy` command.

## Install In Another Project

Until the module path is renamed to a GitHub import path, use a local `replace`:

```go
require baiduyunStorage v0.0.0

replace baiduyunStorage => /path/to/baiduyunStorage
```

Then import:

```go
import "baiduyunStorage/pkg/baidund"
```

## Minimal Usage

```go
client := baidund.New(baidund.Credentials{
    AccessToken:  os.Getenv("BDY_ACCESS_TOKEN"),
    RefreshToken: os.Getenv("BDY_REFRESH_TOKEN"),
})

repo, err := client.Init("./data", baidund.DefaultRemoteRoot("my-project"))
if err != nil {
    panic(err)
}
if err := repo.Add("large-file.bin"); err != nil {
    panic(err)
}
commit, err := repo.Commit("store large-file.bin")
if err != nil {
    panic(err)
}
fmt.Println(commit.OID)

if err := repo.Push(context.Background()); err != nil {
    panic(err)
}
```

## Clone Or Pull

```go
repo, err := client.Clone(ctx, baidund.DefaultRemoteRoot("my-project"), "./restore")
if err != nil {
    panic(err)
}

if err := repo.Pull(ctx); err != nil {
    panic(err)
}
```

## Template

See `examples/baidund-template`:

```bash
cd examples/baidund-template
BDY_ACCESS_TOKEN=... BDY_REFRESH_TOKEN=... go run .
BDY_ACCESS_TOKEN=... BDY_REFRESH_TOKEN=... BDY_PUSH=1 go run .
```

The template never stores credentials in source files. Pass tokens through your
application secret manager, environment variables, or your own SDK auth flow.

## Current Boundary

The public package exposes the stable operations needed by application code:

- `Init`, `Open`, `Clone`
- `Add`, `Commit`, `Push`, `Fetch`, `Pull`
- `Status`, `Log`
- `DefaultRemoteRoot`

The lower-level Baidu API client and object storage implementation remain in
`internal/` so applications do not bind themselves to unstable internals.
