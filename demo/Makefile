# Record the README demo with VHS.  Run from this directory or with make -C demo.
#
#   make            build every GIF whose tape or inputs changed
#   make rebuild    record every GIF again
#   make clean      delete the GIFs
#
# code-review.tape launches Claude Code and takes a few minutes; demo.tape
# records a document review without an agent.  The GIFs are committed, so a
# checkout only rebuilds when a tape or fixture is newer than its GIF.

# Fixtures are built here, outside the checkout (see setup.sh).
DEMO_DIR ?= $(patsubst %/,%,$(or $(TMPDIR),/tmp))/tcrit-demo
export DEMO_DIR

TAPES = code-review.tape demo.tape
GIFS = $(TAPES:.tape=.gif)
ASSETS = $(shell find assets -type f)

.PHONY: build rebuild clean
.DELETE_ON_ERROR:
# Recordings share $(DEMO_DIR), the tmux socket "demo", and the review state,
# so they must not run concurrently.
.NOTPARALLEL:

build: $(GIFS)

# Re-record without deleting first, so a failed recording keeps the old GIF.
rebuild:
	$(MAKE) -B build

clean:
	rm -f $(GIFS)

# Every recording rebuilds tcrit and the fixture, so the GIFs depend on the
# demo scripts as well as the tape; Go sources would rebuild on every edit.
%.gif: %.tape setup.sh tmux.conf clean-gif.sh $(ASSETS)
	./setup.sh
	vhs $<
	./clean-gif.sh $@
