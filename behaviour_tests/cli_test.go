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
	dhBinary string
	buildOnce sync.Once
	buildErr  error
)

func TestMain(m *testing.M) {
	// Build the binary before running tests
	buildOnce.Do(func() {
		// Build from src
		goSrcDir := filepath.Join("..", "src")
		tmpDir, err := os.MkdirTemp("", "dh-test-*")
		if err != nil {
			buildErr = err
			return
		}
		binPath := filepath.Join(tmpDir, "dh")
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/dh")
		cmd.Dir = goSrcDir
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = &BuildError{Output: string(out), Err: err}
			return
		}
		dhBinary = binPath
	})

	os.Exit(testscript.RunMain(m, map[string]func() int{
		"dh": func() int {
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
		t.Fatalf("failed to build dh: %v", buildErr)
	}
	if dhBinary == "" {
		t.Fatal("dh binary not built")
	}

	testscript.Run(t, testscript.Params{
		Dir: "testdata/scripts",
		Setup: func(env *testscript.Env) error {
			// Make dh binary available in PATH
			binDir := filepath.Dir(dhBinary)
			env.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			env.Setenv("DH_HOME", filepath.Join(env.WorkDir, ".dh"))
			return nil
		},
	})
}
