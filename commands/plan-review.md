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

## Step 1: Launch the TUI

When starting a new review task, reset state left over from an earlier task once, from the project root.  This removes saved review comments and the code review session while preserving `.crit/.gitignore`:

```bash
tcrit clear --all
```

Do not run it again when reopening the review after addressing comments.  Keep the existing review state throughout that review-fix-re-review cycle.

Check if `$TMUX` is set:

If in tmux, run this command with a **timeout of 600000** (10 minutes) since it blocks until the user finishes reviewing:
```bash
tcrit review $ARGUMENTS --detach --wait
```

If the command runner yields an execution session ID, the command is still running even if its initial output says that the review opened.  Keep polling that execution session until the process exits.  A quick exit is a valid completed review; do not impose a minimum wait.  Continue to Step 2 only after the process exits, and never use a fixed sleep in place of session polling.

If not in tmux (command fails with "requires a tmux session"), ask the user to run the TUI manually:

> Please run this in your terminal, review the document, and let me know when you're done:
>
> ```
> tcrit review $ARGUMENTS
> ```

Wait for the user to confirm before proceeding.

## Step 2: Read the comments

After the user confirms the review is complete, read the review comments:

```bash
tcrit status $ARGUMENTS
```

This outputs JSON with the file path and comments array.

## Step 3: Address comments

For each comment in the `comments` array:

1. Read the `start_line`/`end_line` numbers and `anchor` to locate where in the document the comment applies
2. Read the `body` for what the reviewer wants changed
3. Edit the document at `$ARGUMENTS` to address the comment
4. Reply to the comment recording what you did (do NOT pass --resolve — resolving is the reviewer's call):

```bash
tcrit comment --reply-to <comment-id> --author 'Claude Code' "<what you did>"
```

After addressing ALL comments, summarize what you changed.

## Step 4: Prompt for next action

After addressing all comments and summarizing the changes, use the `AskUserQuestion` tool to ask:

- **Question:** "I've addressed all your comments. What would you like to do next?"
- **Header:** "Next action"
- **Options:**
  - **Re-review** — Open TCrit again to review the document
  - **Continue** — Done, move on

If the user provides free-form input (via the "Other" option), respond accordingly, then ask again with `AskUserQuestion` until they pick Re-review or Continue.

If the user chooses **Re-review**, use `AskUserQuestion` again to ask:

- **Question:** "Keep existing comments or clear them before re-reviewing?"
- **Header:** "Comments"
- **Options:**
  - **Keep** — Keep existing comments visible during re-review
  - **Clear** — Remove all comments before re-reviewing

If clear, run:
```bash
tcrit clear $ARGUMENTS
```
Then go back to Step 1. If keep, go back to Step 1 directly.

If the user chooses **Continue**, done.

## Important notes

- Do NOT modify the document while the TUI is open — only edit after it exits
- The `anchor` field holds the full text of the commented lines when the comment was created — use it to find the right location even if line numbers have shifted
