# Changelog

## 0.7.1

### Added

- **OpenCode integration** — `tcrit install opencode` installs the `/tcrit` command and the `tcrit-cli` skill; the `all` target now includes OpenCode
- **`tcrit-cli` reference skill** — documents `tcrit comment`, `tcrit comments`, session and plan targeting, bulk JSON input, the review file format, and clearing, so agents no longer guess at headless usage

### Changed

- **Simpler skill set** — a single `tcrit` skill passes its arguments straight to the CLI, which chooses between git changes, a document, and a plan, instead of asking the user which mode to use; Claude Code, Codex, OpenCode, and Gemini all install the same `tcrit` + `tcrit-cli` pair
- The Claude Code plugin ships the skills under `plugin/tcrit/skills/` and is invoked as `/tcrit:tcrit`

### Fixed

- Files deleted since the base ref now show their removed lines in the code review TUI instead of an empty tab
- A commented file whose addition is reverted in a later round now shows a placeholder instead of its stale content; the comments stay in the sidebar and in `review.json`
- Resolved file comments stay in the sidebar as collapsed headers so they can be reopened with `r`, instead of vanishing
- `[` and `]` now visit file comments too, moving focus to their sidebar entry, and work from the sidebar as well as the content pane

### Removed

- The `tcrit-review`, `tcrit-code-review`, and `tcrit-plan-review` skills, the `tcrit:review`, `tcrit:code-review`, and `tcrit:plan-review` plugin commands, and the legacy root-level plugin manifest inherited from upstream

## 0.7.0

### Added

- **Native Herdr workflow** — discover the invoking Herdr workspace, tab, and pane from inherited context or process ancestry, open reviews in a dedicated full-width tab, and restore focus to the agent tab between rounds

### Changed

- Multiplexer discovery now supports Herdr and tmux together and selects whichever context is nearest in the process ancestry when they are nested

## 0.6.3

### Added

- **Codex skill installer** — `tcrit install codex` installs project-local or global review skills with Codex invocation syntax and reply attribution; the `all` target now includes Codex

### Changed

- `Ctrl-PgUp` and `Ctrl-PgDn` scroll through complete code context and thread history while the comment textarea keeps focus

### Fixed

- A reviewer can edit only their latest reply when it is also the thread's latest reply, preventing an earlier response from being overwritten after another participant replies

## 0.6.2

### Changed

- Agent replies now use cyan text so they remain visually distinct from reviewer comments in inline and sidebar threads
- Dialog shortcut keys use highlighted backgrounds, with labels matching their actual keys
- Reply editors keep complete code context and thread history together in one scrollable region while leaving actions accessible in short terminal panes
- Newly saved comments remain selected so they can be deleted immediately if needed

### Fixed

- Inserting a suggestion leaves the cursor at the end of the final suggested code line and scrolls it into view

## 0.6.1

### Added

- **Deleted-line comments** — select and comment on removed lines with the keyboard or mouse, including lines in fully deleted files
- **Direct comment deletion** — press `d` to delete the selected comment after confirmation without opening its thread first

### Fixed

- Application headers consistently use TCrit branding
- The code header remains stable while dragging a mouse selection beyond the viewport

## 0.6.0

### Added

- **Mouse-first TUI controls** — click file tabs, code lines, comment threads, sidebar items, dialog buttons, and the review-finish button; scroll with the mouse wheel, and drag the gutter to select line ranges

## 0.5.1

### Added

- **Suggestion blocks** — press `Ctrl-Y` to insert a GitHub-compatible `suggestion` block for the selected line or a line comment's anchor, including in replies
- **External editor support** — press `Ctrl-O` to edit comment and reply bodies in `$EDITOR` without leaving the review session

### Fixed

- Changed-line backgrounds remain continuous across Markdown bold and code spans

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
