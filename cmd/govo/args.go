package main

import (
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"strings"
)

// errHelp reports a "govo help" invocation. go vet's usage refers users to it.
var errHelp = errors.New("help requested")

// The singlechecker driver owns parsing. This adapter only locates package
// operands, extracts -tags for package loading, and applies govo's CLI policy.
func prepareArgs(args []string, analyzerFlags *flag.FlagSet) ([]string, string, error) {
	if vetInvocation(args) {
		return args, "", nil
	}

	packageStart := len(args)

	var tags string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			packageStart = i + 1
			break
		}

		if !strings.HasPrefix(arg, "-") || arg == "-" {
			packageStart = i
			break
		}

		name, value, inline := splitFlag(arg)
		if !flagTakesValue(name, analyzerFlags) {
			continue
		}

		if !inline {
			if i+1 >= len(args) {
				return nil, "", fmt.Errorf("flag -%s requires a value", name)
			}

			i++
			value = args[i]
		}

		if name == "tags" {
			tags = strings.Join(strings.FieldsFunc(value, func(r rune) bool {
				return r == ',' || r == ' ' || r == '\t'
			}), ",")
			if tags == "" || strings.HasPrefix(tags, "-") {
				return nil, "", fmt.Errorf("-tags requires at least one build tag")
			}
		}
	}

	packages := args[packageStart:]
	if len(packages) > 0 && packages[0] == "help" {
		return nil, "", errHelp
	}

	for _, pkg := range packages {
		if strings.HasSuffix(pkg, ".go") {
			return nil, "", fmt.Errorf(".go file arguments are not supported; specify a package pattern")
		}
	}

	if len(packages) == 0 {
		args = append(append([]string(nil), args...), "./...")
	}

	return args, tags, nil
}

func vetInvocation(args []string) bool {
	if len(args) == 1 {
		arg := args[0]
		if arg == "-flags" || arg == "--flags" || strings.HasPrefix(arg, "-V=") || strings.HasPrefix(arg, "--V=") {
			return true
		}
	}
	// go vet passes the absolute path of its generated vet.cfg as the last
	// operand. A user's -config path ending in .cfg is not that protocol.
	return len(args) > 0 && filepath.IsAbs(args[len(args)-1]) && filepath.Base(args[len(args)-1]) == "vet.cfg"
}

func splitFlag(arg string) (name, value string, inline bool) {
	name = strings.TrimLeft(arg, "-")
	if before, after, ok := strings.Cut(name, "="); ok {
		return before, after, true
	}

	return name, "", false
}

func flagTakesValue(name string, analyzerFlags *flag.FlagSet) bool {
	if f := analyzerFlags.Lookup(name); f != nil {
		boolFlag, ok := f.Value.(interface{ IsBoolFlag() bool })
		return !ok || !boolFlag.IsBoolFlag()
	}
	// These are value flags of the singlechecker driver. It will still
	// validate their values; the adapter only skips the following operand.
	switch name {
	case "tags", "c", "debug", "cpuprofile", "memprofile", "trace":
		return true
	default:
		return false
	}
}
