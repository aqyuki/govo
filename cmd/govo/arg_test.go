package main

import (
	"errors"
	"flag"
	"reflect"
	"testing"

	"github.com/aqyuki/govo"
)

func TestPrepareArgs(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want []string
		tags string
		err  bool
	}{
		{"no arguments", nil, []string{"./..."}, "", false},
		{"config only", []string{"-config", "config.go"}, []string{"-config", "config.go", "./..."}, "", false},
		{"config cfg suffix", []string{"-config=/tmp/custom.cfg"}, []string{"-config=/tmp/custom.cfg", "./..."}, "", false},
		{"tags only", []string{"-tags", "special,other"}, []string{"-tags", "special,other", "./..."}, "special,other", false},
		{"space separated tags", []string{"-tags=a b"}, []string{"-tags=a b", "./..."}, "a,b", false},
		{"package", []string{"-tags=special", "./..."}, []string{"-tags=special", "./..."}, "special", false},
		{"driver value flag", []string{"-debug", "f"}, []string{"-debug", "f", "./..."}, "", false},
		{"end of flags", []string{"--", "./..."}, []string{"--", "./..."}, "", false},
		{"file rejected", []string{"-config=foo.yaml", "code.go"}, nil, "", true},
		{"missing config value", []string{"-config"}, nil, "", true},
		{"missing tags value", []string{"-tags"}, nil, "", true},
		{"empty tags", []string{"-tags="}, nil, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, tags, err := prepareArgs(tc.args, &govo.Analyzer.Flags)
			if (err != nil) != tc.err || !reflect.DeepEqual(got, tc.want) || tags != tc.tags {
				t.Fatalf("prepareArgs(%q) = %q, %q, %v; want %q, %q, error=%v", tc.args, got, tags, err, tc.want, tc.tags, tc.err)
			}
		})
	}
}

func TestPrepareArgsHelp(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"help", "config"}, {"-config=x.yaml", "help"}} {
		if _, _, err := prepareArgs(args, &govo.Analyzer.Flags); !errors.Is(err, errHelpArg) {
			t.Errorf("prepareArgs(%q) error = %v, want errHelpArg", args, err)
		}
	}

	// After --, every operand is a package pattern.
	if _, _, err := prepareArgs([]string{"--", "./help"}, &govo.Analyzer.Flags); err != nil {
		t.Errorf("prepareArgs(./help) error = %v", err)
	}
}

func TestVetInvocation(t *testing.T) {
	for _, args := range [][]string{{"-flags"}, {"-V=full"}, {"-config=x", "/tmp/go-build/vet.cfg"}} {
		got, tags, err := prepareArgs(args, &govo.Analyzer.Flags)
		if err != nil || tags != "" || !reflect.DeepEqual(got, args) {
			t.Errorf("prepareArgs(%q) = %q, %q, %v", args, got, tags, err)
		}
	}

	if argsFromVet([]string{"-config", "/tmp/custom.cfg"}) {
		t.Fatal("explicit config mistaken for vet protocol")
	}
}

func TestAnalyzerFlagKinds(t *testing.T) {
	var flags flag.FlagSet
	flags.Bool("enabled", false, "")
	flags.String("path", "", "")

	if flagArgTakesValue("enabled", &flags) || !flagArgTakesValue("path", &flags) {
		t.Fatal("analyzer flag kind not recognized")
	}
}
