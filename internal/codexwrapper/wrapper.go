// Package codexwrapper preserves the invoking terminal context across Codex's daemon.
package codexwrapper

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// Run replaces this process with Codex. wrapped is true for the codex entrypoint.
func Run(args []string, wrapped bool) error {
	bin, err := findCodex(wrapped)
	if err != nil {
		return err
	}
	prefix := contextArgs(os.Getenv)
	if len(prefix) > 0 && interactive(args) {
		start := exec.Command(bin, "app-server", "daemon", "start")
		start.Stdin = os.Stdin
		output, startErr := start.CombinedOutput()
		if unsupportedDaemon(startErr, output) {
			return syscall.Exec(bin, append([]string{bin}, args...), os.Environ())
		}
		if _, err := os.Stderr.Write(output); err != nil {
			return fmt.Errorf("write Codex daemon output: %w", err)
		}
		if startErr != nil {
			return fmt.Errorf("start Codex daemon: %w", startErr)
		}
		prefix = append(prefix, "--remote", "unix://")
		args = append(prefix, args...)
	}
	return syscall.Exec(bin, append([]string{bin}, args...), os.Environ())
}

func unsupportedDaemon(err error, output []byte) bool {
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 2 {
		return false
	}
	// Match the CLI parser's diagnostic, not arbitrary startup failures.
	for line := range strings.Lines(string(output)) {
		for _, arg := range []string{"app-server", "daemon", "start"} {
			if strings.HasPrefix(line, "error: unrecognized subcommand '"+arg+"'") ||
				strings.HasPrefix(line, "error: unexpected argument '"+arg+"' found") {
				return true
			}
		}
	}
	return false
}

func contextArgs(getenv func(string) string) []string {
	tmux := getenv("TMUX") != "" && getenv("TMUX_PANE") != ""
	herdr := getenv("HERDR_WORKSPACE_ID") != "" && getenv("HERDR_TAB_ID") != "" && getenv("HERDR_PANE_ID") != ""
	if !tmux && !herdr {
		return nil
	}
	var args []string
	for _, key := range []string{"TMUX", "TMUX_PANE", "HERDR_ENV", "HERDR_WORKSPACE_ID", "HERDR_TAB_ID", "HERDR_PANE_ID"} {
		// Include empty values to clear terminal context inherited by the daemon.
		value, _ := json.Marshal(getenv(key)) // A string cannot fail JSON marshaling.
		// JSON strings are TOML basic strings, except TOML also forbids raw DEL.
		quoted := strings.ReplaceAll(string(value), "\x7f", `\u007f`)
		args = append(args, "-c", "shell_environment_policy.set."+key+"="+quoted)
	}
	return args
}
