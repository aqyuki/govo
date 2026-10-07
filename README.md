# govo

govo is a linter that detects code that bypasses the construction and conversion APIs of Go Value Objects. It checks only types marked with `//govo:protect`, so you can adopt it one type at a time in an existing codebase.

## The problem

In Go, even if you define a custom type such as `type Code string`, callers can construct it directly with `Code("...")`. Untyped constants, such as string literals, are also implicitly converted to that type in arguments and assignments. Callers can bypass validation implemented in a constructor, so defining a type alone does not enforce domain rules.

govo treats the functions marked with `//govo:factory`, `//govo:converter`, and `//govo:op` in the file that declares the type as a trust boundary. Everywhere else, including unmarked functions in the same file and other files in the same package, it detects direct construction and conversion to the internal representation, encouraging callers to use the marked APIs.

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

_ = Code("ABC")           // GOV001: direct construction; use a //govo:factory function
var bypass Code = "ABC"   // GOV001: implicit construction from an untyped constant
_ = string(code)          // GOV002: direct extraction; use a //govo:converter function
_ = code == "ABC"         // GOV003: comparison with an untyped constant
```

Direct construction is allowed only in functions marked with `factory` or `op`, and direct conversion to the internal representation only in functions marked with `converter` or `op`. Implicit construction from untyped constants, as well as comparisons and operations involving them, is detected even in marked functions, except for [scaling by a constant](#scaling-by-untyped-constants) in a function marked with `scalar`. Declarations of constants with the protected type, such as `const Admin Code = "ADMIN"`, are allowed anywhere in the type's declaration file.

## Installation and usage

Choose one of the following installation methods:

| Method | Command                                             | Requirements                             |
| ------ | --------------------------------------------------- | ---------------------------------------- |
| Go     | `go install github.com/aqyuki/govo/cmd/govo@latest` | Go 1.27.0 or later                       |
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
| `-json` | `false` | Print diagnostics as JSON, with the [rule ID](#diagnostics) as each diagnostic's `category` |
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
| `ignore.missing-reason` | `off`, `error` | `off` | With `error`, an [`//govo:ignore`](#ignoring-a-diagnostic) without a `// reason` is reported as `GOVD003` |

