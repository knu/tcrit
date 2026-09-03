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
tcrit plan <file>           # a plan written in this conversation: each round is saved as a new version
tcrit review --base <ref>   # git changes against another base
```

With no argument, review a plan file written earlier in this conversation with `tcrit plan <file>`; otherwise run bare `tcrit` for the git changes.

## Step 2: Launch the review and block

When a new review task starts (not a later round of the same task), discard state left by earlier tasks once, from the project root:

```bash
tcrit clear --all
```

Then run the command from Step 1.  It blocks until the reviewer finishes, so give it a long timeout (at least 10 minutes).  TCrit finds the invoking Herdr or tmux context on its own, even when their environment variables were not inherited, and opens the TUI in a dedicated Herdr tab or a tmux split pane.

If the command runner hands back an execution session instead of waiting, keep polling that execution session until the process exits.  A quick exit is a completed review; do not enforce a minimum wait or replace polling with a fixed sleep.

Without a supported multiplexer, ask the user to run the same command in their own terminal and tell you when they are done, then read the comments with `tcrit comments --json` instead of the output described below.

Do not edit files while the TUI is open.  Wait for the command to return.

## Step 3: Read the result

When the command returns, stdout holds the finish prompt and stderr reports `approved: true` or `approved: false`.

- `approved: true`: the review is done.  Leave the loop and continue with the task.
- `approved: false`: the prompt lists the unresolved comments as JSON, the reply command to use, and the command that starts the next round.  Follow it.

Each comment carries `scope`, `path`, `start_line`, `end_line`, `body`, and `anchor`.  Use `anchor`, the text of the commented lines at the time the comment was written, to find the spot even after line numbers have moved.  A comment marked `drifted: true` no longer matches its original text, so treat its line numbers as approximate.  When `quote` is present, the reviewer selected that specific text; focus on it rather than the whole range.

If you need the comments outside this flow, `tcrit comments --json` lists the unresolved ones.

## Step 4: Address each comment

For every unresolved comment:

1. Locate the target from `path`, the line range, and `anchor`.
2. Change the file as the `body` asks.  Apply a `suggestion` block verbatim when the comment contains one.
3. Reply with what you did, using the exact reply form shown in the finish prompt.  Plan reviews add `--plan <slug>`:

```bash
tcrit comment --reply-to <id> --author 'Claude Code' '<what you did>'
```

Never pass `--resolve`.  Resolving is the reviewer's decision.

For three or more replies, write them to a JSON file and submit once:

```bash
tcrit comment --json --file .tmp/replies.json --author 'Claude Code'
```

The `/tcrit-cli` skill documents the JSON format and the other comment commands.

## Step 5: Start the next round

Run the command printed at the end of the finish prompt, again with a long timeout.  For git changes and documents it is `tcrit --session <id>`, which reconnects to the waiting TUI and reloads your edits and replies.  For plans it is `tcrit plan --name <slug> <file>`, which saves the revised file as a new version.

The command blocks until the reviewer finishes the next round.  Return to Step 3.  Stop when a round ends with `approved: true`.
