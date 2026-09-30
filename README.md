# govo

govo is a linter that detects code that bypasses the construction and conversion APIs of Go Value Objects. It checks only types marked with `//govo:protect`, so you can adopt it one type at a time in an existing codebase.

## The problem

In Go, even if you define a custom type such as `type Code string`, callers can construct it directly with `Code("...")`. Untyped constants, such as string literals, are also implicitly converted to that type in arguments and assignments. Callers can bypass validation implemented in a constructor, so defining a type alone does not enforce domain rules.

govo treats the functions marked with `//govo:factory` and `//govo:converter` in the file that declares the type as a trust boundary. Everywhere else, including unmarked functions in the same file and other files in the same package, it detects direct construction and conversion to the internal representation, encouraging callers to use the marked APIs.

## Example

In the file that declares the type, implement a constructor that validates input and a method that returns the internal representation, and mark them.

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

_ = Code("ABC")           // GOVO001: direct construction; use a //govo:factory function
var bypass Code = "ABC"   // GOVO001: implicit construction from an untyped constant
_ = string(code)          // GOVO002: direct extraction; use a //govo:converter function
_ = code == "ABC"         // GOVO003: comparison with an untyped constant
```

Direct construction is allowed only in functions marked with `factory`, and direct conversion to the internal representation only in functions marked with `converter`. Implicit construction from untyped constants, as well as comparisons and operations involving them, is detected even in marked functions. Declarations of constants with the protected type, such as `const Admin Code = "ADMIN"`, are allowed anywhere in the type's declaration file.

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

### Flags

| Flag | Default | Effect |
| --- | --- | --- |
| `-config` | *(`.govo.yaml` in the current directory)* | Path to a YAML config file. Skips the [`.govo.yaml` lookup](#configuration). A missing file is an error |
| `-tags` | None | Build tags used to select files when loading packages. Commas or spaces separate tags |
| `-test` | `true` | Load test packages as well. To load them but skip reporting in `_test.go` files, set [`tests: false`](#configuration) instead |
| `-json` | `false` | Print diagnostics as JSON |
| `-c` | `-1` | Print `N` lines of source context around each diagnostic |
| `-V=full` | | Print the version and exit |

`-test`, `-json`, `-c` and `-V` come from `go/analysis`, which also accepts its debugging and profiling flags (`-debug`, `-cpuprofile`, `-memprofile`, and `-trace`). `govo help` or `govo -help` prints this usage.

The standalone command exits with status `0` when there are no diagnostics, `3` when it reports diagnostics, `2` for an invalid command line, and `1` when packages cannot be loaded or analysis fails. Any nonzero status should fail a CI check.

### Running with go vet

```sh
go vet -vettool="$(command -v govo)" ./...
go vet -vettool="$(command -v govo)" -tags=integration -config="$PWD/.govo.yaml" ./...
```

`go vet` handles package patterns and `-tags` itself and exits with status `1` when govo reports diagnostics. `go vet -help` prints go vet's own usage, which refers to `govo help` for govo's flags. It starts govo once per package in that package's directory, so `.govo.yaml` and a relative `-config` path are resolved there. To apply one configuration to every package, pass an absolute path to `-config`.

## Configuration

Every setting is optional. The YAML block shows every available key with its default value.

```yaml
tests: true               # true | false

ignore:
  missing-reason: off     # off | error
```

| Key | Values | Default | Effect |
| --- | --- | --- | --- |
| `tests` | `true`, `false` | `true` | Report diagnostics in `_test.go` files |
| `ignore.missing-reason` | `off`, `error` | `off` | With `error`, an [`//govo:ignore`](#ignoring-a-diagnostic) without a `// reason` is reported as `GOVO004` |

