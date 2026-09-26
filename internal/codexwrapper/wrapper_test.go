package codexwrapper

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDaemonArgumentInjectionPolicy(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want launchMode
	}{
		{"default interactive launch", nil, daemonLaunch},
		{"resume session", []string{"resume", "--last"}, daemonLaunch},
		{"fork session", []string{"fork", "session", "a prompt"}, daemonLaunch},
		{"config value is not a command", []string{"-c", `model="exec"`, "resume"}, daemonLaunch},
		{"model value is not a command", []string{"--model", "exec", "hello"}, daemonLaunch},
		{"attached option value", []string{"-mexec", "resume"}, daemonLaunch},
		{"literal prompt after separator", []string{"--", "--remote"}, daemonLaunch},
		{"exec stays unchanged", []string{"exec", "--", "resume"}, passthrough},
		{"management command after options", []string{"-c", "a=b", "update"}, passthrough},
		{"explicit embedded mode", []string{"resume", "--no-daemon"}, passthrough},
		{"explicit local connection", []string{"resume", "--remote=unix://"}, localAttach},
		{"explicit socket path", []string{"--remote", "unix:///tmp/codex.sock", "resume", "session-id"}, localAttach},
		{"network connection", []string{"--remote=ws://localhost:1234", "resume"}, passthrough},
		{"missing remote value", []string{"--remote"}, passthrough},
		{"empty remote value", []string{"--remote="}, passthrough},
		{"duplicate remote", []string{"--remote=unix://", "--remote=unix:///tmp/other.sock"}, passthrough},
		{"literal remote prompt", []string{"--", "--remote=unix:///tmp/codex.sock"}, daemonLaunch},
		{"remote config value", []string{"-c", "--remote=unix://"}, daemonLaunch},
		{"local agents", []string{"--remote=unix://", "agents"}, passthrough},
		{"local with profile", []string{"--remote=unix://", "--profile", "personal"}, passthrough},
		{"local without daemon", []string{"--remote=unix://", "--no-daemon"}, passthrough},
		{"local unknown flag", []string{"--remote=unix://", "--unknown"}, passthrough},
		{"help stays unchanged", []string{"resume", "--help"}, passthrough},
		{"profile owns connection settings", []string{"--profile", "personal"}, passthrough},
		{"unknown option stays unchanged", []string{"--unknown", "resume"}, passthrough},
		{"incomplete option stays unchanged", []string{"--config"}, passthrough},
		{"image values before literal prompt", []string{"--image", "a.png", "b.png", "--", "prompt"}, daemonLaunch},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := modeForArgs(tt.args); got != tt.want {
				t.Fatalf("modeForArgs(%q) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestContextArgs(t *testing.T) {
	env := map[string]string{"TMUX": "/socket\"\\\n\x01\x7f,1,0", "TMUX_PANE": "%2"}
	args := contextArgs(func(k string) string { return env[k] })
	if args[1] != `shell_environment_policy.set.TMUX="/socket\"\\\n\u0001\u007f,1,0"` {
		t.Fatalf("invalid quoted config: %q", args[1])
	}
	if !strings.Contains(strings.Join(args, " "), `HERDR_PANE_ID=""`) {
		t.Fatal("missing stale context reset")
	}
	delete(env, "TMUX_PANE")
	if got := contextArgs(func(k string) string { return env[k] }); got != nil {
		t.Fatalf("incomplete context accepted: %v", got)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestResolve(t *testing.T) {
	dir := t.TempDir()
	before := filepath.Join(dir, "before", "codex")
	self := filepath.Join(dir, "self", "codex")
	after := filepath.Join(dir, "after", "codex")
	for _, path := range []string{before, self, after} {
		writeExecutable(t, path, "#!/bin/sh\nexit 0\n")
	}
	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(filepath.Dir(self), alias); err != nil {
		t.Fatal(err)
	}
	path := strings.Join([]string{filepath.Dir(before), filepath.Dir(self), alias, filepath.Dir(after)}, string(os.PathListSeparator))
	got, err := resolve(path, self)
	if err != nil || got != after {
		t.Fatalf("resolve = %q, %v, want %q", got, err, after)
	}
	if _, err := resolve(filepath.Dir(before), self); err == nil {
		t.Fatal("missing wrapper in PATH must fail")
	}
}

func TestResolveSkipsMiseShims(t *testing.T) {
	dir := t.TempDir()
	self := filepath.Join(dir, "wrapper", "codex")
	shim := filepath.Join(dir, "shims", "codex")
	real := filepath.Join(dir, "real", "codex")
	for _, p := range []string{self, real} {
		writeExecutable(t, p, "#!/bin/sh\nexit 0\n")
	}
	writeExecutable(t, shim, "#!/bin/sh\n# mise generated shim\nexec mise x -- codex\n")
	path := strings.Join([]string{filepath.Dir(self), filepath.Dir(shim), filepath.Dir(real)}, string(os.PathListSeparator))
	got, err := resolve(path, self)
	if err != nil || got != real {
		t.Fatalf("resolve shell shim = %q, %v", got, err)
	}
	if err := os.Remove(shim); err != nil {
		t.Fatal(err)
	}
	mise := filepath.Join(dir, "mise")
	writeExecutable(t, mise, "fake mise executable")
	if err := os.Symlink(mise, shim); err != nil {
		t.Fatal(err)
	}
	got, err = resolve(path, self)
	if err != nil || got != real {
		t.Fatalf("resolve symlink shim = %q, %v", got, err)
	}
}

func TestWrapperProcess(t *testing.T) {
	dir := t.TempDir()
	wrapper := filepath.Join(dir, "wrapper", "codex")
	if err := os.MkdirAll(filepath.Dir(wrapper), 0755); err != nil {
		t.Fatal(err)
	}
	build := exec.Command("go", "build", "-o", wrapper, "../../cmd/codex")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	tcrit := filepath.Join(dir, "tcrit")
	build = exec.Command("go", "build", "-o", tcrit, "../../cmd/tcrit")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build tcrit: %v\n%s", err, out)
	}
	foreign := filepath.Join(dir, "foreign", "codex")
	// Model tfil's forward-only lookup without altering the child's PATH.
	writeExecutable(t, foreign, `#!/bin/sh
rest=${PATH#*"${0%/*}:"}
IFS=:
for dir in $rest; do
  if [ -x "$dir/codex" ]; then exec "$dir/codex" "$@"; fi
done
exit 127
`)
	real := filepath.Join(dir, "real", "codex")
	writeExecutable(t, real, `#!/bin/sh
if [ "$#" -gt 0 ]; then printf '%s\000' "$@" >> "$TEST_LOG"; fi
printf '\000' >> "$TEST_LOG"
if [ "$1" = app-server ]; then
  if [ -n "$TEST_START_ERROR" ]; then printf "%s\n" "$TEST_START_ERROR" >&2; fi
  exit "${TEST_START_EXIT:-0}"
fi
exit 23
`)
	for _, key := range []string{"TMUX", "TMUX_PANE", "HERDR_ENV", "HERDR_WORKSPACE_ID", "HERDR_TAB_ID", "HERDR_PANE_ID"} {
		t.Setenv(key, "")
	}
	t.Setenv("TMUX", "/test/socket,1,0")
	t.Setenv("TMUX_PANE", "%42")
	t.Setenv("PATH", filepath.Dir(wrapper)+string(os.PathListSeparator)+filepath.Dir(real))
	for _, tt := range []struct {
		name       string
		args       []string
		startExit  string
		startError string
		wantExit   int
		adapt      string
		entry      string
	}{
		{"interactive", []string{"resume", "--last", "a prompt"}, "0", "", 23, "daemon", "wrapper"},
		{"update", []string{"update"}, "0", "", 23, "", "wrapper"},
		{"explicit remote", []string{"--remote", "ws://example:1234"}, "0", "", 23, "", "wrapper"},
		{"local reconnect", []string{"--remote", "unix:///tmp/codex socket.sock", "resume", "session-id"}, "0", "", 23, "local", "wrapper"},
		{"local reconnect attached", []string{"resume", "session-id", "--remote=unix:///tmp/codex.sock"}, "0", "", 23, "local", "wrapper"},
		{"local default socket", []string{"--remote=unix://", "resume", "--last"}, "0", "", 23, "local", "wrapper"},
		{"local subcommand reconnect", []string{"--remote", "unix:///tmp/codex.sock", "resume", "session-id"}, "0", "", 23, "local", "subcommand"},
		{"local agents", []string{"--remote", "unix:///tmp/codex.sock", "agents"}, "0", "", 23, "", "wrapper"},
		{"local help", []string{"--remote", "unix:///tmp/codex.sock", "resume", "--help"}, "0", "", 23, "", "wrapper"},
		{"no daemon", []string{"--no-daemon", "", "a b"}, "0", "", 23, "", "wrapper"},
		{"start failure", nil, "9", "daemon could not start", 1, "daemon", "wrapper"},
		{"start status two", nil, "2", "invalid configuration", 1, "daemon", "wrapper"},
		{"unsupported daemon", []string{"resume", "--last", "a prompt"}, "2", "error: unrecognized subcommand 'daemon'", 23, "daemon", "wrapper"},
		{"unsupported app server", nil, "2", "error: unrecognized subcommand 'app-server'", 23, "daemon", "wrapper"},
		{"unsupported start", nil, "2", "error: unrecognized subcommand 'start'", 23, "daemon", "wrapper"},
		{"app server without daemon", nil, "2", "error: unexpected argument 'daemon' found", 23, "daemon", "wrapper"},
		{"unrelated parser error", nil, "2", "error: unrecognized subcommand 'other'", 1, "daemon", "wrapper"},
		{"wrong failure status", nil, "1", "error: unrecognized subcommand 'daemon'", 1, "daemon", "wrapper"},
		{"subcommand", []string{"resume", "--last"}, "0", "", 23, "daemon", "subcommand"},
		{"foreign before", []string{"fork", "session", "a b"}, "0", "", 23, "daemon", "before"},
		{"foreign after", []string{"resume", "--last"}, "0", "", 23, "daemon", "after"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			log := filepath.Join(dir, tt.name+".log")
			t.Setenv("TEST_LOG", log)
			t.Setenv("TEST_START_EXIT", tt.startExit)
			t.Setenv("TEST_START_ERROR", tt.startError)
			entry := wrapper
			args := tt.args
			switch tt.entry {
			case "subcommand":
				entry = tcrit
				args = append([]string{"codex"}, args...)
			case "before":
				entry = foreign
				t.Setenv("PATH", strings.Join([]string{filepath.Dir(foreign), filepath.Dir(wrapper), filepath.Dir(real)}, string(os.PathListSeparator)))
			case "after":
				t.Setenv("PATH", strings.Join([]string{filepath.Dir(wrapper), filepath.Dir(foreign), filepath.Dir(real)}, string(os.PathListSeparator)))
			}
			cmd := exec.Command(entry, args...)
			out, err := cmd.CombinedOutput()
			if err == nil || cmd.ProcessState.ExitCode() != tt.wantExit {
				t.Fatalf("exit: %v; output: %s", err, out)
			}
			if tt.wantExit == 23 && len(out) != 0 {
				t.Fatalf("unexpected startup diagnostic: %s", out)
			}
			if tt.wantExit == 1 && !strings.Contains(string(out), tt.startError) {
				t.Fatalf("startup diagnostic lost: %s", out)
			}
			data, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			var want []string
			if tt.adapt == "daemon" {
				want = append(want, "app-server", "daemon", "start", "")
			}
			if tt.wantExit == 23 {
				if tt.adapt != "" && tt.startExit == "0" {
					want = append(want, contextArgs(os.Getenv)...)
					if tt.adapt == "daemon" {
						want = append(want, "--remote", "unix://")
					}
				}
				want = append(want, tt.args...)
				want = append(want, "")
			}
			got := strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("args = %q, want %q", got, want)
			}
		})
	}
}
