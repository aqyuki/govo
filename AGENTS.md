# Guide for development agents

This guide applies to the entire repository. When changing implementation, configuration, or CI, update the procedures documented here to match the actual setup.

## Project and structure

govo is a linter that checks direct construction, extraction of internal representations, and operations with untyped constants for Go Value Objects marked with `//govo:protect`. It uses `golang.org/x/tools/go/analysis` and provides a library and a CLI.

- `govo.go`: The public `govo.Analyzer` API. Keep analysis logic in internal packages.
- `cmd/govo/main.go`: Execution through `singlechecker` and application of build tags.
- `cmd/govo/args.go`: Defaulting to `./...` when no arguments are provided, validation of package arguments, and identification of the vettool protocol.
- `cmd/govo/usage.go`: govo's own help, which replaces the analysis driver's. Keep it consistent with the flags accepted in `args.go` and the CLI section of `README.md`.
- `internal/analyzer/analyzer.go`: Analysis state, collection of protected types, cross-package object facts, and selection of analysis targets.
- `internal/analyzer/operations.go`: Detection of construction, conversions, and operations using the AST and type information.
- `internal/analyzer/directives.go`: Directive validation, diagnostic suppression, and reporting.
- `internal/analyzer/config.go`: YAML configuration loading and validation.
- `internal/analyzer/testdata/src/{a,b,c}`: Inputs for `analysistest`. `// want` specifies expected diagnostics and facts. `c` runs with `ignore.missing-reason: error`.
- `cmd/govo/integration_test.go` and `cmd/govo/testdata/mod`: Integration tests that build the command and run it standalone and through `go vet -vettool` against a separate module. `go test -short` skips them.
- `docs/spec.md`: The detailed detection specification. Read the relevant sections before changing behavior.
- `README.md`: User-facing usage and development instructions.

## Tools and setup

- The minimum Go version is `1.26.0`, as specified in `go.mod`. The `test` job in `check.yaml` runs on a matrix of Go 1.26 and Go 1.27 with `GOTOOLCHAIN=local`; other jobs set up Go from `go.mod` with `go-version-file`. Go is not managed through `mise.toml`.
- `mise.toml` pins Node.js `24.21.0`, pnpm `12.7.0`, golangci-lint `2.13.1`, and GoReleaser `2.18.2`. Run `mise install` as needed. CI does not install tools with mise: the `.github/actions/tool-version` composite action reads a tool's version from `mise.toml` with `mise tool --json` and jq, and workflows pass it to the official setup actions (for example, the `lint` jobs pass it to `golangci/golangci-lint-action`, and the `release` job in `release.yaml` passes it to `goreleaser/goreleaser-action`), so change versions only in `mise.toml`.
- If mise tools are not on PATH, use `mise exec -- <command>`. The presence of configuration does not mean the tools are installed.
- JavaScript is used for commitlint tooling. When dependencies are needed, use `mise exec -- pnpm install --frozen-lockfile`. Node.js is not required for ordinary Go tests.
- `.codex/config.toml` configures `gopls mcp`. This requires a separately installed `gopls`, which is not installed through mise.
- Keep tool versions and dependencies consistent with the existing configuration and lockfiles. Update the corresponding `go.sum` or `pnpm-lock.yaml` only when intentionally updating dependencies.

## Validation procedures

When changing Go code, run the following from the repository root:

```sh
go test ./...
mise exec -- golangci-lint run ./...
```

If formatting is needed, run `mise exec -- golangci-lint fmt` and review the diff. `.golangci.yml` adds `modernize` and `wsl_v5` to the standard linters and also checks formatting with gofmt, goimports, gofumpt, and gci. Group imports in this order: standard library, external dependencies, and this module. Because additional gofumpt settings are enabled, do not consider formatting complete after running gofmt alone.

The following commands can be used for focused checks. For final validation of Go changes, run the full test suite.

```sh
go test ./internal/analyzer -run TestAnalyzer
go test ./internal/analyzer -run TestLoadConfig
go test ./cmd/govo
go test ./cmd/govo -run TestIntegration
go run ./cmd/govo ./...
```

When changing CLI or vettool integration, extend `TestIntegration` and, as needed, build a binary in a temporary directory and also check with `go vet -vettool=/absolute/path/to/govo ./...`. To exercise the Go 1.26 vet protocol locally, run the tests with `GOTOOLCHAIN=go1.26.0`; the integration tests inherit it. When using the same configuration across multiple packages, pass an absolute path to `-config`.

For documentation-only changes, check references, commands, and consistency with configuration. If validation cannot be performed, report the reason and the items that remain unverified.

## Implementation requirements

