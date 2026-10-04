# Demo recordings

The README animation is recorded with [VHS](https://github.com/charmbracelet/vhs) from `code-review.tape`; `demo.tape` records a document review without an agent.  It needs `vhs`, `tmux`, `claude`, and GNU make on `PATH`.

Run these from this directory, or from the repository root with `make -C demo`:

```bash
make            # record GIFs whose tape or scripts changed
make rebuild    # record them again from scratch
make mp4        # record H.264 MP4s instead, for example to post on X
make clean      # delete the GIFs and MP4s
```

Each recording runs `setup.sh`, then `vhs`, then `clean-recording.sh`.  The setup script builds the current checkout and creates the fixtures from `assets` under `$DEMO_DIR` (default `$TMPDIR/tcrit-demo`): a throwaway Git repository with staged changes, a Markdown document, the Claude Code skills with an English-only `CLAUDE.md` and the `dark-ansi` theme (MP4 recordings use Claude Code's full-color default theme, since video has no GIF palette to protect), and an isolated `XDG_STATE_HOME` for review sessions.  The fixtures live outside the checkout because Claude Code reads `CLAUDE.md` from every parent directory.  The tape starts a private tmux server inside the recording so TCrit opens its pane there rather than in the multiplexer running VHS.  Both tapes source `theme.tape`, which selects the `Pro` palette shipped with macOS Terminal so the GIFs show TCrit on a stock theme.  Claude Code runs in auto mode inside the fixture directory; set `DEMO_PERMISSION_MODE=bypassPermissions` before `vhs` if a permission prompt stalls the recording.  The cleanup script removes single blank frames that appear while typing a comment and rebuilds the GIF palette without dithering, which also shrinks the file.

GIFs and MP4s are local build outputs ignored by Git.  Published GIFs are hosted as release assets.  After the first recording, GIFs only rebuild when a tape, script, or fixture is newer.

## Uploading recordings

Use an existing, published release to host recordings without adding them to Git:

```sh
./demo/release.sh -t v0.15.0 list
./demo/release.sh -t v0.15.0 upload demo/review-flow-codex.gif v0.15.0
```

The script requires `gh`, `jq`, and `sha256sum` or `shasum`.  Authenticate `gh` with write access to the repository first.  The repository defaults to the current repository reported by `gh repo view`.  The default release tag is `demo-assets`.  Use `-r OWNER/REPO` and `-t TAG` to select another repository or release.  The release must already exist; the script does not create tags or releases.

`list` prints tab-separated asset names, SHA-256 digests, and download URLs.  `upload FILE VERSION` uploads a GIF as `{name}-{version}.gif`, for example `review-flow-codex-v0.15.0.gif`, and prints its HTTPS URL.  The version must have the form `v0.xx.x`.  An existing asset with that name is reused only when its SHA-256 and size match; different content under the same name is rejected.  Existing assets are never overwritten.  Use the returned URL in a README image link.

Published recordings:

- [Code review](https://github.com/knu/tcrit/releases/download/v0.15.0/code-review-v0.15.0.gif)
- [Document review](https://github.com/knu/tcrit/releases/download/v0.15.0/demo-v0.15.0.gif)
- [Review and commit with Claude Code](https://github.com/knu/tcrit/releases/download/v0.15.0/review-flow-v0.15.0.gif)
- [Review and commit with Codex](https://github.com/knu/tcrit/releases/download/v0.15.0/review-flow-codex-v0.15.0.gif)

## Sending Alt keys and mouse events from a tape

VHS accepts `Alt+p`, but its headless Chrome runs xterm.js on macOS, where Alt with a letter is not turned into an escape sequence, so the TUI receives a bare `p` (charmbracelet/vhs issue #442; `macOptionIsMeta` cannot be set from a tape).  Send the escape byte yourself instead:

```
Escape
Type "p"
```

`Type` waits after each character, not before, so the letter follows the escape within a few milliseconds and tmux delivers both in one read even with the `escape-time 10` in `tmux.conf`.  TCrit then sees `alt+p`.

Mouse events work the same way, as SGR sequences: `Escape` then `Type@1ms "[<0;COL;ROWM"` presses the left button at 1-based column and row, the same with a trailing `m` releases it, `[<64;…M` and `[<65;…M` scroll the wheel up and down, and `[<32;…M` drags.  tmux forwards them to the pane that asked for mouse reporting and converts the coordinates, so in a split add the pane's left offset or zoom the pane first.  Hit areas are exact and column widths follow the font and `Set Width`, so check a coordinate with a `Screenshot` before relying on it.
