---
name: tcrit:plan-review
description: Open a document or plan in TCrit's interactive TUI for review. After the review, address any comments by editing the document.
argument-hint: <file-path>
---

# Review Document

Review the document at `$ARGUMENTS` using TCrit's interactive TUI.

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

Run this command with a **timeout of 600000** (10 minutes) since it blocks until the reviewer finishes.  Tcrit detects the invoking tmux pane even when `$TMUX` and `$TMUX_PANE` were not inherited:

```bash
tcrit plan $ARGUMENTS
```

`tcrit plan` saves the document as a versioned plan (slug derived from its first heading; pass `--name <slug>` to pin it) and opens the review of the latest version.

The TUI opens in a tmux split pane; the command blocks until the reviewer approves or finishes with comments. If the command runner yields an execution session ID, keep polling that execution session until the process exits. A quick exit is a valid completed review; do not impose a minimum wait, and never use a fixed sleep in place of session polling.

If not in tmux (the command fails with "no tmux session"), ask the user to run the TUI manually:

> Please run this in your terminal, review the document, and let me know when you're done:
>
> ```
> tcrit plan $ARGUMENTS
> ```

Wait for the user to confirm, then read the comments with `tcrit comments --json` instead of relying on the command output below.

## Step 2: Read the result

When the command completes, read **stdout** and follow its instructions. Check **stderr** for `approved: true` or `approved: false`.

- `approved: true` — the review is done; stop the loop and proceed.
- `approved: false` — stdout contains the unresolved comments as JSON and the command to run for the next round.

Fallback (mid-round re-entry or headless): `tcrit comments --json` lists the unresolved comments.

## Step 3: Address each comment

For each unresolved comment:

1. Read the `start_line`/`end_line` numbers and `anchor` to locate where in the document the comment applies
2. Read the `body` for what the reviewer wants changed
3. Edit the document at `$ARGUMENTS` to address the comment
4. Reply recording what you did, using the exact command form shown in the finish prompt (for plans it includes `--plan <slug>`; do NOT pass `--resolve` — resolving is the reviewer's call):

```bash
tcrit comment --plan <slug> --reply-to <comment-id> --author 'Claude Code' "<what you did>"
```

## Step 4: Start the next round

Run the command printed at the end of the finish prompt (for plans it looks like `tcrit plan --name <slug> $ARGUMENTS`, which saves the revised document as a new version), again with a long timeout. It signals the waiting TUI to reload your edits and replies, then blocks until the reviewer finishes the next round. Return to Step 2.

When a round ends with `approved: true`, the loop is over.

## Important notes

- Do NOT modify the document while the reviewer is actively reviewing — edit only after the review command returns
- The `anchor` field holds the full text of the commented lines when the comment was created — use it to find the right location even if line numbers have shifted
- `resolved: false` or a missing `resolved` field both mean unresolved