- The file is `.govo.yaml` in the tool's current working directory. Parent directories are not searched, and when the file does not exist the defaults apply.
- [`-config`](#flags) names one file, reads no other, and takes precedence over `.govo.yaml`.
- Unknown keys are ignored, and an invalid value for a known key is an error.
- govo has no warning level: every diagnostic fails the command.

## Directives

Every directive is a `//govo:name` line comment, written without a space after `//`.

| Directive | Attaches to | Effect |
| --- | --- | --- |
| `//govo:protect` | The type declaration immediately after it | Protects that type. Takes no argument |
| `//govo:factory [types]` | The function or method immediately after it, in the type's declaration file | Allows it to construct the named protected types directly |
| `//govo:converter [types]` | The function or method immediately after it, in the type's declaration file | Allows it to extract the underlying representation of the named protected types directly |
| `//govo:ignore [rules] [// reason]` | The statement or declaration after it, or its own line | [Suppresses diagnostics](#ignoring-a-diagnostic) for the listed rules, or for every rule when none are listed |

### Protecting a type

`protect` applies to one type declaration, not to a whole `type (...)` group. Inside a group, each type to protect needs its own directive.

```go
type (
    //govo:protect
    Code string

    //govo:protect
    RegionCode string

    Label string // Not protected
)
```

### Markers

`factory` and `converter` are the only places where direct conversions are allowed. A `factory` may construct the types it names, including with non-empty composite literals and implicit conversions from unnamed array, slice, or map values, and a `converter` may extract their underlying representation. Each marker grants only its own direction, so a method such as `func (c Code) Upper() Code` that does both needs both markers. A conversion between two protected types needs a `factory` of the destination and a `converter` of the source. Function literals inside a marked function share its permission.

Markers can be attached to exported and unexported functions and methods, so internal helpers can be marked too. They also check the shape of the function. Each named type is checked separately, and a type that fails its check is reported as `GOVO004` and dropped from the marker, which then grants nothing for that type.

| Marker | A named type passes when |
| --- | --- |
| `factory` | One of the results has exactly that type. `(Code, error)` passes; `*Code` and `[]Code` do not |
| `converter` | The receiver or an argument has that type or a pointer to it. The results are unrestricted |

- The type names may be omitted when the file declares a single protected type. With more than one, name them.
- List several types on one line, separated by spaces, or repeat the directive. Both forms name the same set.
- A marker on a declaration in another file than the type is reported as `GOVO004` and ignored.

```go
//govo:factory Code RegionCode
func NewCodes(s string) (Code, RegionCode, error) { /* ... */ }

//govo:converter Code
//govo:converter RegionCode
func DescribeCodes(c Code, r RegionCode) string { /* ... */ }
```

### Ignoring a diagnostic

Place `//govo:ignore` on its own line immediately before a statement or declaration, or at the end of a line. Rule IDs are comma-separated, and the reason follows in the same `// reason` form as golangci-lint.

```go
//govo:ignore GOVO001 // Required for compatibility with an external specification
UseCode("ABC")

_ = code == "ABC" //govo:ignore GOVO003 // Temporary migration exception
```

| Placement | Covers |
| --- | --- |
| At the end of a line | Every diagnostic caused on that physical line |
| On its own line | The following statement or declaration, even when it spans several lines |
| On its own line before `if`, `for`, `switch`, or `select` | The header only, such as the condition and initializer, not the block |
| On its own line before `case` or `default` | The clause's expressions or communication only, not the statements after the colon |

A comment on its own line never reaches into the body of a function literal, as in `defer func() { ... }()`, and there is no block-wide suppression. A reason is optional unless [`ignore.missing-reason`](#configuration) is `error`.

<details>
<summary>How the target of an ignore is found</summary>

- The following statement or declaration is searched for only in the innermost declaration list, `const`/`var`/`type` group, block, or clause that contains the comment.
- A comment at the end of a block, or inside an expression such as between the arguments of a multiline call, has no target.
- A label does not change the target: a comment before `L: for ...` covers the header of that `for`.
- In a multiline call, a trailing comment on the closing parenthesis line does not cover arguments on other lines.

</details>

### Invalid and unused directives

These are reported as `GOVO004`. An invalid directive is ignored, but other valid directives at the same location still apply.

| Directive | Report |
| --- | --- |
| `//govo:foo` | `unknown govo directive "foo"` |
| `//govo:protect Code` | `protect does not accept arguments` |
| `//govo:protect` not above a type declaration | `protect is not attached to a type declaration` |
| `//govo:protect` above a type alias | `protect requires a defined type` |
| `//govo:protect` above `type Point struct{ X, Y int }` | `protect requires a basic, array, slice, or map underlying type` |
| `//govo:factory` not above a function or method | `factory requires a function or method` |
| `//govo:factory` in a file with several protected types | `factory requires a type name unless this file declares exactly one protected type` |
| `//govo:factory Other` where `Other` is not protected in this file | `factory: Other is not a protected type declared in this file` |
| `//govo:factory Code` on a function that does not return `Code` | `factory: Code is not returned by this API` |
| `//govo:converter Code` on a function that does not take `Code` | `converter: Code is not accepted by this API` |
| `//govo:ignore GOVO009` | `unknown ignore rule "GOVO009"` |
| `//govo:ignore` with nothing to attach to | `ignore is not followed by a statement or declaration` |
| `//govo:ignore` that suppresses nothing | `unused ignore directive` |
| `//govo:ignore GOVO001,GOVO003` where `GOVO003` suppresses nothing | `unused ignore rule GOVO003` |
| `//govo:ignore` without a reason, with `ignore.missing-reason: error` | `ignore directive has no reason` |

Each report is prefixed with `GOVO004: `. `converter` gives the same messages as `factory` for the cases they share.

## Diagnostics

| Rule | Reports |
| ---- | ------- |
| `GOVO001` | Construction of a protected type outside its `factory` functions, including non-empty composite literals and implicit conversions from unnamed array, slice, or map values; constants of a protected type declared outside its declaration file; and implicit construction from untyped constants or untyped expressions such as comparisons anywhere |
| `GOVO002` | Conversion from a protected type to another concrete type outside its `converter` functions, including implicit conversions of a protected array, slice, or map to an unnamed type |
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
