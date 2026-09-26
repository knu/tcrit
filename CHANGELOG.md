# Changelog

## 0.11.0

### Added

- **File path completion in comment editors** — typing `@` and a path character at the start of a line or after whitespace lists the entries of that directory, one level at a time, below the `@` or above it when the screen is short.  Candidates match fuzzily; `tab` accepts the first one, Up/Down with `enter` accept a chosen one, and `esc` closes the list.  A file completes with a trailing space; a directory completes with `/` and lists its entries.
- **Fuzzy file selector** — `alt+p` opens a dialog with a search field over the review's file tabs.  Space-separated words must all match, and basename matches rank first.  Up/Down or `ctrl+p` / `ctrl+n` choose, `enter` switches tabs, and the dialog keeps a fixed size while typing.

### Changed

- File references accept unescaped letters, digits, and combining marks of any script; non-ASCII punctuation still ends a reference.  Prose may follow a path directly without a space, as full-width text commonly does in some languages; the longest leading part naming an existing file is linked.  Copied and completed paths escape underscores at word boundaries so names such as `__init__.py` survive Markdown rendering; both spellings are recognized.
- Added `github.com/sahilm/fuzzy` as a direct dependency for fuzzy matching.

### Removed

- The `/` tab search.  The key is reserved for a future in-source search.

## 0.10.0

### Added

- **Clipboard images in comments and replies** — `ctrl+v` pastes copied image files or bitmap data on macOS through AppKit, and images on Linux through `wl-paste` or `xclip`.  When no image is available, it falls back to text.  Attachments support PNG, JPEG, GIF, and WebP up to 5 MiB each; Escape cancels a pending paste.
- Image attachments persist across review rounds and stop/resume.  The agent reads images in the final review result before running the printed session cleanup command.  The TUI shows Markdown references rather than inline image previews.

### Changed

- Bundled agent skills reduce excessive token use during human review waits by keeping waits inside tools and avoiding repeated reasoning and status polling.  They now bound the overall wait to ten minutes, then preserve the running review and recoverable output; `hey` resumes result collection when the host cannot resume the agent automatically.
- CI runs formatting checks, vet, and tests on both Linux and macOS, including the AppKit clipboard tests.  The required `test` check succeeds only when both platforms pass.
- Updated Go to 1.27.1, Go dependencies, and Harden-Runner.

## 0.9.5

### Added

- **File references and source navigation** — comments and replies link existing `@path/to/file` references, with optional ` L40` line numbers and backslash escapes for paths containing spaces.  Click to jump within the review or confirm opening other locations in `$EDITOR`.  `alt+g` asks for a line number, and `alt+e` opens the current file at the cursor line.  Editor navigation supports both `--goto FILE:LINE` and `+LINE FILE`.

### Changed

- Bundled agent skills distinguish recoverable launch failures from interruptions after the TUI opens.  They keep waiting for review submission unless the host can automatically resume the agent and preserve the result, and clarify completion notifications and nested wait handling.
- Check previously unhandled errors in CLI setup, socket cleanup, and tests; simplify test code and clear all lint findings.

### Fixed

- Reply editors omit the reply being edited from the reference thread, so its saved body is not duplicated above the input field.  Other replies and their original numbering remain visible.

## 0.9.4

### Changed

- New review rounds focus the first visible thread, in file and display order, with a reply added by another author since the previous submission.  The thread expands and scrolls to its latest new reply, including resolved threads and file comments.  Without a new reply, or when resuming an unfinished round, the starting position is unchanged.  `[` / `]` navigation is unchanged.
- File tabs show both added and deleted line counts, such as `(+12 -5)`, omitting zero counts.
- Syntax highlighting reuses the lexer, style, and formatter within each file, substantially reducing startup time for diffs with many deleted lines while preserving rendered output.
- Refactored TUI internals and consolidated overlapping tests without changing their intended behavior.

## 0.9.3

### Added

- **Kill ring** — comment and reply editors share up to 60 entries during the current TUI run.  Line and word kills (`ctrl+k`, `ctrl+u`, `ctrl+w`, `alt+d`, and their aliases) save deleted text; consecutive kills combine in text order.  `ctrl+y` yanks the latest entry, and `alt+y` immediately after a yank or yank-pop cycles through older entries.  Other input ends the sequence.  The ring is independent of the system clipboard; ordinary Delete/Backspace do not add entries.

### Changed

- File-comment threads appear above the first source or deleted line, directly below the tabs, including for empty, deleted, and binary files.  They remain listed in the sidebar; creating a file comment or navigating to one focuses its inline box.  Inline reply, resolve, delete, folding, and visibility controls apply to file comments too.

### Breaking changes

- Suggestion insertion moves from `ctrl+y` to `alt+s` (`M-s`).  `ctrl+y` now yanks from the kill ring; the Suggest button remains available.

## 0.9.2

### Changed

- `f` replies to an existing file-comment thread, including resolved threads, and creates a new thread only when none exists.  Saving a reply reopens a resolved thread.
- Inline and sidebar thread headers now have clickable **Resolve / Resolved** toggles and a right-aligned red **x** deletion button.  Deletion remains limited to your comments from the current round with no replies and requires confirmation.
- Resolution controls stay in place when toggled; collapsed resolved threads omit the author name.
- Keyboard help uses distinct colors for section headings and key names, with brighter descriptions.
- Bundled agent skills read saved feedback after a nonzero review exit, then stop work and wait for explicit chat instructions instead of automatically restarting TCrit.

