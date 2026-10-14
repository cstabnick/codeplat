// Package process starts operating-system processes.
package process

import (
	"fmt"
	"os"
	"os/exec"
)

// Start runs argv[0] with the arguments argv[1:] in dir, or in the current
// directory if dir is empty. The process's standard output and standard
// error go to this process's standard error, and its standard input is
// empty. Start returns once the process has started; the returned function
// waits for the process to exit and returns the error from that wait.
// Nothing in this package kills the process.
func Start(dir string, argv []string) (wait func() error, err error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("process: empty command")
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("process: start %s: %w", argv[0], err)
	}
	return cmd.Wait, nil
}
