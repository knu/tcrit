# Changelog

## 0.5.0

### Added

- **File-level comments** — press `f` to comment on the active file without attaching feedback to a specific line; file threads appear at the top of the comment sidebar
- **Keyboard help** — press `?` for a compact reference covering navigation, review, search, selection, and dialog controls
- **Native tmux context discovery** — tcrit can find the invoking pane through process ancestry when agent environments do not inherit `TMUX` or `TMUX_PANE`

### Changed

- Resolved comment threads collapse out of the active sidebar while remaining available inline for reopening
- Code review tabs refresh between rounds so newly added and removed files appear without restarting the TUI
- The persistent footer now emphasizes review-specific actions and leaves conventional navigation keys to the help screen

### Fixed

- Concurrent CLI comments and replies no longer overwrite one another
- Long syntax-highlighted source lines wrap instead of being truncated
- Nested Markdown styles preserve ANSI colors and backgrounds correctly

## 0.4.0

### Changed

- **Crit-compatible review workflow** — review state now uses CritJSON, review commands block until feedback is finished, and `tcrit comment` / `tcrit comments` use crit-compatible syntax for agent replies and automation
- **Installation commands** — `tcrit install` and `tcrit check` replace the former `setup-claude` and `setup-gemini` commands

### Added

- **Versioned plan reviews** — `tcrit plan` stores immutable revisions and carries comments forward as plans change
- **Thread lifecycle controls** — reviewers can resolve or reopen threads, reply from the TUI, and delete only comments or replies they authored in the current round
- **Cross-file navigation** — `[` / `]` traverse comment threads, while `n` / `N` traverse changes without wrapping; `<` / `>` jump to the current file's first or last line
- **Resolve-all approval** — finishing a round without new feedback can resolve every remaining thread and approve in one step

### Fixed

- Comments retain their intended location across edited review rounds, including drift detection when the original text disappears
- Inline replacement diffs preserve whitespace and highlight only the words that actually changed

## 0.3.1

### Added

- **`--version` flag** — `tcrit --version` reports the build version, resolved from release ldflags or Go build info

### Fixed

- **Release automation** — GoReleaser still pointed at the pre-rebrand `cmd/crit` path, so tagged releases failed to build; releases now ship `tcrit` binaries again
- Release builds are now gated behind a passing CI run (gofmt, `go vet`, `go test`) with hardened, SHA-pinned workflows, and Renovate keeps dependencies and action refs up to date

## 0.3.0

Forked from [crit](https://github.com/kevindutra/crit) and rebranded to **tcrit**.

### Changed

- **Renamed to tcrit** — the binary, embedded skills, commands, and plugin are now `tcrit`; the Go module moved to `github.com/knu/tcrit`

### Added

- **`tcrit clear`** — clear comments for a document or code review session; `--all` deletes all saved review state (upstream PR #9)
- **Re-review flow** — starting a new review asks whether to keep or clear existing comments
- **Approval confirmation** — quitting a review without new comments asks for confirmation before approving
- **Gemini CLI support** — `tcrit setup-gemini` installs Gemini CLI agents for the review workflow (upstream PR #8)
- **Paging keybindings** — `PgUp`/`PgDn`/`Home`/`End` in the review TUI (upstream PR #10)

### Fixed

- Paths with spaces and special characters are handled correctly in git operations
- `n`/`N` change navigation jumps to the correct offsets in files with long wrapped lines
- Detached reviews open the tmux split from the invoking pane

## 0.2.2

### Changed

- Bumped plugin versions to 1.2.2

## 0.2.1

### Fixed

- `crit review --code --detach --wait` now correctly opens in a tmux split pane instead of failing with a TTY error

## 0.2.0

### Added

- **Multi-file code review** — `crit review --code` detects changed files in your git repo and opens a tabbed TUI with syntax highlighting and diff markers
- **Tabbed file navigation** — `tab`/`shift+tab` to switch files, `/` to search
- **Change navigation** — `n`/`N` to jump between changed lines within a file
- **Aggregate status** — `crit status --code` outputs comments across all reviewed files as JSON
- **Session management** — code review sessions are persisted to `.crit/code-review.yaml`
- **Review router skill** — `/crit:review` asks whether to review code or a document, then routes to the appropriate workflow
- **Code review skill** — `/crit:code-review` runs the full code review workflow in Claude Code
- **Plan review skill** — `/crit:plan-review` routes single-file document reviews

### Changed

- `tab` now switches between file tabs in code review mode (was: switch panes)
- `s` toggles the comment sidebar (was: `tab`)
- Review skill restructured into router pattern with separate code and plan review skills

## 0.1.0

Initial release.

- Interactive TUI for reviewing markdown documents
- Inline comments with visual select mode
- tmux split pane integration (`--detach --wait`)
- Scriptable CLI (`crit comment`, `crit status`)
- Claude Code skill (`/crit-review`)
- Shell completions (bash, zsh, fish)
