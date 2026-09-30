# govo

govo is a linter that detects code that bypasses the construction and conversion APIs of Go Value Objects. It checks only types marked with `//govo:protect`, so you can adopt it one type at a time in an existing codebase.

## The problem

In Go, even if you define a custom type such as `type Code string`, callers can construct it directly with `Code("...")`. Untyped constants, such as string literals, are also implicitly converted to that type in arguments and assignments. Callers can bypass validation implemented in a constructor, so defining a type alone does not enforce domain rules.

govo treats the Go file containing the type declaration and its domain rules as a trust boundary. In other files, even within the same package, it detects direct construction and conversion to the internal representation, encouraging callers to use the public API.

## Example

In the file that declares the type, implement a constructor that validates input and a method that returns the internal representation.

```go
// code.go
package domain

import "errors"

//govo:protect
type Code string

//govo:factory
func NewCode(s string) (Code, error) {
    if s == "" {
        var zero Code
        return zero, errors.New("empty code")
    }
    return Code(s), nil
}

//govo:converter
func (c Code) String() string {
    return string(c)
}
```

Example usage in another file:

```go
code, err := NewCode("ABC") // Allowed: construct through the validation API
if err != nil {
    return err
}
raw := code.String()       // Allowed: retrieve through the public API

_ = Code("ABC")           // GOVO001: direct construction
var bypass Code = "ABC"   // GOVO001: implicit construction from an untyped constant
_ = string(code)          // GOVO002: direct conversion to the internal representation
_ = code == "ABC"         // GOVO003: comparison with an untyped constant
```

`factory` and `converter` are optional markers for public APIs. Direct conversions are allowed throughout the file that declares the type, not just in functions with these markers. Implicit construction from untyped constants, as well as comparisons and operations involving them, is detected even in that file. However, declarations of constants with the protected type are allowed in that file.

## Installation and usage

Choose one of the following installation methods:

| Method | Command                                             | Requirements                             |
| ------ | --------------------------------------------------- | ---------------------------------------- |
| Go     | `go install github.com/aqyuki/govo/cmd/govo@latest` | Go 1.26.0 or later                       |
| mise   | `mise use -g github:aqyuki/govo@latest`             | mise; a release binary for your platform |

The mise command installs a release binary and adds govo to your global mise configuration. With mise activated in your shell, run:

```sh
govo ./...
```

## CLI usage

```text
govo [flags] [packages]
go vet -vettool=/absolute/path/to/govo [go vet flags] [govo flags] [packages]
```

Packages are specified with the usual Go package patterns, such as `./...`, `./internal/domain`, or `example.com/app/...`. With no package arguments, govo checks `./...`. Individual `.go` files are not accepted; specify the package that contains them instead.

| Flag | Description |
| ---- | ----------- |
| `-config=path` | Read configuration from `path` instead of `.govo.yaml` in the current directory. A missing file is an error. |
| `-tags=a,b` | Build tags used to select files when loading packages. Commas or spaces separate tags. |
| `-test=false` | Do not load test packages. To load them but skip reporting in `_test.go` files, use `tests: false` in the configuration instead. |
| `-json` | Print diagnostics as JSON. |
| `-c=N` | Print `N` lines of source context around each diagnostic. |

Run `govo help` or `govo -help` to print this usage. The debugging and profiling flags of the underlying `golang.org/x/tools/go/analysis` driver (`-debug`, `-cpuprofile`, `-memprofile`, and `-trace`) are also accepted.

The standalone command exits with status `0` when there are no diagnostics, `3` when it reports diagnostics, `2` for an invalid command line, and `1` when packages cannot be loaded or analysis fails. Any nonzero status should fail a CI check.

### Running with go vet

```sh
go vet -vettool="$(command -v govo)" ./...
go vet -vettool="$(command -v govo)" -tags=integration -config="$PWD/.govo.yaml" ./...
```

