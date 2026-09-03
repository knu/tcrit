<p align="center">
  <img src="assets/crit_logo.png" alt="tcrit" width="300">
</p>

# tcrit

> **tcrit** is a fork of [kevindutra/crit](https://github.com/kevindutra/crit). It installs the command as `tcrit` and includes fixes and improvements not yet merged upstream.

## Key changes from upstream

- **[Crit](https://crit.md/)-compatible agent workflow** — review commands block until the reviewer finishes, print an agent-facing result, and support iterative rounds through `tcrit --session <id>`; each round refreshes the changed-file set so newly added files appear without restarting the TUI.
- **Native Herdr and tmux workflows** — reviews open in a full-width Herdr tab or a tmux split; tcrit finds the invoking context from process ancestry even when tools such as Codex do not inherit multiplexer environment variables.
- **CritJSON review state and CLI** — comments use [Crit](https://crit.md/)-compatible `review.json` data, with `tcrit comment` and `tcrit comments` for automation.
- **File-level comments** — reviewers can press `f` to comment on the active file, with file threads kept in the comment sidebar instead of attached to a line.
- **Comment editing tools** — `ctrl+y` inserts GitHub-compatible suggestions for selected or anchored lines, including replies, and leaves the cursor at the end of the suggested code; `ctrl+o` edits comment and reply bodies in `$EDITOR`, while `ctrl+PgUp` / `ctrl+PgDn` scroll through code context and thread history.
- **Deleted-line comments** — removed lines, including lines in fully deleted files, can be selected and commented on from the keyboard or gutter.
- **Mouse-first TUI controls** — click file tabs, code lines, comment threads, sidebar items, dialog actions, and the review-finish button; use the wheel to scroll code and drag the gutter to select line ranges.
- **Versioned plan reviews** — `tcrit plan` saves immutable revisions and carries comment threads forward as the plan changes.
- **Richer review lifecycle** — comment threads can be replied to, resolved, reopened, and approved together; agent replies are visually distinct, complete code context and thread history remain scrollable while editing, and comments and changes can be navigated across files.
- **Improved diffs and Git handling** — inline replacements preserve whitespace, long syntax-highlighted lines wrap instead of being truncated, comment anchors survive edited rounds, and paths with spaces or special characters work correctly.
- **Agent integrations** — install review skills for Claude Code and Codex, or a Gemini CLI agent, with one command.
- **[Crit](https://crit.md/) CLI alignment** — customizable finish prompts, unified integration installers, and `tcrit check` were added as part of adopting the Crit CLI workflow.

TUI for reviewing AI-generated code and plans — built for human-in-the-loop agentic coding workflows.

Read a plan or review code changes across multiple files, leave inline comments, and let your coding agent address the feedback automatically.

Your agent writes code or a plan, you review it in the TUI, and the agent reads your comments and makes changes for the next round.

![crit code review demo](demo/code-review.gif)

## Install

### Claude Code Plugin Marketplace (recommended)

tcrit is available as a Claude Code plugin. Add the marketplace and install:

```
/plugin marketplace add knu/tcrit
/plugin install tcrit
```

Then use `/tcrit:review` in Claude Code. It asks whether you want to review code changes or a document, opens the TUI, and has Claude address your comments after each round.

### Command-line binary

With [mise](https://mise.jdx.dev/):

```bash
mise install github:knu/tcrit
```

With Go:

```bash
go install github.com/knu/tcrit/cmd/tcrit@latest
```

Make sure `$GOPATH/bin` (defaults to `~/go/bin`) is in your `PATH`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

### Manual skill install

If you prefer not to use the plugin, you can install the skill directly:

#### Claude Code

```bash
cd ~ && tcrit install claude-code   # Install globally (~/.claude/skills/)
tcrit install claude-code           # From a repo root: install for that project
```

The manual install provides these skills:

- `/tcrit-review` — choose between code and document/plan review
- `/tcrit-code-review` — review Git changes with `tcrit review --code`
- `/tcrit-plan-review <path>` — review immutable versions of a plan with `tcrit plan <path>`

#### Codex

```bash
cd ~ && tcrit install codex        # Install globally (~/.agents/skills/)
tcrit install codex                # From a repo root: install for that project
```

Then use `$tcrit-review` to choose between code and document/plan review. The installer also provides `$tcrit-code-review` and `$tcrit-plan-review` with Codex attribution and invocation syntax.

#### Gemini CLI

```bash
cd ~ && tcrit install gemini        # Install globally (~/.gemini/)
tcrit install gemini                # From a repo root: install for that project
```

Then use `@tcrit` to start a review in Gemini CLI.

#### Prompt templates

```bash
cd ~ && tcrit install prompts        # Install global templates under ~/.config/tcrit/prompts/
tcrit install prompts                # From a repo root: install under .tcrit/prompts/
tcrit check                         # Report stale installed integrations
```

## Requirements

- **Go 1.25+** for building from source
- **Herdr or tmux** for automatic agent review workflows. Without either multiplexer, tcrit can run the TUI directly in an interactive terminal.

### Starting a multiplexer session

For the most spacious review layout, start the agent inside [Herdr](https://herdr.dev/). Tcrit opens each review in a dedicated tab and returns to the agent tab when the review ends.

Alternatively, start tmux before launching your agent:

```bash
tmux new -s work
# Now launch Claude Code inside this tmux session
claude
```

Without Herdr or tmux, launch tcrit yourself in an interactive terminal when an agent asks you to review.

## CLI overview

Running `tcrit` with no subcommand reviews the current Git changes. Running `tcrit <file>` reviews one document.

| Command | Purpose |
|---------|---------|
| `tcrit [file]` | Review current Git changes, or review `file` when given |
| `tcrit review [--base <ref>] [file]` | Explicit form of the default review command; `--code` is accepted but unnecessary without a file |
| `tcrit plan [--name <slug>] [file]` | Create or continue a versioned plan review; reads stdin when `file` is omitted |
| `tcrit --session <id>` | Reconnect to a running review and start its next round |
| `tcrit comment ...` | Add comments or replies, import JSON, or clear the selected review |
| `tcrit comments [--json] [--all]` | List unresolved comments, optionally including resolved comments |
| `tcrit clear <file>` | Clear a document review; use `--code` for code review or `--all` for all reviews in the current directory |
| `tcrit status <file>` / `tcrit status --code` | Print the document or aggregate code-review status as JSON |
| `tcrit install <target>` | Install `claude-code`, `codex`, `gemini`, or `prompts`; `all` installs every agent integration |
| `tcrit check` | Report installed integration files that are stale |
| `tcrit completion <shell>` | Generate completion for Bash, Zsh, Fish, or PowerShell |

## Code Review (multi-file)

```bash
tcrit
# Equivalent explicit form; use --base <ref> to choose another diff base
tcrit review
```

Detects changed files in your git repo and opens a tabbed TUI with syntax highlighting, diff markers, and inline commenting across all changed files.

- Diffs staged, unstaged, and untracked changes against `HEAD` by default; falls back to `HEAD~1` or `main` when the worktree is clean
- Green gutter markers highlight changed lines
- Comments are aggregated across all files in the session

```bash
# Get unresolved comments in the agent-facing format
tcrit comments --json
```

### How code review works

1. An agent (or you) runs `tcrit review --code` — the TUI opens in a Herdr tab or tmux split and the command blocks
2. Navigate between files and leave inline comments on the changes
3. Press `q` or click the footer button — with unresolved comments the button is **Finish Review**, without any it is **Approve**
4. On finish, the blocked command prints the unresolved comments and instructions on stdout and `approved: true|false` on stderr
5. The agent edits the files, replies with `tcrit comment --reply-to`, and runs the printed `tcrit --session <id>` to start the next round; the waiting TUI reloads with the fixes and replies
6. Resolve comments with `r` and approve to end the loop

## Plan Review (versioned)

```bash
tcrit plan docs/plans/my-plan.md            # slug derived from the first heading
tcrit plan --name auth docs/plans/plan.md   # pinned slug
```

Saves the document as an immutable numbered version under `$XDG_STATE_HOME/tcrit/plans/<slug>/` (or `~/.local/state/tcrit/plans/<slug>/` when `XDG_STATE_HOME` is unset) and opens a review of the latest version. Re-running with the same slug saves the next version and starts the next review round; comments carry forward onto the revised text. The command also accepts plan content on stdin.

## Document Review (single file)

```bash
tcrit review docs/plans/my-plan.md
```

Opens a full-screen terminal UI with syntax-highlighted markdown, a comment sidebar, and modal overlays for adding/editing comments.

### Multiplexer mode

When `tcrit review` runs inside Herdr, the TUI automatically opens in a dedicated full-width tab. Inside tmux, it opens in a side-by-side split pane. In both cases the invoking command blocks until you finish the review — the same feedback loop as [crit](https://github.com/tomasz-tomczyk/crit), with a TUI in place of the browser.

Tcrit resolves the Herdr workspace, tab, and pane or the tmux server and pane from the invoking process tree. This also works with agents such as Codex that do not preserve the multiplexer environment in command runners. When multiplexers are nested, the nearest one in the process ancestry owns the review.

### How document review works

1. Claude writes a plan (or you open any markdown file)
2. `tcrit review <path>` opens the TUI — read through and leave inline comments
3. Finish the review with `q`; comments are saved as crit-compatible `review.json` under `$XDG_STATE_HOME/tcrit/reviews/` (or `~/.local/state/tcrit/reviews/`)
4. Claude receives the unresolved comments from the blocking command (or via `tcrit comments --json`), edits the document, and replies to each comment
5. Claude runs the printed `tcrit --session <id>`; the TUI reloads with the fixes for the next round

## Keybindings

| Key                                   | Action                                   |
|---------------------------------------|------------------------------------------|
| `j` / `k`                             | Move down / up through current and deleted lines |
| `ctrl+d` / `ctrl+u` / `PgDn` / `PgUp` | Half page down / up                      |
| `g` / `G` / `Home` / `End`            | Jump to top / bottom                     |
| `enter`                               | Add comment at current line              |
| `f`                                   | Add comment on the current file          |
| `v`                                   | Visual select mode (multi-line comments) |
| `s`                                   | Toggle comment sidebar                   |
| `[` / `]`                             | Jump to prev / next comment              |
| `r`                                   | Resolve / unresolve the focused comment  |
| `d`                                   | Delete the selected comment after confirmation |
| `?`                                   | Show all keyboard shortcuts              |
| `q`                                   | Finish review (Approve when no unresolved comments remain) |

**Comment dialogs:**

| Key      | Action                                                        |
|----------|---------------------------------------------------------------|
| `ctrl+s` | Save the comment or reply                                     |
| `ctrl+o` | Edit the comment or reply in `$EDITOR`                         |
| `ctrl+y` | Insert a suggestion block for the selected or anchored lines  |
| `ctrl+PgUp` / `ctrl+PgDn` | Scroll code context and thread history             |

**Code review only:**

| Key                 | Action                         |
|---------------------|--------------------------------|
| `tab` / `shift+tab` | Next / previous file tab       |
| `n` / `N`           | Jump to next / previous change |
| `/`                 | Search file tabs               |

## Mouse controls

- Click a file tab, code line, inline comment, sidebar, or sidebar comment to focus it.
- Scroll code with the mouse wheel.
- Hover over the `+`/`-` gutter to reveal a yellow `>` comment marker, then click to comment on a current or deleted line, or drag to select multiple lines on the same diff side.  Dragging to the top or bottom edge scrolls one line at a time.
- Click inside a comment text box to focus it and position the cursor, or use the mouse wheel to move through longer comments.
- Click actions in comment and finish dialogs, including **Close**.  The footer **Approve** / **Finish Review** button opens the finish dialog.

## Scriptable CLI

The comment CLI follows [crit](https://github.com/tomasz-tomczyk/crit)'s
syntax, so agent tooling written for crit works against TCrit reviews:

```bash
# Review-level, file-level, and line-level comments
tcrit comment "Overall this looks good"
tcrit comment docs/plan.md "Needs a rewrite"
tcrit comment docs/plan.md:15 "This needs more detail"
tcrit comment docs/plan.md:10-20 "Rethink this section"

# Reply to a comment (e.g. an AI explaining how it addressed feedback)
tcrit comment --reply-to c_a3f8b2 --author 'Claude Code' "Split into two functions"

# Bulk import comments and replies from JSON
tcrit comment --json --file comments.json

# List unresolved comments (add --all for resolved ones, --json for JSON)
tcrit comments
tcrit comments --json

# Clear review state
tcrit clear docs/plan.md
tcrit clear --code
tcrit clear --all

# Get review comments as JSON (single file)
tcrit status docs/plan.md

# Get all code review comments as JSON
tcrit status --code
```

## Shell Completions

```bash
# Bash
tcrit completion bash > /etc/bash_completion.d/tcrit

# Zsh
tcrit completion zsh > "${fpath[1]}/_tcrit"

# Fish
tcrit completion fish > ~/.config/fish/completions/tcrit.fish
```

## Development

```bash
go test ./...
go build ./...
go vet ./...
```

## License

MIT
