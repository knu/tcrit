package cli

import "github.com/tklauser/ps"

func inspectParentProcess(pid int) (int, error) {
	proc, err := ps.FindProcess(pid)
	if err != nil {
		return 0, err
	}
	return proc.PPID(), nil
}
