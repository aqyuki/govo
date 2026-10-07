# govo Detection Specification

## Purpose

`govo` is a linter for Go value objects. It detects direct construction and direct extraction of the underlying representation outside the functions marked as the type's factories, converters, and operations. It also detects implicit construction from untyped constants regardless of the location. The marked functions, which are declared in the same file as the type, are trusted as the boundary for implementing domain rules: a `factory` may construct the type, a `converter` may extract its underlying representation, and an `op` may do both to derive a value of the type from another. Declarations of typed constants are permitted anywhere in the type declaration file.

```go
//govo:protect
type Code string

//govo:factory
func NewCode(s string) (Code, error) {
    if !validFormat(s) {
        var zero Code
        return zero, errors.New("invalid code")
    }
    return Code(s), nil
}

//govo:converter
func (c Code) String() string {
    return string(c)
}
```

Code outside the marked functions, including other functions in the type declaration file, should use an API such as `NewCode` or `Code.String` to construct a `Code` or extract its underlying representation. `govo` does not prove that the validation logic in the marked functions is correct.

## Eligible types

A **defined type** is eligible for protection if its underlying type is a Go basic type (boolean, numeric, or string), an array, a slice, or a map. It remains eligible when the right-hand side of its declaration names another defined type, provided its underlying type is one of these. Thus, `type Codes []Code`, `type Set map[string]bool`, and the array-based `type UserID uuid.UUID` can be protected. A type alias is not treated as a new protected type; operations through an alias of a protected type are detected as operations on the original protected type.

A generic type is eligible on the same terms, as in `type List[T any] []T` or `type ID[E any] string`, where the type parameter of `ID` only distinguishes IDs of different entities. Every instance of a protected generic type, such as `List[int]` or `List[T]` inside a generic function, is protected as that type, and a marker that names the generic type covers all its instances. Distinct instances are nevertheless distinct types: a conversion between them, as in `ID[User](orderID)`, crosses the type boundary like a conversion between two protected types and needs a `factory` or `op` and a `converter` or `op` of the type. Diagnostics name the instance involved, as in `List[int]` or `ID[User]`.

Types with struct, interface, channel, function, or pointer underlying types are deliberately excluded, and marking one with `//govo:protect` is reported as `GOVD001`. Protecting those types is distinct from applying the rules to **protected types that appear within them or in operations involving them**. For example, the normal `Code` rules apply when assigning a `Code` to a struct field, sending one on a `chan Code`, or converting a value obtained by dereferencing a `*Code`. Even if `Codes` itself is not protected, an operation that makes an element of `Codes{"ABC"}` a `Code` is detected.

## Trusted boundary

The declaration of a protected type and its domain rules belong in one Go file, and the functions in that file marked with `//govo:factory`, `//govo:converter`, and `//govo:op` form the trusted boundary. A function marked as a `factory` of a type permits **explicit conversions to the protected type**, and a function marked as a `converter` of a type permits **explicit conversions from the protected type to another concrete type**. Each of these markers grants only its own direction for only the types it names. A function marked as an `op` of a type is an operation inside the boundary that derives a value of the type from another, such as `func (c Code) Upper() Code`, and permits both directions for the types it names, like a `factory` and a `converter` together. A conversion between two protected types, as in `Domain(email)`, needs a `factory` or `op` of the destination and a `converter` or `op` of the source. Throughout this specification, whatever a `factory` or a `converter` of a type is permitted to do, an `op` of that type is permitted as well. Direct conversions are reported everywhere else, including unmarked functions and package-level variable declarations in the type declaration file and files in the same package. Function literals inside a marked function share its permission. A conversion such as `Code(code)` from the same protected type does not cross the type boundary and is permitted anywhere.

Prohibited extraction means explicitly converting a protected type to another concrete type, as in `string(code)`, `[]byte(code)`, or `RawCode(code)`. Passing a protected value to an interface it implements is not treated as extracting its underlying representation; both explicit and implicit conversions to interfaces are permitted. This includes `var x any = code` and `any(code)`, as well as `var s fmt.Stringer = code` and `fmt.Stringer(code)`. Calling a public method through an interface is also permitted.

