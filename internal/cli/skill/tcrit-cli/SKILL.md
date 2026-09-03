---
name: tcrit-cli
description: Reference for TCrit's headless commands. Use when an agent needs to list, add, or reply to review comments with tcrit comment and tcrit comments, target a specific session or plan, read a TCrit review.json file, or clear review state. Not for running the interactive review loop; that is the tcrit skill.
user-invocable: false
---

# TCrit CLI Reference

To run an interactive review round, use the `/tcrit` skill.  This reference covers the commands that read and write review comments without opening the TUI.

## Comment scopes

- **Line** (`scope: "line"`): attached to a line range of one file.  Stored under `files.<path>.comments`.
- **File** (`scope: "file"`): about one file as a whole.  Stored under `files.<path>.comments` with `start_line: 0`.
- **Review** (`scope: "review"`): general feedback.  Stored in the top-level `review_comments` array.

## Listing comments

```bash
tcrit comments                  # unresolved comments, human-readable
tcrit comments --json           # the same as a JSON array
tcrit comments --all            # include resolved comments
tcrit comments --plan <slug>    # a plan review
tcrit comments --session <id>   # a specific review session
tcrit comments <review-path>    # an explicit review.json or its directory
```

Review-level comments come first, then files in path order with file-level comments before line-level ones.  Each JSON entry adds `scope` and `path` to the stored comment fields.

The finish prompt printed by a review round already contains the unresolved comments, so use `tcrit comments` when re-entering a round or working headlessly.

## Targeting a session or plan

`tcrit comment` and `tcrit comments` operate on the review that matches the current directory and branch.  When several reviews are active, pass `--session <id>`; the ID appears in the `tcrit --session <id>` command at the end of the finish prompt.  Plan reviews live in their own storage and always need `--plan <slug>`; the slug is printed when the plan is saved and in the finish prompt.

`tcrit status --code` prints the files and comments of the current code review session as JSON, and `tcrit status <file>` does the same for a document review.

## Review file format

TCrit stores each review as a `review.json` that follows crit's CritJSON layout, so tooling written for either works on both.

```json
{
  "branch": "feat/skills",
  "base_ref": "HEAD",
  "review_round": 2,
  "review_comments": [
    {
      "id": "r_0c4e1a",
      "scope": "review",
      "body": "Split the installer change into its own commit.",
      "author": "Akinori Musha",
      "review_round": 1
    }
  ],
  "files": {
    "internal/cli/install.go": {
      "status": "modified",
      "comments": [
        {
          "id": "c_152b0a",
          "start_line": 56,
          "end_line": 58,
          "body": "Drop the old names here too.",
          "anchor": "\tfor _, name := range []string{\"tcrit-review\"} {",
          "author": "Akinori Musha",
          "review_round": 1,
          "replies": [
            { "id": "rp_9b21d4", "body": "Removed the old names.", "author": "Claude Code", "review_round": 1 }
          ]
        }
      ]
    }
  }
}
```

Field rules:

- `resolved` is either `true` or absent.  Absent means unresolved.
- `anchor` is the text of the commented lines when the comment was written.  Locate content by it rather than trusting `start_line` and `end_line` after edits.
- `drifted: true` means the anchored text could not be found again; the line numbers are only approximate.
- `quote`, when present, is the exact text the reviewer selected inside the range.
- `replies` may contain earlier agent replies.  Read them before acting on a comment.
- `review_round` records the round in which a comment or reply was written.

## Adding and replying to comments

```bash
tcrit comment --author 'Claude Code' '<body>'                       # review-level
tcrit comment --author 'Claude Code' <path> '<body>'                # file-level
tcrit comment --author 'Claude Code' <path>:<line> '<body>'         # one line
tcrit comment --author 'Claude Code' <path>:<start>-<end> '<body>'  # line range
tcrit comment --reply-to <id> --author 'Claude Code' '<body>'       # reply
```

Rules:

- Always pass `--author` with your agent name so the thread shows who wrote what.
- Single-quote the body.  Double quotes let the shell interpret backticks and `$`.
- Line numbers refer to the file on disk, 1-indexed, not to diff line numbers.
- Bodies are Markdown; code fences and inline code render in the TUI.
- Do not pass `--resolve` unless the user explicitly asks for it.  The same applies to the `resolve` field in bulk JSON.

## Bulk comments

For three or more comments or replies, submit one JSON array so the review file is written once:

```bash
tcrit comment --json --file .tmp/comments.json --author 'Claude Code'
```

Write the file with your file tool rather than quoting JSON on the command line; bodies with real newlines or quotes then need no shell escaping.  `--file -` reads stdin.

```json
[
  {"body": "Overall this is ready once the tests pass.", "scope": "review"},
  {"path": "internal/cli/install.go", "body": "Consider a table-driven layout."},
  {"file": "internal/cli/install.go", "line": 42, "body": "This branch is unreachable."},
  {"file": "internal/cli/install.go", "line": "56-58", "body": "Rename the loop variable."},
  {"reply_to": "c_152b0a", "body": "Removed the old names."}
]
```

Entry fields:

| Field | Type | Notes |
| --- | --- | --- |
| `file` or `path` | string | Repository-relative path.  Alone, it makes a file-level comment. |
| `line` | int or string | `42` or `"56-58"`.  Requires `file` or `path`. |
| `end_line` | int | Optional; defaults to `line`. |
| `body` | string | Required. |
| `author` | string | Optional per-entry override for `--author`. |
| `scope` | string | `"review"` or `"file"`; inferred when omitted. |
| `reply_to` | string | Comment ID to reply to (`c_…` or `r_…`). |
| `resolve` | bool | Only when the user asked to resolve. |

Scope is inferred as follows: `reply_to` makes a reply; no path and no line makes a review-level comment; a path without a line makes a file-level comment; a path with a line makes a line comment.

## Duplicate comment IDs

Line and file comment IDs are unique within one file, so the same ID can exist in two files.  If `tcrit comment` reports that a reply target was found in multiple files, add `--path <file>`; in bulk JSON, set `file` on that entry.  Review-level IDs (`r_…`) never collide.

## Plan reviews

`tcrit plan <file>` stores the plan outside the repository and versions it per round.  Every headless command against it needs `--plan <slug>`:

```bash
tcrit comments --plan <slug> --json
tcrit comment --plan <slug> --reply-to <id> --author 'Claude Code' '<body>'
```

## Clearing review state

```bash
tcrit comment --clear           # delete the matching review file
tcrit clear --code              # clear the code review session
tcrit clear <file>              # clear a document review
tcrit clear --all               # delete every review session for the current directory
```

Use `tcrit clear --all` once when a new review task begins, never in the middle of a round.
