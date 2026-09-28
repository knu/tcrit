# Agent instructions

This file is read by coding agents such as Claude Code and Codex.

## Local notes

If `AGENTS.local.md` exists next to this file, read it before starting work and follow it.  It holds a developer's local additions, is not committed, and takes precedence over this file where they differ.

## Development

Build, test, and vet with the standard Go tools:

```bash
go build ./...
go test ./...
go vet ./...
```

See the README for installing the binary and the agent skills, and `docs/upstream-differences.md` for how this fork differs from upstream crit.
