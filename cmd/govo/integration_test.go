package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var integrationDiagnosticLine = regexp.MustCompile(`(?m)^(.+\.go):(\d+):\d+: (GOV\d{3}):`)

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
		construction = "use/use.go:6: GOV001"
		unexported   = "use/use.go:7: GOV003"
		testFile     = "use/use_test.go:10: GOV001"
		tagged       = "tagged/tagged.go:7: GOV001"
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

			got := integrationDiagnostics(t, modDir, string(out))
			if exitCode != tc.exitCode || !slices.Equal(got, tc.want) {
				t.Fatalf("%q exited with %d and reported %q; want %d and %q\noutput:\n%s", tc.command, exitCode, got, tc.exitCode, tc.want, out)
			}
		})
	}
}

// TestIntegrationJSON checks that -json output carries the rule ID as each
// diagnostic's category.
func TestIntegrationJSON(t *testing.T) {
	_, bin := buildGovo(t)

	modDir, err := filepath.Abs(filepath.Join("testdata", "mod"))
	if err != nil {
		t.Fatal(err)
	}

	env := append(os.Environ(), "GOWORK=off", "GOFLAGS=")

	cmd := exec.Command(bin, "-json", "-test=false", "./use")
	cmd.Dir = modDir
	cmd.Env = env

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("run %q: %v", cmd.Args, err)
	}

	var tree map[string]map[string][]struct {
		Category string `json:"category"`
		Message  string `json:"message"`
	}
	if err := json.Unmarshal(out, &tree); err != nil {
		t.Fatalf("decode output: %v\noutput:\n%s", err, out)
	}

	var got []string

	for _, analyzers := range tree {
		for _, d := range analyzers["govo"] {
			if !strings.HasPrefix(d.Message, d.Category+": ") {
				t.Errorf("diagnostic %q has category %q", d.Message, d.Category)
			}

			got = append(got, d.Category)
		}
	}

	slices.Sort(got)

	if want := []string{"GOV001", "GOV003"}; !slices.Equal(got, want) {
		t.Fatalf("got categories %q, want %q\noutput:\n%s", got, want, out)
	}
}

// integrationDiagnostics returns the sorted "file:line: RULE" entries in output with
// file names relative to dir.
func integrationDiagnostics(t *testing.T, dir, output string) []string {
	t.Helper()

	var got []string

	for _, m := range integrationDiagnosticLine.FindAllStringSubmatch(output, -1) {
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