`go vet` handles package patterns and `-tags` itself and exits with status `1` when govo reports diagnostics. `go vet -help` prints go vet's own usage, which refers to `govo help` for govo's flags. It starts govo once per package in that package's directory, so `.govo.yaml` and a relative `-config` path are resolved there. To apply one configuration to every package, pass an absolute path to `-config`.

## Configuration and exceptions

The configuration file specified by `-config` takes precedence. If omitted, govo reads `.govo.yaml` from the tool's current working directory; parent directories are not searched. If that default file does not exist, it uses the default configuration.

```yaml
tests: true
ignore:
  missing-reason: off
```

| Key | Values | Default | Description |
| --- | ------ | ------- | ----------- |
| `tests` | `true`, `false` | `true` | Report diagnostics in `_test.go` files |
| `ignore.missing-reason` | `off`, `error` | `off` | With `error`, an `//govo:ignore` without a `// reason` is reported as `GOVO004` |

Unknown keys are ignored, and invalid values for known keys are errors. govo has no warning level: every diagnostic fails the command.

To allow a necessary exception, place `//govo:ignore` on its own line immediately before the relevant statement or declaration, or at the end of the same line.

```go
//govo:ignore GOVO001 // Required for compatibility with an external specification
UseCode("ABC")

_ = code == "ABC" //govo:ignore GOVO003 // Temporary migration exception
```

Omitting the rule ID applies the exception to all rules. A comment on its own line covers only the header of an `if`, `for`, `switch`, or `select` and only the expressions of a `case` clause, not the statements in their bodies or in the bodies of function literals. Unused exception comments, exception comments not followed by a statement or declaration, and invalid directives are reported as `GOVO004`.

## Diagnostics

| Rule | Reports |
| ---- | ------- |
| `GOVO001` | Construction of a protected type outside its declaration file, including non-empty composite literals and implicit conversions from unnamed array, slice, or map values, and implicit construction from untyped constants or untyped expressions such as comparisons in any file |
| `GOVO002` | Conversion from a protected type to another concrete type outside its declaration file, including implicit conversions of a protected array, slice, or map to an unnamed type |
| `GOVO003` | Comparisons and operations with untyped constants or untyped expressions, including `x++` and `x--` |
| `GOVO004` | Invalid, unattached, or unused directives, and missing `ignore` reasons when required |

## Scope and limitations

Protected types must be defined types with a boolean, numeric, string, array, slice, or map underlying type. Protected types declared in other packages within the same module, including unexported ones whose values are exposed through exported APIs, are also checked.

- Zero-value construction (`var code Code` or `new(Code)`) is allowed, as are `make`, `nil`, and empty composite literals such as `Codes{}`.
- Element operations on a protected array, slice, or map, such as indexing, `range`, slicing, `len`, `append`, `copy`, and `delete`, are not reported. Use a struct with unexported fields when elements must not be read or modified.
- Comparisons and operations between protected values are not reported.
- Construction or conversion through type parameters, and writes through JSON, databases, reflection, or similar mechanisms, are outside the scope of analysis.
- Generated Go files (as recognized by `go/ast.IsGenerated`), vendored dependencies, external modules, and the standard library are excluded from analysis.

govo detects API bypasses, but does not guarantee that validation logic is correct or that every value has passed through a constructor. For details on directives, diagnostic rules, and the scope of exceptions, see the [detection specification](docs/spec.md).

## Development checks

After installing the development tools with `mise install`, use the following commands.

```sh
go test ./...
mise exec -- golangci-lint run ./...
mise exec -- golangci-lint fmt
```

`golangci-lint run` checks the standard linters plus `modernize` and `wsl_v5`, and also reports formatting differences from gofmt, goimports, gofumpt, and gci. `golangci-lint fmt` applies those formatters. `go test ./...` includes integration tests that build govo and run it both standalone and through `go vet -vettool`; use `go test -short ./...` to skip them. CI runs the tests and `go vet ./...` with both Go 1.26 and Go 1.27, and `golangci-lint run`, on pull requests.

## License

This project is licensed under the Apache License 2.0. See [LICENSE](./LICENSE) for details.