A conversion that yields a pointer to the storage of its operand under another type also crosses the type boundary. Go permits it between pointer types whose base types have identical underlying types, as in `(*string)(&code)` or `(*Code)(&s)`, and from a slice to a pointer to an array with the same element type, as in `(*ID)(raw)` or `(*[16]byte)(ids)` for a protected slice type. Because reads and writes through the resulting pointer both construct and extract the protected value, such a conversion is permitted only in a function marked as both a `factory` and a `converter`, or as an `op`, of every protected type whose storage it shares, whether as the base type before or after the conversion. Missing `factory` permission is reported as `GOV001` and otherwise missing `converter` permission as `GOV002`. A conversion to a pointer to the same type, including through an alias, and conversions whose types involve no protected type are permitted. A conversion to a non-pointer array, as in `ID(raw)`, copies the elements and follows the ordinary rules for construction.

Each `const` specification that declares a constant of the protected type is permitted in the type declaration file, including specifications that inherit their type or expression from a preceding specification. Examples include `const (A Code = "A"; B = "B")` and, for a numeric protected type, `const (A Number = iota; B; C)`. Developers are responsible for the validity of constant values as part of the domain rules in that file. The same declarations are reported in other files. These declarations need no marker, including those that construct the constant with an explicit conversion such as `const Support = Code("support")` and those inside a function. Other package-level declarations, such as `var Default = Code("x")`, are not inside a marked function, so their conversions are reported.

Apart from these exceptions and scaling in a `scalar` function, described below, implicit conversion of an untyped constant or other untyped expression to a protected type and operations or comparisons between such an expression and a protected type are reported even in the marked functions. For example, `var c Code = "ABC"`, `code == "ABC"`, and `return x == y` in a function returning a protected boolean type are reported. By contrast, `Code("INVALID")` is permitted in a `factory` of `Code`. Developers are responsible for the correctness of code within the marked functions.

`factory`, `converter`, and `op` all grant permission for direct conversions and validate the shape of the marked function. A marker alone does not guarantee correct validation or encapsulation.

These markers are optional, but without a `factory` or `op` a protected type can be constructed directly only in its typed constant declarations, and without a `converter` or `op` its underlying representation cannot be extracted directly anywhere. Markers may be attached to exported or unexported functions and methods in the same file as the protected type's declaration, so internal helpers such as `mustCode` can also be marked.

### Scaling by untyped constants

In `amount * 3` or `amount / 2`, where `amount` has a protected numeric type `Amount`, Go gives the untyped constant the type `Amount`, but the constant is meant as a dimensionless scalar, not as an amount. Converting it explicitly, as in `amount * Amount(3)`, would state that an amount is multiplied by an amount. The `//govo:scalar` directive therefore permits untyped constants as scalars of the protected numeric types it names, only in the function it is attached to. Scaling yields a new value of the type, so the function must also be a `factory` or `op` of each named type.

```go
//govo:op
//govo:scalar
func (a Amount) Half() Amount { return a / 2 }

//govo:factory
//govo:scalar
func AmountFromYen(yen uint) Amount { return Amount(yen) * 100 }
```

An untyped constant is a scalar when it is the multiplier in `*` or `*=`, on either side, or the divisor in `/` or `/=`. Other untyped constants still denote values of the type and are reported as usual, including the addend in `+`, the operand of a comparison, the divisor in `%`, the dividend in `100 / amount`, and the implicit `1` of `amount++`. Only untyped constants are scalars: an untyped non-constant expression such as `1 << u` is reported, and a non-constant scalar needs an explicit conversion, as in `amount * Amount(n)`, which the type system requires. Outside a `scalar` function, including in other files, scaling by an untyped constant is reported as `GOV003`.

### Array, slice, and map types

For a protected type with an array, slice, or map underlying type, Go also permits construction and extraction without a conversion, because a value of an unnamed type such as `[]Code` is assignable to a defined type with the same underlying type and vice versa. With `type Codes []Code` protected, `var c Codes = raw`, `takeCodes(raw)`, and `return raw` in a function returning `Codes` (where `raw` is `[]Code`) are implicit construction, and `var raw []Code = codes`, `takeRaw(codes)`, and `[][]Code{codes}` are implicit extraction. They are reported like the corresponding explicit conversions, as `GOV001` and `GOV002`, and are permitted only in a `factory` or `converter` of the type, respectively. Multi-valued expressions are checked value by value, as in `raw, err = parseCodes()` or `takeBoth(rawPair())`, including comma-ok expressions such as `raw, ok = byName[key]`. A comparison between a protected array and an unnamed array, as in `id == raw`, converts the unnamed operand to the protected type and is reported as `GOV001`.

