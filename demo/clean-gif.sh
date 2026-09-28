#!/bin/sh
# Drop blank frames from a VHS recording.
#
# While the comment editor is open and Claude Code redraws its pane at the
# same time, VHS occasionally captures a frame whose text layer is empty,
# which shows up as a one-frame flash.  Blank frames are at least 99% dark
# pixels; real frames of this demo stay below that.  The previous frame
# absorbs each removed frame's duration, and the palette is rebuilt from
# the recording itself without dithering so colors stay unchanged.
set -eu

gif=${1:-$(dirname "$0")/code-review.gif}
tmp=$gif.clean.tmp.gif
ffmpeg -v error -y -i "$gif" \
  -vf "blackframe=amount=0:threshold=48,metadata=select:key=lavfi.blackframe.pblack:value=98:function=less,split[a][b];[a]palettegen=max_colors=256:stats_mode=full[p];[b][p]paletteuse=dither=none" \
  -fps_mode vfr "$tmp"
mv "$tmp" "$gif"
