package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
)

var diagnosticLine = regexp.MustCompile(`(?m)^(.+\.go):(\d+):\d+: (GOVO\d{3}):`)

// TestIntegration builds the govo command and runs it both standalone and as
// a go vet tool against the module in testdata/mod.
func TestIntegration(t *testing.T) {
	goCmd, bin := buildGovo(t)
	tmp := t.TempDir()

	modDir, err := filepath.Abs(filepath.Join("testdata", "mod"))
	if err != nil {
		t.Fatal(err)
	}

	// Configuration shared by all packages must be passed as an absolute path.
	noTests := filepath.Join(tmp, "govo.yaml")
	if err := os.WriteFile(noTests, []byte("tests: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	const (
		construction = "use/use.go:6: GOVO001"
		unexported   = "use/use.go:7: GOVO003"
		testFile     = "use/use_test.go:10: GOVO001"
		tagged       = "tagged/tagged.go:7: GOVO001"
	)

	// Keep go vet and package loading independent of the developer's
	// workspace and GOFLAGS.
	env := append(os.Environ(), "GOWORK=off", "GOFLAGS=")

	vet := func(args ...string) []string {
		return append([]string{goCmd, "vet", "-vettool=" + bin}, args...)
	}

	for _, tc := range []struct {
		name     string
		command  []string
		exitCode int
		want     []string
	}{
		{"standalone default packages", []string{bin}, 3, []string{construction, unexported, testFile}},
		{"standalone tags", []string{bin, "-tags=special"}, 3, []string{construction, unexported, testFile, tagged}},
		{"standalone config", []string{bin, "-config=" + noTests}, 3, []string{construction, unexported}},
		{"standalone clean package", []string{bin, "./dom"}, 0, nil},
		{"vet", vet("./..."), 1, []string{construction, unexported, testFile}},
		{"vet tags", vet("-tags=special", "./..."), 1, []string{construction, unexported, testFile, tagged}},
		{"vet config", vet("-config="+noTests, "./..."), 1, []string{construction, unexported}},
		{"vet clean package", vet("./dom"), 0, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(tc.command[0], tc.command[1:]...)
			cmd.Dir = modDir
			cmd.Env = env

			out, err := cmd.CombinedOutput()

			exitCode := 0
			if exitErr := (*exec.ExitError)(nil); errors.As(err, &exitErr) {
				exitCode = exitErr.ExitCode()
			} else if err != nil {
				t.Fatalf("run %q: %v", tc.command, err)
			}

			slices.Sort(tc.want)

			got := diagnostics(t, modDir, string(out))
			if exitCode != tc.exitCode || !slices.Equal(got, tc.want) {
				t.Fatalf("%q exited with %d and reported %q; want %d and %q\noutput:\n%s", tc.command, exitCode, got, tc.exitCode, tc.want, out)
			}
		})
	}
}

// TestHelp checks that govo prints its own usage instead of the analysis
// driver's, whose -tags description says the flag has no effect.
func TestHelp(t *testing.T) {
	_, bin := buildGovo(t)

	for _, tc := range []struct {
		name     string
		args     []string
		exitCode int
		stdout   bool
		message  string
	}{
		// go vet's usage refers users to both "govo help" and "govo -help".
		{"help command", []string{"help"}, 0, true, ""},
		{"help flag", []string{"-help"}, 0, false, ""},
		{"short help flag", []string{"-h"}, 0, false, ""},
		{"unknown flag", []string{"-bogus"}, 2, false, "flag provided but not defined: -bogus"},
		{"invalid flag value", []string{"-c=many"}, 2, false, `invalid value "many" for flag -c: parse error`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(bin, tc.args...)

			var stdout, stderr strings.Builder

			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			err := cmd.Run()

			exitCode := 0
			if exitErr := (*exec.ExitError)(nil); errors.As(err, &exitErr) {
				exitCode = exitErr.ExitCode()
			} else if err != nil {
				t.Fatalf("run govo %q: %v", tc.args, err)
			}

			help, other := stderr.String(), stdout.String()
			if tc.stdout {
				help, other = other, help
			}

			if exitCode != tc.exitCode ||
				!strings.HasPrefix(strings.TrimPrefix(help, tc.message+"\n"), usage) ||
				!strings.Contains(help, tc.message) ||
				strings.Contains(help, "no effect") ||
				other != "" {
				t.Fatalf("govo %q exited with %d\nstdout:\n%s\nstderr:\n%s", tc.args, exitCode, stdout.String(), stderr.String())
			}
		})
	}
}

// buildGovo builds the command into a temporary directory and returns the
// go command and the binary.
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

// diagnostics returns the sorted "file:line: RULE" entries in output with
// file names relative to dir.
func diagnostics(t *testing.T, dir, output string) []string {
	t.Helper()

	var got []string

	for _, m := range diagnosticLine.FindAllStringSubmatch(output, -1) {
		file := m[1]
		if filepath.IsAbs(file) {
			rel, err := filepath.Rel(dir, file)
			if err != nil {
				t.Fatal(err)
			}

			file = rel
		}

		got = append(got, filepath.ToSlash(file)+":"+m[2]+": "+m[3])
	}

	slices.Sort(got)

	return got
}