A non-empty composite literal of the protected type, such as `Codes{code}`, `ID{1, 2}`, or `Set{"a": true}`, is direct construction and is reported as `GOV001` outside a `factory` of the type. This includes literals whose type is elided, as in `[]ID{{1}}`, which are located at their opening brace. An empty literal such as `Codes{}` or `Set{}` holds no elements and is permitted in any file, like `make(Codes, 0)`, `nil`, and the zero value.

Operations on the elements of a protected value are outside the rules, consistent with basic types, for which indexing, slicing, and `len` of a protected string and operations between protected values are not detected. Indexing and assigning to elements, `range`, slicing (`codes[1:]` and `id[:]`), and the built-in functions `len`, `cap`, `append`, `copy`, `delete`, and `clear` are therefore not reported. Arguments that only supply elements to copy, namely the arguments of `copy` and the argument spread by `append(s, x...)`, are permitted even when their type matches the parameter only through the shared underlying type, as in `append(raw, codes...)` and `copy(raw, codes)`. Other arguments of built-in functions are converted as a whole and follow the usual rules, so `append(lists, codes)` with `lists` of type `[][]Code` is implicit extraction (`GOV002`), like `[][]Code{codes}`, and `delete(ids, raw)` with `ids` of type `map[ID]int` is implicit construction (`GOV001`). The usual rules still apply when the element or key type is itself protected, as in `codes[0] = "X"` or `append(codes, "X")` for a protected `Code`. When elements must not be read or modified outside the type declaration file, wrap the collection in a struct with unexported fields.

## Detected operations

With `Code` as the protected type, the following table gives representative examples. In the initial version, `GOV001` covers ordinary uses in which an untyped constant is treated as a protected type, regardless of syntax. Operations and comparisons are classified under `GOV003`, without an additional `GOV001` at the same location. The exceptions described below apply to zero values, operations through type parameters, and declarations of protected typed constants in the type declaration file.

| Category | Examples | Condition |
| --- | --- | --- |
| Explicit construction | `Code(s)`, `Code("ABC")` | Report outside a `factory` of the type; conversions from the same type are permitted |
| Explicit extraction | `string(code)`, `[]byte(code)`, `RawCode(code)` | Report outside a `converter` of the type when converting to another concrete type; conversions to the same type or an interface are permitted |
| Pointer conversions sharing storage | `(*string)(&code)`, `(*Code)(&s)`, `(*ID)(raw)` (where `ID` is a protected array type) | Report outside a function marked as both a `factory` and a `converter`, or as an `op`, of each protected type involved; `GOV001` takes priority over `GOV002` |
| Composite construction and extraction | `Codes{code}`, `var c Codes = raw`, `var raw []Code = codes` (where `Codes` is a protected `[]Code`) | Report outside a `factory` or `converter` of the type as `GOV001` or `GOV002`; empty literals are permitted |
| Typed constant declaration | `const CodeABC Code = "ABC"`, specifications that inherit a type or expression | Report outside the type declaration file |
| Implicit construction | `var c Code = "ABC"`, `UseCode("ABC")`, `return "ABC"` | Report in any file |
| Composite literals and similar uses | `[]Code{"ABC"}`, `map[Code]int{"ABC": 1}`, `S{Code: "ABC"}` | Report when an untyped constant is used as a `Code` |
| Other value transfers | `append(codes, "ABC")`, `ch <- "ABC"`, `m["ABC"]`, `*p = "ABC"` (where `p` is `*Code`) | Report when an untyped constant is used as a `Code` |
| Comparisons and operations | `code == "ABC"`, `code + "X"`, `code += "X"`, `switch code { case "ABC": }` | Report when an untyped constant is treated as the protected type |
| Scaling | `n * 3`, `3 * n`, `n / 2`, `n *= 3`, `n /= 2` (where `n` is a protected numeric type) | Report outside a `scalar` function of the type |
| Increment and decrement | `n++`, `n--` (where `n` is a protected numeric type) | Report in any file, because they add or subtract the untyped constant `1` |

