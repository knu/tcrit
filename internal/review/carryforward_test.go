package review

import (
	"testing"
)

func carryOne(t *testing.T, c Comment, prev, next string) Comment {
	t.Helper()
	out := CarryForwardFile([]Comment{c}, prev, next, Now())
	if len(out) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(out))
	}
	return out[0]
}

func TestCarryForwardRemintsComment(t *testing.T) {
	orig := Comment{
		ID: "c_old111", StartLine: 2, EndLine: 2, Anchor: "line two",
		Body: "fix", Author: "Human", ReviewRound: 1, Resolved: true, ResolvedRound: 1,
		Replies: []Reply{{ID: "rp_1", Body: "done", Author: "AI"}},
	}
	content := "line one\nline two\nline three\n"

	got := carryOne(t, orig, content, content)

	if got.ID == orig.ID || got.ID == "" {
		t.Errorf("expected a fresh ID, got %q", got.ID)
	}
	if !got.CarriedForward {
		t.Error("expected CarriedForward")
	}
	if got.StartLine != 2 || got.EndLine != 2 || got.Drifted {
		t.Errorf("unchanged content should keep position: %+v", got)
	}
	if got.ReviewRound != 1 || !got.Resolved || got.ResolvedRound != 1 {
		t.Errorf("round/resolution not preserved: %+v", got)
	}
	if len(got.Replies) != 1 || got.Replies[0].ID != "rp_1" {
		t.Errorf("replies not preserved: %+v", got.Replies)
	}
}

func TestCarryForwardShiftsWithInsertedLines(t *testing.T) {
	prev := "alpha\nbeta\ngamma\n"
	next := "intro\nmore\nalpha\nbeta\ngamma\n"
	c := Comment{ID: "c_1", StartLine: 2, EndLine: 3, Anchor: "beta\ngamma"}

	got := carryOne(t, c, prev, next)

	if got.StartLine != 4 || got.EndLine != 5 || got.Drifted {
		t.Errorf("expected shift to 4-5, got %d-%d drifted=%v", got.StartLine, got.EndLine, got.Drifted)
	}
}

func TestCarryForwardFindsMovedAnchor(t *testing.T) {
	prev := "target line here\nfiller\nother\n"
	next := "other\nfiller\ntarget line here\n"
	c := Comment{ID: "c_1", StartLine: 1, EndLine: 1, Anchor: "target line here"}

	got := carryOne(t, c, prev, next)

	if got.StartLine != 3 || got.Drifted {
		t.Errorf("expected anchor found at 3, got %d drifted=%v", got.StartLine, got.Drifted)
	}
}

func TestCarryForwardMarksDriftedWhenAnchorRemoved(t *testing.T) {
	prev := "keep\nremove me entirely\nkeep too\n"
	next := "keep\nkeep too\n"
	c := Comment{ID: "c_1", StartLine: 2, EndLine: 2, Anchor: "remove me entirely"}

	got := carryOne(t, c, prev, next)

	if !got.Drifted {
		t.Errorf("expected drifted, got %+v", got)
	}
}

func TestCarryForwardToleratesInPlaceEdit(t *testing.T) {
	prev := "first\nthe quick brown fox jumps\nlast\n"
	next := "first\nthe quick brown fox jumps again\nlast\n"
	c := Comment{ID: "c_1", StartLine: 2, EndLine: 2, Anchor: "the quick brown fox jumps"}

	got := carryOne(t, c, prev, next)

	if got.Drifted || got.StartLine != 2 {
		t.Errorf("in-place edit should stay anchored: %+v", got)
	}
}

func TestCarryForwardSkipsFileScopeAndOldSide(t *testing.T) {
	prev := "a\nb\n"
	next := "x\ny\nz\n"
	fileC := Comment{ID: "c_1", Scope: "file", Body: "file-level"}
	oldC := Comment{ID: "c_2", Side: "old", StartLine: 2, EndLine: 2, Anchor: "gone"}

	out := CarryForwardFile([]Comment{fileC, oldC}, prev, next, Now())

	if out[0].StartLine != 0 || out[0].Drifted {
		t.Errorf("file-scope comment should be untouched: %+v", out[0])
	}
	if out[1].StartLine != 2 || out[1].Drifted {
		t.Errorf("old-side comment should keep its base-ref position: %+v", out[1])
	}
}

func TestCarryForwardClampsBeyondEOF(t *testing.T) {
	prev := "a\nb\nc\nd\n"
	next := "a\n"
	c := Comment{ID: "c_1", StartLine: 4, EndLine: 4, Anchor: "d"}

	got := carryOne(t, c, prev, next)

	// SplitLines keeps the trailing empty line, so the new file has 2 lines.
	if got.StartLine < 1 || got.StartLine > 2 {
		t.Errorf("expected clamp into the new file, got %+v", got)
	}
	if !got.Drifted {
		t.Error("expected drifted when the anchored line disappeared")
	}
}

func TestAnchorSimilar(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"same text", "same text", true},
		{"prefix of a longer line", "prefix of a longer", true}, // containment, >= 8 chars
		{"}", "} // end", false},                                // short anchors never contain-match
		{"the quick brown fox", "the quick brwon fox", true},    // small typo, ratio >= 0.7
		{"completely different", "nothing alike here!", false},
	}
	for _, tt := range tests {
		if got := anchorSimilar(tt.a, tt.b); got != tt.want {
			t.Errorf("anchorSimilar(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}
