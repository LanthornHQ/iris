package browser

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

// xvfbProcess manages a Xvfb virtual framebuffer child process.
type xvfbProcess struct {
	cmd     *exec.Cmd
	display string
}

// startXvfb launches Xvfb on the first available display number and waits
// until its Unix socket appears. Returns an error if the binary is not found
// or the socket does not become ready within 2 s.
func startXvfb(width, height int) (*xvfbProcess, error) {
	display := findFreeDisplay()
	cmd := exec.Command("Xvfb", display,
		"-screen", "0", fmt.Sprintf("%dx%dx24", width, height),
		"-nolisten", "tcp",
		"-ac", // disable access control — required in rootless containers
	)
	// Surface Xvfb errors to the parent's stderr so they appear in logs.
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("Xvfb exec on %s: %w", display, err)
	}

	socket := fmt.Sprintf("/tmp/.X11-unix/X%s", display[1:])
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(socket); err == nil {
			return &xvfbProcess{cmd: cmd, display: display}, nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Socket never appeared — kill the process and surface the failure.
	cmd.Process.Kill() //nolint:errcheck
	cmd.Wait()         //nolint:errcheck
	return nil, fmt.Errorf("Xvfb socket %s did not appear within 2s", socket)
}

func (x *xvfbProcess) stop() {
	if x == nil || x.cmd == nil || x.cmd.Process == nil {
		return
	}
	x.cmd.Process.Kill() //nolint:errcheck
	x.cmd.Wait()         //nolint:errcheck
}

// findFreeDisplay returns the first display number whose X11 socket does not
// yet exist, starting from :99.
func findFreeDisplay() string {
	for n := 99; n < 300; n++ {
		if _, err := os.Stat(fmt.Sprintf("/tmp/.X11-unix/X%d", n)); os.IsNotExist(err) {
			return fmt.Sprintf(":%d", n)
		}
	}
	return ":299"
}
