package behaviour_tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

var (
	dhgBinary string
	buildOnce sync.Once
	buildErr  error
)

func TestMain(m *testing.M) {
	// Build the binary before running tests
	buildOnce.Do(func() {
		// Build from src
		goSrcDir := filepath.Join("..", "src")
		tmpDir, err := os.MkdirTemp("", "dhg-test-*")
		if err != nil {
			buildErr = err
			return
		}
		binPath := filepath.Join(tmpDir, "dhg")
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/dhg")
		cmd.Dir = goSrcDir
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = &BuildError{Output: string(out), Err: err}
			return
		}
		dhgBinary = binPath
	})

	os.Exit(testscript.RunMain(m, map[string]func() int{
		"dhg": func() int {
			// This won't be used; we use the binary path in Setup instead
			return 0
		},
	}))
}

type BuildError struct {
	Output string
	Err    error
}

func (e *BuildError) Error() string {
	return e.Output + ": " + e.Err.Error()
}

func TestBehaviour(t *testing.T) {
	if buildErr != nil {
		t.Fatalf("failed to build dhg: %v", buildErr)
	}
	if dhgBinary == "" {
		t.Fatal("dhg binary not built")
	}

	testscript.Run(t, testscript.Params{
		Dir: "testdata/scripts",
		Setup: func(env *testscript.Env) error {
			// Make dhg binary available in PATH
			binDir := filepath.Dir(dhgBinary)
			env.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			env.Setenv("DHG_HOME", filepath.Join(env.WorkDir, ".dhg"))
			return nil
		},
	})
}
