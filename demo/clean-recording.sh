#!/bin/sh
# Put MP4 playback metadata first without re-encoding or dropping frames.
set -eu

file=${1:-$(dirname "$0")/code-review-claude.mp4}
case $file in
    *.mp4) ;;
    *)
        echo "clean-recording.sh: expected an MP4 recording" >&2
        exit 2
        ;;
esac
tmp=$file.clean.tmp.mp4
ffmpeg -v error -y -i "$file" -c copy -movflags +faststart "$tmp"
mv "$tmp" "$file"
