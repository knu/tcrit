package cli

import (
	"debug/buildinfo"
	"fmt"
	"os"
	"path/filepath"

	"github.com/knu/tcrit/internal/codexwrapper"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:                "codex [args...]",
		Short:              "Launch Codex with terminal context for reviews through its local daemon",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return codexwrapper.Run(args, false)
		},
	})
}

func withCodexWrapperHint(err error) error {
	self, exeErr := os.Executable()
	if exeErr == nil {
		if hint := codexWrapperHint(self, os.Getenv("PATH")); hint != "" {
			return fmt.Errorf("%w\nhint: %s", err, hint)
		}
	}
	return err
}

func codexWrapperHint(self, path string) string {
	self, err := filepath.EvalSymlinks(self)
	if err != nil {
		return ""
	}
	wrapper := filepath.Join(filepath.Dir(self), "codex")
	// A tcrit-only Go install may sit beside the original Codex instead.
	bi, err := buildinfo.ReadFile(wrapper)
	if err != nil || bi.Path != "github.com/knu/tcrit/cmd/codex" {
		return ""
	}
	info, err := os.Stat(wrapper)
	if err != nil || info.Mode().Perm()&0111 == 0 {
		return ""
	}
	var first string
	for _, dir := range filepath.SplitList(path) {
		candidate := filepath.Join(dir, "codex")
		fi, err := os.Stat(candidate)
		if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0111 == 0 {
			continue
		}
		if os.SameFile(info, fi) {
			if first == "" {
				return ""
			}
			return fmt.Sprintf("TCrit provides a Codex wrapper that passes terminal context to shell tools. If this review was launched from Codex, using that wrapper may let TCrit detect your tmux or Herdr session. Currently %q precedes the wrapper %q in PATH; move the wrapper directory earlier. With mise, place github:knu/tcrit before the Codex tool in [tools] and reactivate the shell. Then resume Codex from the intended terminal (or run `tcrit codex resume`).", first, candidate)
		}
		if first == "" {
			first = candidate
		}
	}
	return ""
}
