package review

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/knu/tcrit/internal/xdg"
)

// SessionKey derives the stable 12-hex-char review session key, matching
// crit's algorithm: sha256 over the working directory plus either the file
// arguments (files mode; branch-independent) or the branch (git mode).
func SessionKey(cwd, branch string, args []string) string {
	h := sha256.New()
	h.Write([]byte(cwd))
	if len(args) > 0 {
		sorted := append([]string(nil), args...)
		sort.Strings(sorted)
		for _, a := range sorted {
			h.Write([]byte{0})
			h.Write([]byte(a))
		}
	} else {
		h.Write([]byte{0})
		h.Write([]byte(branch))
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// ReviewsRoot returns the directory that holds all review folders,
// honoring an explicit data root (--output / config) when non-empty.
func ReviewsRoot(dataRoot string) string {
	if dataRoot != "" {
		return filepath.Join(dataRoot, "reviews")
	}
	return filepath.Join(xdg.StateHome(), "reviews")
}

// Dir returns the review folder for a session key.
func Dir(dataRoot, key string) string {
	return filepath.Join(ReviewsRoot(dataRoot), key)
}

// JSONPath returns the review.json path inside a review folder.
func JSONPath(dir string) string {
	return filepath.Join(dir, "review.json")
}

// legacyDir is the pre-CritJSON per-project state directory.
const legacyDir = ".crit"

// WarnLegacyState prints a one-line warning when the obsolete per-project
// .crit directory from earlier TCrit releases is present in cwd.
func WarnLegacyState() {
	if fi, err := os.Stat(legacyDir); err == nil && fi.IsDir() {
		fmt.Fprintf(os.Stderr,
			"tcrit: found legacy review state in %s/ (no longer used); remove it with `rm -r %s`\n",
			legacyDir, legacyDir)
	}
}
