// Command govo checks protected Go value object types.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"golang.org/x/tools/go/analysis/singlechecker"

	"github.com/aqyuki/govo"
)

func main() {
	// singlechecker assigns flag.Usage, but flag.Parse reports -help and
	// invalid flags through the command line's own Usage field.
	flag.CommandLine.Usage = func() { printUsage(flag.CommandLine.Output()) }

	args, tags, err := prepareArgs(os.Args[1:], &govo.Analyzer.Flags)
	if errors.Is(err, errHelp) {
		printUsage(os.Stdout)
		os.Exit(0)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "govo:", err)
		os.Exit(2)
	}

	if tags != "" {
		// singlechecker accepts -tags but does not apply it to go/packages.
		// go/packages invokes go list, which reads GOFLAGS.
		goFlags := strings.TrimSpace(os.Getenv("GOFLAGS"))
		if goFlags != "" {
			goFlags += " "
		}

		if err := os.Setenv("GOFLAGS", goFlags+"-tags="+tags); err != nil {
			fmt.Fprintf(os.Stderr, "govo: set GOFLAGS: %v\n", err)
			os.Exit(2)
		}
	}

	os.Args = append(os.Args[:1], args...)

	singlechecker.Main(govo.Analyzer)
}