- The file is `.govo.yaml` in the tool's current working directory. Parent directories are not searched, and when the file does not exist the defaults apply.
- [`-config`](#flags) names one file, reads no other, and takes precedence over `.govo.yaml`.
- An unknown key, such as a misspelled `test` for `tests`, and an invalid value for a known key are errors.
- govo has no warning level: every diagnostic fails the command.

## Directives

Every directive is a `//govo:name` line comment, written without a space after `//`.

| Directive | Attaches to | Effect |
| --- | --- | --- |
| `//govo:protect` | The type declaration immediately after it | Protects that type. Takes no argument |
| `//govo:factory [types]` | The function or method immediately after it, in the type's declaration file | Allows it to construct the named protected types directly |
| `//govo:converter [types]` | The function or method immediately after it, in the type's declaration file | Allows it to extract the underlying representation of the named protected types directly |
| `//govo:op [types]` | The function or method immediately after it, in the type's declaration file | Allows it to both construct and extract the named protected types directly |
| `//govo:scalar [types]` | A `factory` or `op` of the named types | Allows it to [multiply and divide](#scaling-by-untyped-constants) the named numeric types by untyped constants |
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

`factory`, `converter`, and `op` are the only places where direct conversions are allowed. A `factory` may construct the types it names, including with non-empty composite literals and implicit conversions from unnamed array, slice, or map values, and a `converter` may extract their underlying representation. Each of them grants only its own direction. An `op` is an operation inside the trust boundary that derives a value of a type from another, such as `func (c Code) Upper() Code`, and may do both. A conversion between two protected types needs a `factory` or `op` of the destination and a `converter` or `op` of the source. A pointer conversion that shares a protected value's storage under another type, such as `(*string)(&code)` or `(*ID)(raw)` from a slice, allows both reads and writes, so it needs both directions for each protected type involved. Function literals inside a marked function share its permission.

```go
//govo:op
func (c Code) Upper() Code {
    return Code(strings.ToUpper(string(c)))
}
```

Markers can be attached to exported and unexported functions and methods, so internal helpers can be marked too. They also check the shape of the function. Each named type is checked separately, and a type that fails its check is reported as `GOVD001` and dropped from the marker, which then grants nothing for that type.

| Marker | A named type passes when |
| --- | --- |
| `factory` | One of the results has exactly that type, or for a generic type any instance of it, such as `List[T]` or `List[int]`. `(Code, error)` passes; `*Code` and `[]Code` do not |
| `converter` | The receiver or an argument has that type or a pointer to it, or for a generic type an instance of it. The results are unrestricted |
| `op` | Both of the above: it takes that type or a pointer to it, and one of the results has exactly that type |

- The type names may be omitted when the file declares a single protected type. With more than one, name them.
- List several types on one line, separated by spaces, or repeat the directive. Both forms name the same set.
- A marker on a declaration in another file than the type is reported as `GOVD001` and ignored.
- Name each type once per function and command. A repeated type, or a `factory` or `converter` of a type that an `op` of the same function already names, is reported as `GOVD004`.

```go
//govo:factory Code RegionCode
func NewCodes(s string) (Code, RegionCode, error) { /* ... */ }

//govo:converter Code
//govo:converter RegionCode
func DescribeCodes(c Code, r RegionCode) string { /* ... */ }
```

### Scaling by untyped constants

In `amount * 3`, Go gives the constant `3` the type `Amount`, but it means a dimensionless factor, not an amount, and writing `amount * Amount(3)` would claim otherwise. `//govo:scalar` lets a `factory` or `op` of a protected numeric type multiply and divide it by untyped constants as they are.

```go
//govo:protect
type Amount uint

//govo:op
//govo:scalar
func (a Amount) Half() Amount { return a / 2 }

//govo:factory
//govo:scalar
func AmountFromYen(yen uint) Amount { return Amount(yen) * 100 }
```

- A constant is a scalar as either operand of `*` and as the divisor of `/`, including `*=` and `/=`. `a + 1`, `a > 0`, `a % 3`, `100 / a`, and `a++` still treat the constant as an amount and are reported as `GOV003`.
- Only untyped constants, including named ones, are scalars. A variable factor still needs a conversion, as in `a * Amount(n)`.
- Elsewhere, including callers in other files, scaling by an untyped constant is reported as `GOV003`. Call a method such as `Half` instead.
- The directive can appear before or after the marker, and it names types in the same way as the markers.

### Ignoring a diagnostic

Place `//govo:ignore` on its own line immediately before a statement or declaration, or at the end of a line. Rule IDs are comma-separated, and the reason follows in the same `// reason` form as golangci-lint.

```go
//govo:ignore GOV001 // Required for compatibility with an external specification
UseCode("ABC")

_ = code == "ABC" //govo:ignore GOV003 // Temporary migration exception
```

| Placement | Covers |
| --- | --- |
| At the end of a line | Every diagnostic caused on that physical line |
| On its own line | The following statement or declaration, even when it spans several lines, and the directives between them, such as the rest of a doc comment |
| On its own line before `if`, `for`, `switch`, or `select` | The header only, such as the condition and initializer, not the block |
| On its own line before `case` or `default` | The clause's expressions or communication only, not the statements after the colon |

Write one `//govo:ignore` per target. A second one for the same target, a rule ID listed twice, or an `//govo:ignore` without rule IDs beside one that lists them is reported as `GOVD004`; the listed rule IDs are kept. This report cannot be suppressed.

A comment on its own line never reaches into the body of a function literal, as in `defer func() { ... }()`, and there is no block-wide suppression. A reason is optional unless [`ignore.missing-reason`](#configuration) is `error`.

<details>
<summary>How the target of an ignore is found</summary>

- The following statement or declaration is searched for only in the innermost declaration list, `const`/`var`/`type` group, block, or clause that contains the comment.
- A comment at the end of a block, or inside an expression such as between the arguments of a multiline call, has no target.
- A label does not change the target: a comment before `L: for ...` covers the header of that `for`.
- In a multiline call, a trailing comment on the closing parenthesis line does not cover arguments on other lines.

</details>

### Invalid, unused, and redundant directives

An invalid directive is ignored, but other valid directives at the same location still apply.

| Directive | Report |
| --- | --- |
| `//govo:foo` | `GOVD001: unknown govo directive "foo"` |
| `//govo:protect Code` | `GOVD001: protect does not accept arguments` |
| `//govo:protect` not above a type declaration | `GOVD001: protect is not attached to a type declaration` |
| `//govo:protect` above a type alias | `GOVD001: protect requires a defined type` |
| `//govo:protect` above `type Point struct{ X, Y int }` | `GOVD001: protect requires a basic, array, slice, or map underlying type` |
| `//govo:factory` not above a function or method | `GOVD001: factory requires a function or method` |
| `//govo:factory` in a file with several protected types | `GOVD001: factory requires a type name unless this file declares exactly one protected type` |
| `//govo:factory Other` where `Other` is not protected in this file | `GOVD001: factory: Other is not a protected type declared in this file` |
| `//govo:factory Code` on a function that does not return `Code` | `GOVD001: factory: Code is not returned by this API` |
| `//govo:converter Code` on a function that does not take `Code` | `GOVD001: converter: Code is not accepted by this API` |
| `//govo:scalar Code` where `Code` is not numeric | `GOVD001: scalar: Code does not have a numeric underlying type` |
| `//govo:scalar Amount` on a function that is not a `factory` or `op` of `Amount` | `GOVD001: scalar: this API is not a factory or op of Amount` |
| `//govo:factory Code` twice on one function | `GOVD004: factory: Code is named more than once for this function` |
| `//govo:factory Code` with `//govo:op Code` on one function | `GOVD004: factory: Code is already permitted by op Code` |
| `//govo:ignore GOV001` twice before one statement | `GOVD004: redundant ignore rule GOV001; another ignore directive already suppresses it here` |
| `//govo:ignore GOV001,GOV001` | `GOVD004: redundant ignore rule GOV001; it is listed more than once` |
| `//govo:ignore` and `//govo:ignore GOV001` before one statement | `GOVD004: redundant ignore directive; another ignore directive lists the rules to suppress here` |
| `//govo:ignore GOV009` | `GOVD001: unknown ignore rule "GOV009"` |
| `//govo:ignore GOVD002` | `GOVD001: ignore rule GOVD002 cannot be suppressed` |
| `//govo:ignore` with nothing to attach to | `GOVD001: ignore is not followed by a statement or declaration` |
| `//govo:ignore` that suppresses nothing | `GOVD002: unused ignore directive` |
| `//govo:ignore GOV001,GOV003` where `GOV003` suppresses nothing | `GOVD002: unused ignore rule GOV003` |
| `//govo:ignore` without a reason, with `ignore.missing-reason: error` | `GOVD003: ignore directive has no reason` |

`converter`, `op`, and `scalar` give the same messages as `factory` for the cases they share, and `op` reports both shape messages.

## Diagnostics

Rule IDs are grouped by category, and each category is numbered independently. `GOV` rules report operations on protected types, and `GOVD` rules report problems with the directives themselves. Both kinds can be listed in [`//govo:ignore`](#ignoring-a-diagnostic), except `GOVD002`.

Each diagnostic message starts with its rule ID, as in `GOV001: direct construction of protected type Code; use a //govo:factory function`. The rule ID is also set as the diagnostic's category, so `-json` output includes it as `"category"` and tools that run analyzers can identify and filter diagnostics by rule without parsing the message.

| Rule | Reports |
| ---- | ------- |
| `GOV001` | Construction of a protected type outside its `factory` functions, including pointer conversions such as `(*string)(&code)`, non-empty composite literals and implicit conversions from unnamed array, slice, or map values; constants of a protected type declared outside its declaration file; and implicit construction from untyped constants or untyped expressions such as comparisons anywhere |
| `GOV002` | Conversion from a protected type to another concrete type outside its `converter` functions, including pointer conversions in a `factory` that is not also a `converter`, and implicit conversions of a protected array, slice, or map to an unnamed type |
| `GOV003` | Comparisons and operations with untyped constants or untyped expressions, including `x++` and `x--`, and scaling by an untyped constant outside a `scalar` function |
| `GOVD001` | Unknown, invalid, or unattached directives |
| `GOVD002` | `ignore` directives or listed rules that suppress nothing |
| `GOVD003` | `ignore` directives without a reason, when [`ignore.missing-reason`](#configuration) is `error` |
| `GOVD004` | `factory`, `converter`, `op`, or `scalar` directives that repeat a permission the function already has, and `ignore` directives that repeat another for the same target |

## Scope and limitations

Protected types must be defined types with a boolean, numeric, string, array, slice, or map underlying type. Generic types such as `type List[T any] []T` are allowed: every instance is protected, markers name the generic type (`//govo:factory List`), and a conversion between distinct instances such as `ID[User](orderID)` crosses the type boundary. Protected types declared in other packages within the same module, including unexported ones whose values are exposed through exported APIs, are also checked.

- Zero-value construction (`var code Code` or `new(Code)`) is allowed, as are `make`, `nil`, and empty composite literals such as `Codes{}`.
- Element operations on a protected array, slice, or map, such as indexing, `range`, slicing, `len`, `append`, `copy`, and `delete`, are not reported. Use a struct with unexported fields when elements must not be read or modified.
- Comparisons and operations between protected values are not reported.
- Construction or conversion through type parameters or `unsafe.Pointer`, and writes through JSON, databases, reflection, or similar mechanisms, are outside the scope of analysis.
- Generated Go files (as recognized by `go/ast.IsGenerated`), vendored dependencies, external modules, and the standard library are excluded from analysis.

govo detects API bypasses, but does not guarantee that validation logic is correct or that every value has passed through a constructor. For details on directives, diagnostic rules, and the scope of exceptions, see the [detection specification](docs/spec.md).

## Development checks

After installing the development tools with `mise install`, use the following commands.

```sh
go test ./...
mise exec -- golangci-lint run ./...
mise exec -- golangci-lint fmt
mise exec -- declscope shrink ./...
mise exec -- declscope ./...
```

`golangci-lint run` checks the standard linters plus `modernize` and `wsl_v5`, and also reports formatting differences from gofmt, goimports, gofumpt, and gci. `golangci-lint fmt` applies those formatters. `go test ./...` includes integration tests that build govo and run it both standalone and through `go vet -vettool`; use `go test -short ./...` to skip them. [declscope](https://github.com/mpyw/declscope) checks that each declaration is used only where its scope allows, with each file as a namespace, and that names carry their file's namespace. `.declscope.yaml` configures it. Run `declscope shrink` before `declscope`. CI runs the tests, `go vet ./...`, `golangci-lint run`, and both declscope commands on pull requests using only Go 1.27, as specified in `go.mod`.

## License

This project is licensed under the Apache License 2.0. See [LICENSE](./LICENSE) for details.
