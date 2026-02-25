package discovery

import (
	"fmt"
	"os/exec"
)

// Kill stops the server running on the given port.
// It discovers the server first, then sends SIGTERM (for processes)
// or runs docker stop (for containers).
func Kill(port int) error {
	servers, err := Discover()
	if err != nil {
		return fmt.Errorf("discovering servers: %w", err)
	}

	for _, s := range servers {
		if s.Port != port {
			continue
		}
		if s.ContainerID != "" {
			return killDocker(s.ContainerID)
		}
		if s.PID > 0 {
			return killProcess(s.PID)
		}
		return fmt.Errorf("server on port %d has no PID or container ID", port)
	}

	return &NotFoundError{Port: port}
}

// NotFoundError is returned when no server is found on the specified port.
type NotFoundError struct {
	Port int
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("no server found on port %d", e.Port)
}

func killDocker(containerID string) error {
	cmd := exec.Command("docker", "stop", containerID)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker stop %s: %s: %w", containerID, string(out), err)
	}
	return nil
}
