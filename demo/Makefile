# Record the README demo with VHS.  Run from this directory or with make -C demo.
#
#   make            build every GIF whose tape or inputs changed
#   make rebuild    record every GIF again
#   make mp4        convert the GIFs to MP4 for sites that want video, such as X
#   make clean      delete the GIFs and MP4s
#
# code-review.tape launches Claude Code and takes a few minutes; demo.tape
# records a document review without an agent.  The GIFs are committed, so a
# checkout only rebuilds when a tape or fixture is newer than its GIF.

# Fixtures are built here, outside the checkout (see setup.sh).
DEMO_DIR ?= $(patsubst %/,%,$(or $(TMPDIR),/tmp))/tcrit-demo
export DEMO_DIR

TAPES = code-review.tape demo.tape
GIFS = $(TAPES:.tape=.gif)
MP4S = $(TAPES:.tape=.mp4)
ASSETS = $(shell find assets -type f)

.PHONY: build rebuild mp4 clean
.DELETE_ON_ERROR:
# Recordings share $(DEMO_DIR), the tmux socket "demo", and the review state,
# so they must not run concurrently.
.NOTPARALLEL:

build: $(GIFS)

# Re-record without deleting first, so a failed recording keeps the old GIF.
rebuild:
	$(MAKE) -B build

mp4: $(MP4S)

clean:
	rm -f $(GIFS) $(MP4S)

# Every recording rebuilds tcrit and the fixture, so the GIFs depend on the
# demo scripts as well as the tape; Go sources would rebuild on every edit.
%.gif: %.tape theme.tape setup.sh tmux.conf clean-gif.sh $(ASSETS)
	./setup.sh
	vhs $<
	./clean-gif.sh $@

# X accepts MP4 with H.264 video (and AAC audio, which a silent clip omits),
# up to 512 MB, 2:20, 1920x1200, and 40 fps.  The GIFs already fit those
# limits, so this keeps their frames and timing; yuv420p and even dimensions
# are what H.264 players expect.
%.mp4: %.gif
	ffmpeg -y -loglevel error -i $< -movflags +faststart -c:v libx264 -crf 18 -pix_fmt yuv420p -vf "scale=trunc(iw/2)*2:trunc(ih/2)*2" $@
