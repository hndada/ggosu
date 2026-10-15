package screenshot

import (
	"os/exec"
	"syscall"
)

// present opens Explorer with the file selected. Explorer wants the quoted path joined
// to "/select," in one argument, so the command line is written out as is.
func present(path string) error {
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + path + `"`}
	return cmd.Start()
}
