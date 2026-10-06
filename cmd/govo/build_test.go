package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// buildGovo builds the command into a temporary directory and returns the
// go command and the binary.
//
//declscope:shared // integration_test.go and usage_test.go run the binary
func buildGovo(t *testing.T) (goCmd, bin string) {
	t.Helper()

	if testing.Short() {
		t.Skip("builds the command and runs the go tool")
	}

	goCmd, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("go command not found: %v", err)
	}

	bin = filepath.Join(t.TempDir(), "govo")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	if out, err := exec.Command(goCmd, "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	return goCmd, bin
}