### Fixed

- Keep the right outer frame visible when the comment sidebar is open, with mouse targets aligned to the pane layout.

## 0.9.1

### Added

- **Ignore whitespace** — `w` toggles whitespace-insensitive diffs across all files in Git and supplied-diff reviews.  Whitespace-only changes are excluded from highlighting and `n` / `N` navigation while source text, line numbers, and comment context remain intact.  Added and deleted blank lines remain visible; the setting lasts for the current TUI run.

### Changed

- `n` / `N` visits change hunks and unresolved comment threads in display order from either pane, including file comments, deleted-line comments, and multiple threads on one line.  Selecting a hidden comment reveals it; resolved threads are skipped.
- `Tab` / `Shift+Tab` switches file tabs from the sidebar as well as the content pane, preserving pane focus.

## 0.9.0

### Added

- **Saved review sessions** — stop a review with `tcrit stop --session <id>`, discover saved sessions with `tcrit status`, and resume after process exit with `tcrit --session <id>`.  Source snapshots preserve comment context across rounds; `tcrit clear --session <id>` deletes a stopped review.
- **Read diffs without comment boxes** — `H` hides inline comments and replaces the sidebar with a narrow gutter.  Click a comment marker to open its thread; press `H` again to restore comments without changing resolved-comment folding.

### Changed

- Every submitted round closes its TUI and dedicated Herdr tab or tmux pane.  The next round opens a new process and restores saved comments and replies.
- Review results include resolved threads and replies, even on approval, so final instructions survive automatic cleanup.  Bundled skills read all returned threads and reply only to new feedback or substantive updates.
- Focused comments and editors use bright, thick borders; unfocused comments use thin blue borders.  Modal dialogs also use thick borders.
- Saving an empty new comment or reply closes the editor without creating an entry.  Saving an unchanged edit leaves review state unchanged; clearing an existing entry and saving deletes it without another confirmation.
- Legacy review state is ignored without a warning.

### Breaking changes

- Each new review receives an independent session ID, even for the same directory, scope, or plan name.  Continue an existing review explicitly with `--session <id>` instead of repeating its original command.
- Plan revisions are stored inside their review session directory.  Submit revised plans with `tcrit plan --session <id> <file>`; plain `tcrit --session <id>` reopens saved plan or supplied-diff input.
- `tcrit clear` refuses active reviews.  Stop the review before deleting its saved state.

## 0.8.2

### Changed

- Resolving a thread with `r` now moves focus to the next unresolved thread, wrapping across files and skipping resolved threads even when unfolded with `h`.  When none remain, focus returns to the source.  Reopening a thread keeps its focus.

## 0.8.1

### Fixed

- `[` / `]` now visit resolved line and file comments when unfolded with `h`, while continuing to skip them when folded

### Documentation

- Clarified comment editing shortcuts, suggestion replacement, and scrolling through code context and thread history

## 0.8.0

### Added

- **Supplied diff reviews** — `tcrit --diff[=FILE]` accepts Git unified diffs from a file or stdin, including outside a repository; snapshots survive Herdr/tmux launches and can be replaced for another review round
- **Fixed review scopes** — `--scope=all|staged|unstaged|A..B|A...B` keeps each comparison and its comments in a separate session; `--staged` and `--unstaged` are aliases, and committed comparisons display the right-hand commit's content
- **Native input cursor** — comment and reply editors expose the real terminal cursor so IME composition follows the insertion point after wrapping and scrolling
- **Resolved-thread display toggle** — `h` folds or unfolds resolved comments across all files, including their visibility in the sidebar

### Changed

- **Author-based comment colors** — comments and replies share a color per author, with your own messages in white
- Unfocused threads show only the latest message; focused histories use at most 10 wrapped rows, and reply dialogs use up to 16, with older or longer content accessible by scrolling
- The mouse wheel focuses and scrolls comment threads; resolved threads expand on focus and fold again on blur unless folding is disabled
- `[` / `]` skip resolved comments; vertical movement still visits them, while visual range selection skips comment boxes
- Suggest selects the inserted code for immediate deletion or replacement while preserving the fences and existing comment
- Gemini installs the same shared `tcrit` and `tcrit-cli` skills as the other integrations, with review rounds handled by the original agent

### Breaking changes

- Git review defaults to `--scope=all` (staged, unstaged, and untracked changes).  To review against a branch, specify a comparison such as `--scope=main...HEAD`
- Removed `--base`, `--base-branch`, the `base_branch` setting, and automatic base-branch fallback; use `--scope` instead
- Existing installations of the old Gemini subagent may retain `.gemini/agents/tcrit.md`; check for customizations before removing it and reinstall the shared skills with `tcrit install gemini`

## 0.7.3

### Added

- **Staged-only reviews** — `tcrit --staged` and `tcrit review --staged` read the file list, content, and diffs exclusively from the Git index; bundled agent integrations document the safer scope
- **Visible review scope** — the code-review header always identifies staged changes, the working tree, or the active base ref

## 0.7.2

### Fixed

- `tcrit install opencode` run from the home directory now installs into OpenCode's global config directory (`$XDG_CONFIG_HOME/opencode`, `~/.config/opencode` by default) instead of `~/.opencode`

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
