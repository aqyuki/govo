package main

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

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
