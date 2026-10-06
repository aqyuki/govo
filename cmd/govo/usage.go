package main

import (
	"fmt"
	"io"
)

// usage replaces the help of the singlechecker driver, which describes -tags
// as having no effect and lists flags that do nothing for govo. go vet never
// shows it: it prints its own usage and refers users to this command.
const usage = `govo checks Go value objects marked with //govo:protect.

It reports direct construction, extraction of the underlying representation,
and operations with untyped constants that bypass a protected type's API.

Usage:
  govo [flags] [packages]
  go vet -vettool=/absolute/path/to/govo [go vet flags] [govo flags] [packages]

Packages are Go package patterns such as ./... or ./internal/domain. With no
packages, govo checks ./... . Individual .go files are not accepted.

Flags:
  -config path
        Read configuration from path instead of .govo.yaml in the current
        directory. A missing file is an error. go vet runs govo in each
        package's directory, so pass an absolute path to share one file.
  -tags list
        Build tags, separated by commas or spaces, for loading packages.
        With go vet, go vet's own -tags selects the files instead.
  -test
        Load test packages as well (default true). To load them but not
        report diagnostics in _test.go files, set "tests: false" in the
        configuration instead.
  -json
        Print diagnostics as JSON.
  -c N
        Print N lines of source context around each diagnostic.
  -V
        Print the version and exit.

The analysis driver's debugging and profiling flags (-debug, -cpuprofile,
-memprofile, and -trace) are also accepted.

Exit status:
  0  no diagnostics
  1  packages could not be loaded or analysis failed
  2  invalid command line
  3  diagnostics were reported
go vet exits with status 1 when govo reports diagnostics.

Rules, directives, and configuration: https://github.com/aqyuki/govo
`

// printUsage writes govo's usage to w.
//
//declscope:shared // main.go prints it for -help and "govo help"
func printUsage(w io.Writer) {
	_, _ = fmt.Fprint(w, usage)
}