A named untyped constant such as `const raw = "ABC"` is also covered when assigned or passed to a protected type, including one declared in another package and referenced as `pkg.Raw`. A call of the built-in `min`, `max`, `real`, `imag`, or `complex` whose arguments are all untyped constants is itself an untyped constant and is covered in the same way, as in `var n Number = min(1, 2)`. Other constant expressions with typed results, such as `len("ABC")`, are not untyped constants. A typed constant declaration such as `const defaultCode Code = raw`, including one that inherits its type or expression from a preceding specification, is permitted only in the type declaration file. `NewCode("ABC")` is outside the rule because its parameter has type `string`. Comparisons and operations between `Code` values are not detected in the initial version.

The count of a shift operation never takes the type of the shifted operand, so `n << 1`, `n >> 2`, `n <<= 1`, and `n >>= 1` are not reported.

Besides untyped constants, some non-constant expressions are untyped and take the type of their context, so they are treated in the same way. The result of a comparison is an untyped boolean even when its operands are typed, and a shift of an untyped constant by a non-constant count, such as `1 << u`, is untyped. For a protected `Flag` with a boolean underlying type and a protected `Number` with a numeric one, `var f Flag = int(1) == int(1)`, `var f Flag = x == y`, and `var n Number = 1 << u` are reported as `GOV001`, and `f == (x == y)` and `n + (1 << u)` as `GOV003`. An explicit conversion such as `Flag(x == y)` or `Number(1 << u)` is a construction from a non-protected value, not a conversion from the same type, so it is reported as `GOV001` outside a `factory` of the type. Diagnostics say "untyped constant" or "untyped expression" accordingly.

In the initial version, diagnostics for value operations are grouped by meaning, rather than by syntax. Diagnostics concerning the declaration or use of directives form a separate category.

Rule IDs consist of a category prefix and a three-digit number, and each category is numbered independently, so adding a rule to one category does not shift or interleave with the other. `GOV` identifies rules for operations on protected types, and `GOVD` identifies rules for directives. A number is never reused for a different rule within its category.

Each diagnostic is reported with its rule ID as the `Category` of the `analysis.Diagnostic`, and its message starts with the rule ID followed by `: `, as in `GOV001: direct construction of protected type Code; use a //govo:factory function`. The category appears as `"category"` in `-json` output and lets drivers that run the analyzer identify a rule without parsing the message.

| Rule ID | Rule | Scope |
| --- | --- | --- |
| `GOV001` | Invalid construction | Explicit conversions to a protected type, pointer conversions sharing the storage of a protected type outside a `factory` of that type, non-empty composite literals of a protected type, and implicit conversions from unnamed array, slice, or map values outside a `factory` of the type, declarations of protected typed constants in other files, and implicit construction from untyped constants at ordinary use sites |
| `GOV002` | Extraction of the underlying representation | Explicit conversions from a protected type to another concrete type, pointer conversions sharing the storage of a protected type in a `factory` but outside a `converter` of that type, and implicit conversions of a protected array, slice, or map to an unnamed type, outside a `converter` of the type |
| `GOV003` | Operations and comparisons with untyped constants | Comparisons, binary operations, compound assignments, `switch` cases, and similar operations in which an untyped constant is treated as a protected type, including scaling outside a `scalar` function of the type |
| `GOVD001` | Invalid directives | Unknown directives, directives with invalid arguments, directives that cannot be attached to a target, `protect` on a type that cannot be protected, markers that fail their shape check, `scalar` on a non-numeric type or a function that is not a `factory` or `op` of it, and unknown or unsuppressible rule IDs in `ignore` |
| `GOVD002` | Unused `ignore` directives | `ignore` directives that suppress no diagnostics, and rule IDs listed in an `ignore` directive that suppress no diagnostics |
| `GOVD003` | Missing `ignore` reasons | `ignore` directives without a reason when reason checking is enabled |
| `GOVD004` | Redundant directives | `factory`, `converter`, `op`, or `scalar` directives that name a type for a function whose other directives already grant the same permission for it, and `ignore` directives or listed rule IDs that repeat another `ignore` directive for the same target |

