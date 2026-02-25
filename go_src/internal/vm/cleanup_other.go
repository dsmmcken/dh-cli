//go:build !linux

package vm

func CleanupStaleInstances(_ *VMPaths) {}
