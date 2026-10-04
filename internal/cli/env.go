package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

var envJSON bool

var envCmd = &cobra.Command{
	Use:   "env [--json] [-- <command> [args...]]",
	Short: "Print the environment that identifies the review terminal, or run a command with it",
	Long: `Locate the Herdr or tmux terminal the way review commands do, then print the
variables a process needs to address it: TMUX, TMUX_PANE, and TTY for tmux,
or HERDR_ENV, HERDR_SOCKET_PATH, HERDR_WORKSPACE_ID, HERDR_TAB_ID, HERDR_PANE_ID,
and TTY for Herdr.  Each variable is printed as NAME=value; --json prints one
JSON object instead.  With a command, this process is replaced by the command
with those variables added to its environment.

Detection uses the multiplexer environment when present and the process
ancestry otherwise, so a tool started from the terminal's process tree can
recover its terminal even without the variables.  A background server such as
Codex's daemon has no such ancestry; pass --terminal-request <id> from a
marker prepared by ` + "`tcrit terminal prepare`" + ` to locate the pane that displays it.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if envJSON && len(args) > 0 {
			return fmt.Errorf("--json cannot be combined with a command")
		}
		vars, err := terminalEnvironment()
		if err != nil {
			return err
		}
		if len(args) > 0 {
			return execWithEnvironment(vars, args)
		}
		if envJSON {
			object := make(map[string]string, len(vars))
			for _, v := range vars {
				object[v.name] = v.value
			}
			return printJSON(cmd.OutOrStdout(), object)
		}
		for _, v := range vars {
			fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", v.name, v.value)
		}
		return nil
	},
}

type envVar struct {
	name  string
	value string
}

func init() {
	envCmd.Flags().BoolVar(&envJSON, "json", false, "output as JSON")
	envCmd.Flags().SetInterspersed(false)
	rootCmd.AddCommand(envCmd)
}

func terminalEnvironment() ([]envVar, error) {
	ctx, err := reviewTerminalContext()
	if err != nil {
		return nil, err
	}
	switch ctx := ctx.(type) {
	case tmuxContext:
		return tmuxEnvironment(ctx)
	case herdrContext:
		return herdrEnvironment(ctx)
	}
	return nil, fmt.Errorf("no Herdr or tmux terminal found; run inside Herdr or tmux")
}

// tmuxEnvironment asks the server for the pane's current identity, so a
// context known only by socket and pane also yields TMUX.
func tmuxEnvironment(tmux tmuxContext) ([]envVar, error) {
	if tmux.pane == "" {
		if tmux.session == "" {
			return nil, fmt.Errorf("tmux pane unknown")
		}
		return []envVar{{"TMUX", tmux.session}}, nil
	}
	tmuxBin, err := lookPath("tmux")
	if err != nil {
		return nil, err
	}
	out, err := commandOutput(tmuxCommand(tmuxBin, tmux, "display-message", "-p", "-t", tmux.pane, "#{socket_path}\t#{pid}\t#{session_id}\t#{pane_id}\t#{pane_tty}"))
	if err != nil {
		return nil, fmt.Errorf("querying tmux pane %s: %w", tmux.pane, err)
	}
	fields := strings.Split(strings.TrimSuffix(string(out), "\n"), "\t")
	if len(fields) != 5 || slices.Contains(fields[:4], "") {
		return nil, fmt.Errorf("unexpected tmux pane description: %q", out)
	}
	vars := []envVar{
		{"TMUX", strings.Join([]string{fields[0], fields[1], strings.TrimPrefix(fields[2], "$")}, ",")},
		{"TMUX_PANE", fields[3]},
	}
	if fields[4] != "" {
		vars = append(vars, envVar{"TTY", fields[4]})
	}
	return vars, nil
}

func herdrEnvironment(herdr herdrContext) ([]envVar, error) {
	socket := herdr.socket
	if socket == "" {
		socket = os.Getenv("HERDR_SOCKET_PATH")
	}
	vars := []envVar{{"HERDR_ENV", "1"}}
	if socket != "" {
		vars = append(vars, envVar{"HERDR_SOCKET_PATH", socket})
	}
	vars = append(vars,
		envVar{"HERDR_WORKSPACE_ID", herdr.workspace},
		envVar{"HERDR_TAB_ID", herdr.tab},
		envVar{"HERDR_PANE_ID", herdr.pane},
	)
	if tty := herdrPaneTTY(herdr); tty != "" {
		vars = append(vars, envVar{"TTY", tty})
	}
	return vars, nil
}

// herdrPaneTTY is best effort: a pane without a shell has no terminal to report.
func herdrPaneTTY(herdr herdrContext) string {
	herdrBin, err := lookPath("herdr")
	if err != nil {
		return ""
	}
	out, err := commandOutput(herdr.command(herdrBin, "pane", "process-info", "--pane", herdr.pane))
	if err != nil {
		return ""
	}
	var response struct {
		Result struct {
			ProcessInfo struct {
				TTY string `json:"tty"`
			} `json:"process_info"`
		} `json:"result"`
	}
	if json.Unmarshal(out, &response) != nil {
		return ""
	}
	tty := response.Result.ProcessInfo.TTY
	if tty != "" && !strings.HasPrefix(tty, "/") {
		tty = "/dev/" + tty
	}
	return tty
}

func execWithEnvironment(vars []envVar, argv []string) error {
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return err
	}
	return syscall.Exec(path, argv, mergeEnvironment(os.Environ(), vars))
}

// mergeEnvironment replaces existing values of the given variables and appends the rest.
func mergeEnvironment(environ []string, vars []envVar) []string {
	merged := make([]string, 0, len(environ)+len(vars))
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		if !slices.ContainsFunc(vars, func(v envVar) bool { return v.name == name }) {
			merged = append(merged, entry)
		}
	}
	for _, v := range vars {
		merged = append(merged, v.name+"="+v.value)
	}
	return merged
}