A single conversion expression does not receive overlapping diagnostics. For example, if both types in `OtherCode(code)` are protected, construction of the destination and extraction from the source are evaluated against the `factory` and `converter` markers of the enclosing function. If both violate the rules, only `GOV001` is reported. If only construction violates the rules, `GOV001` is reported; if only extraction violates them, `GOV002` is reported.

Diagnostics for direct or implicit construction and extraction name the marker that would permit the operation, as in `direct construction of protected type Code; use a //govo:factory function` and `implicit extraction from protected type Codes; use a //govo:converter function`. Outside the type declaration file, this means calling such a function; inside it, it means performing the conversion in a function marked for that type. Likewise, scaling by an untyped constant is reported as `untyped constant scales protected type Amount; use a //govo:scalar function`. Other diagnostics for untyped constants and for typed constant declarations outside the type declaration file carry no such hint, because no marker permits them.

Operations through an alias of a protected type are evaluated as operations on the original protected type. The same rules apply to protected types declared in another package within the same module. This includes unexported protected types, whose values can reach other packages through exported functions, fields, and aliases.

## Directives

| Directive | Purpose |
| --- | --- |
| `//govo:protect` | Protects the individual type declaration immediately following it; it does not apply to an entire `type (...)` group |
| `//govo:factory` | Marks a function that may construct a protected type directly |
| `//govo:converter` | Marks a function that may extract the underlying representation of a protected type directly |
| `//govo:op` | Marks an operation inside the trusted boundary that may both construct and extract a protected type directly |
| `//govo:scalar` | Permits a `factory` or `op` of a protected numeric type to multiply and divide it by untyped constants |
| `//govo:ignore` | Suppresses diagnostics for specified operations |

`protect` takes no arguments and attaches to the individual type declaration immediately after it. Within a `type (...)` group, each type to be protected needs its own directive. `factory`, `converter`, `op`, and `scalar` attach to the immediately following function or method declaration, exported or not, and name a protected type declared in the same file. The type name may be omitted if the file declares only one protected type; it is required if the file declares more than one. If the marked declaration is in a different file from the type declaration, `GOVD001` is reported and the directive is ignored. The directives of a function may appear in any order.

When a function or method handles multiple protected types, the standard form lists their names on one line, separated by spaces. Multiple directive lines are also accepted and treated as a set of target types.

```go
//govo:factory Code RegionCode
func NewCodes(s string) (Code, RegionCode, error) { /* ... */ }

//govo:converter Code
//govo:converter RegionCode
func DescribeCodes(c Code, r RegionCode) string { /* ... */ }
```

Each type named in a `factory` directive is checked separately. If none of the function's or method's return values has exactly that protected type, `GOVD001` is reported for the missing type and only that type's marker is ignored; the function does not gain permission to construct that type. A return signature such as `(Code, error)` is valid; `*Code` and `[]Code` do not count as direct returns of `Code`. For a generic protected type, any instance counts: a generic function such as `func NewList[T any](xs []T) List[T]` and a function that returns one instance, such as `func NewInts(xs []int) List[int]`, are both factories of `List`. The directive names the generic type without type arguments, as in `//govo:factory List`.

Each type named in a `converter` directive is also checked separately. The directive is valid when the receiver or an argument has that protected type or a pointer to it, or, for a generic protected type, any instance of it or a pointer to one. Because some APIs write the converted value to a destination passed as an argument, return types are unrestricted. `GOVD001` is reported for a type that fails this check, and only that type's marker is ignored; the function does not gain permission to extract from that type.

Each type named in an `op` directive must pass both checks: the receiver or an argument has that protected type or a pointer to it, and one of the return values has exactly that type. `GOVD001` is reported for a type that fails either check, and the function gains neither permission for it. A function that only constructs or only extracts should be a `factory` or a `converter`. These checks verify the shape of the API, not the correctness of validation or conversion logic.

Each type named in a `scalar` directive must have a numeric underlying type, and the function must be a `factory` or `op` of that type with a valid marker. `GOVD001` is reported for a type that fails either condition, and the function does not gain permission to scale it.