- Protected types must be defined types with a basic, array, slice, or map underlying type. A type alias does not itself become a new protected type, but operations through aliases of protected types are checked.
- The trust boundary is the functions marked with `//govo:factory` / `//govo:converter` in the file that declares the protected type. A valid `factory T` allows construction of `T` and a valid `converter T` allows extraction from `T`, each only for the named types and only in that direction; function literals share the enclosing declaration's permission. Declarations of typed constants of the protected type (`const` specs, including explicit conversions in them) are allowed anywhere in the declaration file without a marker; other conversions outside marked functions, including unmarked functions and package-level `var` declarations in that file, are reported. Implicit conversions between a protected array, slice, or map and an unnamed type with the same underlying type count as construction or extraction, like explicit conversions; non-empty composite literals of the protected type count as construction. Element operations on such types are not checked. Implicit conversions and operations involving untyped constants or other untyped expressions (comparison results and shifts of untyped constants) are checked even in the same file. go/types records the contextual type of untyped expressions, so decide untypedness from the expression's syntax, not from its recorded type.
- `factory` / `converter` may be attached to exported or unexported functions and methods in the declaration file. They grant permission for direct conversions and validate the function's shape; a type that fails the shape check is dropped from the marker and grants nothing.
- Conversions to the same protected type and to interfaces are allowed. Do not emit both construction and extraction diagnostics for a single conversion; if it violates both rules, prioritize `GOVO001`.
- Diagnostic IDs are `GOVO001` (construction), `GOVO002` (extraction), `GOVO003` (operations or comparisons with untyped constants), and `GOVO004` (directives). Treat diagnostic locations and the scope of ignore directives as part of the specification.
- Preserve the exclusion of generated files (`ast.IsGenerated`), vendored packages (files in `vendor/<import path>`), external modules, and the standard library, as well as fact propagation for all protected types, exported or not, within the same module. Pay attention to differences in vet module information between Go 1.26 and 1.27.
- Configuration precedence is an explicit `-config`, followed by `.govo.yaml` directly in the current working directory. Do not search parent directories. A missing explicitly specified file is an error. Defaults are `tests: true` and `ignore.missing-reason: off`; `ignore.missing-reason` accepts only `off` or `error`.
- Zero values, conversions through type parameters, and runtime construction through reflection or similar mechanisms are outside the detection guarantees. If extending the scope, update the specification and tests as well.

## Tests and scope of changes

- For diagnostic fixes, add `analysistest` fixtures and verify both violations and allowed cases. Include relevant cases when changes involve file boundaries, package boundaries, aliases, or suppression.
- Fixtures contain intentional violations and generated-file headers. Do not fix them as ordinary production code or remove `// want` comments. Verify expectations even when formatting changes comment positions.
- Extend existing table-driven tests for configuration or CLI behavior. Preserve cleanup in tests that modify `configPath` or the current working directory, and do not casually parallelize tests that handle shared state.
- When changing user-visible behavior, update `docs/spec.md` and `README.md` as needed. Do not change unrelated code or the user's working changes.

## CI, commits, and releases

- `.github/workflows/check.yaml` runs on pull requests and pushes to `main`. The `gomod` job checks `go mod tidy -diff`; after it passes, the `lint` job runs golangci-lint (including formatting checks) and the `test` job runs the full test suite (including the integration tests) and `go vet ./...` with Go 1.26 and Go 1.27, uploading coverage from the Go 1.26 run. The `coverage` job then reports coverage and test execution time with octocov (configured in `.octocov.yml`, which times steps named `Run tests`). Run `go mod tidy` when changing imports or dependencies.
- Follow Conventional Commits for PR titles and commit subjects (for example, `fix(analyzer): handle protected aliases`). `.github/workflows/pr-title.yaml` checks PR titles with commitlint when a pull request is opened or edited. `commitlint.config.js` uses `@commitlint/config-conventional`, and you can check a subject locally with `printf '%s\n' 'fix(analyzer): handle protected aliases' | mise exec -- pnpm exec commitlint`.
- Pin every third-party action to a full commit SHA with a `# vX.Y.Z` comment; Renovate keeps the pins updated.
- Caches use `actions/cache` only. Disable built-in caches in setup actions (for example, `cache: false` for `actions/setup-go` and `skip-cache: true` for `golangci/golangci-lint-action`) and cache Go modules, build outputs, and tool caches with `actions/cache` in each job. `.github/workflows/cleanup-cache.yaml` deletes a pull request's caches (`refs/pull/<number>/merge`) when it is merged or closed; it uses `pull_request_target` so fork pull requests are covered, so never check out or run pull request code in it.
- Distribution configuration is in `.goreleaser.yaml`. GoReleaser v2 builds for macOS/Linux on amd64 and arm64, and Windows on amd64. `check.yaml` does not run GoReleaser, so when changing distribution configuration, validate with `mise exec -- goreleaser check` and `mise exec -- goreleaser release --snapshot --clean`. Output goes to `dist/`.
- Official releases use manual execution of `.github/workflows/release.yaml` from the default branch with a `tag` input in the stable `vMAJOR.MINOR.PATCH` form. The `validate` job checks the branch, tag format, and that the tag does not already exist; then `test` (tidy check, `go test`, `go vet`) and `lint` run in order. Finally the `release` job tags the tested commit, pushes the tag, and runs `goreleaser release --clean`, which builds the artifacts and creates the GitHub Release with GoReleaser's generated release notes. Only the `release` job has `contents: write`. Create tags, push, and publish only when release work has been requested.
