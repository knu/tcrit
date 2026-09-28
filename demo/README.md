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

The GIFs are committed and only rebuild when a tape, script, or fixture is newer.  The MP4s are recorded on demand and ignored by Git.