A function's directives should state each permission once. When a valid `factory`, `converter`, `op`, or `scalar` names a type that the function's directives already name with a valid directive of the same command, including a repetition within one directive such as `//govo:factory Code Code` and a type that an omitted name stands for, `GOVD004` is reported at each later directive for that type. A valid `factory` or `converter` of a type that a valid `op` of the same function names is reported as `GOVD004`, wherever it appears, because the `op` already grants its permission. A `factory` and a `converter` of the same type are not redundant, since each grants a different permission. Only types that pass their checks count, so a marker reported as `GOVD001` for a type neither is reported as redundant nor makes another one redundant. A redundant directive still grants its permission.

In every directive, text from the first `//` after `//govo:` is not parsed as arguments, so `//govo:factory Code // needed for compatibility` names only `Code`. Only `ignore` treats that text as its reason; other directives ignore it.

Unknown `govo` directives, extra arguments, and directives that cannot be attached to a target are reported as `GOVD001`, and the invalid directive is ignored. Such diagnostics do not invalidate other valid directives at the same location.

An `ignore` directive may specify comma-separated rule IDs of either category; omitting them targets all rules. `GOVD002` is determined after suppression, so it cannot be suppressed, and listing it is reported as `GOVD001`, like an unknown rule ID. `GOVD004` for redundant `ignore` directives cannot be suppressed either. A reason uses the same `// reason` form as golangci-lint.

```go
//govo:ignore GOV001 // Required for compatibility with an external specification
UseCode("ABC")

UseCode("ABC"); _ = code == "ABC" //govo:ignore GOV001,GOV003 // Temporary migration exception
```

A standalone comment line applies to the immediately following statement or declaration, even when that statement or declaration spans multiple lines. It also applies to comments between it and that statement or declaration, so an `ignore` placed before other directives in a doc comment covers diagnostics on those directives, such as a marker that names an unknown type or fails its shape check. A directive before the `ignore` is not covered. A trailing comment applies to all operations on its physical line. If multiple operations share that line, all are covered. A standalone comment immediately before an `if`, `for`, `switch`, or `select` applies to operations in its header, such as conditions and initializers, but not to statements inside its block. Before a `case` or `default` clause, it applies only to the clause's expressions or communication, not to the statements after the colon. A label does not change the covered statement: a comment before `L: for ...` applies to the header of that `for`. A standalone comment does not extend into the body of a function literal in the covered statement or declaration either, as in `defer func() { ... }()`. There is no block-wide suppression.

The following statement or declaration is searched for only in the innermost declaration list, `const`/`var`/`type` group, block, or clause that contains the standalone comment. A standalone comment at the end of a block, or inside an expression such as between the arguments of a multiline call, has no target and is reported as `GOVD001`.

Diagnostics are located at the syntax that causes them: the destination type name for an explicit conversion, inside any parentheses as in `(Code)(s)` (the `*` for a pointer conversion such as `(*string)(&code)`), the type of a composite literal (or its opening brace when the type is elided), the start of the expression for an implicit conversion of an array, slice, or map value, the declared identifier for a protected typed constant, and the start of the constant expression for implicit conversion from an untyped constant or an operation or comparison involving one, and the `++` or `--` operator for an increment or decrement. If a statement has multiple offending constants, each receives a diagnostic. A standalone comment suppresses diagnostics in the header or expression of the following statement or declaration, but does not extend into the body of an `if` or `for`. A trailing comment suppresses every diagnostic whose cause is on its physical line; this includes the body of a single-line `if` when it is on that line. In a multiline call, a trailing comment on the closing parenthesis line does not suppress diagnostics for arguments on other lines.

An `ignore` directive on its own line is redundant when another one has the same target. Rule IDs listed in a directive take precedence over a directive that targets all rules: a directive that omits rule IDs is reported as `GOVD004` when another directive for the same target lists rule IDs, wherever it appears, and when it follows another directive for the same target that also omits them. A listed rule ID is reported as `GOVD004` when it is listed more than once in one directive, or when an earlier directive for the same target lists it. Directives for the same target that list different rule IDs are not redundant. Ignore directives at the end of a line are compared only within themselves, for rule IDs listed more than once. Like `GOVD002`, these diagnostics are determined from the ignore directives themselves, so they cannot be suppressed; listing `GOVD004` in an `ignore` still suppresses redundant markers. A redundant directive or rule ID still suppresses diagnostics and is not also reported as `GOVD002`.

