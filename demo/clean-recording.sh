#!/bin/sh
# Drop blank frames from a VHS recording, GIF or MP4.
#
# While the comment editor is open and Claude Code redraws its pane at the
# same time, VHS occasionally captures a frame whose text layer is empty,
# which shows up as a one-frame flash.  Blank frames are at least 99% dark
# pixels; real frames of this demo stay below that.  The previous frame
# absorbs each removed frame's duration.  A GIF gets its palette rebuilt
# from the recording itself without dithering so colors stay unchanged; an
# MP4 is re-encoded with H.264 at a quality that keeps text crisp.
set -eu

file=${1:-$(dirname "$0")/code-review.gif}
select="blackframe=amount=0:threshold=48,metadata=select:key=lavfi.blackframe.pblack:value=98:function=less"
case $file in
*.gif)
  tmp=$file.clean.tmp.gif
  ffmpeg -v error -y -i "$file" \
    -vf "$select,split[a][b];[a]palettegen=max_colors=256:stats_mode=full[p];[b][p]paletteuse=dither=none" \
    -fps_mode vfr "$tmp"
  ;;
*.mp4)
  tmp=$file.clean.tmp.mp4
  ffmpeg -v error -y -i "$file" -vf "$select" -fps_mode vfr \
    -c:v libx264 -crf 18 -pix_fmt yuv420p -movflags +faststart "$tmp"
  ;;
*)
  echo "clean-recording.sh: unsupported file: $file" >&2
  exit 2
  ;;
esac
mv "$tmp" "$file"
