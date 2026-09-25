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

## Step 2: Launch and wait

Run the Step 1 command; retain its working directory, session ID, and execution handle.  TCrit opens a Herdr tab or tmux pane and closes it after each round.  Without a supported multiplexer, ask the user to run it in their terminal and return the session ID for reading results.

Before waiting, announce in the conversation's language that the review is open and you will wait for submission.  Ask the user to send `hey` after submitting if the wait times out, so you can collect the result.

Use one bounded wait for the original command to finish, within the host's timeout limits.  If launch yields an execution handle immediately, use that handle for the wait.  The wait must leave TCrit running and preserve its final output when it times out; a timeout that kills the process is unsuitable.  If the host cannot preserve the process and output, ask the user to run TCrit in their terminal and return its finish output instead.

If the command finishes, continue to Step 3.  If the wait times out or yields while the command is still running, retain the working directory, session ID, execution handle, and any captured output, then end the turn.  Resume on a completion notification or the user's message.  A notification must actually resume the agent; otherwise rely on the user's message.  Collect the original command's result before starting another review.  Approval may remove saved review data, so the command's output must remain recoverable.

While awaiting submission, leave files unchanged and perform no other work.  Do not repeat waits, poll status, or send unchanged waiting notices to keep the turn alive.  Follow higher-priority host notification requirements during the bounded wait, then yield to the user.  A wait timeout is not review cancellation or approval.

Keep reviewed content unchanged until submission.  On task cancellation or replacement, run `tcrit stop --session <id>` and collect the result.  Preserve saved reviews; use `clear` only for explicitly requested deletion.

## Step 3: Read the result

Handle the command outcome before continuing:

- **Launch failure before the TUI was usable:** diagnose and correct the cause; sandbox failures may use normal escalation without a new review request.  Confirm the command exited and inspect `tcrit status` (a printed ID does not prove the TUI opened).  Retry with `--session <id>` if saved, otherwise the original command.  If another review is running, launch state is uncertain, escalation is denied, or the error persists, preserve state and report the blocker.
- **Interrupted after opening, or cancelled:** read new saved comments and replies, preserve state, and report the interruption.  End the turn; wait for explicit chat instructions before acting on feedback or resuming from the original directory.
- **Exit 0:** read the finish prompt and all returned threads, including resolved comments and replies.  Approval may have deleted saved data.  For `approved: false`, follow the feedback and next-round commands.  For `approved: true`, address any new instructions and continue the task; changes to approved content require another review before committing.

If the result cannot be recovered, report that limitation.  Silence, elapsed time, and missing saved data are not approval.

Locate feedback using `path`, line range, and `anchor` (the original text).  Treat `drifted: true` line numbers as approximate; focus on `quote` when present.  Outside the finish prompt, `tcrit comments --session <id> --json` lists unresolved comments.  Use the session ID for all comment commands, including supplied-diff reviews, replies, and bulk input.

For image attachments, follow the finish prompt's base directory and open referenced images with your image-viewing tool, including those in resolved threads.  On approval, after reading them and recording any remaining instructions, run the cleanup command printed by TCrit.  This is the approval cleanup step; it deletes the completed session and its images.  If image reading fails, preserve the session and report the failure.  Unapproved rounds and stopped reviews retain attachments.

## Step 4: Address new feedback

Read each comment together with its replies, authors, and the work already recorded in this conversation.  Unresolved means the reviewer has not resolved it; it does not by itself request another edit or reply.

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

After an unapproved round submitted with exit status 0, run the command printed in the finish prompt from the original working directory.  `tcrit --session <id>` opens a new TUI from saved comments and round context; it retains the scope and refreshes code or document contents.  It advances past a submitted round, or resumes an interrupted round when explicitly requested by the user in chat.

For a revised plan, use `tcrit plan --session <id> <file>` (or pipe the plan to it) to save a new version.  Plain `tcrit --session <id>` opens the saved plan without replacing its content.

For a revised supplied diff, regenerate the input with the original producer:

```bash
git diff <base> <head> | tcrit --diff --session <id>
```

For file input, update it first, then run `tcrit --diff=changes.diff --session <id>`.  Plain `tcrit --session <id>` opens the saved diff without replacing it.  Keep the explicit session ID; omitting it creates an independent review.  Inspect drifted comments against their anchors when the supplied context is incomplete.

Wait for the command to finish, then return to Step 3.  Stop when the result is `approved: true`, or follow the stopping procedure when the user cancels or replaces the task.
