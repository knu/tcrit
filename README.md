<p align="center">
  <img src="assets/crit_logo.png" alt="tcrit" width="300">
</p>

# tcrit

> **tcrit** is a fork of [kevindutra/crit](https://github.com/kevindutra/crit). It installs the command as `tcrit` and includes fixes and improvements not yet merged upstream.

## Key changes from upstream

- **Read diffs without comment boxes** — press `H` to hide inline comments and replace the sidebar with a narrow gutter, giving the source more space.  A `💬` marks commented lines, including deleted lines; click a marker to open its thread.  Press `H` again to restore comments; the `h` setting for resolved comments is preserved.  Opening the sidebar with `s` or jumping to a file comment restores the sidebar.  Comment editors still show the full thread.
- **[Crit](https://crit.md/)-compatible agent workflow** — review commands block until the reviewer finishes, print an agent-facing result, and support iterative rounds through `tcrit --session <id>`; each round closes the TUI and its dedicated pane or tab, and the next round restores saved state in a new process.
- **Native Herdr and tmux workflows** — reviews open in a full-width Herdr tab or a tmux split; tcrit finds the invoking context from process ancestry even when tools such as Codex do not inherit multiplexer environment variables.
- **CritJSON review state and CLI** — comments use [Crit](https://crit.md/)-compatible `review.json` data, with `tcrit comment` and `tcrit comments` for automation.
- **Independent saved sessions** — every new review gets its own ID, even for the same directory, scope, or plan name. `tcrit stop --session <id>` preserves state; `tcrit --session <id>` resumes it after process exit.
- **Fixed review scopes** — choose `--scope=all|staged|unstaged` or a committed comparison such as `--scope=main..HEAD` / `--scope=main...`.  Each scope keeps its comments in a separate session and stays visible in the TUI header.  The default is `all`; `--staged` and `--unstaged` are shortcuts.
- **Supplied diff reviews** — `git diff <base> <head> | tcrit --diff` reviews arbitrary Git unified diffs, including outside a repository. Input snapshots survive Herdr/tmux launches and can be replaced for the next review round.
- **File-level comments** — reviewers can press `f` to comment on the active file, with file threads kept in the comment sidebar instead of attached to a line.
- **Readable comment threads** — your comments and replies appear in white; other authors use cyan, green, orange, purple, then yellow.  Unfocused inline and sidebar threads shrink to the latest message, down to one row.  Focus expands the history to at most 10 wrapped rows; reply dialogs show up to 16, shrinking on small terminals.  Expanded threads show the latest message at the bottom with earlier history above, or start at its first line when it exceeds the available height.  No blank padding follows the latest message; older messages and long bodies remain available by scrolling.
- **Comment editing tools** — `ctrl+y` inserts GitHub-compatible suggestions for selected or anchored lines, including replies, and selects the inserted code for immediate deletion or replacement, leaving the suggestion fences intact; `ctrl+o` edits comment and reply bodies in `$EDITOR`, while `ctrl+PgUp` / `ctrl+PgDn` scroll through code context and thread history.
- **Visible comment focus** — focused comments and editors use bright, thick borders; unfocused comments use thin blue borders.  The sidebar separator follows the same focus distinction, and modal dialogs use thick borders.
- **Native input cursor** — comment and reply editors position the real terminal cursor at the insertion point so terminal IMEs can display composition there, including after wrapping and scrolling.
- **Deleted-line comments** — removed lines, including lines in fully deleted files, can be selected and commented on from the keyboard or gutter.
- **Mouse-first TUI controls** — click file tabs, code lines, comment threads, sidebar items, dialog actions, and the review-finish button; use the wheel to scroll code and drag the gutter to select line ranges.  Click **☐ Resolve** / **☑︎ Resolved** in a thread header to toggle its resolution, including file comments.  Eligible comments also have a red **x** button at the right edge of the header to request deletion.
- **Combined change and comment navigation** — `n` / `N` visit change hunks and unresolved comments in display order across files, from either pane.  File comments and separate threads on the same line are included; resolved comments are skipped even when unfolded.
- **Versioned plan reviews** — `tcrit plan` saves immutable revisions and carries comment threads forward as the plan changes.
- **Richer review lifecycle** — comment threads can be replied to, resolved, reopened, and approved together.  Resolving with `r` advances to the next unresolved thread across files, or returns focus to the source when none remain.  Resolved inline and file-comment threads expand on keyboard focus, click, or wheel scrolling, keeping their resolved status, and collapse again when focus leaves.  Press `h` to unfold resolved comments across all files, including line comments in the sidebar; press it again to restore folding.  Unfolded threads use the same compact latest-message view as open threads until focused.  Complete code context and thread history remain scrollable while editing, and `[` / `]` and `n` / `N` navigate comments (including file comments) and changes across files.  Comment navigation skips resolved threads while folded and includes them when unfolded with `h`.
- **Continue file discussions** — press `f` to reply to an existing file-comment thread, or create one when the file has none.
- **Improved diffs and Git handling** — inline replacements preserve whitespace, long syntax-highlighted lines wrap instead of being truncated, comment anchors survive edited rounds, and paths with spaces or special characters work correctly.
- **Ignore whitespace** — press `w` to toggle whitespace-insensitive diffs across all files, including supplied patches.  Source text and comment anchors stay intact, and `n` / `N` skips whitespace-only changes.
- **Review navigation across panes** — `n` / `N` visits changes and unresolved threads in display order, including file and deleted-line comments, and reveals hidden comments when selected.  `Tab` / `Shift+Tab` switches files from either the content pane or the sidebar while preserving pane focus.
- **Agent integrations** — one command installs the shared `tcrit` review loop and `tcrit-cli` reference for Claude Code, Codex, OpenCode, and Gemini CLI. The current agent handles review rounds with the original task context; the loop picks code, document, or plan review from its arguments instead of asking.
  The skills limit replies to new feedback or substantive updates, and require stopping an abandoned review before opening another TUI in the same directory.
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

Then use `/tcrit:tcrit [file]` in Claude Code. It opens the TUI on the git changes or on the given document and has Claude address your comments after each round.

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

If you prefer not to use the plugin, install the integration for your agent directly. Every target provides two pieces:

- `tcrit [file]` — the interactive review loop. It opens the TUI on the git changes (`tcrit`), a document (`tcrit <file>`), or a versioned plan (`tcrit plan <file>`), then has the agent address the comments round by round.
- `tcrit-cli` — a reference skill the agent loads when it needs `tcrit comment`, `tcrit comments`, session or plan targeting, bulk JSON input, or the review file format.

The finish output includes resolved threads and replies, even on approval, so final reviewer instructions remain available after automatic cleanup. The review loop reads all returned threads for new instructions before committing or continuing. Unanswered agent comments and completion replies remain unchanged until there is new feedback or a substantive update. When you cancel a review or switch tasks, the agent stops its TUI and checks that its dedicated pane or tab has closed before opening a replacement. Each round closes its TUI automatically. `tcrit stop --session <id>` preserves saved state for later resumption; `tcrit clear` explicitly deletes it and refuses active reviews.

Run the installer from your home directory to install globally, or from a repository root to install for that project only.

#### Claude Code

```bash
cd ~ && tcrit install claude-code   # ~/.claude/skills/{tcrit,tcrit-cli}/
tcrit install claude-code           # From a repo root: install for that project
```

Then use `/tcrit [file]`.

#### Codex

```bash
cd ~ && tcrit install codex        # ~/.agents/skills/{tcrit,tcrit-cli}/
tcrit install codex                # From a repo root: install for that project
```

Then use `$tcrit`. The installed skills use Codex invocation syntax and reply attribution.

#### OpenCode

```bash
cd ~ && tcrit install opencode     # ~/.config/opencode/commands/tcrit.md and ~/.config/opencode/skills/tcrit-cli/
tcrit install opencode             # From a repo root: install for that project
```

Then use `/tcrit [file]`.

#### Gemini CLI

```bash
cd ~ && tcrit install gemini        # ~/.gemini/skills/tcrit/ and ~/.gemini/skills/tcrit-cli/
tcrit install gemini                # From a repo root: install for that project
```

Ask Gemini to use the `tcrit` skill to review your changes or a document. The current agent runs the review loop using the same instructions as the other integrations. Use `/skills reload` if Gemini CLI is already running, and `/skills list` to check discovery.

When upgrading from the previous `@tcrit` subagent integration, remove the old `.gemini/agents/tcrit.md` (or `~/.gemini/agents/tcrit.md` for a global install) after preserving any customizations. The installer does not delete existing agent definitions.

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
| `tcrit --staged` | Review only changes staged in the index |
| `tcrit --unstaged` | Review unstaged and untracked changes |
| `tcrit --scope <scope>` | Select all (default), staged, unstaged, or a committed comparison |
| `tcrit --diff[=FILE]` | Review a supplied diff; omit FILE or use `-` for stdin |
| `tcrit review [--scope <scope>] [file]` | Explicit form of the default review command; `--staged` reviews only the index |
| `tcrit plan [--name <slug>] [file]` | Create an independent versioned plan review; use `--session <id>` to continue one |
| `tcrit --session <id>` | Resume a saved review after process exit, from its original directory |
| `tcrit stop --session <id>` | Stop the TUI while keeping saved comments and round context |
| `tcrit status` | List saved sessions in this directory, including whether each is running |
| `tcrit clear --session <id>` | Delete one stopped review |
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
# Equivalent explicit form
tcrit review
# Review only changes staged in the index
tcrit --staged
# Equivalent explicit form
tcrit review --staged
# Review unstaged and untracked changes
tcrit --unstaged
# Review an arbitrary commit range from stdin
git diff main feature | tcrit --diff
# Equivalent explicit form
git diff main feature | tcrit review --diff
```

Detects changed files in your git repo and opens a tabbed TUI with syntax highlighting, diff markers, and inline commenting across all changed files.

- Diffs staged, unstaged, and untracked changes against `HEAD` by default (`--scope=all`); reports no changes when the worktree is clean
- With `--staged`, reads both the file list and displayed contents from the index, excluding unstaged and untracked work
- With `--diff`, reads the supplied unified diff and labels the scope **Supplied diff**; it cannot be combined with `--scope`, `--code`, `--staged`, or `--unstaged`
- Green gutter markers highlight changed lines
- Comments are aggregated across all files in the session

```bash
# Get unresolved comments in the agent-facing format
tcrit comments --json
```

### Supplied diffs

`tcrit --diff=changes.diff` or `tcrit --diff changes.diff` accepts one Git unified diff, with additions, deletions, renames, and binary-file placeholders. Relative and absolute paths are accepted; bare `--diff`, `--diff=-`, and `--diff -` read standard input. It works without a Git repository and uses the controlling terminal for keyboard input when stdin is a pipe. TCrit saves the input diff and the file content prepared for display in the review session directory. The separate TUI process launched in Herdr or tmux reads this saved data, so it shows the same changes without needing access to the original input.

When the diff identifies the pre-change file content stored in the local Git object database, TCrit reads that content and applies the diff in memory to reconstruct the complete changed file. Otherwise it shows only the supplied context and changes at their original line numbers, marking omitted context explicitly. It never fills missing context from the working tree. Suggestions cannot span omitted lines. Input is limited to 64 MiB; when only partial file content is available, line numbers up to 1,000,000 are supported.

Every supplied diff starts an independent session. For another round with updated input, regenerate it and target the saved session explicitly from its original directory:

```bash
git diff main feature | tcrit --diff --session <id>
```

A new TUI opens with the replaced snapshot and saved comments. Plain `tcrit --session <id>` opens the saved diff without replacing its input. When incomplete context prevents reliable comment relocation, changed snapshots preserve the original coordinates and mark the comments as drifted for inspection.

### How code review works

`--base`, its `--base-branch` alias, and the `base_branch` configuration key have been removed. Use `--scope=A..B` or `--scope=A...B` for committed comparisons; these compare committed snapshots, whereas the old `--base` compared against the working tree.

Choose an explicit scope or a committed comparison:

```bash
tcrit --scope=all                 # HEAD versus working tree, including untracked files
tcrit --scope=staged              # HEAD versus index (--staged remains an alias)
tcrit --scope=unstaged            # index versus working tree, including untracked files
tcrit --scope=main..HEAD          # compare the two committed snapshots
tcrit --scope=main...             # compare the merge base with HEAD (omitted B)
tcrit --scope=v0.7.0..v0.7.3      # review a historical comparison
```

`--scope` also works with `tcrit review` and cannot be combined with a document or `--diff`.  In `A..B` and `A...B`, endpoints are resolved by Git and an omitted endpoint means HEAD: `main..` compares main with HEAD, while `main...` compares their merge base with HEAD.  Three-dot comparisons require a unique merge base.  The displayed contents come from the right endpoint, even if the working tree differs.  Keep the dots when omitting B so the comparison method remains explicit.

The selected scope stays fixed for the session.  To inspect another comparison, start a separate review with a different scope; each new invocation receives an independent session ID, even with identical scope arguments.  `--staged` and `--scope=staged` select the same kind of comparison, as do `--unstaged` and `--scope=unstaged`.  Conflicting scope flags are rejected.  Use `tcrit comments --session <id>` and `tcrit comment --session <id>` for these reviews; the finish prompt identifies the session.  Reconnecting for the next round retains the selected scope and refreshes comparison endpoints.  An empty comparison is rejected for a new review; a resumed review can still display saved comments when all changes have been removed.  Without explicit flags, the scope is all: HEAD versus the working tree plus untracked files.  A clean working tree does not fall back to committed changes.

1. An agent (or you) runs `tcrit review --code` — the TUI opens in a Herdr tab or tmux split and the command blocks
2. Navigate between files and leave inline comments on the changes
3. Press `q` or click the footer button — with unresolved comments the button is **Finish Review**, without any it is **Approve**
4. On finish, the blocked command prints all comment threads and replies, including resolved threads on approval, with instructions on stdout and `approved: true|false` on stderr
5. The agent edits the files, replies with `tcrit comment --reply-to`, and runs the printed `tcrit --session <id>` to start the next round; a new TUI restores the comments and remaps their anchors onto the updated contents
6. Resolve comments with `r` and approve to end the loop

## Stopping and resuming

```bash
tcrit status                    # list saved sessions in this working directory
tcrit stop --session <id>        # stop without deleting or approving
tcrit --session <id>             # reopen from the original directory
tcrit clear --session <id>       # explicitly delete a stopped review
```

The TUI and its dedicated Herdr tab or tmux pane close after every submitted round, including rounds with unresolved comments. Stopping mid-round keeps saved comments, replies, resolution status, and the current round number. Submitting a round advances the number when it is reopened. Saved source context supports comment relocation after edits; unsaved editor input and cursor position are not restored. Code and document reviews read their current source when resumed, while plan and supplied-diff reviews reopen the saved input unless replacement input is provided.

The session ID is printed when a review starts and included in the finish prompt. Use it for all comment operations when multiple reviews exist. Starting a new task does not delete earlier reviews. Approval retains the existing `cleanup_on_approve` behavior, which deletes approved review data by default.

## Plan Review (versioned)

```bash
tcrit plan docs/plans/my-plan.md            # slug derived from the first heading
tcrit plan --name auth docs/plans/plan.md   # pinned slug
```

Saves numbered versions and `current.md` inside the session directory, normally `~/.local/state/tcrit/reviews/<id>/`. Each new invocation creates an independent review, even with the same plan name. Run `tcrit plan --session <id> <file>` to submit a revised version, or `tcrit --session <id>` to reopen the saved version. The plan command also accepts stdin. Comments carry forward onto revised text.

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
4. Claude reads all returned threads for new instructions, including resolved threads, edits the document, and replies where there is new feedback or a substantive update.  Use `tcrit comments --json --all` to retrieve all saved threads separately.
5. Claude runs the printed `tcrit --session <id>`; a new TUI restores the review with the fixes for the next round

## Keybindings

| Key                                   | Action                                   |
|---------------------------------------|------------------------------------------|
| `j` / `k`                             | Move down / up through current and deleted lines |
| `ctrl+d` / `ctrl+u` / `PgDn` / `PgUp` | Half page down / up                      |
| `g` / `G` / `Home` / `End`            | Jump to top / bottom                     |
| `enter`                               | Add comment at current line              |
| `f`                                   | Comment on the file or reply to its existing thread |
| `v`                                   | Visual select mode (multi-line comments) |
| `s`                                   | Toggle comment sidebar                   |
| `[` / `]`                             | Jump to prev / next comment; skip resolved comments unless unfolded with `h` |
| `h`                                   | Toggle folding resolved comments across all files |
| `H`                                   | Hide/show comment boxes across all files; show line markers in a narrow right gutter |
| `w`                                   | Toggle ignore whitespace across all files in code reviews |
| `r`                                   | Resolve / unresolve the focused comment; resolving jumps to the next unresolved thread, or returns focus to the source if none remain |
| `d`                                   | Delete the selected comment after confirmation |
| `ctrl+PgUp` / `ctrl+PgDn`               | Scroll the selected inline or sidebar thread |
| `?`                                   | Show all keyboard shortcuts              |
| `q`                                   | Finish review (Approve when no unresolved comments remain) |

Ignore whitespace is off by default and lasts for the current TUI run.  It ignores changes in spaces, tabs, carriage returns (CR, `\r`, including LF ↔ CRLF changes), and other ASCII whitespace within a line, including inside strings; it still shows added or deleted blank lines.  The header indicates when it is enabled.  Existing comments on ignored old-side lines retain their context, and files remain available even if all their changes are ignored.

**Comment dialogs:**

| Key      | Action                                                        |
|----------|---------------------------------------------------------------|
| `ctrl+s` | Save the comment or reply; delete an existing entry if cleared; close if unchanged or a new entry is empty |
| `ctrl+o` | Edit the comment or reply in `$EDITOR`                         |
| `ctrl+y` | Insert a suggestion block and select its code for replacement |
| `ctrl+v` | Paste text from the clipboard on the machine running TCrit    |
| `ctrl+k` | Delete the selection, or delete from the cursor to line end; at line end, join the next line |
| `ctrl+u` | Delete the selection, or delete back to line start; at line start, join the previous line |
| `Delete` / `Backspace` | Delete the selected text, or the next / previous character |
| `ctrl+PgUp` / `ctrl+PgDn` | Scroll code context and thread history             |

`ctrl+v` uses the host's clipboard (`pbpaste` on macOS).  When TCrit runs over SSH, this is the remote host's clipboard.  Deletion shortcuts do not save text to a clipboard or kill ring, and the input box has no undo/redo.  Use `ctrl+o` to edit in your external editor when you need its editing commands.  `ctrl+y` inserts a suggestion; it does not yank deleted text.

**Code review only:**

| Key                 | Action                         |
|---------------------|--------------------------------|
| `tab` / `shift+tab` | Next / previous file tab from the content pane or sidebar; keep pane focus |
| `n` / `N`           | Jump to next / previous change or unresolved comment from either pane |
| `/`                 | Search file tabs               |

`f` opens a new reply to the first existing file-comment thread, including resolved threads, or creates a file comment if none exists.  Saving a reply reopens a resolved thread.

`n` / `N` visit change hunks and unresolved comments in display order across files, including file comments and separate threads on the same line.  They stop at the review boundaries and skip resolved comments even when unfolded with `h`.  Jumping to a comment reveals comments hidden with `H`.

## Mouse controls

- Click a file tab, code line, inline comment, sidebar, or sidebar comment to focus it.
- Click **☐ Resolve** in an inline or sidebar thread header to resolve it, or **☑︎ Resolved** to reopen it.  File comments support the same toggle.  Both states reserve the same button width, and collapsed headers keep the reply count without adding the author's name, so the checkbox stays in the same position within the header.  Resolving advances to the next unresolved thread, as with `r`.
- Click the red **x** to the right of the resolution toggle to delete a comment after confirmation.  It appears only on your own comments from the current round that have no replies, including file comments.
- Scroll code with the mouse wheel.  Over an inline or sidebar thread, the wheel focuses it and scrolls its full history; at the thread's limit, scrolling continues through the surrounding pane.
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
