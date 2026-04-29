# Repository Rules

## File Size

- Keep every code file at 500 lines or fewer.
- If a code file would exceed 500 lines, split it by responsibility before adding more code.
- Prefer small files with one clear purpose over large command or utility files.
- Documentation and generated planning artifacts are not code files, but keep them readable and split long documents when practical.

Run the local check before committing code changes:

```bash
./scripts/check-file-lines.sh
```
