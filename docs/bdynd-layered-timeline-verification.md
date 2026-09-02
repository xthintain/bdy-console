# bdynd Layered Timeline Verification

This file records the minimum verification closure for the layered timeline snapshot database plan.

## Scope

The current implementation is still at the design and scaffold stage. The verified closure is:

- Architecture decisions are documented.
- Project principles are documented.
- `internal/bdynd/timeline` package exists.
- Default sizing and lifecycle configuration are compiled and tested.
- Local layout helpers are compiled and tested.
- Request id and logging skeletons are compiled and tested.
- Existing bdynd/auth/config/CLI packages still build and test successfully.

## Verified Files

- `docs/bdynd-layered-timeline-decisions.md`
- `docs/project-principles.md`
- `internal/bdynd/timeline/doc.go`
- `internal/bdynd/timeline/config.go`
- `internal/bdynd/timeline/layout.go`
- `internal/bdynd/timeline/logging.go`
- `internal/bdynd/timeline/config_test.go`
- `internal/bdynd/timeline/logging_test.go`

## Commands

Minimum package closure:

```bash
go test ./internal/bdynd/timeline ./internal/bdynd ./internal/auth ./internal/config
go build ./cmd/bdy
```

Result:

```text
ok  baiduyunStorage/internal/bdynd/timeline
ok  baiduyunStorage/internal/bdynd
ok  baiduyunStorage/internal/auth
ok  baiduyunStorage/internal/config
build ok
```

Full repository test:

```bash
go test ./...
```

Result:

```text
?   baiduyunStorage/cmd/bdy [no test files]
ok  baiduyunStorage/internal/auth
ok  baiduyunStorage/internal/baidu
ok  baiduyunStorage/internal/bdynd
ok  baiduyunStorage/internal/bdynd/timeline
ok  baiduyunStorage/internal/cli
ok  baiduyunStorage/internal/config
ok  baiduyunStorage/internal/lfs
ok  baiduyunStorage/internal/repo
ok  baiduyunStorage/pkg/baidund
```

## Notes

A help text assertion was updated during verification so that the restored OAuth login help still includes the existing `SDK token` wording expected by the CLI tests. This preserves the dual auth model: OAuth device-code login and external SDK token import.

No remote Baidu Netdisk mutation was required for this verification step.
