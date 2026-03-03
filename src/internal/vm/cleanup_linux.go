//go:build linux

package vm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// CleanupStaleInstances scans the run directory for orphaned instances
// and removes them (kill process, remove files).
func CleanupStaleInstances(paths *VMPaths) {
	runDir := paths.RunDir
	entries, err := os.ReadDir(runDir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}

		// Skip recently-created directories — they may be mid-restore
		// (instance.json is written after Firecracker starts, so there's
		// a window where a valid in-progress directory has no JSON yet).
		if fi, err := e.Info(); err == nil && time.Since(fi.ModTime()) < 60*time.Second {
			continue
		}

		instanceDir := filepath.Join(runDir, e.Name())
		infoPath := filepath.Join(instanceDir, "instance.json")

		data, err := os.ReadFile(infoPath)
		if err != nil {
			os.RemoveAll(instanceDir)
			continue
		}

		var info InstanceInfo
		if err := json.Unmarshal(data, &info); err != nil {
			os.RemoveAll(instanceDir)
			continue
		}

		// Check if process is still alive (signal 0 = existence check)
		if info.PID > 0 {
			process, err := os.FindProcess(info.PID)
			if err == nil {
				if err := process.Signal(syscall.Signal(0)); err == nil {
					// Process is alive — skip
					continue
				}
			}
		}

		// Process is dead — cleanup
		os.RemoveAll(instanceDir)
	}
}
