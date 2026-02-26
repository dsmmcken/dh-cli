package discovery

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// discoverDocker finds Deephaven servers running in Docker containers.
// Returns empty list (not error) if docker is not installed.
func discoverDocker() ([]Server, error) {
	out, err := exec.Command("docker", "ps", "--format", "{{.ID}}\t{{.Image}}\t{{.Ports}}").Output()
	if err != nil {
		// docker not installed or not running — not an error
		return nil, nil
	}
	return ParseDockerPSOutput(string(out)), nil
}

// portMapping matches port mappings like "0.0.0.0:10000->10000/tcp"
var portMapping = regexp.MustCompile(`(\d+\.\d+\.\d+\.\d+):(\d+)->(\d+)/tcp`)

// deephaven image patterns
var dhImagePatterns = []string{
	"deephaven",
	"ghcr.io/deephaven",
}

// ParseDockerPSOutput parses docker ps output into servers.
// Exported for testing.
func ParseDockerPSOutput(content string) []Server {
	var servers []Server
	lines := strings.Split(strings.TrimSpace(content), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		containerID := parts[0]
		image := parts[1]
		ports := parts[2]

		if !isDeephavenImage(image) {
			continue
		}

		for _, match := range portMapping.FindAllStringSubmatch(ports, -1) {
			hostPort, err := strconv.Atoi(match[2])
			if err != nil {
				continue
			}
			servers = append(servers, Server{
				Port:        hostPort,
				Source:      "docker",
				ContainerID: containerID,
				Script:      image,
			})
		}
	}
	return servers
}

// isDeephavenImage checks if a Docker image name looks like a Deephaven image.
func isDeephavenImage(image string) bool {
	lower := strings.ToLower(image)
	for _, pattern := range dhImagePatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}
