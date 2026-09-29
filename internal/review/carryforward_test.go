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

func TestCarryForwardPreservesCommentIdentity(t *testing.T) {
	orig := Comment{
		ID: "c_old111", StartLine: 2, EndLine: 2, Anchor: "line two",
		Body: "fix", Author: "Human", ReviewRound: 1, Resolved: true, ResolvedRound: 1,
		Replies: []Reply{{ID: "rp_1", Body: "done", Author: "AI"}},
	}
	content := "line one\nline two\nline three\n"

	got := carryOne(t, orig, content, content)

	if got.ID != orig.ID {
		t.Errorf("expected stable ID %q, got %q", orig.ID, got.ID)
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

func TestCarryForwardToleratesMiddleCut(t *testing.T) {
	for _, tt := range []struct{ name, before, after string }{
		{"removed clause", `continue. If this organization has no locations yet,{" "}`, `continue.{" "}`},
		{"inserted clause", `continue.{" "}`, `continue. If this organization has no locations yet,{" "}`},
		{"Japanese clause", "処理を開始し、必要な設定をすべて読み込んでから終了します。", "処理を終了します。"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := Comment{ID: "c_1", StartLine: 2, EndLine: 2, Anchor: tt.before}
			got := carryOne(t, c, "first\n"+tt.before+"\nlast\n", "first\n"+tt.after+"\nlast\n")
			if got.Drifted || got.StartLine != 2 || got.EndLine != 2 {
				t.Errorf("middle cut should keep position: %+v", got)
			}
		})
	}
}

func TestCarryForwardPrefersExactMovedAnchor(t *testing.T) {
	const anchor = "the quick brown fox jumps"
	prev := anchor + "\nfirst\nsecond\nthird\n"
	next := anchor + " again\nfirst\nsecond\nthird\n" + anchor + "\n"
	c := Comment{ID: "c_1", StartLine: 1, EndLine: 1, Anchor: anchor}
	got := carryOne(t, c, prev, next)
	if got.Drifted || got.StartLine != 5 || got.EndLine != 5 {
		t.Errorf("exact moved anchor should beat similar text: %+v", got)
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

func TestOneMiddleCut(t *testing.T) {
	for _, tt := range []struct {
		short, long string
		want        bool
	}{
		{"abef", "abcdef", true},
		{"ab", "abcdef", true},
		{"ef", "abcdef", true},
		{"", "abcdef", true},
		{"abde", "abcdexf", false},
		{"efab", "abcdef", false},
		{"abcdef", "abcdef", false},
		{"abcdefg", "abcdef", false},
		{"a—f", "a—cdf", true},
		{"af", "a—f", true},
		{"é", "ê©", false}, // Matching bytes must not split UTF-8 runes.
	} {
		t.Run(tt.short+"/"+tt.long, func(t *testing.T) {
			if got := oneMiddleCut(tt.short, tt.long); got != tt.want {
				t.Errorf("oneMiddleCut(%q, %q) = %v, want %v", tt.short, tt.long, got, tt.want)
			}
		})
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
		{"return nil, err", `return fmt.Errorf("failed to open config file %q: %w", path, err)`, false},
		{"t.Fatal(err)", `t.Fatalf("unexpected error reading %s: %v", path, err)`, false},
	}
	for _, tt := range tests {
		if got := anchorSimilar(tt.a, tt.b); got != tt.want {
			t.Errorf("anchorSimilar(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}
