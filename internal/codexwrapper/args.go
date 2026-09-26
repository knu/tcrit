package codexwrapper

import "strings"

// interactive recognizes the CLI forms whose local daemon needs terminal context.
// Unknown options are passed through unchanged until their arity is known here.
func interactive(args []string) bool {
	command := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return true
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			if !command {
				command = true
				switch arg {
				case "resume", "fork":
					continue
				case "agents", "exec", "e", "review", "login", "logout", "mcp", "plugin", "app-server", "remote-control", "app", "completion", "update", "doctor", "sandbox", "debug", "execpolicy", "apply", "a", "queue", "archive", "delete", "migrate-rollouts", "unarchive", "cloud", "cloud-tasks", "features", "help", "tcp-tunnel":
					return false
				}
			}
			continue
		}
		name, _, attached := strings.Cut(arg, "=")
		switch name {
		case "--help", "-h", "--version", "-V", "--no-daemon", "--remote", "--remote-auth-token-env", "--oss", "--profile", "-p":
			return false
		case "--config", "-c", "--model", "-m", "--sandbox", "-s", "--ask-for-approval", "-a", "--cd", "-C", "--add-dir", "--local-provider", "--enable", "--disable":
			if !attached {
				i++
				if i == len(args) {
					return false
				}
			}
		case "--image", "-i":
			// Images consume multiple values, so skip them up to the next flag.
			for i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
			}
		case "--search", "--no-alt-screen", "--strict-config", "--approve-for-me", "--not-so-yolo", "--dangerously-bypass-approvals-and-sandbox", "--yolo", "--dangerously-bypass-hook-trust", "--worktree", "--last", "--all", "--include-non-interactive":
		default:
			if len(arg) > 2 && strings.ContainsRune("cmsaC", rune(arg[1])) && arg[0:2] != "--" {
				continue
			}
			return false
		}
	}
	return true
}
