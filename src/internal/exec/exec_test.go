package exec

import (
	"strings"
	"testing"
)

func TestRun_VMAndHostMutuallyExclusive(t *testing.T) {
	cfg := &ExecConfig{
		Code:   "print('hello')",
		VMMode: true,
		Host:   "example.com",
		Port:   10000,
	}

	exitCode, _, err := Run(cfg)
	if err == nil {
		t.Fatal("expected error for --vm + --host, got nil")
	}
	if !strings.Contains(err.Error(), "cannot use both --vm and --host") {
		t.Errorf("unexpected error: %v", err)
	}
	if exitCode == 0 {
		t.Error("expected non-zero exit code")
	}
}

func TestExecConfig_VMModeFlagExists(t *testing.T) {
	cfg := &ExecConfig{}
	// Verify VMMode field exists and defaults to false
	if cfg.VMMode {
		t.Error("VMMode should default to false")
	}
}