By default, omitting the reason from an `ignore` directive produces no diagnostic. When `ignore.missing-reason` is set to `error`, a missing reason is reported as `GOVD003`. Only missing reasons are disabled by default; diagnostics for unused `ignore` directives and invalid directives still apply. If rule IDs are listed, usage is checked separately for each ID, and an ID that suppresses no diagnostics is reported as unused (`GOVD002`). If IDs are omitted, the directive is reported as unused only when it suppresses no diagnostics at all.

## Configuration

The configuration file format is YAML. Configuration is resolved in this order: (1) a file explicitly specified by `-config`, then (2) `.govo.yaml` directly in the tool's current working directory. A relative `-config` path is resolved against that directory. If an explicitly specified file does not exist, this is an error; there is no fallback to `.govo.yaml`. Without `-config`, parent and child directories are not searched, and the defaults apply when `.govo.yaml` is absent. Under `go vet -vettool`, automatic configuration discovery and relative paths are based on each analyzed package's directory. To share one configuration across packages, pass an absolute path to `-config`. Invalid YAML, unknown keys at any level, and invalid values for known keys cause an error; the error names the unknown key and its line. A configuration that uses keys added in a newer govo therefore fails on an older one. The standalone command and `go vet -vettool` use the same discovery order. Within one process, each resolved configuration file is read once, and its result, or its error, is reused for every package; changes to the file during a run are not picked up.

```yaml
tests: true
ignore:
  missing-reason: off
```

`tests` controls whether `_test.go` files are checked and defaults to `true`. `ignore.missing-reason` accepts `off` or `error` and defaults to `off`. govo has no warning level: every diagnostic, including the `GOVD` rules, is an error. Both the standalone `govo` command and `go vet -vettool` fail if any diagnostic is emitted.

## Exclusions and guarantees

- Go zero values are permitted: `var c Code` and `new(Code)` are not prohibited. Consequently, `govo` does not guarantee that every `Code` has passed validation in `NewCode`.
- Construction or extraction through type parameters, as when a protected value is passed as a type argument whose constraint permits conversions, and detection by tracing instantiations of generic functions, are outside the initial scope. The ordinary rules still apply to a direct conversion such as `Code(s)` or `List[T](xs)` written inside a generic function. Protected generic types themselves are eligible, as described in Eligible types.
- Direct writes through libraries or runtime mechanisms such as JSON, databases, and reflection are outside the detection scope.
- Conversions through `unsafe.Pointer`, as in `(*string)(unsafe.Pointer(&code))`, are outside the detection scope.
- Types with struct, interface, channel, function, or pointer underlying types are ineligible for protection. Operations between protected values and operations on the elements of protected arrays, slices, and maps are outside the detection scope.
- Go zero values extend to `make`, `nil`, and empty composite literals of protected array, slice, and map types, which are permitted.
- Assignments by a `range` clause with `=`, as in `for _, codes = range rawLists`, are not checked for implicit construction or extraction.

The initial version of `govo` detects the specified bypasses in ordinary Go code. It does not prevent every construction path involving zero values, type parameters, or runtime mechanisms.

## Adoption and analysis scope

Only types marked with `//govo:protect` are protected. There is no separate baseline feature or feature that inserts `//govo:ignore` directives in bulk.

Generated Go files, packages under `vendor`, packages from external modules, and the standard library are excluded from analysis. Generated files are identified with `go/ast.IsGenerated`, which recognizes the standard Go `// Code generated ... DO NOT EDIT.` comment before the package clause. A package is treated as vendored when its directory is `vendor/<import path>`, so a main-module package that merely has a directory named `vendor` in its path is still analyzed. A protected type, exported or not, declared in a dependency package within the same module is also recognized as protected at its use sites. A type marked with `//govo:protect` in an external module is not recognized as protected at its use sites.

With `go.work`, multiple modules treated as main modules may be analyzed.
