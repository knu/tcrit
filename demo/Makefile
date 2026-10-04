# Record the README demo with VHS.  Run from this directory or with make -C demo.
#
#   make            build every GIF whose tape or inputs changed
#   make rebuild    record every GIF again
#   make mp4        record MP4s for sites that want video, such as X
#   make clean      delete the GIFs and MP4s
#
# code-review.tape launches Claude Code and takes a few minutes; demo.tape
# records a document review without an agent.  GIFs are local build outputs;
# published recordings are hosted as release assets.

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
%.gif: %.tape theme.tape setup.sh tmux.conf clean-recording.sh $(ASSETS)
	./setup.sh
	vhs $<
	./clean-recording.sh $@

# X accepts MP4 with H.264 video (and AAC audio, which a silent clip omits),
# up to 512 MB, 2:20, 1920x1200, and 40 fps; the tapes fit those limits.
# Video has no GIF palette to protect, so Claude Code keeps its full-color
# default theme here.  VHS records the tape again from a copy whose Output
# line names the MP4, since `vhs -o` adds an output instead of replacing
# the GIF.
%.mp4: %.tape theme.tape setup.sh tmux.conf clean-recording.sh $(ASSETS)
	DEMO_CLAUDE_THEME=dark ./setup.sh
	sed 's|^Output .*|Output $@|' $< > .$@.tape
	vhs .$@.tape
	rm -f .$@.tape
	./clean-recording.sh $@
