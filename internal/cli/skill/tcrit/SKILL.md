---
name: tcrit
description: Review code changes or a document with TCrit's terminal UI and address the reviewer's inline comments round by round. Use when the user invokes this skill by name or asks for a TCrit review; a generic review request does not count.
allowed-tools: Bash(tcrit *), Read, Edit, Grep, Glob
argument-hint: "[file]"
---

# Review with TCrit

TCrit opens a terminal UI where a human leaves inline comments, then hands those comments back to you.  Run this loop only when the user invokes `/tcrit` or explicitly asks for a TCrit review.

## Prerequisites

The `tcrit` binary must be on PATH.  If it is missing:

```bash
go install github.com/knu/tcrit/cmd/tcrit@latest
```

## Step 1: Choose the command from the arguments

The CLI picks the review mode from its arguments, so do not ask the user which mode they want.

```bash
tcrit $ARGUMENTS            # a file reviews that document; no argument reviews the git changes
tcrit --staged              # review only changes staged in the index
tcrit --scope=all           # HEAD versus working tree, including untracked files
tcrit --scope=unstaged      # index versus working tree, including untracked files
tcrit --unstaged            # alias for --scope=unstaged
tcrit --scope=main...       # merge base with main versus HEAD, using committed contents
tcrit plan <file>           # a plan written in this conversation: each round is saved as a new version
git diff <base> <head> | tcrit --diff  # review a supplied Git unified diff
```

With no argument, review a plan file written earlier in this conversation with `tcrit plan <file>`; otherwise run bare `tcrit` for the git changes.

For a supplied diff, use `tcrit --diff=changes.diff` or `tcrit --diff changes.diff`, or pipe the producer into `tcrit --diff` (`tcrit review --diff` is equivalent).  Relative and absolute paths are accepted; bare `--diff` and `-` as its input read stdin.  This works outside a Git repository.  Do not combine `--diff` with `--scope`, `--code`, `--staged`, or `--unstaged`.  Keep the producer command and working directory for later rounds: TCrit reviews a saved snapshot, not the current working-tree files.

`--scope=staged` is equivalent to `--staged`; `--scope=unstaged` is equivalent to `--unstaged`.  Conflicting scope flags are rejected.  `--scope=A..B` compares two committed snapshots; `--scope=A...B` compares their merge base with B.  B can be omitted to mean HEAD (`--scope=main..` or `--scope=main...`).  `--scope` cannot be combined with a document or `--diff`.  It stays fixed for that session; each new review has independent comment storage, even with identical scope arguments.  Use the session ID from the finish prompt on comment commands (`tcrit comments --session <id>` and `tcrit comment --session <id>`, including replies and bulk input).  Reconnecting with `--session <id>` retains the selected scope.  Committed comparisons read the right endpoint, which may differ from files on disk.  Without explicit flags, the scope is all; a clean working tree does not fall back to committed changes.

## Step 2: Launch the review and block

Run the command from Step 1 and wait for it to finish.  Each new invocation creates an independent saved session.  Record the working directory, session ID printed at startup, and command-runner handle.  `tcrit status` lists saved sessions in the current directory.

TCrit opens a Herdr tab or tmux pane and closes it at the end of each round, including rounds with unresolved comments.  Give the blocking command a long timeout (at least 10 minutes).  If the runner returns an execution handle, poll it until completion.  Wait for the reviewer to finish before editing.

When the user cancels or replaces the task, run `tcrit stop --session <id>` and collect the original command's result before continuing.  This preserves saved comments and round context; cancellation is not approval.  Use `tcrit --session <id>` to resume later from the original directory.  Keep earlier review data when starting a different task; `clear` is only for explicitly requested deletion.

Without a supported multiplexer, ask the user to run the command in their terminal.  Record its session ID, then read that session's comments after they finish.

## Step 3: Read the result

When the command returns, stdout holds the finish prompt and stderr reports `approved: true` or `approved: false`.

- `approved: true`: the review is done.  Leave the loop and continue with the task.
- `approved: false`: the prompt lists the unresolved comments as JSON, the reply command to use, and the command that starts the next round.  Follow it.

Each comment carries `scope`, `path`, `start_line`, `end_line`, `body`, and `anchor`.  Use `anchor`, the text of the commented lines at the time the comment was written, to find the spot even after line numbers have moved.  A comment marked `drifted: true` no longer matches its original text, so treat its line numbers as approximate.  When `quote` is present, the reviewer selected that specific text; focus on it rather than the whole range.

If you need the comments outside this flow, `tcrit comments --json` lists the unresolved ones.

Supplied diffs also receive a new session ID on each new invocation.  Use `--session <id>` on `tcrit comments` and `tcrit comment`, including replies and bulk input, to target that diff review.  Use the session ID from the finish prompt.

## Step 4: Address new feedback

Read each unresolved comment together with its replies, authors, and the work already recorded in this conversation.  Unresolved means the reviewer has not resolved it; it does not by itself request another edit or reply.

- Act on reviewer feedback that has not yet been addressed, including a new reply or an edited request.
- When the latest substantive message is your own comment or reply and the reviewer has not responded, leave the thread unchanged.  This also applies to threads you started and to earlier agents' completion replies.
- Add another reply only for new reviewer feedback or a material correction or additional result not already reported.  A new round, an unresolved flag, or repeating that work is complete is not new information.

For feedback that needs action:

1. Locate the target from `path`, the line range, and `anchor`.
2. Change the file as the `body` asks.  Apply a `suggestion` block verbatim when the comment contains one.
3. Reply once with the change or answer, using the reply form shown in the finish prompt and the session ID.  Plan reviews use the same `--session <id>` form:

```bash
tcrit comment --session <session-id> --reply-to <id> --author 'Claude Code' '<what you did>'
```

Never pass `--resolve`.  Resolving is the reviewer's decision.

For three or more needed replies, write them to a JSON file and submit once:

```bash
tcrit comment --session <session-id> --json --file .tmp/replies.json --author 'Claude Code'
```

The `/tcrit-cli` skill documents the JSON format and the other comment commands.  If no thread needs action, proceed to the next round without edits or additional comments.  Only `approved: true` completes the review.

## Step 5: Start the next round

Run the command printed in the finish prompt from the original working directory.  `tcrit --session <id>` opens a new TUI from saved comments and round context; it retains the scope and refreshes code or document contents.  After an interrupted round it resumes that round; after a submitted round it advances to the next one.

For a revised plan, use `tcrit plan --session <id> <file>` (or pipe the plan to it) to save a new version.  Plain `tcrit --session <id>` opens the saved plan without replacing its content.

For a revised supplied diff, regenerate the input with the original producer:

```bash
git diff <base> <head> | tcrit --diff --session <id>
```

For file input, update it first, then run `tcrit --diff=changes.diff --session <id>`.  Plain `tcrit --session <id>` opens the saved diff without replacing it.  Keep the explicit session ID; omitting it creates an independent review.  Inspect drifted comments against their anchors when the supplied context is incomplete.

Wait for the command to finish, then return to Step 3.  Stop when the result is `approved: true`, or follow the stopping procedure when the user cancels or replaces the task.
