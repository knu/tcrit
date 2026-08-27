---
name: tcrit:code-review
description: Review code changes in TCrit's multi-file TUI with syntax highlighting and diff markers. After the review, address any comments.
---

# Code Review

Review code changes using TCrit's multi-file code review TUI.

## Prerequisites

The `tcrit` binary must be installed and on PATH. If not installed:

```bash
go install github.com/knu/tcrit/cmd/tcrit@latest
```

## Step 1: Launch the review and block

When starting a new review task (not a later round of the same task), reset state left over from an earlier task once, from the project root:

```bash
tcrit clear --all
```

Check if `$TMUX` is set:

If in tmux, run this command with a **timeout of 600000** (10 minutes) since it blocks until the reviewer finishes (pass `--base <ref>` to change the diff base):

```bash
tcrit review --code
```

The TUI opens in a tmux split pane; the command blocks until the reviewer approves or finishes with comments. If the command runner yields an execution session ID, keep polling that execution session until the process exits. A quick exit is a valid completed review; do not impose a minimum wait, and never use a fixed sleep in place of session polling.

If not in tmux (the command fails with "no tmux session"), ask the user to run the TUI manually:

> Please run this in your terminal, review the changes, and let me know when you're done:
>
> ```
> tcrit review --code
> ```

Wait for the user to confirm, then read the comments with `tcrit comments --json` instead of relying on the command output below.

## Step 2: Read the result

When the command completes, read **stdout** and follow its instructions. Check **stderr** for `approved: true` or `approved: false`.

- `approved: true` — the review is done; stop the loop and proceed.
- `approved: false` — stdout contains the unresolved comments as JSON and the command to run for the next round.

Fallback (mid-round re-entry or headless): `tcrit comments --json` lists the unresolved comments.

## Step 3: Address each comment

For each unresolved comment:

1. Read the `path` and `start_line`/`end_line` numbers and `anchor` to locate where the comment applies
2. Read the `body` for what the reviewer wants changed
3. Edit the file to address the comment
4. Reply recording what you did (do NOT pass `--resolve` — resolving is the reviewer's call):

```bash
tcrit comment --reply-to <comment-id> --author 'Claude Code' "<what you did>"
```

Use `tcrit comment --json --file <path>` for a single bulk call when replying to 3+ comments.

## Step 4: Start the next round

Run the command printed at the end of the finish prompt (it looks like `tcrit --session <id>`), again with a long timeout. It signals the waiting TUI to reload your edits and replies, then blocks until the reviewer finishes the next round. Return to Step 2.

When a round ends with `approved: true`, the loop is over.

## Important notes

- Do NOT modify files while the reviewer is actively reviewing — edit only after the review command returns
- The `anchor` field holds the full text of the commented lines when the comment was created — use it to find the right location even if line numbers have shifted
- `resolved: false` or a missing `resolved` field both mean unresolved
