package codexwrapper

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

func findCodex(wrapped bool) (string, error) {
	if !wrapped {
		return exec.LookPath("codex")
	}
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	return resolve(os.Getenv("PATH"), self)
}

func resolve(path, self string) (string, error) {
	info, err := os.Stat(self)
	if err != nil {
		return "", err
	}
	after := false
	for _, dir := range filepath.SplitList(path) {
		candidate := filepath.Join(dir, "codex")
		fi, err := os.Stat(candidate)
		if err != nil {
			continue
		}
		if os.SameFile(info, fi) {
			after = true
			continue
		}
		if after && fi.Mode().IsRegular() && fi.Mode().Perm()&0111 != 0 && !miseShim(candidate) {
			return filepath.Abs(candidate)
		}
	}
	return "", fmt.Errorf("codex: no executable found after this wrapper in PATH (mise users: activate tool directories)")
}

// A shim may select this wrapper again even though it appears later in PATH.
func miseShim(path string) bool {
	if target, err := filepath.EvalSymlinks(path); err == nil && filepath.Base(target) == "mise" {
		return true
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	const header = "#!/bin/sh\n# mise generated shim\n"
	b, err := io.ReadAll(io.LimitReader(f, int64(len(header))))
	closeErr := f.Close()
	return err == nil && closeErr == nil && string(b) == header
}
